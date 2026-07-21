package work_session_test

import (
	"testing"
	"time"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
)

func setupDeps(t *testing.T) (*organization.Service, *person.Service, *project.Service, *team.Service, *team.MembershipService, *task.Service, *work_session.Service) {
	cleanup(t)

	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), nil)
	wsSvc := work_session.NewService(work_session.NewStore(testClient), task.NewStore(testClient))

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc
}

func cleanup(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE work_sessions CASCADE")
	testDB.Exec("TRUNCATE TABLE tasks CASCADE")
	testDB.Exec("TRUNCATE TABLE team_memberships CASCADE")
	testDB.Exec("TRUNCATE TABLE teams CASCADE")
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE people CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}

func TestService_ClockInSuccess(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)

	session, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("clock in failed: %v", err)
	}
	if session.EndAt != nil {
		t.Error("expected end_at to be nil after clock in")
	}
	if session.TaskID.String() != task1.ID.String() {
		t.Errorf("task id mismatch")
	}
	if session.PersonID.String() != p.ID.String() {
		t.Errorf("person id mismatch")
	}
}

func TestService_ClockIn_MissingTask(t *testing.T) {
	_, _, _, _, _, _, wsSvc := setupDeps(t)

	_, err := wsSvc.ClockIn("00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003")
	if err == nil {
		t.Fatal("expected error for missing task, got nil")
	}
}

func TestService_ClockIn_DifferentProject(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	projA, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	projB, _ := projSvc.Create(org.ID.String(), "Project B", "", 0, nil, nil)
	tm, _ := teamSvc.Create(projA.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(projA.ID.String(), "Task", "", p.ID.String(), nil)

	_, err := wsSvc.ClockIn(projB.ID.String(), task1.ID.String(), p.ID.String())
	if err == nil {
		t.Fatal("expected error for task in wrong project, got nil")
	}
}

func TestService_ClockOut_Success(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)

	wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String())
	time.Sleep(time.Millisecond)

	session, err := wsSvc.ClockOut(p.ID.String())
	if err != nil {
		t.Fatalf("clock out failed: %v", err)
	}
	if session.EndAt == nil {
		t.Error("expected end_at to be set after clock out")
	}
	if !session.EndAt.After(session.StartAt) {
		t.Error("expected end_at after start_at")
	}
}

func TestService_ClockOut_NoActiveSession(t *testing.T) {
	_, _, _, _, _, _, wsSvc := setupDeps(t)

	_, err := wsSvc.ClockOut("00000000-0000-0000-0000-000000000001")
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
}

func TestService_OverlappingSessionRejected(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	taskA, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	taskB, _ := taskSvc.Create(proj.ID.String(), "Task B", "", p.ID.String(), nil)

	wsSvc.ClockIn(proj.ID.String(), taskA.ID.String(), p.ID.String())
	_, err := wsSvc.ClockIn(proj.ID.String(), taskB.ID.String(), p.ID.String())
	if err == nil {
		t.Fatal("expected error for overlapping session, got nil")
	}
}

func TestService_TotalTime(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)

	wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String())
	time.Sleep(time.Millisecond)
	wsSvc.ClockOut(p.ID.String())

	taskID := task1.ID.String()
	personID := p.ID.String()
	result, err := wsSvc.TotalTime(proj.ID.String(), &taskID, &personID)
	if err != nil {
		t.Fatalf("total time failed: %v", err)
	}
	if result.TotalSeconds <= 0 {
		t.Errorf("expected positive total seconds, got %f", result.TotalSeconds)
	}
}
