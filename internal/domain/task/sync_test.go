package task_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

// hookFixture monta um projeto com uma tarefa e um gancho que anota os avisos.
type hookFixture struct {
	store   *task.Store
	svc     *task.Service
	project string
	person  string
	taskID  uuid.UUID
	mu      sync.Mutex
	calls   []uuid.UUID
}

func newHookFixture(t *testing.T) *hookFixture {
	t.Helper()
	orgSvc, personSvc, projSvc, _, _, _ := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Projeto", "", 0, project.Routine{})
	f := &hookFixture{store: task.NewStore(testClient), project: proj.ID.String(), person: p.ID.String()}
	f.svc = task.NewService(f.store, team.NewMembershipStore(testClient), nil)
	tk, err := f.svc.CreateAs(f.person, f.project, "Tarefa", "", "", nil, task.Attrs{})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	f.taskID = tk.ID
	f.store.SetChangeHook(func(id uuid.UUID) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, id)
	})
	return f
}

// took devolve quantos avisos chegaram desde a última vez e zera a conta.
func (f *hookFixture) took() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.calls)
	f.calls = nil
	return n
}

// Toda escrita que muda a tarefa por dentro do sistema avisa o gancho; a sincronização, não.
func TestStore_ChangeHook(t *testing.T) {
	f := newHookFixture(t)
	id := f.taskID.String()

	if _, err := f.svc.UpdateAs(f.person, id, "Nova", "texto", nil, nil, task.Attrs{}); err != nil || f.took() != 1 {
		t.Errorf("edit: err = %v, the hook must be told once", err)
	}
	status := "closed"
	if _, err := f.svc.UpdateAttrs(id, task.Attrs{Status: &status}); err != nil || f.took() != 1 {
		t.Errorf("quick update: err = %v, the hook must be told once", err)
	}
	if _, err := f.svc.Claim(f.person, id); err != nil || f.took() != 1 {
		t.Errorf("claim: err = %v, the hook must be told once", err)
	}
	if _, err := f.svc.Claim(f.person, id); err != nil || f.took() != 0 {
		t.Errorf("claiming a task that is already yours changes nothing and must not tell the hook")
	}
	if err := f.store.StartProgress(f.taskID); err != nil || f.took() != 1 {
		t.Errorf("start progress on a closed task: err = %v, the hook must be told once", err)
	}
	if err := f.store.StartProgress(f.taskID); err != nil || f.took() != 0 {
		t.Errorf("start progress on a task already in progress changes nothing and must not tell the hook")
	}
	if _, err := f.svc.LinkExternalItem(id, createIntegration(t, f.project), "7", "https://github.com/o/r/issues/7"); err != nil || f.took() != 1 {
		t.Errorf("link: err = %v, the hook must be told once", err)
	}

	// O que a sincronização grava não volta a ela como mudança.
	name := "da issue"
	if err := f.store.ApplyRemote(f.taskID, task.RemotePatch{Name: &name}); err != nil || f.took() != 0 {
		t.Errorf("ApplyRemote must not tell the hook: err = %v", err)
	}
	if _, err := f.store.CreateImported(task.Imported{ProjectID: uuid.MustParse(f.project), IntegrationID: uuid.MustParse(createIntegration(t, f.project)), ItemID: "9", Name: "x"}); err != nil || f.took() != 0 {
		t.Errorf("CreateImported must not tell the hook: err = %v", err)
	}
}

// Renomear ou excluir uma etiqueta avisa as tarefas que a têm (e só elas).
func TestService_LabelChangesTellTheHook(t *testing.T) {
	f := newHookFixture(t)
	bug, _ := f.svc.CreateLabel(f.project, "bug")
	other, _ := f.svc.CreateLabel(f.project, "outra")
	ids := []string{bug.ID.String()}
	if _, err := f.svc.UpdateAttrs(f.taskID.String(), task.Attrs{LabelIDs: &ids}); err != nil {
		t.Fatalf("label the task: %v", err)
	}
	f.took()

	if _, err := f.svc.RenameLabel(f.project, other.ID.String(), "renomeada"); err != nil || f.took() != 0 {
		t.Errorf("renaming a label no task has: err = %v, must not tell the hook", err)
	}
	if _, err := f.svc.RenameLabel(f.project, bug.ID.String(), "defeito"); err != nil || f.took() != 1 {
		t.Errorf("renaming the label of the task: err = %v, the hook must be told once", err)
	}
	if err := f.svc.DeleteLabel(f.project, bug.ID.String()); err != nil || f.took() != 1 {
		t.Errorf("deleting the label of the task: err = %v, the hook must be told once", err)
	}
}

func TestStore_ApplyRemote(t *testing.T) {
	f := newHookFixture(t)
	pid := uuid.MustParse(f.project)
	assignee := uuid.MustParse(f.person)
	labels, err := f.store.FindOrCreateLabels(pid, []string{"bug", "ux"})
	if err != nil {
		t.Fatalf("labels: %v", err)
	}

	name, status := "Título da issue", "closed"
	long := strings.Repeat("é", task.MaxDescriptionLen+500) // o que vem da issue não passa pelo limite de quem edita
	if err := f.store.ApplyRemote(f.taskID, task.RemotePatch{
		Name: &name, Description: &long, Status: &status, SetAssignee: true, AssigneeID: &assignee, Labels: &labels,
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := f.store.GetByID(f.taskID.String())
	if got.Name != name || got.Description != long || got.Status != "closed" || got.AssigneeID == nil || *got.AssigneeID != assignee || len(got.Labels) != 2 {
		t.Errorf("task after apply = %+v", got)
	}

	// Só o que vem é tocado: sem responsável nem etiquetas no patch, eles ficam.
	other := "Outro"
	if err := f.store.ApplyRemote(f.taskID, task.RemotePatch{Name: &other}); err != nil {
		t.Fatalf("apply name: %v", err)
	}
	got, _ = f.store.GetByID(f.taskID.String())
	if got.Name != "Outro" || got.AssigneeID == nil || len(got.Labels) != 2 || got.Status != "closed" {
		t.Errorf("a partial patch changed more than it carried: %+v", got)
	}

	none := []task.Label{}
	if err := f.store.ApplyRemote(f.taskID, task.RemotePatch{SetAssignee: true, Labels: &none}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ = f.store.GetByID(f.taskID.String())
	if got.AssigneeID != nil || len(got.Labels) != 0 {
		t.Errorf("assignee and labels must be cleared: %+v", got)
	}
	if err := f.store.ApplyRemote(uuid.New(), task.RemotePatch{Name: &other}); err == nil {
		t.Error("applying to a task that does not exist must fail")
	}
}

func TestStore_CreateImportedAndLinked(t *testing.T) {
	f := newHookFixture(t)
	pid := uuid.MustParse(f.project)
	integ := uuid.MustParse(createIntegration(t, f.project))
	labels, _ := f.store.FindOrCreateLabels(pid, []string{"bug"})
	assignee := uuid.MustParse(f.person)

	got, err := f.store.CreateImported(task.Imported{
		ProjectID: pid, IntegrationID: integ, ItemID: "42", URL: "https://github.com/owner/repo/issues/42",
		Name: "Corrigir login", Description: "corpo", Labels: labels, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatalf("create imported: %v", err)
	}
	if got.Status != "backlog" || got.Priority != "none" || got.Name != "Corrigir login" || got.Description != "corpo" ||
		got.AssigneeID == nil || len(got.Labels) != 1 || got.ExternalIntegration == nil || got.ExternalIntegration.ID != integ ||
		got.ExternalItemID == nil || *got.ExternalItemID != "42" || got.ExternalItemURL == nil || !strings.HasSuffix(*got.ExternalItemURL, "/issues/42") {
		t.Errorf("imported task = %+v", got)
	}
	if got.Deadline.After(time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("an imported task has no deadline, got %v", got.Deadline)
	}

	// Duas tarefas ligadas à mesma issue à mão: a mais antiga vem primeiro.
	second, _ := f.store.CreateImported(task.Imported{ProjectID: pid, IntegrationID: integ, ItemID: "42", Name: "Duplicada"})
	linked, err := f.store.FindLinked(integ, "42")
	if err != nil || len(linked) != 2 || linked[0].ID != got.ID || linked[1].ID != second.ID {
		t.Errorf("FindLinked = %v, %v, want the oldest first", linked, err)
	}
	if none, _ := f.store.FindLinked(integ, "43"); len(none) != 0 {
		t.Errorf("FindLinked(43) = %d tasks", len(none))
	}
	if all, _ := f.store.ListLinked(integ); len(all) != 2 {
		t.Errorf("ListLinked = %d tasks, want 2 (the task without a link is not one)", len(all))
	}
	if all, _ := f.store.ListLinked(uuid.New()); len(all) != 0 {
		t.Errorf("another integration lists %d tasks", len(all))
	}
}

func TestStore_FindOrCreateLabels(t *testing.T) {
	f := newHookFixture(t)
	pid := uuid.MustParse(f.project)
	existing, _ := f.svc.CreateLabel(f.project, "Bug")

	got, err := f.store.FindOrCreateLabels(pid, []string{"bug", "  Nova  ", "NOVA", "", strings.Repeat("x", 80)})
	if err != nil {
		t.Fatalf("find or create: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d labels, want 3 (the existing one, one new one, the cut one): %+v", len(got), got)
	}
	if got[0].ID != existing.ID || got[0].Name != "Bug" {
		t.Errorf("a case-insensitive match must return the existing label with its own case: %+v", got[0])
	}
	if got[1].Name != "Nova" || len([]rune(got[2].Name)) != 50 {
		t.Errorf("new labels = %+v, %+v", got[1], got[2])
	}
	again, _ := f.store.FindOrCreateLabels(pid, []string{"nova"})
	if len(again) != 1 || again[0].ID != got[1].ID {
		t.Errorf("a second call must find the label it created: %+v", again)
	}

	// Duas rodadas criando a mesma etiqueta juntas acabam na mesma.
	var wg sync.WaitGroup
	ids := make([]uuid.UUID, 8)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ls, err := f.store.FindOrCreateLabels(pid, []string{"corrida"})
			if err != nil || len(ls) != 1 {
				t.Errorf("concurrent create: %v %v", ls, err)
				return
			}
			ids[i] = ls[0].ID
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("the concurrent calls ended on different labels: %v", ids)
		}
	}
}

// O prazo e a data de criação vindos da plataforma (o Trello tem as duas): a sincronização grava o prazo
// sem avisar o gancho, e a tarefa importada nasce com a data em que o cartão nasceu.
func TestStore_DeadlineAndCreationDateFromThePlatform(t *testing.T) {
	f := newHookFixture(t)
	pid := uuid.MustParse(f.project)
	integ := uuid.MustParse(createIntegration(t, f.project))
	due := time.Date(2026, 11, 3, 17, 30, 0, 0, time.UTC)
	born := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	got, err := f.store.CreateImported(task.Imported{
		ProjectID: pid, IntegrationID: integ, ItemID: "H0TZyzbK", Name: "Cartão", Deadline: due, CreatedAt: born,
	})
	if err != nil {
		t.Fatalf("create imported: %v", err)
	}
	if !got.Deadline.Equal(due) || !got.CreatedAt.Equal(born) {
		t.Errorf("imported task: deadline = %v, created_at = %v, want %v and %v", got.Deadline, got.CreatedAt, due, born)
	}
	// Sem as datas, a tarefa nasce agora e sem prazo, como a de uma issue.
	plain, _ := f.store.CreateImported(task.Imported{ProjectID: pid, IntegrationID: integ, ItemID: "OutroQdr", Name: "Sem datas"})
	if time.Since(plain.CreatedAt) > time.Minute || plain.Deadline.After(time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("a task imported without dates: deadline = %v, created_at = %v", plain.Deadline, plain.CreatedAt)
	}

	f.took()
	next := due.Add(48 * time.Hour)
	if err := f.store.ApplyRemote(got.ID, task.RemotePatch{Deadline: &next}); err != nil || f.took() != 0 {
		t.Fatalf("apply deadline: err = %v; it must not tell the hook", err)
	}
	if after, _ := f.store.GetByID(got.ID.String()); !after.Deadline.Equal(next) || after.Name != "Cartão" {
		t.Errorf("after applying a deadline: %+v", after)
	}
	// O tempo zero tira o prazo; sem Deadline no patch ele não é tocado.
	other := "Outro nome"
	if err := f.store.ApplyRemote(got.ID, task.RemotePatch{Name: &other}); err != nil {
		t.Fatalf("apply name: %v", err)
	}
	if after, _ := f.store.GetByID(got.ID.String()); !after.Deadline.Equal(next) {
		t.Errorf("a patch without a deadline changed it to %v", after.Deadline)
	}
	var cleared time.Time
	if err := f.store.ApplyRemote(got.ID, task.RemotePatch{Deadline: &cleared}); err != nil {
		t.Fatalf("clear deadline: %v", err)
	}
	if after, _ := f.store.GetByID(got.ID.String()); after.Deadline.After(time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("the zero time must leave the task with no deadline, got %v", after.Deadline)
	}
}

// Criar uma tarefa por dentro do sistema avisa o gancho de criação; a tarefa que a sincronização importa não.
func TestStore_CreateHook(t *testing.T) {
	f := newHookFixture(t)
	var mu sync.Mutex
	var created []uuid.UUID
	f.store.SetCreateHook(func(id uuid.UUID) {
		mu.Lock()
		defer mu.Unlock()
		created = append(created, id)
	})
	got := func() []uuid.UUID {
		mu.Lock()
		defer mu.Unlock()
		out := created
		created = nil
		return out
	}

	f.took()
	tk, err := f.svc.CreateAs(f.person, f.project, "Nova", "", "", nil, task.Attrs{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ids := got(); len(ids) != 1 || ids[0] != tk.ID {
		t.Errorf("create hook calls = %v, want one for %s", ids, tk.ID)
	}
	if f.took() != 0 {
		t.Error("creating a task must not tell the change hook")
	}
	// Editar não é criar.
	if _, err := f.svc.UpdateAs(f.person, tk.ID.String(), "Nova (editada)", "", nil, nil, task.Attrs{}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if ids := got(); len(ids) != 0 {
		t.Errorf("an edit told the create hook: %v", ids)
	}
	// A que a sincronização importa não avisa, e sem gancho registrado criar segue funcionando.
	pid := uuid.MustParse(f.project)
	integ := uuid.MustParse(createIntegration(t, f.project))
	if _, err := f.store.CreateImported(task.Imported{ProjectID: pid, IntegrationID: integ, ItemID: "Abc12345", Name: "Importada"}); err != nil {
		t.Fatalf("create imported: %v", err)
	}
	if ids := got(); len(ids) != 0 {
		t.Errorf("an imported task told the create hook: %v", ids)
	}
	f.store.SetCreateHook(nil)
	if _, err := f.svc.CreateAs(f.person, f.project, "Sem gancho", "", "", nil, task.Attrs{}); err != nil {
		t.Errorf("create with no hook: %v", err)
	}
}
