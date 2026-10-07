package work_session_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/testutil"
)

// sessionFixture é a Ana, com 20,00 por hora, num projeto com tarefas sem responsável.
type sessionFixture struct {
	t     *testing.T
	ws    *work_session.Service
	tasks *task.Service
	proj  string
	org   string
	ana   uuid.UUID
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	ana, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	setRate(t, proj.ID.String(), ana.ID.String(), 2000)
	return &sessionFixture{t: t, ws: wsSvc, tasks: taskSvc, proj: proj.ID.String(), org: org.ID.String(), ana: ana.ID}
}

func (f *sessionFixture) task(name string) string {
	f.t.Helper()
	created, err := f.tasks.Create(f.proj, name, "", "", nil)
	if err != nil {
		f.t.Fatalf("create task %s: %v", name, err)
	}
	return created.ID.String()
}

func (f *sessionFixture) status(taskID string) string {
	f.t.Helper()
	got, err := f.tasks.Get(taskID)
	if err != nil {
		f.t.Fatal(err)
	}
	return got.Status
}

// past grava uma sessão encerrada da Ana na tarefa, que começou há ago e durou length.
func (f *sessionFixture) past(taskID string, ago, length time.Duration) *work_session.WorkSession {
	f.t.Helper()
	start := time.Now().Add(-ago).Truncate(time.Second)
	end := start.Add(length)
	pay := 2000
	created := testutil.Session(f.t, testClient, uuid.MustParse(taskID), f.ana, start, &end, &pay, nil)
	got, err := f.ws.Get(f.proj, created.ID.String())
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func (f *sessionFixture) total(taskID, personID string) *work_session.TotalTimeResult {
	f.t.Helper()
	var tp, pp *string
	if taskID != "" {
		tp = &taskID
	}
	if personID != "" {
		pp = &personID
	}
	got, err := f.ws.TotalTime(f.proj, tp, pp)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func near(got, want float64) bool { return math.Abs(got-want) < 0.5 }

func linkOf(t *testing.T, s *work_session.WorkSession, taskID string) work_session.SessionTask {
	t.Helper()
	for _, l := range s.Tasks {
		if l.TaskID.String() == taskID {
			return l
		}
	}
	t.Fatalf("task %s is not in the session %+v", taskID, s.Tasks)
	return work_session.SessionTask{}
}

// O exemplo do pedido: uma sessão de 6 horas em que se trabalhou na tarefa A e, a partir da
// segunda hora, também na B; depois da pausa, outra sessão de 2 horas só na A. A sessão e
// o projeto contam 8 horas, a tarefa A conta 8 e a B conta as 4 em que esteve na sessão.
func TestSessionTasks_TheExample(t *testing.T) {
	f := newSessionFixture(t)
	a, b := f.task("A"), f.task("B")

	first := f.past(a, 72*time.Hour, 6*time.Hour)
	joined := first.StartAt.Add(2 * time.Hour)
	if _, err := f.ws.AddTask(f.proj, first.ID.String(), b, &joined, nil); err != nil {
		t.Fatalf("add B: %v", err)
	}
	f.past(a, 60*time.Hour, 2*time.Hour)

	if got := f.total(a, ""); !near(got.TotalSeconds, 8*3600) || *got.PayAmountCents != 16000 {
		t.Errorf("task A = %.0fs, %v cents; want 8h, 160,00", got.TotalSeconds, got.PayAmountCents)
	}
	if got := f.total(b, ""); !near(got.TotalSeconds, 4*3600) || *got.PayAmountCents != 8000 {
		t.Errorf("task B = %.0fs, %v cents; want the 4h it was in the session, 80,00", got.TotalSeconds, got.PayAmountCents)
	}
	// A pessoa e o projeto contam o tempo das sessões, uma vez só: A e B em paralelo não o dobram.
	if got := f.total("", f.ana.String()); !near(got.TotalSeconds, 8*3600) || *got.PayAmountCents != 16000 {
		t.Errorf("person = %.0fs, %v cents; want 8h, 160,00", got.TotalSeconds, got.PayAmountCents)
	}

	all, err := f.ws.ListByProject(f.proj, nil, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("sessions = %d, %v; want 2", len(all), err)
	}
	bTask := b
	onlyB, _ := f.ws.ListByProject(f.proj, &bTask, nil)
	if len(onlyB) != 1 || len(onlyB[0].Tasks) != 2 {
		t.Fatalf("sessions with B = %+v, want the first one, with both tasks", onlyB)
	}
	if got := onlyB[0].Seconds(time.Now()); !near(got, 6*3600) {
		t.Errorf("the session counts %.0fs, want 6h", got)
	}
	if l := linkOf(t, &onlyB[0], b); !near(l.Seconds, 4*3600) || *l.PayAmountCents != 8000 {
		t.Errorf("B in the session = %.0fs, %v cents; want 4h, 80,00", l.Seconds, l.PayAmountCents)
	}
	if l := linkOf(t, &onlyB[0], a); !near(l.Seconds, 6*3600) {
		t.Errorf("A in the session = %.0fs, want all 6h", l.Seconds)
	}
}

// Numa sessão aberta a tarefa nova entra agora, passa a estar em progresso e, sem responsável,
// é de quem está com o ponto aberto, como no clock-in. Uma tarefa não entra duas vezes ao mesmo
// tempo, mas pode sair e voltar; a sessão aberta mantém sempre uma tarefa em andamento.
func TestSessionTasks_OpenSession(t *testing.T) {
	f := newSessionFixture(t)
	a, b := f.task("A"), f.task("B")

	session, err := f.ws.ClockIn(f.proj, a, f.ana.String())
	if err != nil {
		t.Fatal(err)
	}
	sid := session.ID.String()

	session, err = f.ws.AddTask(f.proj, sid, b, nil, nil)
	if err != nil {
		t.Fatalf("add B: %v", err)
	}
	if len(session.Tasks) != 2 || session.Tasks[1].Task == nil || session.Tasks[1].Task.Name != "B" {
		t.Fatalf("tasks = %+v, want A then B", session.Tasks)
	}
	if got := f.status(b); got != "in_progress" {
		t.Errorf("B is %q after entering the open session, want in_progress", got)
	}
	if got, _ := f.tasks.Get(b); got.AssigneeID == nil || *got.AssigneeID != f.ana {
		t.Errorf("B assignee = %v, want Ana, who has the clock open", got.AssigneeID)
	}
	if _, err := f.ws.AddTask(f.proj, sid, b, nil, nil); !errors.Is(err, work_session.ErrTaskOverlap) {
		t.Errorf("adding B again while it is in the session: %v, want ErrTaskOverlap", err)
	}

	linkA, linkB := linkOf(t, session, a), linkOf(t, session, b)
	session, err = f.ws.UpdateTask(f.proj, sid, linkA.ID.String(), work_session.TaskChange{Stop: true})
	if err != nil {
		t.Fatalf("stop A: %v", err)
	}
	if linkOf(t, session, a).UntilAt == nil {
		t.Error("A has no end after stopping it")
	}
	// B é a única em andamento: nem parar nem tirar.
	if _, err := f.ws.UpdateTask(f.proj, sid, linkB.ID.String(), work_session.TaskChange{Stop: true}); !errors.Is(err, work_session.ErrLastTask) {
		t.Errorf("stopping the last task in progress: %v, want ErrLastTask", err)
	}
	if _, err := f.ws.RemoveTask(f.proj, sid, linkB.ID.String()); !errors.Is(err, work_session.ErrLastTask) {
		t.Errorf("removing the last task in progress: %v, want ErrLastTask", err)
	}

	// A volta, num intervalo novo; agora B pode parar.
	session, err = f.ws.AddTask(f.proj, sid, a, nil, nil)
	if err != nil || len(session.Tasks) != 3 {
		t.Fatalf("A coming back: %v, %d tasks; want 3 (A twice, B)", err, len(session.Tasks))
	}
	if _, err := f.ws.UpdateTask(f.proj, sid, linkB.ID.String(), work_session.TaskChange{Stop: true}); err != nil {
		t.Errorf("stopping B with A back in progress: %v", err)
	}
	// O primeiro intervalo de A, já encerrado, pode ser tirado.
	if session, err = f.ws.RemoveTask(f.proj, sid, linkA.ID.String()); err != nil || len(session.Tasks) != 2 {
		t.Errorf("removing the stopped interval of A: %v, %d tasks; want 2", err, len(session.Tasks))
	}

	// Parar o ponto não mexe nas tarefas, e a sessão fechada guarda todas.
	closed, err := f.ws.ClockOut(f.proj, f.ana.String())
	if err != nil || len(closed.Tasks) != 2 {
		t.Fatalf("clock out: %v, %d tasks; want 2", err, len(closed.Tasks))
	}
}

// Numa sessão encerrada a tarefa entra do início ao fim, sem mudar o status nem o
// responsável, e o intervalo pode ser corrigido dentro da sessão.
func TestSessionTasks_ClosedSession(t *testing.T) {
	f := newSessionFixture(t)
	a, c := f.task("A"), f.task("C")
	closed := f.past(a, 10*time.Hour, 4*time.Hour)
	sid := closed.ID.String()

	for name, in := range map[string][2]*time.Time{
		"before the start": {ptr(closed.StartAt.Add(-time.Minute)), nil},
		"at the end":       {ptr(*closed.EndAt), nil},
		"after the end":    {ptr(closed.StartAt.Add(time.Hour)), ptr(closed.EndAt.Add(time.Minute))},
		"end before start": {ptr(closed.StartAt.Add(2 * time.Hour)), ptr(closed.StartAt.Add(time.Hour))},
	} {
		if _, err := f.ws.AddTask(f.proj, sid, c, in[0], in[1]); !errors.Is(err, work_session.ErrInvalidInterval) {
			t.Errorf("%s: %v, want ErrInvalidInterval", name, err)
		}
	}

	session, err := f.ws.AddTask(f.proj, sid, c, nil, nil)
	if err != nil {
		t.Fatalf("add C: %v", err)
	}
	link := linkOf(t, session, c)
	if !link.FromAt.Equal(closed.StartAt) || link.UntilAt != nil || !near(link.Seconds, 4*3600) {
		t.Errorf("C entered %v until %v for %.0fs; want the whole session, 4h", link.FromAt, link.UntilAt, link.Seconds)
	}
	if got := f.status(c); got != "backlog" {
		t.Errorf("C is %q after entering a closed session, want it untouched (backlog)", got)
	}
	if got, _ := f.tasks.Get(c); got.AssigneeID != nil {
		t.Errorf("C assignee = %v, want none", got.AssigneeID)
	}
	if _, err := f.ws.AddTask(f.proj, sid, c, nil, nil); !errors.Is(err, work_session.ErrTaskOverlap) {
		t.Errorf("adding C over its own interval: %v, want ErrTaskOverlap", err)
	}

	// Corrigir o intervalo: da 2ª à 3ª hora, depois de volta até o fim.
	from, until := closed.StartAt.Add(time.Hour), closed.StartAt.Add(2*time.Hour)
	session, err = f.ws.UpdateTask(f.proj, sid, link.ID.String(), work_session.TaskChange{From: &from, Until: &until})
	if err != nil || !near(linkOf(t, session, c).Seconds, 3600) {
		t.Fatalf("narrowing C: %v, %.0fs; want 1h", err, linkOf(t, session, c).Seconds)
	}
	session, err = f.ws.UpdateTask(f.proj, sid, link.ID.String(), work_session.TaskChange{ClearUntil: true})
	if err != nil || !near(linkOf(t, session, c).Seconds, 3*3600) {
		t.Fatalf("clearing the end of C: %v, %.0fs; want 3h, up to the end of the session", err, linkOf(t, session, c).Seconds)
	}

	// Voltar à sessão depois de sair, sem sobrepor: 1h + 1h.
	if _, err := f.ws.UpdateTask(f.proj, sid, link.ID.String(), work_session.TaskChange{Until: &until}); err != nil {
		t.Fatal(err)
	}
	later := closed.StartAt.Add(3 * time.Hour)
	if _, err := f.ws.AddTask(f.proj, sid, c, &from, nil); !errors.Is(err, work_session.ErrTaskOverlap) {
		t.Errorf("C over its first interval: %v, want ErrTaskOverlap", err)
	}
	if session, err = f.ws.AddTask(f.proj, sid, c, &later, nil); err != nil {
		t.Fatalf("C coming back: %v", err)
	}
	if got := f.total(c, ""); !near(got.TotalSeconds, 2*3600) {
		t.Errorf("C total = %.0fs, want 1h + 1h", got.TotalSeconds)
	}

	// Tira tudo, menos uma: a sessão guarda sempre uma tarefa.
	for _, l := range session.Tasks[:len(session.Tasks)-1] {
		if session, err = f.ws.RemoveTask(f.proj, sid, l.ID.String()); err != nil {
			t.Fatalf("remove: %v", err)
		}
	}
	if _, err := f.ws.RemoveTask(f.proj, sid, session.Tasks[0].ID.String()); !errors.Is(err, work_session.ErrLastTask) {
		t.Errorf("removing the only task: %v, want ErrLastTask", err)
	}
}

// A sessão, a tarefa e o intervalo precisam ser do projeto da rota.
func TestSessionTasks_Scope(t *testing.T) {
	f := newSessionFixture(t)
	a := f.task("A")
	other, _ := project.NewService(project.NewStore(testClient)).Create(f.org, "Other", "", 0, project.Routine{})
	foreign, err := f.tasks.Create(other.ID.String(), "Foreign", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	session := f.past(a, 10*time.Hour, 2*time.Hour)
	sid := session.ID.String()

	if _, err := f.ws.AddTask(f.proj, sid, foreign.ID.String(), nil, nil); !errors.Is(err, work_session.ErrTaskOtherProject) {
		t.Errorf("task of another project: %v, want ErrTaskOtherProject", err)
	}
	if _, err := f.ws.AddTask(f.proj, sid, uuid.NewString(), nil, nil); !errors.Is(err, work_session.ErrTaskNotFound) {
		t.Errorf("unknown task: %v, want ErrTaskNotFound", err)
	}
	if _, err := f.ws.AddTask(other.ID.String(), sid, foreign.ID.String(), nil, nil); !errors.Is(err, work_session.ErrSessionNotFound) {
		t.Errorf("session through another project: %v, want ErrSessionNotFound", err)
	}
	if _, err := f.ws.Get(f.proj, "not-a-uuid"); !errors.Is(err, work_session.ErrSessionNotFound) {
		t.Errorf("malformed session id: %v, want ErrSessionNotFound", err)
	}
	if _, err := f.ws.RemoveTask(f.proj, sid, uuid.NewString()); !errors.Is(err, work_session.ErrTaskLinkNotFound) {
		t.Errorf("unknown interval: %v, want ErrTaskLinkNotFound", err)
	}
}

func ptr[T any](v T) *T { return &v }

// ListOpenByOrganization serve ao painel de quem trabalha agora: só as sessões abertas da
// organização, a que começou primeiro na frente, cada uma com todas as suas tarefas na ordem
// em que entraram. Uma sessão fechada e a de outra organização não entram.
func TestService_ListOpenByOrganization(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)
	ctx := context.Background()

	orgA, _ := orgSvc.Create("Org A")
	orgB, _ := orgSvc.Create("Org B")
	ana, _ := personSvc.Create(orgA.ID.String(), "Ana", "ana@test.com")
	bruno, _ := personSvc.Create(orgA.ID.String(), "Bruno", "bruno@test.com")
	carla, _ := personSvc.Create(orgA.ID.String(), "Carla", "carla@test.com")
	zed, _ := personSvc.Create(orgB.ID.String(), "Zed", "zed@test.com")
	x, _ := projSvc.Create(orgA.ID.String(), "Projeto X", "", 0, project.Routine{})
	y, _ := projSvc.Create(orgA.ID.String(), "Projeto Y", "", 0, project.Routine{})
	z, _ := projSvc.Create(orgB.ID.String(), "Projeto Z", "", 0, project.Routine{})
	login, _ := taskSvc.Create(x.ID.String(), "Login", "", "", nil)
	report, _ := taskSvc.Create(x.ID.String(), "Relatório", "", "", nil)
	review, _ := taskSvc.Create(x.ID.String(), "Revisão", "", "", nil)
	slips, _ := taskSvc.Create(y.ID.String(), "Boletos", "", "", nil)
	other, _ := taskSvc.Create(z.ID.String(), "Da outra", "", "", nil)

	now := time.Now().Truncate(time.Second)
	// A Ana está há 2h numa sessão com três tarefas: Login desde o início, Relatório entrou 30
	// min depois e Revisão já saiu.
	start := now.Add(-2 * time.Hour)
	open := testutil.Session(t, testClient, login.ID, ana.ID, start, nil, nil, nil)
	testClient.WorkSessionTask.Create().SetSessionID(open.ID).SetTaskID(review.ID).
		SetFromAt(start.Add(10 * time.Minute)).SetUntilAt(start.Add(20 * time.Minute)).SaveX(ctx)
	testClient.WorkSessionTask.Create().SetSessionID(open.ID).SetTaskID(report.ID).
		SetFromAt(start.Add(30 * time.Minute)).SaveX(ctx)
	// O Bruno abriu o ponto depois da Ana.
	testutil.Session(t, testClient, slips.ID, bruno.ID, now.Add(-time.Hour), nil, nil, nil)
	// A Carla já fechou o dela, e o Zed, de outra organização, está com o ponto aberto.
	closed := now.Add(-30 * time.Minute)
	testutil.Session(t, testClient, login.ID, carla.ID, now.Add(-3*time.Hour), &closed, nil, nil)
	testutil.Session(t, testClient, other.ID, zed.ID, now.Add(-time.Hour), nil, nil, nil)

	got, err := wsSvc.ListOpenByOrganization(orgA.ID.String())
	if err != nil {
		t.Fatalf("list open sessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d open sessions, want 2 (Ana's and Bruno's): the closed one and the other organization's stay out", len(got))
	}
	if got[0].PersonID != ana.ID || got[1].PersonID != bruno.ID {
		t.Errorf("order = %s, %s; want the one that started first (Ana) and then Bruno", got[0].PersonID, got[1].PersonID)
	}
	var tasks []string
	for _, l := range got[0].Tasks {
		state := "open"
		if l.UntilAt != nil {
			state = "left"
		}
		if l.Task == nil {
			t.Fatalf("task link %s came without the task", l.ID)
		}
		tasks = append(tasks, l.Task.Name+":"+state)
	}
	if want := []string{"Login:open", "Revisão:left", "Relatório:open"}; !slices.Equal(tasks, want) {
		t.Errorf("Ana's tasks = %q, want %q (in the order they entered, with the name)", tasks, want)
	}
	if got[1].ProjectID != y.ID || len(got[1].Tasks) != 1 || got[1].Tasks[0].Task.Name != "Boletos" {
		t.Errorf("Bruno's session = project %s, %d tasks; want Projeto Y with Boletos", got[1].ProjectID, len(got[1].Tasks))
	}

	if _, err := wsSvc.ListOpenByOrganization("not-a-uuid"); err == nil {
		t.Error("an invalid organization id should fail")
	}
}
