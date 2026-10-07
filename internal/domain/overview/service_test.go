package overview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/testutil"
)

const day = 24 * time.Hour

// fixture tem os services sobre o banco de teste e os atalhos para criar o que
// a visão geral lê. now é o instante de referência do cenário, em segundos
// inteiros, para as durações e as datas voltarem do banco iguais.
type fixture struct {
	t             *testing.T
	svc           *overview.Service
	projects      *project.Service
	teams         *team.Service
	members       *team.MembershipService
	allocations   *allocation.Service
	collaborators *collaborator.Service
	orgID         string
	now           time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	testutil.Truncate(t, testDB)
	taskStore := task.NewStore(testClient)
	allocationStore := allocation.NewStore(testClient)
	f := &fixture{
		t:             t,
		projects:      project.NewService(project.NewStore(testClient)),
		teams:         team.NewService(team.NewStore(testClient)),
		members:       team.NewMembershipService(team.NewMembershipStore(testClient)),
		allocations:   allocation.NewService(allocationStore),
		collaborators: collaborator.NewService(collaborator.NewStore(testClient)),
		now:           time.Now().Truncate(time.Second),
	}
	f.svc = overview.NewService(overview.Deps{
		Projects:      f.projects,
		Collaborators: f.collaborators,
		Teams:         f.teams,
		Sessions:      work_session.NewService(work_session.NewStore(testClient), taskStore, allocationStore),
		Integrations:  integration.NewService(integration.NewStore(testClient), "test-key"),
		Tasks:         taskStore,
	})
	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	if err != nil {
		t.Fatalf("fixture org: %v", err)
	}
	f.orgID = org.ID.String()
	return f
}

func cents(v int) *int { return &v }

func (f *fixture) person(name string) uuid.UUID {
	f.t.Helper()
	p, err := person.NewService(person.NewStore(testClient)).Create(f.orgID, name, strings.ToLower(name)+"@test.com")
	if err != nil {
		f.t.Fatalf("fixture person %s: %v", name, err)
	}
	return p.ID
}

// project é o único lugar que cria projeto, para uma mudança na assinatura de
// Create ser acertada num ponto só.
func (f *fixture) project(name string) string {
	f.t.Helper()
	p, err := f.projects.Create(f.orgID, name, "", 0, nil, nil)
	if err != nil {
		f.t.Fatalf("fixture project %s: %v", name, err)
	}
	return p.ID.String()
}

func (f *fixture) team(projectID, name string) string {
	f.t.Helper()
	tm, err := f.teams.Create(projectID, name)
	if err != nil {
		f.t.Fatalf("fixture team %s: %v", name, err)
	}
	return tm.ID.String()
}

func (f *fixture) rate(projectID string, personID uuid.UUID, payRateCents int) {
	f.t.Helper()
	if _, err := f.allocations.Set(projectID, personID.String(), payRateCents); err != nil {
		f.t.Fatalf("fixture rate: %v", err)
	}
}

func (f *fixture) join(teamID string, personID uuid.UUID) {
	f.t.Helper()
	if _, err := f.members.Add(teamID, personID.String()); err != nil {
		f.t.Fatalf("fixture member: %v", err)
	}
}

// task cria a tarefa direto no banco: o que interessa aqui é o prazo, não as
// regras de quem pode ser responsável.
func (f *fixture) task(projectID string, assignee uuid.UUID, deadline *time.Time) uuid.UUID {
	f.t.Helper()
	q := testClient.Task.Create().SetProjectID(uuid.MustParse(projectID)).SetName("Tarefa").SetAssigneeID(assignee)
	if deadline != nil {
		q = q.SetDeadline(*deadline)
	}
	return q.SaveX(context.Background()).ID
}

// session grava uma sessão fechada que começou há startedAgo e durou length,
// com os valores por hora que ficariam travados no clock-in.
func (f *fixture) session(taskID, personID uuid.UUID, startedAgo, length time.Duration, payRate, billRate *int) {
	f.t.Helper()
	start := f.now.Add(-startedAgo)
	testClient.WorkSession.Create().SetTaskID(taskID).SetPersonID(personID).
		SetStartAt(start).SetEndAt(start.Add(length)).
		SetNillablePayRateCents(payRate).SetNillableBillRateCents(billRate).
		SaveX(context.Background())
}

func (f *fixture) get(projectID string) *overview.Overview {
	f.t.Helper()
	o, err := f.svc.Get(projectID)
	if err != nil {
		f.t.Fatalf("overview: %v", err)
	}
	return o
}

func show(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprint(*v)
}

// describe resume uma linha de "por pessoa".
func describe(p overview.PersonTotal) string {
	where := "in the project"
	if !p.InProject {
		where = "left the project"
	}
	return fmt.Sprintf("%s: %.0fs in %d sessions, pay %s, bill %s, %s",
		p.Person.Name, p.TotalSeconds, p.SessionCount, show(p.PayAmountCents), show(p.BillAmountCents), where)
}

// O cenário: o Projeto X é de um cliente que paga 100,00 por hora e foi
// cadastrado há 95 dias. A Ana tem valor e está em dois times, o Bruno só tem
// valor, a Carla está num time sem valor e o Diego trabalhou e saiu. Há um time
// vazio, três tarefas (uma atrasada, uma no prazo, uma sem prazo) e seis
// sessões. O Projeto Y, da mesma organização, tem tarefa atrasada e sessão que
// não podem entrar na conta do X.
func TestService_Totals(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ana, bruno, carla, diego := f.person("Ana"), f.person("Bruno"), f.person("Carla"), f.person("Diego")

	x, y := f.project("Projeto X"), f.project("Projeto Y")
	started := f.now.Add(-95 * day)
	if _, err := testDB.Exec(`UPDATE projects SET created_at = $1 WHERE id = $2`, started, x); err != nil {
		t.Fatalf("backdate project: %v", err)
	}
	name := "Cliente"
	c, err := customer.NewService(customer.NewStore(testClient)).Create(f.orgID, customer.Input{Name: &name})
	if err != nil {
		t.Fatalf("fixture customer: %v", err)
	}
	customerID := c.ID.String()
	if _, err := f.projects.SetBilling(x, &customerID, cents(10000)); err != nil {
		t.Fatalf("fixture billing: %v", err)
	}

	// O valor vem antes do time, que é a ordem em que uma pessoa entra no projeto.
	f.rate(x, ana, 2000)
	f.rate(x, bruno, 2000)
	f.rate(x, diego, 3000)
	backend, mobile := f.team(x, "Backend"), f.team(x, "Mobile")
	f.team(x, "Vazio")
	f.join(backend, ana)
	f.join(mobile, ana)
	// A Carla entra direto no banco: é a linha de quem está num time sem valor.
	testClient.TeamMembership.Create().SetTeamID(uuid.MustParse(backend)).SetPersonID(carla).SaveX(ctx)

	late, onTime := f.now.Add(-2*day), f.now.Add(5*day)
	overdue, due, open := f.task(x, ana, &late), f.task(x, ana, &onTime), f.task(x, ana, nil)
	other := f.task(y, ana, &late)

	pay, bill := cents(2000), cents(10000)
	f.session(overdue, ana, 40*day, 2*time.Hour, pay, bill)      // 40,00 e 200,00, fora dos 30 dias
	f.session(overdue, ana, 10*day, 90*time.Minute, pay, bill)   // 30,00 e 150,00
	f.session(due, bruno, 3*day, 100*time.Second, pay, bill)     // 0,56 e 2,78: 55,56 e 277,78 arredondados
	f.session(due, bruno, 2*day, 100*time.Second, pay, bill)     // outra igual: a soma é 1,12, e não 1,11
	f.session(open, diego, 20*day, time.Hour, cents(3000), bill) // 30,00 e 100,00
	f.session(open, carla, 5*day, 30*time.Minute, nil, nil)      // de antes dos valores: só conta o tempo
	f.session(other, ana, day, time.Hour, pay, bill)             // do Projeto Y
	if err := f.collaborators.Remove(x, diego.String()); err != nil {
		t.Fatalf("remove Diego: %v", err)
	}

	got := f.get(x)

	if p := got.Project; p.Name != "Projeto X" || !p.CreatedAt.Equal(started) || p.Age != (overview.Age{Days: 95, Weeks: 13, Months: 3}) {
		t.Errorf("project = %q created %s age %+v, want Projeto X created %s age 95 days, 13 weeks, 3 months", p.Name, p.CreatedAt, p.Age, started)
	}
	if p := got.Project; p.Customer == nil || p.Customer.Name != "Cliente" || p.BillRateCents == nil || *p.BillRateCents != 10000 {
		t.Errorf("billing = customer %+v, rate %s; want Cliente at 10000", p.Customer, show(p.BillRateCents))
	}
	// Ana, Bruno e Carla: a Ana conta uma vez, e o Diego já saiu.
	if want := (overview.People{Total: 3, WithoutTeam: 1, WithoutRate: 1}); got.People != want {
		t.Errorf("people = %+v, want %+v", got.People, want)
	}
	if got.Teams.Total != 3 {
		t.Errorf("teams = %d, want 3 (the empty one counts)", got.Teams.Total)
	}
	if want := (overview.Tasks{Total: 3, Overdue: 1}); got.Tasks != want {
		t.Errorf("tasks = %+v, want %+v", got.Tasks, want)
	}

	tm := got.Time
	if tm.TotalSeconds != 18200 || tm.SessionCount != 6 {
		t.Errorf("time = %.0fs in %d sessions, want 18200s in 6", tm.TotalSeconds, tm.SessionCount)
	}
	if tm.Last7DaysSeconds != 2000 || tm.Last30DaysSeconds != 11000 {
		t.Errorf("last 7 days = %.0fs, last 30 days = %.0fs; want 2000s and 11000s", tm.Last7DaysSeconds, tm.Last30DaysSeconds)
	}
	first, last := f.now.Add(-40*day), f.now.Add(-2*day)
	if tm.FirstSessionAt == nil || !tm.FirstSessionAt.Equal(first) || tm.LastSessionAt == nil || !tm.LastSessionAt.Equal(last) {
		t.Errorf("first and last session = %v and %v, want %s and %s", tm.FirstSessionAt, tm.LastSessionAt, first, last)
	}

	m := got.Money
	if show(m.PayAmountCents) != "10112" || show(m.BillAmountCents) != "45556" || show(m.MarginCents) != "35444" {
		t.Errorf("money = pay %s, bill %s, margin %s; want 10112, 45556 and 35444", show(m.PayAmountCents), show(m.BillAmountCents), show(m.MarginCents))
	}

	rows := make([]string, len(got.ByPerson))
	var seconds float64
	var paid, billed int
	for i, p := range got.ByPerson {
		rows[i] = describe(p)
		seconds += p.TotalSeconds
		if p.PayAmountCents != nil {
			paid += *p.PayAmountCents
		}
		if p.BillAmountCents != nil {
			billed += *p.BillAmountCents
		}
		if p.WorkingNow {
			t.Errorf("%s is marked as working now, and every session is closed", p.Person.Name)
		}
	}
	want := []string{
		"Ana: 12600s in 2 sessions, pay 7000, bill 35000, in the project",
		"Diego: 3600s in 1 sessions, pay 3000, bill 10000, left the project",
		"Carla: 1800s in 1 sessions, pay -, bill -, in the project",
		"Bruno: 200s in 2 sessions, pay 112, bill 556, in the project",
	}
	if !slices.Equal(rows, want) {
		t.Errorf("by person:\n got %q\nwant %q", rows, want)
	}
	// As linhas somam os totais, mesmo com quem já saiu do projeto.
	if seconds != tm.TotalSeconds || paid != *m.PayAmountCents || billed != *m.BillAmountCents {
		t.Errorf("rows add up to %.0fs, pay %d, bill %d; totals are %.0fs, %d, %d", seconds, paid, billed, tm.TotalSeconds, *m.PayAmountCents, *m.BillAmountCents)
	}
}

// A sessão aberta conta até agora no tempo e no valor, e marca quem está nela.
func TestService_OpenSession(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	x := f.project("Projeto X")
	f.rate(x, ana, 6000)
	testClient.WorkSession.Create().SetTaskID(f.task(x, ana, nil)).SetPersonID(ana).
		SetStartAt(f.now.Add(-30 * time.Minute)).SetPayRateCents(6000).SaveX(context.Background())

	got := f.get(x)

	// Meia hora a 60,00 são 30,00. A folga cobre o tempo que o próprio teste leva.
	if s := got.Time.TotalSeconds; s < 1800 || s > 1830 || got.Time.SessionCount != 1 || got.Time.Last7DaysSeconds != s {
		t.Errorf("time = %.1fs in %d sessions, last 7 days %.1fs; want about 1800s in 1, all of it in the window", s, got.Time.SessionCount, got.Time.Last7DaysSeconds)
	}
	if pay := got.Money.PayAmountCents; pay == nil || *pay < 3000 || *pay > 3050 {
		t.Errorf("pay = %s, want about 3000", show(pay))
	}
	if got.People.WorkingNow != 1 {
		t.Errorf("working now = %d, want 1", got.People.WorkingNow)
	}
	if len(got.ByPerson) != 1 || !got.ByPerson[0].WorkingNow {
		t.Errorf("by person = %+v, want Ana marked as working now", got.ByPerson)
	}
}

// As janelas de 7 e 30 dias contam só o trecho da sessão que caiu dentro delas.
func TestService_WindowClipsSession(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	x := f.project("Projeto X")
	// De 8 dias atrás até 6 dias atrás: um dos dois dias está dentro dos 7.
	f.session(f.task(x, ana, nil), ana, 8*day, 2*day, cents(1000), nil)

	got := f.get(x).Time

	if got.TotalSeconds != 172800 || got.Last30DaysSeconds != 172800 {
		t.Errorf("total = %.0fs, last 30 days = %.0fs; want 172800s in both", got.TotalSeconds, got.Last30DaysSeconds)
	}
	// A janela anda com o relógio, então o trecho encolhe um pouco até a conta rodar.
	if s := got.Last7DaysSeconds; s > 86400 || s < 86370 {
		t.Errorf("last 7 days = %.1fs, want just under 86400s (one of the two days)", s)
	}
}

// Um projeto recém-criado responde com zeros, sem valores e com listas vazias,
// não com null onde a tela espera uma lista.
func TestService_EmptyProject(t *testing.T) {
	f := setup(t)

	got := f.get(f.project("Projeto novo"))

	if got.People != (overview.People{}) || got.Teams.Total != 0 || got.Tasks != (overview.Tasks{}) || got.Integrations.Total != 0 {
		t.Errorf("empty project counts: people %+v, teams %d, tasks %+v, integrations %d", got.People, got.Teams.Total, got.Tasks, got.Integrations.Total)
	}
	if got.Time != (overview.Time{}) || got.Money != (overview.Money{}) || got.Project.Age != (overview.Age{}) {
		t.Errorf("empty project: time %+v, money %+v, age %+v; want all zero", got.Time, got.Money, got.Project.Age)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"by_person":[]`, `"items":[]`, `"customer":null`, `"bill_rate_cents":null`,
		`"pay_amount_cents":null`, `"bill_amount_cents":null`, `"margin_cents":null`,
		`"first_session_at":null`, `"last_session_at":null`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("empty project JSON lacks %s: %s", want, body)
		}
	}
}

// Projeto interno não tem cliente nem valor cobrado: o custo aparece, e a
// receita e a margem ficam sem valor.
func TestService_InternalProject(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	x := f.project("Site da empresa")
	f.session(f.task(x, ana, nil), ana, day, time.Hour, cents(4000), nil)

	got := f.get(x)

	if got.Project.Customer != nil || got.Project.BillRateCents != nil {
		t.Errorf("internal project shows customer %+v and rate %s", got.Project.Customer, show(got.Project.BillRateCents))
	}
	if m := got.Money; show(m.PayAmountCents) != "4000" || m.BillAmountCents != nil || m.MarginCents != nil {
		t.Errorf("money = pay %s, bill %s, margin %s; want 4000 and no revenue or margin", show(m.PayAmountCents), show(m.BillAmountCents), show(m.MarginCents))
	}
}

// A lista de integrações diz o que cada uma é e se está em uso, e nada da
// credencial nem dos campos da plataforma.
func TestService_Integrations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	x, y := f.project("Projeto X"), f.project("Projeto Y")
	create := func(projectID, kind, name string, enabled bool, credentials map[string]interface{}, createdAgo time.Duration) {
		testClient.Integration.Create().SetProjectID(uuid.MustParse(projectID)).SetType(kind).SetDisplayName(name).
			SetEnabled(enabled).SetCredentials(credentials).SetMetadata(map[string]interface{}{"repository": "jatoba/segredo-do-repo"}).
			SetCreatedAt(f.now.Add(-createdAgo)).SaveX(ctx)
	}
	create(x, "github", "Repositório", true, map[string]interface{}{"encrypted": "token-cifrado"}, 2*day)
	create(x, "trello", "Quadro", false, nil, day)
	create(y, "gitlab", "De outro projeto", true, nil, day)

	got := f.get(x)

	if got.Integrations.Total != 2 || got.Integrations.Enabled != 1 {
		t.Errorf("integrations = %d, %d enabled; want 2 and 1", got.Integrations.Total, got.Integrations.Enabled)
	}
	rows := make([]string, len(got.Integrations.Items))
	for i, it := range got.Integrations.Items {
		rows[i] = fmt.Sprintf("%s %s enabled=%t token=%t", it.Type, it.DisplayName, it.Enabled, it.HasToken)
	}
	// A mais nova primeiro, como na aba Integrações.
	want := []string{"trello Quadro enabled=false token=false", "github Repositório enabled=true token=true"}
	if !slices.Equal(rows, want) {
		t.Errorf("items:\n got %q\nwant %q", rows, want)
	}
	body, _ := json.Marshal(got)
	for _, secret := range []string{"token-cifrado", "segredo-do-repo", "credentials", "metadata"} {
		if strings.Contains(string(body), secret) {
			t.Errorf("overview JSON carries %q: %s", secret, body)
		}
	}
}

func TestService_NotFound(t *testing.T) {
	f := setup(t)

	for name, id := range map[string]string{"malformed id": "not-a-uuid", "unknown project": uuid.NewString()} {
		if _, err := f.svc.Get(id); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}
