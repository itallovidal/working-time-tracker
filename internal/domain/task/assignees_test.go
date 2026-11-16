package task_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/validate"
)

// stopRecorder é o WorkStopper dos testes: anota quem foi parado em que tarefa. Os testes de corrida o chamam de mais
// de uma goroutine, daí a trava.
type stopRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (s *stopRecorder) StopTaskFor(personID, taskID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, personID.String()+"/"+taskID.String())
	return nil
}

// asgEnv é o reqEnv com uma terceira pessoa no projeto e o WorkStopper ligado. Ana e Bia estão no projeto, Cai também;
// a Ana tem conta no GitHub e a Bia não.
type asgEnv struct {
	reqEnv
	store   *task.Store
	cai     string
	stopped *stopRecorder
}

func newAsgEnv(t *testing.T) asgEnv {
	t.Helper()
	e := asgEnv{reqEnv: newReqEnv(t), stopped: &stopRecorder{}}
	cai := mkPerson(t, e.org.String(), e.project, "Cai", "cai@test.com")
	e.cai = cai
	e.store = task.NewStore(testClient)
	e.store.SetAccounts(personaccount.NewStore(testClient))
	e.store.SetWorkStopper(e.stopped)
	e.svc = task.NewService(e.store, teamStore(), nil)
	return e
}

// mkPerson cria uma pessoa com valor por hora no projeto e devolve o id dela.
func mkPerson(t *testing.T, orgID, projectID, name, email string) string {
	t.Helper()
	p, err := person.NewService(person.NewStore(testClient)).Create(orgID, name, email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(projectID, p.ID.String(), 1000); err != nil {
		t.Fatal(err)
	}
	return p.ID.String()
}

func teamStore() *team.MembershipStore { return team.NewMembershipStore(testClient) }

func noDeadline() validate.Optional[time.Time] { return validate.Optional[time.Time]{} }

func (e asgEnv) actor(id string) task.Actor { return task.Actor{ID: id} }
func (e asgEnv) admin(id string) task.Actor { return task.Actor{ID: id, Manage: true} }

// set muda a lista inteira de responsáveis em nome de quem pede.
func (e asgEnv) set(actor task.Actor, taskID string, ids ...string) (*task.Task, error) {
	if ids == nil {
		ids = []string{}
	}
	return e.svc.PatchAttrsAs(actor, taskID, task.Attrs{Assignees: &ids}, nil, noDeadline())
}

// delta põe e tira algumas pessoas em nome de quem pede, como a tela manda.
func (e asgEnv) delta(actor task.Actor, taskID string, add, remove []string) (*task.Task, error) {
	return e.svc.PatchAttrsAs(actor, taskID, task.Attrs{AddAssignees: add, RemoveAssignees: remove}, nil, noDeadline())
}

func names(t *task.Task) []string {
	out := []string{}
	for _, p := range t.Assignees {
		out = append(out, p.Name)
	}
	return out
}

func sameNames(t *task.Task, want ...string) bool {
	got := names(t)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestAssignees_ClaimOnlyAFreeTask(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	tk, err := e.svc.Create(e.project, "Livre", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Claim(ana, tk.ID.String()); err != nil {
		t.Fatalf("claim a free task: %v", err)
	}
	if _, err := e.svc.Claim(bia, tk.ID.String()); !errors.Is(err, task.ErrAlreadyAssigned) {
		t.Errorf("claim a task with people = %v, want ErrAlreadyAssigned", err)
	}
	if _, err := e.svc.Claim(ana, tk.ID.String()); err != nil {
		t.Errorf("claiming your own task changes nothing: %v", err)
	}
}

func TestAssignees_WhoAddsSomeone(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	tk, _ := e.svc.Create(e.project, "Livre", "", "", nil)
	id := tk.ID.String()

	// Livre: quem não administra o projeto só a pega para si.
	if _, err := e.set(e.actor(ana), id, bia); !errors.Is(err, task.ErrShareForbidden) {
		t.Errorf("a member putting someone else on a free task = %v, want ErrShareForbidden", err)
	}
	if _, err := e.set(e.actor(ana), id, ana, bia); !errors.Is(err, task.ErrShareForbidden) {
		t.Errorf("taking a free task with someone else at once = %v, want ErrShareForbidden", err)
	}
	if got, err := e.set(e.actor(ana), id, ana); err != nil || !sameNames(got, "Ana") {
		t.Fatalf("taking the free task for yourself = %v, %v", got, err)
	}
	// Com gente: quem não está nela não entra sozinho, e quem está a divide.
	if _, err := e.set(e.actor(e.cai), id, ana, e.cai); !errors.Is(err, task.ErrAlreadyAssigned) {
		t.Errorf("joining a task you are not on = %v, want ErrAlreadyAssigned", err)
	}
	if got, err := e.set(e.actor(ana), id, ana, bia); err != nil || !sameNames(got, "Ana", "Bia") {
		t.Fatalf("an assignee sharing the task = %v, %v; want Ana and Bia", got, err)
	}
	// A Bia, que agora está, também a divide.
	if got, err := e.set(e.actor(bia), id, ana, bia, e.cai); err != nil || !sameNames(got, "Ana", "Bia", "Cai") {
		t.Fatalf("another assignee sharing it = %v, %v", got, err)
	}
	// O admin põe quem quiser numa tarefa livre.
	other, _ := e.svc.Create(e.project, "Outra", "", "", nil)
	if got, err := e.set(e.admin(e.cai), other.ID.String(), ana, bia); err != nil || !sameNames(got, "Ana", "Bia") {
		t.Errorf("the admin choosing people for a free task = %v, %v", got, err)
	}
	// Mexer em outro campo sem mudar a lista não pede regra nenhuma.
	high := "high"
	if _, err := e.svc.PatchAttrsAs(e.actor(e.cai), other.ID.String(), task.Attrs{Priority: &high}, nil, noDeadline()); err != nil {
		t.Errorf("someone off the task changing the priority: %v", err)
	}
}

func TestAssignees_WhoRemovesSomeone(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	tk, _ := e.svc.Create(e.project, "Dividida", "", "", nil)
	id := tk.ID.String()
	if _, err := e.set(task.Actor{}, id, ana, bia); err != nil {
		t.Fatal(err)
	}
	if _, err := e.set(e.actor(ana), id, ana); !errors.Is(err, task.ErrRemoveForbidden) {
		t.Errorf("an assignee removing another = %v, want ErrRemoveForbidden", err)
	}
	if len(e.stopped.calls) != 0 {
		t.Errorf("a refused removal stopped someone: %v", e.stopped.calls)
	}
	got, err := e.set(e.actor(bia), id, ana)
	if err != nil || !sameNames(got, "Ana") {
		t.Fatalf("leaving the task = %v, %v", got, err)
	}
	if len(e.stopped.calls) != 1 || e.stopped.calls[0] != bia+"/"+id {
		t.Errorf("stopped = %v, want Bia stopped on the task she left", e.stopped.calls)
	}
	// O admin tira qualquer um; sem ninguém, a tarefa volta a ficar livre.
	if got, err := e.set(e.admin(e.cai), id); err != nil || !got.Free() {
		t.Errorf("the admin removing the last assignee = %v, %v; want a free task", got, err)
	}
}

func TestAssignees_OnCreate(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	both := []string{ana, bia}
	if _, err := e.svc.CreateAs(e.actor(ana), e.project, "Nova", "", "", nil, task.Attrs{Assignees: &both}); !errors.Is(err, task.ErrShareForbidden) {
		t.Errorf("a member creating a task for someone else = %v, want ErrShareForbidden", err)
	}
	if got, err := e.svc.CreateAs(e.actor(ana), e.project, "Minha", "", ana, nil, task.Attrs{}); err != nil || !sameNames(got, "Ana") {
		t.Errorf("a member creating a task for themself = %v, %v", got, err)
	}
	if got, err := e.svc.CreateAs(e.admin(ana), e.project, "Dividida", "", "", nil, task.Attrs{Assignees: &both}); err != nil || !sameNames(got, "Ana", "Bia") {
		t.Errorf("the admin creating a shared task = %v, %v", got, err)
	}
}

func TestAssignees_Limit(t *testing.T) {
	e := newAsgEnv(t)
	tk, _ := e.svc.Create(e.project, "Cheia", "", "", nil)
	many := make([]string, task.MaxAssignees+1)
	for i := range many {
		many[i] = uuid.NewString()
	}
	if _, err := e.set(task.Actor{}, tk.ID.String(), many...); !errors.Is(err, task.ErrTooManyAssignees) {
		t.Errorf("%d assignees = %v, want ErrTooManyAssignees", len(many), err)
	}
}

func TestAssignees_LockIsPerPerson(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	tk, err := e.svc.CreateAs(e.admin(ana), e.project, "Só GitHub", "", ana, nil, task.Attrs{RequiredAccounts: github()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.set(e.actor(ana), tk.ID.String(), ana, bia)
	wantRequired(t, err, "sharing with someone who has no GitHub account")
}

func TestAssignees_Filters(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	shared, _ := e.svc.Create(e.project, "Dividida", "", "", nil)
	if _, err := e.set(task.Actor{}, shared.ID.String(), ana, bia); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Create(e.project, "Livre", "", "", nil); err != nil {
		t.Fatal(err)
	}
	count := func(f task.ListFilter) int {
		t.Helper()
		items, err := e.svc.ListByProject(e.project, f)
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	anaID, biaID, caiID := e.ana.ID, e.bia.ID, uuid.MustParse(e.cai)
	cases := []struct {
		name string
		f    task.ListFilter
		want int
	}{
		{"one of the assignees", task.ListFilter{AssigneeID: &biaID}, 1},
		{"someone not on it", task.ListFilter{AssigneeID: &caiID}, 0},
		{"others of Ana: she is on it", task.ListFilter{OthersOf: &anaID}, 0},
		{"others of Cai", task.ListFilter{OthersOf: &caiID}, 1},
		{"unassigned", task.ListFilter{Unassigned: true}, 1},
		{"assigned", task.ListFilter{Assigned: true}, 1},
	}
	for _, c := range cases {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: %d tasks, want %d", c.name, got, c.want)
		}
	}
}

func TestAssignees_RemoteRemovalStopsWork(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	tk, _ := e.svc.Create(e.project, "Da issue", "", "", nil)
	if _, err := e.set(task.Actor{}, tk.ID.String(), ana, bia); err != nil {
		t.Fatal(err)
	}
	if err := e.store.ApplyRemote(tk.ID, task.RemotePatch{RemoveAssignees: []uuid.UUID{e.bia.ID}, AddAssignees: []uuid.UUID{e.ana.ID}}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(tk.ID.String())
	if !sameNames(got, "Ana") {
		t.Errorf("after the remote patch = %v, want only Ana (adding who is already there is a no-op)", names(got))
	}
	if len(e.stopped.calls) != 1 || e.stopped.calls[0] != bia+"/"+tk.ID.String() {
		t.Errorf("stopped = %v, want Bia", e.stopped.calls)
	}
}

// A tela que leu a tarefa antes de alguém entrar manda só o que mudou nela: quem entrou no meio fica e não tem o ponto
// parado, e o Sair pela tela velha tira só quem sai.
func TestAssignees_DeltaKeepsWhoJoinedMeanwhile(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	tk, err := e.svc.Create(e.project, "Dividida", "", ana, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := tk.ID.String()
	// A Ana acrescenta a Bia enquanto a tela do admin ainda mostra só a Ana.
	if _, err := e.delta(e.actor(ana), id, []string{bia}, nil); err != nil {
		t.Fatalf("Ana sharing the task: %v", err)
	}
	// O admin marca o Cai na tela velha: vai só o Cai, e a Bia fica.
	got, err := e.delta(e.admin(e.cai), id, []string{e.cai}, nil)
	if err != nil || !sameNames(got, "Ana", "Bia", "Cai") {
		t.Fatalf("the admin adding Cai from a stale page = %v, %v; want Ana, Bia and Cai", names(got), err)
	}
	if len(e.stopped.calls) != 0 {
		t.Errorf("stopped = %v, want nobody: nobody left", e.stopped.calls)
	}
	// A Ana sai pela tela velha, que não conhece o Cai: sai só ela, sem o 403 de tirar outra pessoa.
	got, err = e.delta(e.actor(ana), id, nil, []string{ana})
	if err != nil || !sameNames(got, "Bia", "Cai") {
		t.Fatalf("Ana leaving from a stale page = %v, %v; want Bia and Cai", names(got), err)
	}
	if len(e.stopped.calls) != 1 || e.stopped.calls[0] != ana+"/"+id {
		t.Errorf("stopped = %v, want only Ana", e.stopped.calls)
	}
	// As regras valem para o que entra e o que sai: quem não é admin não tira outra pessoa.
	if _, err := e.delta(e.actor(bia), id, nil, []string{e.cai}); !errors.Is(err, task.ErrRemoveForbidden) {
		t.Errorf("an assignee removing another by delta = %v, want ErrRemoveForbidden", err)
	}
}

// A lista inteira e o que entra e o que sai são dois jeitos de mudar a lista, um de cada vez.
func TestAssignees_WholeListAndDeltaConflict(t *testing.T) {
	e := newAsgEnv(t)
	ana := e.ana.ID.String()
	tk, _ := e.svc.Create(e.project, "Livre", "", "", nil)
	list := []string{ana}
	_, err := e.svc.PatchAttrsAs(e.actor(ana), tk.ID.String(), task.Attrs{Assignees: &list, AddAssignees: []string{ana}}, nil, noDeadline())
	if !errors.Is(err, task.ErrAssigneesConflict) {
		t.Errorf("assignee_ids with add_assignee_ids = %v, want ErrAssigneesConflict", err)
	}
	_, err = e.svc.PatchAs(e.actor(ana), tk.ID.String(), "Livre", nil, &ana, noDeadline(), task.Attrs{RemoveAssignees: []string{ana}})
	if !errors.Is(err, task.ErrAssigneesConflict) {
		t.Errorf("assignee_id with remove_assignee_ids = %v, want ErrAssigneesConflict", err)
	}
	if _, err := e.delta(e.actor(ana), tk.ID.String(), []string{"nope"}, nil); !errors.Is(err, task.ErrInvalidAssignee) {
		t.Errorf("a delta with a bad id = %v, want ErrInvalidAssignee", err)
	}
}

// Uma edição que não muda a lista não a toca: a tarefa lida antes de alguém entrar, gravada depois, não tira essa
// pessoa nem para o ponto dela.
func TestAssignees_EditWithoutTheListKeepsIt(t *testing.T) {
	e := newAsgEnv(t)
	tk, _ := e.svc.Create(e.project, "Livre", "", "", nil)
	stale, err := e.svc.Get(tk.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Claim(e.ana.ID.String(), tk.ID.String()); err != nil {
		t.Fatal(err)
	}
	stale.Name = "Renomeada"
	if err := e.store.Update(stale, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(tk.ID.String())
	if got.Name != "Renomeada" || !sameNames(got, "Ana") {
		t.Errorf("after a rename from a stale read = %q with %v, want the new name and Ana still on it", got.Name, names(got))
	}
	if len(e.stopped.calls) != 0 {
		t.Errorf("stopped = %v, want nobody", e.stopped.calls)
	}
}

// Com a tarefa travada para o ponto (HoldForWork), outra conexão grava um trecho de sessão nela: a trava é FOR NO KEY
// UPDATE, que não barra a chave estrangeira do trecho. Com FOR UPDATE, o clock-in ficaria parado para sempre.
func TestStore_HoldForWorkLetsTheSessionIn(t *testing.T) {
	e := newAsgEnv(t)
	tk, _ := e.svc.Create(e.project, "Travada", "", e.ana.ID.String(), nil)
	hold, err := e.store.HoldForWork(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Release()
	done := make(chan error, 1)
	go func() {
		ctx := context.Background()
		s, err := testClient.WorkSession.Create().
			SetProjectID(uuid.MustParse(e.project)).SetPersonID(e.ana.ID).SetStartAt(time.Now()).Save(ctx)
		if err != nil {
			done <- err
			return
		}
		done <- testClient.WorkSessionTask.Create().SetSessionID(s.ID).SetTaskID(tk.ID).SetFromAt(time.Now()).Exec(ctx)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write a session stretch on the held task: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writing a session stretch on a held task blocked: the lock must be FOR NO KEY UPDATE")
	}
}

// Pegar a tarefa livre e se pôr nela pela edição, ao mesmo tempo: a trava da tarefa deixa uma pessoa só entrar, e a
// outra recebe o já tem responsável.
func TestAssignees_ClaimAndJoinAtOnce(t *testing.T) {
	e := newAsgEnv(t)
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	for round := range 20 {
		tk, err := e.svc.Create(e.project, fmt.Sprintf("Livre %d", round), "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		id := tk.ID.String()
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 2)
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, errs[0] = e.svc.Claim(ana, id) }()
		go func() { defer wg.Done(); <-start; _, errs[1] = e.delta(e.actor(bia), id, []string{bia}, nil) }()
		close(start)
		wg.Wait()
		got, _ := e.svc.Get(id)
		if len(got.Assignees) != 1 {
			t.Fatalf("round %d: assignees = %v (errors %v), want exactly one", round, names(got), errs)
		}
		lost := 0
		for _, err := range errs {
			switch {
			case errors.Is(err, task.ErrAlreadyAssigned):
				lost++
			case err != nil:
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		if lost != 1 {
			t.Fatalf("round %d: errors %v, want one ErrAlreadyAssigned", round, errs)
		}
	}
}
