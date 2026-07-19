package service

import (
	"testing"
	"working-time-tracker/internal/store"
)

func TestTeamService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewTeamService(store.NewTeamStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	project, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	team, err := svc.Create(project.ID.String(), "Team A")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if team.Name != "Team A" {
		t.Errorf("name = %q, want %q", team.Name, "Team A")
	}
}

func TestTeamService_ListByProject(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewTeamService(store.NewTeamStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	project, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	svc.Create(project.ID.String(), "Team A")
	svc.Create(project.ID.String(), "Team B")

	teams, err := svc.ListByProject(project.ID.String())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(teams) != 2 {
		t.Errorf("got %d teams, want 2", len(teams))
	}
}

func TestTeamService_AddRemoveMembers(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	personSvc := NewPersonService(store.NewPersonStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	teamSvc := NewTeamService(store.NewTeamStore(testDB))
	memberSvc := NewTeamMembershipService(store.NewTeamMembershipStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team A")

	// Add member
	membership, err := memberSvc.Add(team.ID.String(), person.ID.String())
	if err != nil {
		t.Fatalf("add member failed: %v", err)
	}
	if membership.PersonID.String() != person.ID.String() {
		t.Errorf("person id mismatch")
	}
	if membership.TeamID.String() != team.ID.String() {
		t.Errorf("team id mismatch")
	}

	// List members
	members, err := memberSvc.ListByTeam(team.ID.String())
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("got %d members, want 1", len(members))
	}

	// Remove member
	err = memberSvc.Remove(team.ID.String(), person.ID.String())
	if err != nil {
		t.Fatalf("remove member failed: %v", err)
	}

	members, _ = memberSvc.ListByTeam(team.ID.String())
	if len(members) != 0 {
		t.Errorf("got %d members after remove, want 0", len(members))
	}
}

func TestTeamMembership_DuplicateRejected(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	personSvc := NewPersonService(store.NewPersonStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	teamSvc := NewTeamService(store.NewTeamStore(testDB))
	memberSvc := NewTeamMembershipService(store.NewTeamMembershipStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	person, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	project, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	team, _ := teamSvc.Create(project.ID.String(), "Team A")

	memberSvc.Add(team.ID.String(), person.ID.String())
	_, err := memberSvc.Add(team.ID.String(), person.ID.String())
	if err == nil {
		t.Fatal("expected error for duplicate membership, got nil")
	}
}
