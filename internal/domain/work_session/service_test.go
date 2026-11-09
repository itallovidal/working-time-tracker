package work_session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/testutil"
)

func setupDeps(t *testing.T) (*organization.Service, *person.Service, *project.Service, *team.Service, *team.MembershipService, *task.Service, *work_session.Service) {
	cleanup(t)

	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), nil)
	wsSvc := work_session.NewService(work_session.NewStore(testClient), task.NewStore(testClient), allocation.NewStore(testClient))

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc
}

// setRate define quanto a pessoa recebe por hora no projeto. Sem isso ela não
// bate ponto.
func setRate(t *testing.T, projectID, personID string, cents int) {
	t.Helper()
	if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(projectID, personID, cents); err != nil {
		t.Fatalf("set rate: %v", err)
	}
}

// join põe a pessoa no time. Um time só aceita quem já está no projeto, então
// ela recebe antes um valor por hora nele; os testes que dependem do valor
// trocam por outro com setRate.
func join(t *testing.T, memberSvc *team.MembershipService, tm *team.Team, personID string) {
	t.Helper()
	setRate(t, tm.ProjectID.String(), personID, 1000)
	if _, err := memberSvc.Add(tm.ID.String(), personID); err != nil {
		t.Fatalf("add member: %v", err)
	}
}

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_ClockInSuccess(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)
	setRate(t, proj.ID.String(), p.ID.String(), 2000)

	session, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("clock in failed: %v", err)
	}
	if session.EndAt != nil {
		t.Error("expected end_at to be nil after clock in")
	}
	if len(session.Tasks) != 1 || session.Tasks[0].TaskID != task1.ID || session.ProjectID != proj.ID {
		t.Errorf("tasks = %+v, want the one the clock in started on, in the project", session.Tasks)
	}
	if session.PersonID.String() != p.ID.String() {
		t.Errorf("person id mismatch")
	}
}

// Bater o ponto numa tarefa sem responsável a torna de quem bateu, mesmo sem time;
// quem bate depois, numa tarefa que já tem dono, não a leva.
func TestService_ClockIn_ClaimsAnUnassignedTask(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	ana, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	bia, _ := personSvc.Create(org.ID.String(), "Bia", "bia@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	free, _ := taskSvc.Create(proj.ID.String(), "Free", "", "", nil)
	setRate(t, proj.ID.String(), ana.ID.String(), 2000)
	setRate(t, proj.ID.String(), bia.ID.String(), 2000)

	if _, err := wsSvc.ClockIn(proj.ID.String(), free.ID.String(), ana.ID.String()); err != nil {
		t.Fatalf("clock in: %v", err)
	}
	got, _ := taskSvc.Get(free.ID.String())
	if got.AssigneeID == nil || *got.AssigneeID != ana.ID {
		t.Fatalf("assignee after the clock in = %v, want Ana", got.AssigneeID)
	}

	if _, err := wsSvc.ClockIn(proj.ID.String(), free.ID.String(), bia.ID.String()); err != nil {
		t.Fatalf("second clock in: %v", err)
	}
	got, _ = taskSvc.Get(free.ID.String())
	if got.AssigneeID == nil || *got.AssigneeID != ana.ID {
		t.Errorf("assignee after another person clocked in = %v, want still Ana", got.AssigneeID)
	}
}

// Um ponto recusado não leva a tarefa.
func TestService_ClockIn_RefusedDoesNotClaim(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	ana, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	free, _ := taskSvc.Create(proj.ID.String(), "Free", "", "", nil)

	if _, err := wsSvc.ClockIn(proj.ID.String(), free.ID.String(), ana.ID.String()); !errors.Is(err, work_session.ErrNoRate) {
		t.Fatalf("err = %v, want ErrNoRate", err)
	}
	if got, _ := taskSvc.Get(free.ID.String()); got.AssigneeID != nil {
		t.Errorf("assignee after a refused clock in = %v, want none", got.AssigneeID)
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
	projA, _ := projSvc.Create(org.ID.String(), "Project A", "", 0, project.Routine{})
	projB, _ := projSvc.Create(org.ID.String(), "Project B", "", 0, project.Routine{})
	tm, _ := teamSvc.Create(projA.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(projA.ID.String(), "Task", "", p.ID.String(), nil)

	_, err := wsSvc.ClockIn(projB.ID.String(), task1.ID.String(), p.ID.String())
	if err == nil {
		t.Fatal("expected error for task in wrong project, got nil")
	}
}

// Bater o ponto numa tarefa a põe em progresso, seja qual for o status; parar ou pausar
// (o clock-out) não a tira de lá, e um ponto recusado não a move.
func TestService_ClockIn_StartsTheTask(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	ana, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	pid := proj.ID.String()
	fresh, _ := taskSvc.Create(pid, "Nova", "", "", nil)
	if fresh.Status != "backlog" {
		t.Fatalf("a new task has status %q, want backlog", fresh.Status)
	}

	// Sem valor por hora o ponto é recusado, e a tarefa continua no backlog.
	if _, err := wsSvc.ClockIn(pid, fresh.ID.String(), ana.ID.String()); !errors.Is(err, work_session.ErrNoRate) {
		t.Fatalf("err = %v, want ErrNoRate", err)
	}
	if got, _ := taskSvc.Get(fresh.ID.String()); got.Status != "backlog" {
		t.Fatalf("status after a refused clock in = %q, want backlog", got.Status)
	}

	setRate(t, pid, ana.ID.String(), 2000)
	status := func(id string) string {
		t.Helper()
		got, err := taskSvc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		return got.Status
	}
	if _, err := wsSvc.ClockIn(pid, fresh.ID.String(), ana.ID.String()); err != nil {
		t.Fatalf("clock in: %v", err)
	}
	if got := status(fresh.ID.String()); got != "in_progress" {
		t.Errorf("status after the clock in = %q, want in_progress", got)
	}
	if _, err := wsSvc.ClockOut(pid, ana.ID.String()); err != nil {
		t.Fatalf("clock out: %v", err)
	}
	if got := status(fresh.ID.String()); got != "in_progress" {
		t.Errorf("status after the clock out = %q, want it to stay in_progress", got)
	}

	// Retomar uma tarefa que já estava aguardando fechamento ou fechada a põe em progresso de novo.
	for _, from := range []string{"awaiting_closure", "closed"} {
		if _, err := taskSvc.UpdateAs("", fresh.ID.String(), "Nova", "", nil, nil, task.Attrs{Status: &from}); err != nil {
			t.Fatal(err)
		}
		if _, err := wsSvc.ClockIn(pid, fresh.ID.String(), ana.ID.String()); err != nil {
			t.Fatalf("clock in on a %s task: %v", from, err)
		}
		if got := status(fresh.ID.String()); got != "in_progress" {
			t.Errorf("status after clocking in on a %s task = %q, want in_progress", from, got)
		}
		if _, err := wsSvc.ClockOut(pid, ana.ID.String()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestService_ClockOut_Success(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)
	setRate(t, proj.ID.String(), p.ID.String(), 2000)

	if _, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)

	session, err := wsSvc.ClockOut(proj.ID.String(), p.ID.String())
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
	orgSvc, personSvc, projSvc, _, _, _, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})

	_, err := wsSvc.ClockOut(proj.ID.String(), p.ID.String())
	if !errors.Is(err, work_session.ErrNotOpen) {
		t.Fatalf("expected no active session error, got %v", err)
	}
}

func TestService_ClockOut_PersonFromAnotherOrganization(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, _, wsSvc := setupDeps(t)

	orgA, _ := orgSvc.Create("Org A")
	orgB, _ := orgSvc.Create("Org B")
	projA, _ := projSvc.Create(orgA.ID.String(), "Project A", "", 0, project.Routine{})
	personB, _ := personSvc.Create(orgB.ID.String(), "Bia", "bia@test.com")

	if _, err := wsSvc.ClockOut(projA.ID.String(), personB.ID.String()); err == nil {
		t.Fatal("expected clock out of a person from another organization to fail")
	}
}

func TestService_OverlappingSessionRejected(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	taskA, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	taskB, _ := taskSvc.Create(proj.ID.String(), "Task B", "", p.ID.String(), nil)
	setRate(t, proj.ID.String(), p.ID.String(), 2000)

	if _, err := wsSvc.ClockIn(proj.ID.String(), taskA.ID.String(), p.ID.String()); err != nil {
		t.Fatal(err)
	}
	_, err := wsSvc.ClockIn(proj.ID.String(), taskB.ID.String(), p.ID.String())
	if err == nil {
		t.Fatal("expected error for overlapping session, got nil")
	}
}

// Uma sessão aberta por projeto (Decisão 27): a pessoa abre o ponto em dois projetos ao mesmo tempo, as duas sessões
// aparecem em Open, e fechar uma deixa a outra correndo.
func TestService_OneOpenSessionPerProject(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	projA, _ := projSvc.Create(org.ID.String(), "Alfa", "", 0, project.Routine{})
	projB, _ := projSvc.Create(org.ID.String(), "Beta", "", 0, project.Routine{})
	tmA, _ := teamSvc.Create(projA.ID.String(), "Team A")
	tmB, _ := teamSvc.Create(projB.ID.String(), "Team B")
	join(t, memberSvc, tmA, p.ID.String())
	join(t, memberSvc, tmB, p.ID.String())
	setRate(t, projA.ID.String(), p.ID.String(), 2000)
	setRate(t, projB.ID.String(), p.ID.String(), 3000)
	taskA, _ := taskSvc.Create(projA.ID.String(), "Task A", "", p.ID.String(), nil)
	taskB, _ := taskSvc.Create(projB.ID.String(), "Task B", "", p.ID.String(), nil)

	a, err := wsSvc.ClockIn(projA.ID.String(), taskA.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("clock in on A: %v", err)
	}
	b, err := wsSvc.ClockIn(projB.ID.String(), taskB.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("clock in on B with A open: %v", err)
	}
	if *a.PayRateCents != 2000 || *b.PayRateCents != 3000 {
		t.Errorf("rates = %d and %d, want each project's own (2000 and 3000)", *a.PayRateCents, *b.PayRateCents)
	}

	open, err := wsSvc.Open(p.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != a.ID || open[1].ID != b.ID {
		t.Fatalf("open = %d sessions, want A then B", len(open))
	}
	if open[0].Project == nil || open[0].Project.Name != "Alfa" || open[1].Project == nil || open[1].Project.Name != "Beta" {
		t.Errorf("open sessions should carry the project name, got %+v and %+v", open[0].Project, open[1].Project)
	}
	if got, _ := wsSvc.OpenIn(p.ID.String(), projB.ID.String()); got == nil || got.ID != b.ID {
		t.Errorf("OpenIn(B) = %v, want the B session", got)
	}

	// Um segundo ponto no mesmo projeto continua recusado.
	taskA2, _ := taskSvc.Create(projA.ID.String(), "Task A2", "", p.ID.String(), nil)
	if _, err := wsSvc.ClockIn(projA.ID.String(), taskA2.ID.String(), p.ID.String()); !errors.Is(err, work_session.ErrAlreadyOpen) {
		t.Fatalf("second clock in on A = %v, want ErrAlreadyOpen", err)
	}

	// Parar o A não mexe no B.
	if _, err := wsSvc.ClockOut(projA.ID.String(), p.ID.String()); err != nil {
		t.Fatalf("clock out of A: %v", err)
	}
	open, _ = wsSvc.Open(p.ID.String())
	if len(open) != 1 || open[0].ID != b.ID {
		t.Fatalf("after closing A, open = %d sessions, want only B", len(open))
	}
	if _, err := wsSvc.ClockOut(projA.ID.String(), p.ID.String()); !errors.Is(err, work_session.ErrNotOpen) {
		t.Errorf("clock out of A again = %v, want ErrNotOpen", err)
	}
}

// MergedSeconds conta uma vez o que a mesma pessoa trabalhou ao mesmo tempo, e soma pessoas diferentes.
func TestMergedSeconds(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 9, h, m, 0, 0, time.UTC) }
	end := func(h, m int) *time.Time { v := at(h, m); return &v }
	ana, bia := uuid.New(), uuid.New()
	now := at(18, 0)
	sessions := []work_session.WorkSession{
		{PersonID: ana, StartAt: at(12, 0), EndAt: end(15, 0)},  // projeto A
		{PersonID: ana, StartAt: at(12, 0), EndAt: end(16, 0)},  // projeto B, ao mesmo tempo
		{PersonID: ana, StartAt: at(16, 30), EndAt: end(17, 0)}, // depois, sem sobrepor
		{PersonID: bia, StartAt: at(12, 0), EndAt: end(13, 0)},  // outra pessoa, mesma hora
		{PersonID: ana, StartAt: at(17, 30)},                    // aberta: conta até now
	}
	cases := []struct {
		name     string
		from, to time.Time
		want     float64
	}{
		{"everything", time.Time{}, time.Time{}, (4 + 0.5 + 1 + 0.5) * 3600},
		{"a window cuts the overlap", at(14, 0), at(17, 0), (2 + 0.5) * 3600},
		{"before anything", at(8, 0), at(9, 0), 0},
	}
	for _, c := range cases {
		if got := work_session.MergedSeconds(sessions, c.from, c.to, now); got != c.want {
			t.Errorf("%s: got %v seconds, want %v", c.name, got, c.want)
		}
	}

	// Uma sessão dentro de outra não estica nem encurta a de fora, e a ordem em que elas chegam não importa.
	contained := []work_session.WorkSession{
		{PersonID: ana, StartAt: at(13, 0), EndAt: end(14, 0)},
		{PersonID: ana, StartAt: at(12, 0), EndAt: end(16, 0)},
		{PersonID: ana, StartAt: at(12, 30), EndAt: end(15, 0)},
	}
	if got := work_session.MergedSeconds(contained, time.Time{}, time.Time{}, now); got != 4*3600 {
		t.Errorf("sessions inside another: got %v seconds, want %v", got, 4*3600)
	}
}

func TestService_TotalTime(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)
	setRate(t, proj.ID.String(), p.ID.String(), 2000)

	if _, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err := wsSvc.ClockOut(proj.ID.String(), p.ID.String()); err != nil {
		t.Fatal(err)
	}

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

// Quem não tem valor por hora no projeto não bate ponto. É o caso de quem foi
// tirado do projeto e ficou com uma tarefa dele.
func TestService_ClockIn_RequiresRate(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	other, _ := projSvc.Create(org.ID.String(), "Other", "", 0, project.Routine{})
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task", "", p.ID.String(), nil)
	if err := collaborator.NewService(collaborator.NewStore(testClient)).Remove(proj.ID.String(), p.ID.String()); err != nil {
		t.Fatalf("remove from the project: %v", err)
	}

	// Ter valor em outro projeto não libera este.
	setRate(t, other.ID.String(), p.ID.String(), 2000)

	if _, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String()); !errors.Is(err, work_session.ErrNoRate) {
		t.Fatalf("clock in without a rate: err = %v, want ErrNoRate", err)
	}
	if open, _ := wsSvc.Open(p.ID.String()); len(open) != 0 {
		t.Error("a session was opened for a person without a rate")
	}

	// Com o valor definido neste projeto, a pessoa bate ponto.
	setRate(t, proj.ID.String(), p.ID.String(), 1000)
	session, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), p.ID.String())
	if err != nil {
		t.Fatalf("clock in with a rate: %v", err)
	}
	if session.PayRateCents == nil || *session.PayRateCents != 1000 {
		t.Errorf("pay_rate_cents = %v, want 1000", session.PayRateCents)
	}
}

// O dono da organização bate ponto sem valor por hora: a sessão guarda valor pago zero
// e o valor cobrado do projeto, e ele passa a ser colaborador do projeto.
func TestService_ClockIn_OwnerWorksAtTheBilledRate(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	owner, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	testClient.Person.UpdateOneID(owner.ID).SetIsOwner(true).ExecX(context.Background())
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	// O valor cobrado é o do cliente (Decisão 21).
	customerID := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Cliente").SetHourlyRateCents(8000).SaveX(context.Background()).ID.String()
	if _, err := projSvc.SetBilling(proj.ID.String(), &customerID, 0); err != nil {
		t.Fatalf("set billing: %v", err)
	}
	task1, err := taskSvc.Create(proj.ID.String(), "Task", "", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	session, err := wsSvc.ClockIn(proj.ID.String(), task1.ID.String(), owner.ID.String())
	if err != nil {
		t.Fatalf("the owner clocks in without a rate: %v", err)
	}
	if !session.OwnerHours || session.PayRateCents == nil || *session.PayRateCents != 0 ||
		session.BillRateCents == nil || *session.BillRateCents != 8000 {
		t.Errorf("owner session = owner %v, pay %v, bill %v; want owner, 0 and 8000", session.OwnerHours, session.PayRateCents, session.BillRateCents)
	}

	// Quem trabalha no projeto é colaborador dele, com valor zero.
	list, err := collaborator.NewService(collaborator.NewStore(testClient)).ListByProject(proj.ID.String())
	if err != nil || len(list) != 1 || !list[0].Person.IsOwner || list[0].PayRateCents == nil || *list[0].PayRateCents != 0 {
		t.Errorf("collaborators after the owner clocked in = %+v, %v; want the owner with a zero rate", list, err)
	}

	// Outra pessoa sem valor continua sem bater ponto.
	other, _ := personSvc.Create(org.ID.String(), "Bia", "bia@test.com")
	task2, err := taskSvc.Create(proj.ID.String(), "Task 2", "", "", nil)
	if err != nil {
		t.Fatalf("create task 2: %v", err)
	}
	if _, err := wsSvc.ClockIn(proj.ID.String(), task2.ID.String(), other.ID.String()); !errors.Is(err, work_session.ErrNoRate) {
		t.Errorf("a non-owner without a rate: err = %v, want ErrNoRate", err)
	}
}

// O valor é copiado para a sessão no clock-in. Mudar o valor depois vale só
// para as sessões seguintes.
func TestService_RateIsSnapshottedAtClockIn(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	projectID, personID := proj.ID.String(), p.ID.String()
	tm, _ := teamSvc.Create(projectID, "Team")
	join(t, memberSvc, tm, personID)
	task1, _ := taskSvc.Create(projectID, "Task", "", personID, nil)

	// Projeto interno: a sessão tem valor pago, mas não tem valor cobrado.
	setRate(t, projectID, personID, 2000)
	first, err := wsSvc.ClockIn(projectID, task1.ID.String(), personID)
	if err != nil {
		t.Fatalf("first clock in: %v", err)
	}
	if first.PayRateCents == nil || *first.PayRateCents != 2000 || first.BillRateCents != nil {
		t.Errorf("first session rates = %v / %v, want 2000 / nil", first.PayRateCents, first.BillRateCents)
	}
	if _, err := wsSvc.ClockOut(projectID, personID); err != nil {
		t.Fatal(err)
	}

	// O admin muda os dois valores: o projeto passa a ter um cliente, que paga 100,00 por hora.
	setRate(t, projectID, personID, 2500)
	customer := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Cliente").SetHourlyRateCents(10000).SaveX(context.Background())
	customerID := customer.ID.String()
	if _, err := projSvc.SetBilling(projectID, &customerID, 0); err != nil {
		t.Fatalf("set billing: %v", err)
	}
	if _, err := wsSvc.ClockIn(projectID, task1.ID.String(), personID); err != nil {
		t.Fatalf("second clock in: %v", err)
	}
	// Mudar de novo com o ponto aberto não mexe na sessão em andamento.
	setRate(t, projectID, personID, 9999)
	if _, err := wsSvc.ClockOut(projectID, personID); err != nil {
		t.Fatal(err)
	}

	// O cliente é reajustado para 120,00 e o projeto ganha 5,00 de acréscimo: vale para a sessão seguinte.
	customer.Update().SetHourlyRateCents(12000).ExecX(context.Background())
	if _, err := projSvc.SetBilling(projectID, &customerID, 500); err != nil {
		t.Fatalf("set the extra: %v", err)
	}
	if _, err := wsSvc.ClockIn(projectID, task1.ID.String(), personID); err != nil {
		t.Fatalf("third clock in: %v", err)
	}
	if _, err := wsSvc.ClockOut(projectID, personID); err != nil {
		t.Fatal(err)
	}

	sessions, err := wsSvc.ListByProject(projectID, nil, nil)
	if err != nil || len(sessions) != 3 {
		t.Fatalf("list = %d sessions, %v; want 3", len(sessions), err)
	}
	newest, middle, oldest := sessions[0], sessions[1], sessions[2]
	if oldest.PayRateCents == nil || *oldest.PayRateCents != 2000 || oldest.BillRateCents != nil {
		t.Errorf("old session rates = %v / %v, want them unchanged (2000 / nil)", oldest.PayRateCents, oldest.BillRateCents)
	}
	if middle.PayRateCents == nil || *middle.PayRateCents != 2500 || middle.BillRateCents == nil || *middle.BillRateCents != 10000 {
		t.Errorf("second session rates = %v / %v, want 2500 / 10000 (the raise came after it)", middle.PayRateCents, middle.BillRateCents)
	}
	if newest.BillRateCents == nil || *newest.BillRateCents != 12500 {
		t.Errorf("third session bill rate = %v, want 12500 (the customer's 120.00 plus the 5.00 extra)", newest.BillRateCents)
	}
}

// O valor de uma sessão é o tempo vezes o valor por hora, arredondado para o
// centavo, e os totais somam as sessões.
func TestService_Amounts(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	projectID, personID := proj.ID.String(), p.ID.String()
	tm, _ := teamSvc.Create(projectID, "Team")
	join(t, memberSvc, tm, personID)
	task1, _ := taskSvc.Create(projectID, "Task", "", personID, nil)

	// Sessões com duração exata, criadas direto no banco.
	start := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	create := func(offset, duration time.Duration, pay, bill *int) {
		end := start.Add(offset + duration)
		testutil.Session(t, testClient, task1.ID, p.ID, start.Add(offset), &end, pay, bill)
	}
	pay, bill := 2000, 10000
	create(0, 90*time.Minute, &pay, &bill)          // 1h30: 30,00 e 150,00
	create(3*time.Hour, 100*time.Second, &pay, nil) // 100s a 20,00/h: 55,56 centavos, arredonda para 56
	create(6*time.Hour, time.Hour, nil, nil)        // sessão antiga, sem valor

	sessions, err := wsSvc.ListByProject(projectID, nil, nil)
	if err != nil || len(sessions) != 3 {
		t.Fatalf("list = %d sessions, %v; want 3", len(sessions), err)
	}
	value := func(v *int) int {
		if v == nil {
			return -1
		}
		return *v
	}
	// A lista vem da mais nova para a mais antiga.
	want := [][2]int{{-1, -1}, {56, -1}, {3000, 15000}}
	for i, s := range sessions {
		if got := [2]int{value(s.PayAmountCents), value(s.BillAmountCents)}; got != want[i] {
			t.Errorf("session %d amounts = %v, want %v", i, got, want[i])
		}
	}

	total, err := wsSvc.TotalTime(projectID, nil, &personID)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total.TotalSeconds != 90*60+100+3600 {
		t.Errorf("total_seconds = %v, want %v", total.TotalSeconds, 90*60+100+3600)
	}
	if value(total.PayAmountCents) != 3056 || value(total.BillAmountCents) != 15000 {
		t.Errorf("total amounts = %d / %d, want 3056 / 15000", value(total.PayAmountCents), value(total.BillAmountCents))
	}

	if _, err := wsSvc.TotalTime(projectID, nil, nil); err == nil {
		t.Error("total without a filter should fail")
	}
	bad := "não-é-uuid"
	if _, err := wsSvc.TotalTime(projectID, &bad, nil); err == nil {
		t.Error("total with an invalid task id should fail")
	}
}
