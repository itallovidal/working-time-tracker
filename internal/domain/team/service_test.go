package team_test

import (
	"testing"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/team"
)

func cleanup(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE team_memberships CASCADE")
	testDB.Exec("TRUNCATE TABLE teams CASCADE")
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE people CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := team.NewService(team.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	tm, err := svc.Create(proj.ID.String(), "Team A")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if tm.Name != "Team A" {
		t.Errorf("name = %q, want %q", tm.Name, "Team A")
	}
}

func TestService_ListByProject(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := team.NewService(team.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	svc.Create(proj.ID.String(), "Team A")
	svc.Create(proj.ID.String(), "Team B")

	teams, err := svc.ListByProject(proj.ID.String())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(teams) != 2 {
		t.Errorf("got %d teams, want 2", len(teams))
	}
}

func TestService_AddRemoveMembers(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team A")

	membership, err := memberSvc.Add(tm.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("add member failed: %v", err)
	}
	if membership.PersonID.String() != p.ID.String() {
		t.Errorf("person id mismatch")
	}
	if membership.TeamID.String() != tm.ID.String() {
		t.Errorf("team id mismatch")
	}

	members, err := memberSvc.ListByTeam(tm.ID.String())
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("got %d members, want 1", len(members))
	}

	err = memberSvc.Remove(tm.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("remove member failed: %v", err)
	}

	members, _ = memberSvc.ListByTeam(tm.ID.String())
	if len(members) != 0 {
		t.Errorf("got %d members after remove, want 0", len(members))
	}
}

func TestMembership_DuplicateRejected(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team A")

	memberSvc.Add(tm.ID.String(), p.ID.String())
	_, err := memberSvc.Add(tm.ID.String(), p.ID.String())
	if err == nil {
		t.Fatal("expected error for duplicate membership, got nil")
	}
}
