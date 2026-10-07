package team_test

import (
	"context"
	"errors"
	"testing"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

// setRate põe a pessoa no projeto, com um valor por hora: é o que a deixa
// entrar num time dele.
func setRate(t *testing.T, projectID, personID string) {
	t.Helper()
	if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(projectID, personID, 1000); err != nil {
		t.Fatalf("set rate: %v", err)
	}
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
	setRate(t, proj.ID.String(), p.ID.String())

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
	setRate(t, proj.ID.String(), p.ID.String())

	if _, err := memberSvc.Add(tm.ID.String(), p.ID.String()); err != nil {
		t.Fatalf("add member: %v", err)
	}
	_, err := memberSvc.Add(tm.ID.String(), p.ID.String())
	if err == nil {
		t.Fatal("expected error for duplicate membership, got nil")
	}
}

// Um time só aceita quem já está no projeto dele, com valor por hora. Ter
// valor em outro projeto não conta.
func TestMembership_RequiresRateInTheProject(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Projeto", "", 0, nil, nil)
	other, _ := projSvc.Create(org.ID.String(), "Outro", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Time")
	setRate(t, other.ID.String(), p.ID.String())

	if _, err := memberSvc.Add(tm.ID.String(), p.ID.String()); !errors.Is(err, team.ErrNoRate) {
		t.Fatalf("add a person without a rate in the project: err = %v, want ErrNoRate", err)
	}
	if members, _ := memberSvc.ListByTeam(tm.ID.String()); len(members) != 0 {
		t.Errorf("the refused person is in the team: %+v", members)
	}

	// Zero é um valor: a pessoa está no projeto, só não recebe por hora.
	if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(proj.ID.String(), p.ID.String(), 0); err != nil {
		t.Fatalf("set a zero rate: %v", err)
	}
	if _, err := memberSvc.Add(tm.ID.String(), p.ID.String()); err != nil {
		t.Errorf("add a person with a zero rate: %v", err)
	}
}

func TestService_Delete_CascadesMemberships(t *testing.T) {
	cleanup(t)
	ctx := context.Background()
	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Projeto", "", 0, nil, nil)
	tm, _ := svc.Create(proj.ID.String(), "Time")
	setRate(t, proj.ID.String(), p.ID.String())
	if _, err := memberSvc.Add(tm.ID.String(), p.ID.String()); err != nil {
		t.Fatalf("add member: %v", err)
	}

	if err := svc.Delete(tm.ID.String()); err != nil {
		t.Fatalf("delete team with members failed: %v", err)
	}
	if n := testClient.TeamMembership.Query().CountX(ctx); n != 0 {
		t.Errorf("team_memberships: %d rows left, want 0", n)
	}
	if n := testClient.Person.Query().CountX(ctx); n != 1 {
		t.Errorf("persons: %d rows, want 1", n)
	}
}

func TestMembership_PersonFromAnotherOrgRejected(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))

	org, _ := orgSvc.Create("Org")
	other, _ := orgSvc.Create("Outra Org")
	outsider, _ := personSvc.Create(other.ID.String(), "Caio", "caio@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Projeto", "", 0, nil, nil)
	tm, _ := svc.Create(proj.ID.String(), "Time")

	if _, err := memberSvc.Add(tm.ID.String(), outsider.ID.String()); err == nil {
		t.Fatal("expected error when adding a person from another organization")
	}
}
