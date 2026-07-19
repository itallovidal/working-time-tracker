package service

import (
	"testing"
	"time"
	"working-time-tracker/internal/store"
)

func setupTimeEntryDeps(t *testing.T) (*OrganizationService, *PersonService, *ProjectService, *TeamService, *TeamMembershipService, *TaskService, *TimeEntryService) {
	cleanup(t)

	orgStore := store.NewOrganizationStore(testDB)
	personStore := store.NewPersonStore(testDB)
	projStore := store.NewProjectStore(testDB)
	teamStore := store.NewTeamStore(testDB)
	memberStore := store.NewTeamMembershipStore(testDB)
	taskStore := store.NewTaskStore(testDB)
	sessionStore := store.NewWorkSessionStore(testDB)

	orgSvc := NewOrganizationService(orgStore)
	personSvc := NewPersonService(personStore)
	projSvc := NewProjectService(projStore)
	teamSvc := NewTeamService(teamStore)
	memberSvc := NewTeamMembershipService(memberStore)
	taskSvc := NewTaskService(taskStore, memberStore, nil)
	timeSvc := NewTimeEntryService(sessionStore, taskStore)

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, timeSvc
}

func TestTimeEntryService_ClockInSuccess(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, timeSvc := setupTimeEntryDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())
	task, _ := taskSvc.Create(project.ID.String(), "Task", "", person.ID.String(), nil)

	session, err := timeSvc.ClockIn(project.ID.String(), task.ID.String(), person.ID.String())
	if err != nil {
		t.Fatalf("clock in failed: %v", err)
	}
	if session.EndAt != nil {
		t.Error("expected end_at to be nil after clock in")
	}
	if session.TaskID.String() != task.ID.String() {
		t.Errorf("task id mismatch")
	}
	if session.PersonID.String() != person.ID.String() {
		t.Errorf("person id mismatch")
	}
}

func TestTimeEntryService_ClockIn_MissingTask(t *testing.T) {
	_, _, _, _, _, _, timeSvc := setupTimeEntryDeps(t)

	_, err := timeSvc.ClockIn("00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003")
	if err == nil {
		t.Fatal("expected error for missing task, got nil")
	}
}

func TestTimeEntryService_ClockIn_DifferentProject(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, timeSvc := setupTimeEntryDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	projectA, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	projectB, _ := projSvc.Create(org.ID.String(), "Project B", "", 0, nil, nil)
	team, _ := teamSvc.Create(projectA.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())
	task, _ := taskSvc.Create(projectA.ID.String(), "Task", "", person.ID.String(), nil)

	_, err := timeSvc.ClockIn(projectB.ID.String(), task.ID.String(), person.ID.String())
	if err == nil {
		t.Fatal("expected error for task in wrong project, got nil")
	}
}

func TestTimeEntryService_ClockOut_Success(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, timeSvc := setupTimeEntryDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())
	task, _ := taskSvc.Create(project.ID.String(), "Task", "", person.ID.String(), nil)

	timeSvc.ClockIn(project.ID.String(), task.ID.String(), person.ID.String())
	time.Sleep(time.Millisecond)

	session, err := timeSvc.ClockOut(person.ID.String())
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

func TestTimeEntryService_ClockOut_NoActiveSession(t *testing.T) {
	_, _, _, _, _, _, timeSvc := setupTimeEntryDeps(t)

	_, err := timeSvc.ClockOut("00000000-0000-0000-0000-000000000001")
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
}

func TestTimeEntryService_OverlappingSessionRejected(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, timeSvc := setupTimeEntryDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())
	taskA, _ := taskSvc.Create(project.ID.String(), "Task A", "", person.ID.String(), nil)
	taskB, _ := taskSvc.Create(project.ID.String(), "Task B", "", person.ID.String(), nil)

	timeSvc.ClockIn(project.ID.String(), taskA.ID.String(), person.ID.String())
	_, err := timeSvc.ClockIn(project.ID.String(), taskB.ID.String(), person.ID.String())
	if err == nil {
		t.Fatal("expected error for overlapping session, got nil")
	}
}

func TestTimeEntryService_TotalTime(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, timeSvc := setupTimeEntryDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())
	task, _ := taskSvc.Create(project.ID.String(), "Task", "", person.ID.String(), nil)

	timeSvc.ClockIn(project.ID.String(), task.ID.String(), person.ID.String())
	time.Sleep(time.Millisecond)
	timeSvc.ClockOut(person.ID.String())

	taskID := task.ID.String()
	personID := person.ID.String()
	result, err := timeSvc.TotalTime(project.ID.String(), &taskID, &personID)
	if err != nil {
		t.Fatalf("total time failed: %v", err)
	}
	if result.TotalSeconds <= 0 {
		t.Errorf("expected positive total seconds, got %f", result.TotalSeconds)
	}
}
