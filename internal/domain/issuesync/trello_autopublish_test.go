package issuesync_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/testutil"
)

// create cria uma tarefa como a tela cria, o que avisa a sincronização.
func (e *tenv) create(name, desc string, deadline *time.Time, labelIDs *[]string) *task.Task {
	e.t.Helper()
	tk, err := e.taskSvc.CreateAs("", e.project, name, desc, "", deadline, task.Attrs{LabelIDs: labelIDs})
	must(e.t, err)
	return tk
}

func (e *tenv) cardsCount() int { return len(e.fake.Cards(board)) }

// A tarefa criada aqui vira um cartão sozinha, na primeira lista, com o que o Trello guarda, e a pessoa não
// espera: criar só avisa, e o worker (aqui, o Flush) a posta. A rodada seguinte não escreve nada.
func TestTrello_NewTaskIsPostedByItself(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddLabel(board, "Bug", "red")
	entrada := e.fake.AddList(board, "Entrada", 1)
	labels, err := e.tasks.FindOrCreateLabels(uuid.MustParse(e.project), []string{"Bug", "Só aqui"})
	must(t, err)
	ids := []string{labels[0].ID.String(), labels[1].ID.String()}
	deadline := due

	tk := e.create("Nova daqui", "descrição", &deadline, &ids)
	if e.fake.Writes() != 0 || e.cardsCount() != 0 {
		t.Fatal("creating the task must not wait for, or write to, Trello")
	}
	if got := e.syncer.Flush(context.Background()); got != 1 {
		t.Errorf("Flush touched %d, want the one new task", got)
	}
	cards := e.fake.Cards(board)
	if len(cards) != 1 {
		t.Fatalf("%d cards, want 1", len(cards))
	}
	c := cards[0]
	if c.Name != "Nova daqui" || c.Desc != "descrição" || !c.Due.Equal(due) || c.List != entrada || len(c.Labels) != 2 {
		t.Errorf("card = %+v, want name, description, date, the first list and both labels (the missing one is created)", c)
	}
	linked, err := e.tasks.GetByID(tk.ID.String())
	must(t, err)
	if linked.ExternalItemID == nil || *linked.ExternalItemID != c.ShortLink || linked.ExternalIntegration == nil || linked.ExternalIntegration.ID != e.it.ID {
		t.Errorf("task = %+v, want it linked to the new card", linked)
	}

	e.fake.Reset()
	if e.syncer.Flush(context.Background()); e.fake.Writes() != 0 {
		t.Errorf("a second Flush wrote: %v", e.fake.Requests())
	}
	if sum := e.sync(issuesync.Full); sum.Created != 0 || sum.Pushed != 0 || e.fake.Writes() != 0 || e.tasksCount() != 1 {
		t.Errorf("the round after: %+v, %d writes, %d tasks; it must be settled (the new card must not come back as a task)", sum, e.fake.Writes(), e.tasksCount())
	}
}

// A tarefa que já está ligada a um item (a etapa Integrações do modal a postou antes, ou alguém a ligou) não
// ganha um segundo; a que nasce onde a sincronização está desligada, ou numa integração desativada, não é postada;
// a que a própria sincronização importa não volta como cartão.
func TestTrello_NewTaskIsNotPostedWhenThereIsNothingToDo(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	linked := e.create("Já ligada", "", nil, nil)
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Manual01", Name: "Do Trello"})
	if _, err := e.taskSvc.LinkExternalItem(linked.ID.String(), e.it.ID.String(), "Manual01", ""); err != nil {
		t.Fatalf("link: %v", err)
	}
	e.fake.Reset()
	e.syncer.Flush(context.Background())
	if e.fake.Writes() != 0 || e.cardsCount() != 1 {
		t.Errorf("a task already linked got a second card: %d cards, %v", e.cardsCount(), e.fake.Requests())
	}

	// Sincronização desligada, ou integração desativada.
	no := false
	if _, err := e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &no}); err != nil {
		t.Fatalf("turn the sync off: %v", err)
	}
	off := e.create("Com a sincronização desligada", "", nil, nil)
	e.syncer.Flush(context.Background())
	yes := true
	if _, err := e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &yes}); err != nil {
		t.Fatalf("turn the sync on: %v", err)
	}
	if got, _ := e.tasks.GetByID(off.ID.String()); got.ExternalItemID != nil || e.cardsCount() != 1 {
		t.Errorf("a task made with the sync off was posted: %+v, %d cards", got, e.cardsCount())
	}
	disabled := false
	if _, err := e.integ.Edit(e.it.ID.String(), integration.EditInput{Enabled: &disabled, SyncIssues: &no}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	e.create("Com a integração desativada", "", nil, nil)
	e.syncer.Flush(context.Background())
	if e.cardsCount() != 1 {
		t.Errorf("a task made with the integration disabled was posted: %d cards", e.cardsCount())
	}
}

func TestTrello_ImportedTasksAreNotPostedBack(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Veio0001", Name: "Veio do Trello"})
	e.sync(issuesync.Full)
	e.fake.Reset()
	if got := e.syncer.Flush(context.Background()); got != 0 || e.fake.Writes() != 0 || e.cardsCount() != 1 {
		t.Errorf("an imported task was posted back: touched %d, %d cards, %v", got, e.cardsCount(), e.fake.Requests())
	}
}

// O Trello que recusa por si (um quadro sem lista) deixa a tarefa criada e sem cartão, com o aviso na integração,
// e a rotina não insiste.
func TestTrello_NewTaskTrelloRefuses(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.ClearLists(board)
	tk := e.create("Sem lista", "", nil, nil)
	e.syncer.Flush(context.Background())
	if got := e.lastError(); got != "integration.trello_no_list" {
		t.Errorf("integration warning = %q, want integration.trello_no_list", got)
	}
	if got, _ := e.tasks.GetByID(tk.ID.String()); got.ExternalItemID != nil {
		t.Errorf("the task was linked although no card was made: %+v", got)
	}
	e.fake.AddList(board, "Nova lista", 1)
	e.fake.Reset()
	e.syncer.SyncNow(context.Background(), e.it.ID)
	if e.cardsCount() != 0 {
		t.Errorf("a refused task was tried again by the button: %d cards", e.cardsCount())
	}
}

// O Trello fora do ar (o limite de requisições) não perde a tarefa: ela fica esperando, a integração avisa, e o botão
// Sincronizar (ou a rotina de fundo) a posta quando passa.
func TestTrello_NewTaskWaitsForTrello(t *testing.T) {
	now := time.Now()
	e := newTrelloEnv(t, issuesync.Config{Now: func() time.Time { return now }})
	e.fake.RateLimitedFor(time.Hour)
	tk := e.create("Espera o Trello", "", nil, nil)

	e.syncer.Flush(context.Background())
	if got := e.lastError(); got != "integration.rate_limited" {
		t.Errorf("integration warning = %q, want the rate limit", got)
	}
	if got, _ := e.tasks.GetByID(tk.ID.String()); got.ExternalItemID != nil {
		t.Fatal("the task was linked although Trello refused")
	}

	// Enquanto a integração espera o limite acabar, a tarefa não é tentada (nem pelo botão).
	e.fake.RateLimitedFor(0)
	e.fake.Reset()
	e.syncer.SyncNow(context.Background(), e.it.ID)
	if e.cardsCount() != 0 {
		t.Errorf("the task was tried while the integration waits: %d cards", e.cardsCount())
	}

	// Passada a espera, o botão posta a tarefa que ficou, uma vez só.
	now = now.Add(2 * time.Minute)
	if _, err := e.syncer.SyncNow(context.Background(), e.it.ID); err != nil {
		t.Fatalf("sync now: %v", err)
	}
	if e.cardsCount() != 1 {
		t.Fatalf("%d cards after the wait, want the waiting task posted", e.cardsCount())
	}
	if got, _ := e.tasks.GetByID(tk.ID.String()); got.ExternalItemID == nil {
		t.Error("the task was not linked after the retry")
	}
	if _, err := e.syncer.SyncNow(context.Background(), e.it.ID); err != nil || e.cardsCount() != 1 {
		t.Errorf("a later round posted again: %v, %d cards", err, e.cardsCount())
	}
}

// run sobe a rotina de fundo e devolve quem a para.
func (e *tenv) run() (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { e.syncer.Run(ctx); close(stopped) }()
	return func() {
		cancel()
		<-stopped
	}
}

// Cada plataforma tem o seu intervalo na rotina de fundo: o Trello com intervalo próprio é olhado mesmo com o
// comum longo, e com o seu em zero ele não é olhado, mesmo com o comum curto (o botão e o gancho seguem valendo).
func TestTrello_ItHasItsOwnPollingInterval(t *testing.T) {
	// O comum é de uma hora; o do Trello, de 50 ms: o cartão chega sozinho.
	e := newTrelloEnv(t, issuesync.Config{Interval: time.Hour, Intervals: map[string]time.Duration{"trello": 50 * time.Millisecond}})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Sozinho1", Name: "Chega sozinho"})
	stop := e.run()
	deadline := time.Now().Add(20 * time.Second)
	for e.tasksCount() == 0 {
		if time.Now().After(deadline) {
			stop()
			t.Fatal("the background routine did not poll Trello at its own interval")
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()

	// O comum é curto, mas o do Trello é zero: a rotina não o olha.
	e = newTrelloEnv(t, issuesync.Config{Interval: 50 * time.Millisecond, Intervals: map[string]time.Duration{"trello": 0}})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Parado01", Name: "Não chega sozinho"})
	stop = e.run()
	time.Sleep(1500 * time.Millisecond)
	stop()
	if e.tasksCount() != 0 || e.fake.Count("GET", "/boards/"+board+"/cards") != 0 {
		t.Errorf("the routine polled Trello although its interval is off: %d tasks, %v", e.tasksCount(), e.fake.Requests())
	}
	// O botão segue valendo.
	if sum, err := e.syncer.SyncNow(context.Background(), e.it.ID); err != nil || sum.Created != 1 {
		t.Errorf("the button with the routine off for Trello: %+v, %v", sum, err)
	}
}
