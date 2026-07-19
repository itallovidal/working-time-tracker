package service

import (
	"testing"
	"time"
	"working-time-tracker/internal/store"
)

func setupTaskDeps(t *testing.T) (*OrganizationService, *PersonService, *ProjectService, *TeamService, *TeamMembershipService, *TaskService) {
	cleanup(t)

	orgStore := store.NewOrganizationStore(testDB)
	personStore := store.NewPersonStore(testDB)
	projStore := store.NewProjectStore(testDB)
	teamStore := store.NewTeamStore(testDB)
	memberStore := store.NewTeamMembershipStore(testDB)
	taskStore := store.NewTaskStore(testDB)

	orgSvc := NewOrganizationService(orgStore)
	personSvc := NewPersonService(personStore)
	projSvc := NewProjectService(projStore)
	teamSvc := NewTeamService(teamStore)
	memberSvc := NewTeamMembershipService(memberStore)
	taskSvc := NewTaskService(taskStore, memberStore, nil)

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc
}

func TestTaskService_Create_DefaultDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())

	task, err := taskSvc.Create(project.ID.String(), "Task A", "desc", person.ID.String(), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if task.Name != "Task A" {
		t.Errorf("name = %q, want %q", task.Name, "Task A")
	}
	if task.Deadline.IsZero() {
		t.Error("expected non-zero deadline")
	}
	expected := time.Now().Add(7 * 24 * time.Hour)
	if task.Deadline.Before(expected.Add(-time.Hour)) || task.Deadline.After(expected.Add(time.Hour)) {
		t.Errorf("deadline = %v, expected ~%v", task.Deadline, expected)
	}
}

func TestTaskService_Create_ExplicitDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())

	dl := time.Now().Add(30 * 24 * time.Hour)
	task, err := taskSvc.Create(project.ID.String(), "Task A", "", person.ID.String(), &dl)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !task.Deadline.Equal(dl) {
		t.Errorf("deadline = %v, want %v", task.Deadline, dl)
	}
}

func TestTaskService_Create_AssigneeNotTeamMember(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := taskSvc.Create(project.ID.String(), "Task A", "", person.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for non-member assignee, got nil")
	}
}

func TestTaskService_Create_MissingAssignee(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := taskSvc.Create(project.ID.String(), "Task A", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty assignee, got nil")
	}
}

func TestTaskService_Create_MissingName(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())

	_, err := taskSvc.Create(project.ID.String(), "", "", person.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestTaskService_Update(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())

	created, _ := taskSvc.Create(project.ID.String(), "Old", "", person.ID.String(), nil)
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

func TestTaskService_Delete(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())

	task, _ := taskSvc.Create(project.ID.String(), "Task A", "", person.ID.String(), nil)
	err := taskSvc.Delete(task.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = taskSvc.Get(task.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestTaskService_LinkUnlinkExternalItem(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupTaskDeps(t)

	org, _ := orgSvc.Create("Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team")
	memberSvc.Add(team.ID.String(), person.ID.String())

	task, _ := taskSvc.Create(project.ID.String(), "Task A", "", person.ID.String(), nil)

	// Link
	linked, err := taskSvc.LinkExternalItem(task.ID.String(), "00000000-0000-0000-0000-000000000001", "42", "https://example.com/42")
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}
	if linked.ExternalItemID == nil || *linked.ExternalItemID != "42" {
		t.Errorf("external_item_id = %v, want 42", linked.ExternalItemID)
	}

	// Unlink
	unlinked, err := taskSvc.UnlinkExternalItem(task.ID.String())
	if err != nil {
		t.Fatalf("unlink failed: %v", err)
	}
	if unlinked.ExternalItemID != nil {
		t.Errorf("expected nil external_item_id after unlink, got %v", *unlinked.ExternalItemID)
	}
}
