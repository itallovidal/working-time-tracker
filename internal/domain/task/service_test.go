package task_test

import (
	"testing"
	"time"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

func setupDeps(t *testing.T) (*organization.Service, *person.Service, *project.Service, *team.Service, *team.MembershipService, *task.Service) {
	cleanup(t)

	orgSvc := organization.NewService(organization.NewStore(testDB))
	personSvc := person.NewService(person.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	teamSvc := team.NewService(team.NewStore(testDB))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testDB))
	taskSvc := task.NewService(task.NewStore(testDB), team.NewMembershipStore(testDB), nil)

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc
}

func cleanup(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE tasks CASCADE")
	testDB.Exec("TRUNCATE TABLE team_memberships CASCADE")
	testDB.Exec("TRUNCATE TABLE teams CASCADE")
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE people CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}

func TestService_Create_DefaultDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	task1, err := taskSvc.Create(proj.ID.String(), "Task A", "desc", p.ID.String(), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if task1.Name != "Task A" {
		t.Errorf("name = %q, want %q", task1.Name, "Task A")
	}
	if task1.Deadline.IsZero() {
		t.Error("expected non-zero deadline")
	}
	expected := time.Now().Add(7 * 24 * time.Hour)
	if task1.Deadline.Before(expected.Add(-time.Hour)) || task1.Deadline.After(expected.Add(time.Hour)) {
		t.Errorf("deadline = %v, expected ~%v", task1.Deadline, expected)
	}
}

func TestService_Create_ExplicitDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	dl := time.Now().Add(30 * 24 * time.Hour)
	task1, err := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), &dl)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !task1.Deadline.Equal(dl) {
		t.Errorf("deadline = %v, want %v", task1.Deadline, dl)
	}
}

func TestService_Create_AssigneeNotTeamMember(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for non-member assignee, got nil")
	}
}

func TestService_Create_MissingAssignee(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := taskSvc.Create(proj.ID.String(), "Task A", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty assignee, got nil")
	}
}

func TestService_Create_MissingName(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	_, err := taskSvc.Create(proj.ID.String(), "", "", p.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Update(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	created, _ := taskSvc.Create(proj.ID.String(), "Old", "", p.ID.String(), nil)
	updated, err := taskSvc.Update(created.ID.String(), "New", "new desc", nil, nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("name = %q, want %q", updated.Name, "New")
	}
	if updated.Description != "new desc" {
		t.Errorf("description = %q, want %q", updated.Description, "new desc")
	}
}

func TestService_Delete(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	err := taskSvc.Delete(task1.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = taskSvc.Get(task1.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_LinkUnlinkExternalItem(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	linked, err := taskSvc.LinkExternalItem(task1.ID.String(), "00000000-0000-0000-0000-000000000001", "42", "https://example.com/42")
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}
	if linked.ExternalItemID == nil || *linked.ExternalItemID != "42" {
		t.Errorf("external_item_id = %v, want 42", linked.ExternalItemID)
	}

	unlinked, err := taskSvc.UnlinkExternalItem(task1.ID.String())
	if err != nil {
		t.Fatalf("unlink failed: %v", err)
	}
	if unlinked.ExternalItemID != nil {
		t.Errorf("expected nil external_item_id after unlink, got %v", *unlinked.ExternalItemID)
	}
}
