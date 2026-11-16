package work_session_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/internal/validate"
)

// sharedEnv liga o ponto à tarefa como o servidor: o mesmo store, com o ponto parando quem sai da tarefa.
type sharedEnv struct {
	tasks    *task.Service
	sessions *work_session.Service
	project  string
	ana, bia string
}

func newSharedEnv(t *testing.T) sharedEnv {
	t.Helper()
	cleanup(t)
	org, _ := organization.NewService(organization.NewStore(testClient)).Create("Org")
	prj, _ := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Projeto", "", 0, project.Routine{})
	people := person.NewService(person.NewStore(testClient))
	ana, _ := people.Create(org.ID.String(), "Ana", "ana@test.com")
	bia, _ := people.Create(org.ID.String(), "Bia", "bia@test.com")
	setRate(t, prj.ID.String(), ana.ID.String(), 2000)
	setRate(t, prj.ID.String(), bia.ID.String(), 3000)
	store := task.NewStore(testClient)
	e := sharedEnv{
		tasks:    task.NewService(store, team.NewMembershipStore(testClient), nil),
		sessions: work_session.NewService(work_session.NewStore(testClient), store, allocation.NewStore(testClient)),
		project:  prj.ID.String(), ana: ana.ID.String(), bia: bia.ID.String(),
	}
	store.SetWorkStopper(e.sessions)
	return e
}

func (e sharedEnv) shared(t *testing.T, name string) string {
	t.Helper()
	tk, err := e.tasks.Create(e.project, name, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	both := []string{e.ana, e.bia}
	if _, err := e.tasks.PatchAttrsAs(task.Actor{}, tk.ID.String(), task.Attrs{Assignees: &both}, nil, validate.Optional[time.Time]{}); err != nil {
		t.Fatal(err)
	}
	return tk.ID.String()
}

// Cada responsável bate o seu ponto na mesma tarefa, ao mesmo tempo: as horas e o valor ficam com quem bateu.
func TestAssignees_EachOneClocksTheirOwnHours(t *testing.T) {
	e := newSharedEnv(t)
	id := e.shared(t, "Dividida")
	a, err := e.sessions.ClockIn(e.project, id, e.ana)
	if err != nil {
		t.Fatalf("Ana clocks in: %v", err)
	}
	b, err := e.sessions.ClockIn(e.project, id, e.bia)
	if err != nil {
		t.Fatalf("Bia clocks in on the same task: %v", err)
	}
	if a.PersonID == b.PersonID || *a.PayRateCents != 2000 || *b.PayRateCents != 3000 {
		t.Errorf("sessions = %v at %v, %v at %v; want one each, at each rate", a.PersonID, *a.PayRateCents, b.PersonID, *b.PayRateCents)
	}
}

// Quem sai da tarefa com o ponto aberto nela para de trabalhar nela; as outras tarefas da sessão seguem.
func TestAssignees_LeavingStopsOnlyThatTask(t *testing.T) {
	e := newSharedEnv(t)
	first, second := e.shared(t, "A"), e.shared(t, "B")
	s, err := e.sessions.ClockIn(e.project, first, e.bia)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sessions.AddTask(e.project, s.ID.String(), second, nil, nil); err != nil {
		t.Fatal(err)
	}
	onlyAna := []string{e.ana}
	if _, err := e.tasks.PatchAttrsAs(task.Actor{ID: e.ana, Manage: true}, first, task.Attrs{Assignees: &onlyAna}, nil, validate.Optional[time.Time]{}); err != nil {
		t.Fatalf("the admin removes Bia from A: %v", err)
	}
	open, err := e.sessions.Open(e.bia)
	if err != nil || len(open) != 1 {
		t.Fatalf("Bia's open sessions = %v, %v; want the session still open", open, err)
	}
	for _, link := range open[0].Tasks {
		switch link.TaskID.String() {
		case first:
			if link.UntilAt == nil {
				t.Error("task A is still running for Bia after she left it")
			}
		case second:
			if link.UntilAt != nil {
				t.Error("task B stopped too")
			}
		}
	}
}

// Se a tarefa era a única rodando na sessão, sair dela fecha a sessão.
func TestAssignees_LeavingTheOnlyTaskClosesTheSession(t *testing.T) {
	e := newSharedEnv(t)
	id := e.shared(t, "Única")
	if _, err := e.sessions.ClockIn(e.project, id, e.bia); err != nil {
		t.Fatal(err)
	}
	onlyAna := []string{e.ana}
	if _, err := e.tasks.PatchAttrsAs(task.Actor{ID: e.bia}, id, task.Attrs{Assignees: &onlyAna}, nil, validate.Optional[time.Time]{}); err != nil {
		t.Fatalf("Bia leaves the task: %v", err)
	}
	if open, _ := e.sessions.Open(e.bia); len(open) != 0 {
		t.Errorf("Bia's session is still open: %+v", open)
	}
	// E depois de sair ela não bate mais o ponto nela.
	if _, err := e.sessions.ClockIn(e.project, id, e.bia); !errors.Is(err, work_session.ErrNotAssignee) {
		t.Errorf("clocking in after leaving = %v, want ErrNotAssignee", err)
	}
}

// Pôr na sessão uma tarefa de outra pessoa é recusado, aberta ou fechada.
func TestAssignees_AddingSomeoneElsesTask(t *testing.T) {
	e := newSharedEnv(t)
	mine, err := e.tasks.Create(e.project, "Minha", "", e.bia, nil)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := e.tasks.Create(e.project, "Da Ana", "", e.ana, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := e.sessions.ClockIn(e.project, mine.ID.String(), e.bia)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sessions.AddTask(e.project, s.ID.String(), theirs.ID.String(), nil, nil); !errors.Is(err, work_session.ErrNotAssignee) {
		t.Errorf("adding someone else's task to the open session = %v, want ErrNotAssignee", err)
	}
	time.Sleep(5 * time.Millisecond)
	closed, err := e.sessions.ClockOut(e.project, e.bia)
	if err != nil {
		t.Fatal(err)
	}
	from := closed.StartAt
	if _, err := e.sessions.AddTask(e.project, closed.ID.String(), theirs.ID.String(), &from, nil); !errors.Is(err, work_session.ErrNotAssignee) {
		t.Errorf("adding someone else's task to a closed session = %v, want ErrNotAssignee", err)
	}
}

// leave tira a pessoa da tarefa, como o Sair da tela.
func (e sharedEnv) leave(t *testing.T, personID, taskID string) {
	t.Helper()
	if _, err := e.tasks.PatchAttrsAs(task.Actor{ID: personID}, taskID, task.Attrs{RemoveAssignees: []string{personID}}, nil, validate.Optional[time.Time]{}); err != nil {
		t.Fatalf("leave the task: %v", err)
	}
}

// Quem saiu da tarefa não volta a trabalhar nela reabrindo o trecho que a saída parou.
func TestAssignees_ReopeningAfterLeavingIsRefused(t *testing.T) {
	e := newSharedEnv(t)
	mine, err := e.tasks.Create(e.project, "Minha", "", e.ana, nil)
	if err != nil {
		t.Fatal(err)
	}
	shared := e.shared(t, "Dividida")
	s, err := e.sessions.ClockIn(e.project, mine.ID.String(), e.ana)
	if err != nil {
		t.Fatal(err)
	}
	if s, err = e.sessions.AddTask(e.project, s.ID.String(), shared, nil, nil); err != nil {
		t.Fatal(err)
	}
	e.leave(t, e.ana, shared)
	s, err = e.sessions.Get(e.project, s.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	link := linkOf(t, s, shared)
	if link.UntilAt == nil {
		t.Fatal("leaving the task did not stop the stretch on it")
	}
	_, err = e.sessions.UpdateTask(e.project, s.ID.String(), link.ID.String(), work_session.TaskChange{ClearUntil: true})
	if !errors.Is(err, work_session.ErrNotAssignee) {
		t.Errorf("reopening the stretch of a task you left = %v, want ErrNotAssignee", err)
	}
	if s, _ = e.sessions.Get(e.project, s.ID.String()); linkOf(t, s, shared).UntilAt == nil {
		t.Error("the refused reopen left the stretch running")
	}
}

// Reabrir, com a sessão aberta, o trecho de uma tarefa que ficou livre a toma para quem reabre, como pôr na sessão.
func TestAssignees_ReopeningAFreeTaskTakesIt(t *testing.T) {
	e := newSharedEnv(t)
	mine, _ := e.tasks.Create(e.project, "Minha", "", e.ana, nil)
	free, _ := e.tasks.Create(e.project, "Livre", "", "", nil)
	s, err := e.sessions.ClockIn(e.project, mine.ID.String(), e.ana)
	if err != nil {
		t.Fatal(err)
	}
	if s, err = e.sessions.AddTask(e.project, s.ID.String(), free.ID.String(), nil, nil); err != nil {
		t.Fatal(err)
	}
	e.leave(t, e.ana, free.ID.String()) // a tarefa volta a ser livre, e o trecho para
	s, _ = e.sessions.Get(e.project, s.ID.String())
	link := linkOf(t, s, free.ID.String())
	if _, err := e.sessions.UpdateTask(e.project, s.ID.String(), link.ID.String(), work_session.TaskChange{ClearUntil: true}); err != nil {
		t.Fatalf("reopening the stretch of a free task: %v", err)
	}
	got, _ := e.tasks.Get(free.ID.String())
	if !got.HasAssignee(uuid.MustParse(e.ana)) || len(got.Assignees) != 1 {
		t.Errorf("after reopening, assignees = %v, want Ana", got.Assignees)
	}
}

// Dois clock-ins juntos da mesma pessoa na mesma tarefa livre: abre uma sessão só, e a tarefa fica com ela. Antes, o
// ponto recusado devolvia a tarefa que o outro acabava de usar.
func TestAssignees_DoubleClockInKeepsTheTask(t *testing.T) {
	e := newSharedEnv(t)
	ana := uuid.MustParse(e.ana)
	for round := range 10 {
		free, err := e.tasks.Create(e.project, fmt.Sprintf("Livre %d", round), "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 2)
		for i := range errs {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; _, errs[i] = e.sessions.ClockIn(e.project, free.ID.String(), e.ana) }()
		}
		close(start)
		wg.Wait()
		opened := 0
		for _, err := range errs {
			switch {
			case err == nil:
				opened++
			case !errors.Is(err, work_session.ErrAlreadyOpen):
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		if opened != 1 {
			t.Fatalf("round %d: %d sessions opened (errors %v), want one", round, opened, errs)
		}
		if got, _ := e.tasks.Get(free.ID.String()); !got.HasAssignee(ana) {
			t.Fatalf("round %d: the task the clock runs on is not Ana's: %v", round, got.Assignees)
		}
		if _, err := e.sessions.ClockOut(e.project, e.ana); err != nil {
			t.Fatal(err)
		}
	}
}

// Tirar a pessoa da tarefa e ela bater o ponto nela, ao mesmo tempo: no fim, ninguém fica com o ponto aberto numa
// tarefa de que não é responsável.
func TestAssignees_RemovalAndClockInAtOnce(t *testing.T) {
	e := newSharedEnv(t)
	bia := uuid.MustParse(e.bia)
	for round := range 10 {
		id := e.shared(t, fmt.Sprintf("Dividida %d", round))
		var wg sync.WaitGroup
		start := make(chan struct{})
		var removeErr, clockErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, removeErr = e.tasks.PatchAttrsAs(task.Actor{ID: e.ana, Manage: true}, id, task.Attrs{RemoveAssignees: []string{e.bia}}, nil, validate.Optional[time.Time]{})
		}()
		go func() { defer wg.Done(); <-start; _, clockErr = e.sessions.ClockIn(e.project, id, e.bia) }()
		close(start)
		wg.Wait()
		if removeErr != nil {
			t.Fatalf("round %d: removing Bia: %v", round, removeErr)
		}
		if clockErr != nil && !errors.Is(clockErr, work_session.ErrNotAssignee) {
			t.Fatalf("round %d: clock-in: %v", round, clockErr)
		}
		got, _ := e.tasks.Get(id)
		open, err := e.sessions.Open(e.bia)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range open {
			for _, l := range s.Tasks {
				if l.TaskID.String() == id && l.UntilAt == nil && !got.HasAssignee(bia) {
					t.Fatalf("round %d: Bia is clocked in on a task she is no longer on (clock-in error %v)", round, clockErr)
				}
			}
			if _, err := e.sessions.ClockOut(e.project, e.bia); err != nil {
				t.Fatal(err)
			}
		}
	}
}
