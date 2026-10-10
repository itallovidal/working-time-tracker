package projectaccess_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entperson "working-time-tracker/ent/person"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/projectaccess"
	"working-time-tracker/testutil"
)

// O cenário: o Projeto X tem a Ana com valor por hora e o Bruno só num time (o que só existe em dado de antes de o
// valor ser obrigatório); o Projeto Y tem a Carla com valor e o Diego com valor e no grupo de gerente. A Eva não
// está em projeto nenhum, e a Zoe é de outra organização.
type scenario struct {
	x, y                               uuid.UUID
	ana, bruno, carla, diego, eva, zoe uuid.UUID
}

func setup(t *testing.T) scenario {
	t.Helper()
	testutil.Truncate(t, testDB)
	ctx := context.Background()
	org := testClient.Organization.Create().SetName("Org").SaveX(ctx)
	other := testClient.Organization.Create().SetName("Outra").SaveX(ctx)
	person := func(name string, orgID uuid.UUID) uuid.UUID {
		return testClient.Person.Create().SetName(name).SetEmail(name + "@test.com").SetOrganizationID(orgID).SaveX(ctx).ID
	}
	project := func(name string) uuid.UUID {
		return testClient.Project.Create().SetName(name).SetOrganizationID(org.ID).SaveX(ctx).ID
	}
	s := scenario{x: project("X"), y: project("Y")}
	s.ana, s.bruno, s.carla, s.diego, s.eva = person("ana", org.ID), person("bruno", org.ID), person("carla", org.ID), person("diego", org.ID), person("eva", org.ID)
	s.zoe = person("zoe", other.ID)

	alloc := func(projectID, personID uuid.UUID, keys ...string) {
		q := testClient.Allocation.Create().SetProjectID(projectID).SetPersonID(personID).SetPayRateCents(2000)
		if keys != nil {
			q = q.SetPermissions(keys)
		}
		q.SaveX(ctx)
	}
	alloc(s.x, s.ana)
	alloc(s.y, s.carla)
	alloc(s.y, s.diego, permission.CollaboratorsManage)
	teamX := testClient.Team.Create().SetName("Time X").SetProjectID(s.x).SaveX(ctx)
	testClient.TeamMembership.Create().SetTeamID(teamX.ID).SetPersonID(s.bruno).SaveX(ctx)
	return s
}

func names(t *testing.T, client *ent.Client, q *ent.PersonQuery) []string {
	t.Helper()
	people, err := q.Order(ent.Asc(entperson.FieldName)).All(context.Background())
	if err != nil {
		t.Fatalf("query people: %v", err)
	}
	var out []string
	for _, p := range people {
		out = append(out, p.Name)
	}
	return out
}

// Estar no projeto é ter valor por hora nele ou estar em um time dele: quem só está num time (dado antigo) também
// está, e quem não tem nenhum dos dois, não.
func TestPersonIn_RateOrTeam(t *testing.T) {
	s := setup(t)
	if got := names(t, testClient, testClient.Person.Query().Where(projectaccess.PersonIn(s.x))); !slices.Equal(got, []string{"ana", "bruno"}) {
		t.Errorf("people in X = %v, want ana (rate) and bruno (team only)", got)
	}
	if got := names(t, testClient, testClient.Person.Query().Where(projectaccess.PersonIn(s.y))); !slices.Equal(got, []string{"carla", "diego"}) {
		t.Errorf("people in Y = %v, want carla and diego", got)
	}
}

func TestProjectsOfAndHas(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	projectsOf := func(person uuid.UUID) []string {
		var out []string
		ps, err := testClient.Project.Query().Where(projectaccess.ProjectsOf(person)).Order(ent.Asc(entproject.FieldName)).All(ctx)
		if err != nil {
			t.Fatalf("projects of: %v", err)
		}
		for _, p := range ps {
			out = append(out, p.Name)
		}
		return out
	}
	for who, c := range map[string]struct {
		id   uuid.UUID
		want []string
	}{
		"ana (rate)":        {s.ana, []string{"X"}},
		"bruno (team only)": {s.bruno, []string{"X"}},
		"carla":             {s.carla, []string{"Y"}},
		"eva (none)":        {s.eva, nil},
		"zoe (other org)":   {s.zoe, nil},
	} {
		if got := projectsOf(c.id); !slices.Equal(got, c.want) {
			t.Errorf("projects of %s = %v, want %v", who, got, c.want)
		}
	}
	for who, c := range map[string]struct {
		person, project uuid.UUID
		want            bool
	}{
		"ana in X":          {s.ana, s.x, true},
		"bruno in X (team)": {s.bruno, s.x, true},
		"bruno in Y":        {s.bruno, s.y, false},
		"eva in X":          {s.eva, s.x, false},
		"zoe in X":          {s.zoe, s.x, false},
	} {
		got, err := projectaccess.Has(ctx, testClient, c.person, c.project)
		if err != nil || got != c.want {
			t.Errorf("Has(%s) = %v (%v), want %v", who, got, err, c.want)
		}
	}
}

// Os colegas de alguém são ele mesmo e quem está em algum projeto dele, por valor ou por time; quem não está em
// projeto nenhum só vê a si mesmo.
func TestColleaguesOf(t *testing.T) {
	s := setup(t)
	colleagues := func(person uuid.UUID) []string {
		return names(t, testClient, testClient.Person.Query().Where(projectaccess.ColleaguesOf(person)))
	}
	for who, c := range map[string]struct {
		id   uuid.UUID
		want []string
	}{
		"ana":   {s.ana, []string{"ana", "bruno"}},
		"bruno": {s.bruno, []string{"ana", "bruno"}},
		"carla": {s.carla, []string{"carla", "diego"}},
		"eva":   {s.eva, []string{"eva"}},
	} {
		if got := colleagues(c.id); !slices.Equal(got, c.want) {
			t.Errorf("colleagues of %s = %v, want %v", who, got, c.want)
		}
	}
}

// Quem pode pôr gente num projeto em que está (collaborators.manage na alocação) precisa achar qualquer pessoa da
// organização; as outras, não.
func TestManagesPeople(t *testing.T) {
	s := setup(t)
	for who, c := range map[string]struct {
		id   uuid.UUID
		want bool
	}{"diego (collaborators.manage)": {s.diego, true}, "carla (plain)": {s.carla, false}, "bruno (team only)": {s.bruno, false}, "eva (none)": {s.eva, false}} {
		got, err := projectaccess.ManagesPeople(context.Background(), testClient, c.id)
		if err != nil || got != c.want {
			t.Errorf("ManagesPeople(%s) = %v (%v), want %v", who, got, err, c.want)
		}
	}
}

// Quem tem collaborators.manage num projeto alcança as pessoas dele (com valor ou só no time), e só elas: o Diego é
// gerente do Y, então traz a Carla e ele mesmo, e não a Ana nem o Bruno, que são do X.
func TestPeopleOfManagedProjects(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	got, err := projectaccess.PeopleOfManagedProjects(ctx, testClient, s.diego)
	if err != nil {
		t.Fatalf("people of managed projects: %v", err)
	}
	if !got[s.carla] || !got[s.diego] || got[s.ana] || got[s.bruno] || got[s.eva] {
		t.Errorf("Diego's people = %v, want only Carla and Diego", got)
	}
	// Quem não administra projeto nenhum não alcança ninguém.
	if none, _ := projectaccess.PeopleOfManagedProjects(ctx, testClient, s.carla); len(none) != 0 {
		t.Errorf("Carla's people = %v, want none", none)
	}
}
