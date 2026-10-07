package collaborator_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

// O cenário: no Projeto X, a Ana tem valor e está em dois times, o Bruno só
// tem valor e o Diego está fora. A Carla está num time sem valor, o que só
// existe em banco de antes de o valor ser obrigatório. No Projeto Y, a Ana tem
// outro valor e está num time, que não pode aparecer no X.
type fixture struct {
	svc                       *collaborator.Service
	allocationSvc             *allocation.Service
	memberSvc                 *team.MembershipService
	taskSvc                   *task.Service
	projectX, projectY        string
	ana, bruno, carla, diego  string
	backend, mobile, backendY string
}

func setup(t *testing.T) fixture {
	t.Helper()
	testutil.Truncate(t, testDB)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	membershipStore := team.NewMembershipStore(testClient)
	f := fixture{
		svc:           collaborator.NewService(collaborator.NewStore(testClient)),
		allocationSvc: allocation.NewService(allocation.NewStore(testClient)),
		memberSvc:     team.NewMembershipService(membershipStore),
		taskSvc:       task.NewService(task.NewStore(testClient), membershipStore, nil),
	}

	org, err := orgSvc.Create("Org")
	if err != nil {
		t.Fatalf("fixture org: %v", err)
	}
	orgID := org.ID.String()
	create := func(name string) string {
		t.Helper()
		p, err := personSvc.Create(orgID, name, name+"@test.com")
		if err != nil {
			t.Fatalf("fixture person %s: %v", name, err)
		}
		return p.ID.String()
	}
	f.ana, f.bruno, f.carla, f.diego = create("Ana"), create("Bruno"), create("Carla"), create("Diego")

	x, _ := projSvc.Create(orgID, "Projeto X", "", 0, nil, nil, nil)
	y, _ := projSvc.Create(orgID, "Projeto Y", "", 0, nil, nil, nil)
	f.projectX, f.projectY = x.ID.String(), y.ID.String()
	newTeam := func(projectID, name string) string {
		t.Helper()
		tm, err := teamSvc.Create(projectID, name)
		if err != nil {
			t.Fatalf("fixture team %s: %v", name, err)
		}
		return tm.ID.String()
	}
	// Mobile é criado antes de Backend: a lista tem que vir por nome, não por criação.
	f.mobile, f.backend, f.backendY = newTeam(f.projectX, "Mobile"), newTeam(f.projectX, "Backend"), newTeam(f.projectY, "Backend Y")

	for _, a := range []struct {
		project, person string
		cents           int
	}{{f.projectX, f.ana, 9000}, {f.projectX, f.bruno, 0}, {f.projectY, f.ana, 11000}} {
		if _, err := f.allocationSvc.Set(a.project, a.person, a.cents); err != nil {
			t.Fatalf("fixture rate: %v", err)
		}
	}
	for _, m := range [][2]string{{f.mobile, f.ana}, {f.backend, f.ana}, {f.backendY, f.ana}} {
		if _, err := f.memberSvc.Add(m[0], m[1]); err != nil {
			t.Fatalf("fixture member: %v", err)
		}
	}
	// A Carla entra direto pelo Ent: o service já não deixa pôr num time quem
	// não tem valor.
	testClient.TeamMembership.Create().
		SetTeamID(uuid.MustParse(f.backend)).SetPersonID(uuid.MustParse(f.carla)).
		SaveX(context.Background())
	return f
}

// describe resume um colaborador numa linha: nome, valor e times.
func describe(c collaborator.Collaborator) string {
	rate := "no rate"
	if c.PayRateCents != nil {
		rate = fmt.Sprintf("rate %d", *c.PayRateCents)
	}
	teams := make([]string, len(c.Teams))
	for i, tm := range c.Teams {
		teams[i] = tm.Name
	}
	return fmt.Sprintf("%s: %s, teams %v", c.Person.Name, rate, teams)
}

// A lista traz quem tem valor por hora no projeto, com os times de cada um, e
// também quem ficou num time sem valor, para um admin poder resolver.
func TestService_ListByProject_RatesAndTeams(t *testing.T) {
	f := setup(t)

	list, err := f.svc.ListByProject(f.projectX)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, len(list))
	for i, c := range list {
		got[i] = describe(c)
	}
	want := []string{
		"Ana: rate 9000, teams [Backend Mobile]", // valor e dois times, só os deste projeto
		"Bruno: rate 0, teams []",                // valor zero é valor; sem time
		"Carla: no rate, teams [Backend]",        // num time sem valor, de antes da regra
	}
	if !slices.Equal(got, want) {
		t.Errorf("collaborators of X:\n got %q\nwant %q", got, want)
	}
	for _, c := range list {
		if c.Teams == nil {
			t.Errorf("%s: teams is nil, want an empty list", c.Person.Name)
		}
	}

	other, _ := f.svc.ListByProject(f.projectY)
	if len(other) != 1 || describe(other[0]) != "Ana: rate 11000, teams [Backend Y]" {
		t.Errorf("collaborators of Y = %+v, want only Ana with the rate and the team of Y", other)
	}
}

// Remover tira o valor e todos os times deste projeto, e mais nada: o outro
// projeto, as tarefas e as sessões de trabalho ficam.
func TestService_Remove(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tk, err := f.taskSvc.Create(f.projectX, "Tarefa da Ana", "", f.ana, nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	testClient.WorkSession.Create().SetTaskID(tk.ID).SetPersonID(uuid.MustParse(f.ana)).
		SetStartAt(time.Now().Add(-time.Hour)).SetEndAt(time.Now()).SaveX(ctx)

	if err := f.svc.Remove(f.projectX, f.ana); err != nil {
		t.Fatalf("remove: %v", err)
	}

	left, _ := f.svc.ListByProject(f.projectX)
	for _, c := range left {
		if c.Person.Name == "Ana" {
			t.Errorf("Ana is still a collaborator of X: %s", describe(c))
		}
	}
	if len(left) != 2 {
		t.Errorf("X has %d collaborators left, want Bruno and Carla", len(left))
	}
	if other, _ := f.svc.ListByProject(f.projectY); len(other) != 1 || describe(other[0]) != "Ana: rate 11000, teams [Backend Y]" {
		t.Errorf("collaborators of Y after removing Ana from X = %+v, want her untouched", other)
	}
	if still, err := f.taskSvc.Get(tk.ID.String()); err != nil || still.AssigneeID.String() != f.ana {
		t.Errorf("task after removal: %+v, err %v; want it still assigned to Ana", still, err)
	}
	if n := testClient.WorkSession.Query().CountX(ctx); n != 1 {
		t.Errorf("work sessions after removal = %d, want 1", n)
	}

	// Quem ficou num time sem valor também sai.
	if err := f.svc.Remove(f.projectX, f.carla); err != nil {
		t.Errorf("remove a person with only a team: %v", err)
	}
	// Quem só tinha valor, sem time, também.
	if err := f.svc.Remove(f.projectX, f.bruno); err != nil {
		t.Errorf("remove a person with only a rate: %v", err)
	}
	if left, _ := f.svc.ListByProject(f.projectX); len(left) != 0 {
		t.Errorf("X still has collaborators: %+v", left)
	}
}

func TestService_Remove_NotACollaborator(t *testing.T) {
	f := setup(t)

	for name, personID := range map[string]string{
		"outside the project": f.diego,
		"unknown person":      uuid.NewString(),
		"malformed id":        "not-a-uuid",
	} {
		if err := f.svc.Remove(f.projectX, personID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
	if err := f.svc.Remove("not-a-uuid", f.ana); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("malformed project id: err = %v, want ErrNotFound", err)
	}
	// Uma remoção recusada não mexe em nada.
	if list, _ := f.svc.ListByProject(f.projectX); len(list) != 3 {
		t.Errorf("X has %d collaborators after refused removals, want 3", len(list))
	}
}
