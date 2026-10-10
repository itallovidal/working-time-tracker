package issuesync_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

// tenv é um projeto com uma integração do Trello sincronizando com um Trello fake. O quadro começa vazio.
type tenv struct {
	t       *testing.T
	fake    *testutil.Trello
	integ   *integration.Service
	tasks   *task.Store
	taskSvc *task.Service
	rows    *issuesync.Store
	syncer  *issuesync.Syncer
	it      *integration.Integration
	project string
}

const board = testutil.TrelloBoardID

func newTrelloEnv(t *testing.T, cfg issuesync.Config) *tenv {
	t.Helper()
	testutil.Truncate(t, testDB)

	e := &tenv{t: t, fake: testutil.NewTrello()}
	e.fake.DeleteCard("H0TZyzbK")
	e.fake.DeleteCard("ArqUiv4d")
	srv := httptest.NewServer(e.fake)
	t.Cleanup(srv.Close)
	adapter.Register("trello", func() adapter.Integration { return &adapter.TrelloIntegration{BaseURL: srv.URL} })

	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	must(t, err)
	proj, err := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Projeto", "", 0, project.Routine{})
	must(t, err)
	e.project = proj.ID.String()

	e.integ = integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")
	e.it, err = e.integ.Create(e.project, "trello", "Trello", "trello-token",
		map[string]any{"api_key": testutil.TrelloKey, "board_id": board}, true)
	must(t, err)
	yes := true
	e.it, err = e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &yes})
	must(t, err)

	e.tasks = task.NewStore(testClient)
	e.taskSvc = task.NewService(e.tasks, team.NewMembershipStore(testClient), e.integ)
	e.rows = issuesync.NewStore(testClient)
	e.syncer = issuesync.New(issuesync.Deps{
		Integrations: e.integ, Tasks: e.tasks, People: person.NewStore(testClient),
		Members: team.NewMembershipStore(testClient), Rows: e.rows,
	}, cfg)
	// Como no servidor: criar uma tarefa avisa a sincronização, que a posta se há onde.
	e.tasks.SetCreateHook(e.syncer.NotifyCreated)
	e.fake.Reset()
	return e
}

func (e *tenv) sync(mode issuesync.Mode) *issuesync.Summary {
	e.t.Helper()
	sum, err := e.syncer.Sync(context.Background(), e.it.ID, mode)
	if err != nil {
		e.t.Fatalf("sync: %v", err)
	}
	return sum
}

// taskFor devolve a tarefa que a sincronização ligou ao cartão (pelo link curto).
func (e *tenv) taskFor(shortLink string) *task.Task {
	e.t.Helper()
	rows, err := e.rows.ByIntegration(e.it.ID)
	must(e.t, err)
	row := rows[shortLink]
	if row == nil || row.TaskID == nil {
		e.t.Fatalf("card %s has no task (row %+v)", shortLink, row)
	}
	tk, err := e.tasks.GetByID(row.TaskID.String())
	must(e.t, err)
	return tk
}

func (e *tenv) tasksCount() int {
	list, err := e.taskSvc.ListByProject(e.project, task.ListFilter{})
	must(e.t, err)
	return len(list)
}

func (e *tenv) lastError() string {
	it, err := e.integ.Get(e.it.ID.String())
	must(e.t, err)
	return it.LastSyncError
}

// edit muda a tarefa aqui, como quem a edita na tela.
func (e *tenv) edit(tk *task.Task, name, desc string, deadline *time.Time, labelIDs *[]string) {
	e.t.Helper()
	_, err := e.taskSvc.UpdateAs("", tk.ID.String(), name, desc, nil, deadline, task.Attrs{LabelIDs: labelIDs})
	must(e.t, err)
}

func noDeadline(d time.Time) bool { return d.Year() < 1971 }

var (
	due  = time.Date(2026, 11, 3, 17, 30, 0, 0, time.UTC)
	born = time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
)

// A primeira rodada traz os cartões abertos e só eles — com nome, descrição, etiquetas, data de entrega e a
// data em que o cartão nasceu — e não escreve nada no Trello; a segunda, sem diferença, também não, e cada
// rodada é uma listagem só.
func TestTrello_ImportsOpenCards(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	bug := e.fake.AddLabel(board, "Bug", "red")
	green := e.fake.AddLabel(board, "", "green")
	e.fake.AddCard(testutil.TrelloCard{
		ShortLink: "Rich0001", Name: "Corrigir login", Desc: "linha 1\r\nlinha 2", Labels: []string{bug, green},
		Due: due.Add(500 * time.Millisecond), Created: born,
	})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Simples1", Name: "Sem nada"})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Arquiv01", Name: "Arquivado", Closed: true})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Feito001", Name: "Feito sem arquivar", Due: due, DueComplete: true})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "OutroBrd", Name: "De outro quadro", Board: testutil.TrelloOtherBoardID})

	sum := e.sync(issuesync.Full)
	if sum.Created != 2 || sum.Errors != 0 || sum.Partial || sum.Pushed != 0 {
		t.Errorf("summary = %+v, want the two open cards created and nothing else", sum)
	}
	if e.tasksCount() != 2 {
		t.Errorf("%d tasks, want 2 (the archived, the completed and the other board's cards do not become tasks)", e.tasksCount())
	}
	rich := e.taskFor("Rich0001")
	if rich.Name != "Corrigir login" || rich.Description != "linha 1\nlinha 2" || rich.Status != "backlog" ||
		!eq(labelNames(rich), []string{"Bug", "green"}) {
		t.Errorf("rich task = %q / %q / %s / %v", rich.Name, rich.Description, rich.Status, labelNames(rich))
	}
	if !rich.Deadline.Equal(due) {
		t.Errorf("deadline = %v, want %v (to the second)", rich.Deadline, due)
	}
	if !rich.CreatedAt.Equal(born) {
		t.Errorf("created_at = %v, want the date the card was created, %v", rich.CreatedAt, born)
	}
	if len(rich.Links) != 1 || rich.Links[0].ItemID != "Rich0001" || rich.Links[0].URL != "https://trello.com/c/Rich0001" {
		t.Errorf("links = %+v, want the card Rich0001", rich.Links)
	}
	if simple := e.taskFor("Simples1"); !noDeadline(simple.Deadline) || len(simple.Labels) != 0 {
		t.Errorf("a card with no date and no labels: deadline %v, labels %v", simple.Deadline, labelNames(simple))
	}
	if e.fake.Writes() != 0 {
		t.Errorf("the first round wrote to Trello: %v", e.fake.Requests())
	}
	if n := e.fake.Count("GET", "/boards/"+board+"/cards"); n != 1 {
		t.Errorf("a round is one listing, got %d", n)
	}

	e.fake.Reset()
	if sum := e.sync(issuesync.Full); sum.Created+sum.Updated+sum.Pushed != 0 || e.fake.Writes() != 0 {
		t.Errorf("second round = %+v, writes %d; a round with no difference changes nothing (not even the deadline, which Trello answers in milliseconds)", sum, e.fake.Writes())
	}
}

// O que muda no Trello chega à tarefa: nome, descrição, etiquetas e data; arquivar e concluir a data fecham
// a tarefa, e desarquivar a reabre em backlog.
func TestTrello_CardChangesComeIn(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	bug := e.fake.AddLabel(board, "Bug", "red")
	ux := e.fake.AddLabel(board, "UX", "blue")
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Um000001", Name: "Um", Desc: "texto", Labels: []string{bug}, Due: due})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Dois0002", Name: "Dois", Due: due})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Tres0003", Name: "Três"})
	e.sync(issuesync.Full)

	newDue := due.Add(72 * time.Hour)
	e.fake.EditCard("Um000001", func(c *testutil.TrelloCard) {
		c.Name, c.Desc, c.Labels, c.Due = "Um (renomeado)", "novo\r\ntexto", []string{ux}, newDue
	})
	e.fake.EditCard("Dois0002", func(c *testutil.TrelloCard) { c.DueComplete = true })
	e.fake.EditCard("Tres0003", func(c *testutil.TrelloCard) { c.Closed = true })

	sum := e.sync(issuesync.Full)
	if sum.Updated != 3 || sum.Closed != 2 || sum.Pushed != 0 || sum.Created != 0 {
		t.Errorf("summary = %+v", sum)
	}
	one := e.taskFor("Um000001")
	if one.Name != "Um (renomeado)" || one.Description != "novo\ntexto" || !eq(labelNames(one), []string{"UX"}) || !one.Deadline.Equal(newDue) {
		t.Errorf("task one = %q / %q / %v / %v", one.Name, one.Description, labelNames(one), one.Deadline)
	}
	if e.taskFor("Dois0002").Status != "closed" {
		t.Error("completing the due date must close the task")
	}
	if e.taskFor("Tres0003").Status != "closed" {
		t.Error("archiving the card must close the task")
	}
	if e.fake.Writes() != 0 {
		t.Errorf("wrote to Trello: %v", e.fake.Requests())
	}

	// Tirar a data de entrega tira o prazo da tarefa.
	e.fake.EditCard("Um000001", func(c *testutil.TrelloCard) { c.Due = time.Time{} })
	e.sync(issuesync.Full)
	if !noDeadline(e.taskFor("Um000001").Deadline) {
		t.Errorf("removing the due date must clear the deadline, got %v", e.taskFor("Um000001").Deadline)
	}

	// Desarquivar reabre a tarefa em backlog; a data concluída que continua fecha: só desmarcar a reabre.
	e.fake.EditCard("Tres0003", func(c *testutil.TrelloCard) { c.Closed = false })
	e.fake.EditCard("Dois0002", func(c *testutil.TrelloCard) { c.DueComplete = false })
	e.sync(issuesync.Full)
	for _, id := range []string{"Tres0003", "Dois0002"} {
		if got := e.taskFor(id).Status; got != "backlog" {
			t.Errorf("task of %s after reopening the card = %q, want backlog", id, got)
		}
	}
}

// O que muda aqui chega ao cartão, e a rodada seguinte não repete: nome, descrição, data, etiquetas (as que
// o quadro não tem são criadas), fechar arquiva e conclui a data, reabrir desfaz.
func TestTrello_TaskChangesGoOut(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	bug := e.fake.AddLabel(board, "Bug", "red")
	id := e.fake.AddCard(testutil.TrelloCard{ShortLink: "Alvo0001", Name: "Alvo", Desc: "texto", Labels: []string{bug}, Due: due})
	e.sync(issuesync.Full)
	tk := e.taskFor("Alvo0001")

	nova, err := e.taskSvc.CreateLabel(e.project, "Nova etiqueta")
	must(t, err)
	bugLabel, _ := e.tasks.FindOrCreateLabels(uuid.MustParse(e.project), []string{"Bug"})
	ids := []string{bugLabel[0].ID.String(), nova.ID.String()}
	newDue := due.Add(24 * time.Hour)
	e.edit(tk, "Alvo (daqui)", "texto daqui", &newDue, &ids)
	e.fake.Reset()

	sum := e.sync(issuesync.Full)
	if sum.Pushed != 1 || sum.Updated != 0 {
		t.Errorf("summary = %+v, want the one card pushed", sum)
	}
	card, _ := e.fake.Card(id)
	if card.Name != "Alvo (daqui)" || card.Desc != "texto daqui" || !card.Due.Equal(newDue) || len(card.Labels) != 2 {
		t.Errorf("card after the push = %+v", card)
	}
	var made *testutil.TrelloLabel
	for _, l := range e.fake.Labels(board) {
		if l.Name == "Nova etiqueta" {
			made = &l
		}
	}
	if made == nil || made.Color == "" {
		t.Errorf("the label that the board lacked must be created, with a color: %+v", e.fake.Labels(board))
	}
	if e.fake.Count("PUT", "/cards/") != 1 {
		t.Errorf("one card, one PUT; got %d", e.fake.Count("PUT", "/cards/"))
	}
	e.fake.Reset()
	if sum := e.sync(issuesync.Full); sum.Pushed != 0 || e.fake.Writes() != 0 {
		t.Errorf("after the push the next round has nothing to do, got %+v and %d writes", sum, e.fake.Writes())
	}

	// Fechar a tarefa arquiva o cartão e conclui a data; reabrir a tarefa desfaz as duas coisas.
	closed := "closed"
	if _, err := e.taskSvc.UpdateAttrs(tk.ID.String(), task.Attrs{Status: &closed}); err != nil {
		t.Fatalf("close the task: %v", err)
	}
	e.sync(issuesync.Full)
	if card, _ = e.fake.Card(id); !card.Closed || !card.DueComplete {
		t.Errorf("closing the task must archive the card and complete the due date: %+v", card)
	}
	backlog := "backlog"
	if _, err := e.taskSvc.UpdateAttrs(tk.ID.String(), task.Attrs{Status: &backlog}); err != nil {
		t.Fatalf("reopen the task: %v", err)
	}
	e.sync(issuesync.Full)
	if card, _ = e.fake.Card(id); card.Closed || card.DueComplete {
		t.Errorf("reopening the task must unarchive the card and uncomplete the due date: %+v", card)
	}
	e.fake.Reset()
	if sum := e.sync(issuesync.Full); sum.Pushed+sum.Updated != 0 || e.fake.Writes() != 0 {
		t.Errorf("a settled round: %+v, %d writes", sum, e.fake.Writes())
	}
	if got := e.taskFor("Alvo0001").Status; got != "backlog" {
		t.Errorf("task status = %q", got)
	}
}

// No conflito vale o Trello, como no GitHub, inclusive na data.
func TestTrello_TrelloWinsConflicts(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Disputa1", Name: "Original", Due: due})
	e.sync(issuesync.Full)
	tk := e.taskFor("Disputa1")

	mine := due.Add(24 * time.Hour)
	e.edit(tk, "Meu nome", "", &mine, nil)
	theirs := due.Add(96 * time.Hour)
	e.fake.EditCard("Disputa1", func(c *testutil.TrelloCard) { c.Name, c.Due = "Nome do Trello", theirs })
	e.sync(issuesync.Full)

	got := e.taskFor("Disputa1")
	if got.Name != "Nome do Trello" || !got.Deadline.Equal(theirs) {
		t.Errorf("task = %q / %v, want Trello's name and date", got.Name, got.Deadline)
	}
	if card, _ := e.fake.Card("Disputa1"); card.Name != "Nome do Trello" || !card.Due.Equal(theirs) {
		t.Errorf("card = %+v; the Trello side must stay as it was", card)
	}
}

// O cartão apagado ou levado para outro quadro vira "sumiu": a tarefa fica como está e a rodada segue.
func TestTrello_GoneCards(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Apagado1", Name: "Vai sumir"})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Movido01", Name: "Vai mudar de quadro"})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Fica0001", Name: "Fica"})
	e.sync(issuesync.Full)

	e.fake.DeleteCard("Apagado1")
	e.fake.MoveCard("Movido01", testutil.TrelloOtherBoardID)
	sum := e.sync(issuesync.Full)
	if sum.Errors != 0 || sum.Partial {
		t.Errorf("summary = %+v; a card that is gone is not an error", sum)
	}
	if e.tasksCount() != 3 {
		t.Errorf("%d tasks; the tasks of gone cards stay", e.tasksCount())
	}
	rows, _ := e.rows.ByIntegration(e.it.ID)
	for _, id := range []string{"Apagado1", "Movido01"} {
		if rows[id] == nil || rows[id].State != "gone" || rows[id].TaskID == nil {
			t.Errorf("row of %s = %+v, want it marked gone and still bound to its task", id, rows[id])
		}
	}
	if rows["Fica0001"].State != "open" {
		t.Errorf("the card that stayed: %+v", rows["Fica0001"])
	}
	e.fake.Reset()
	e.sync(issuesync.Full)
	if n := e.fake.Count("GET", "/cards/"); n != 0 {
		t.Errorf("a card already known to be gone is not asked again, got %d GETs", n)
	}
}

// Um token que só observa o quadro traz os cartões e não empurra nada, e a integração avisa.
func TestTrello_ReadOnlyToken(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "SoLer001", Name: "Só leitura"})
	e.fake.SetWrite(board, false)
	e.sync(issuesync.Full)
	tk := e.taskFor("SoLer001")
	e.edit(tk, "Mudei aqui", "", nil, nil)
	e.fake.Reset()

	sum := e.sync(issuesync.Full)
	if sum.Pushed != 0 || e.fake.Writes() != 0 {
		t.Errorf("a read-only token pushed: %+v, %d writes", sum, e.fake.Writes())
	}
	if got := e.lastError(); got != "issue_sync.read_only" {
		t.Errorf("integration warning = %q, want issue_sync.read_only", got)
	}
	if card, _ := e.fake.Card("SoLer001"); card.Name != "Só leitura" {
		t.Errorf("the card changed: %+v", card)
	}
	if _, err := e.syncer.Publish(context.Background(), e.mustTask("Nova").ID, e.it.ID); !errors.Is(err, issuesync.ErrPublishReadOnly) {
		t.Errorf("publishing with a read-only token: %v", err)
	}
}

func (e *tenv) mustTask(name string) *task.Task {
	e.t.Helper()
	tk, err := e.taskSvc.CreateAs("", e.project, name, "", "", nil, task.Attrs{})
	must(e.t, err)
	return tk
}

// O que o Trello descarta sem avisar não faz a rodada insistir.
func TestTrello_SilentDiscardDoesNotLoop(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddLabel(board, "UX", "blue")
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Teima001", Name: "Teimoso"})
	e.sync(issuesync.Full)
	tk := e.taskFor("Teima001")

	ux, _ := e.tasks.FindOrCreateLabels(uuid.MustParse(e.project), []string{"UX"})
	ids := []string{ux[0].ID.String()}
	e.edit(tk, "Teimoso", "", nil, &ids)
	e.fake.Discard(board, "idLabels")
	e.fake.Reset()

	e.sync(issuesync.Full)
	if got := e.lastError(); got != "issue_sync.push_discarded" {
		t.Errorf("integration warning = %q, want issue_sync.push_discarded", got)
	}
	puts := e.fake.Count("PUT", "/cards/")
	e.sync(issuesync.Full)
	e.sync(issuesync.Full)
	if again := e.fake.Count("PUT", "/cards/"); again != puts {
		t.Errorf("the discarded push was repeated: %d PUTs, then %d", puts, again)
	}
}

// Um token revogado acaba a rodada com o erro certo, que fica na integração.
func TestTrello_RevokedToken(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Um000001", Name: "Um"})
	e.sync(issuesync.Full)
	e.fake.RevokeToken("trello-token")

	_, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full)
	if err == nil || !errors.Is(err, adapter.ErrInvalidToken) {
		t.Fatalf("sync with a revoked token: %v", err)
	}
	if got := e.lastError(); got != "integration.invalid_token" {
		t.Errorf("integration error = %q", got)
	}
	if e.tasksCount() != 1 {
		t.Errorf("%d tasks; a failed round must not touch them", e.tasksCount())
	}
}

// O Trello não filtra por data: a rodada "incremental" olha o quadro inteiro, e a integração entra na rotina
// de fundo como qualquer outra.
func TestTrello_EveryRoundIsAFullOne(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Um000001", Name: "Um"})
	e.sync(issuesync.Full)
	e.fake.Reset()

	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Dois0002", Name: "Dois"})
	if sum := e.sync(issuesync.Incremental); sum.Created != 1 {
		t.Errorf("an incremental round must still find the new card: %+v", sum)
	}
	if n := e.fake.Count("GET", "/boards/"+board+"/cards/open"); n != 1 || e.fake.Count("GET", "/boards/"+board+"/cards/all") != 0 {
		t.Errorf("an incremental round asked for %d open listings; it must be the open listing of a full round", n)
	}
	list, err := e.integ.ListSyncing()
	if err != nil || len(list) != 1 || list[0].ID != e.it.ID {
		t.Errorf("ListSyncing = %v, %v, want the Trello integration", list, err)
	}
}

// Uma tarefa ligada à mão ao cartão pelo link curto passa a ser sincronizada, e ninguém importa o cartão de novo.
func TestTrello_AdoptsManuallyLinkedTask(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	bug := e.fake.AddLabel(board, "Bug", "red")
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "Manual01", Name: "Do Trello", Desc: "corpo", Labels: []string{bug}})
	mine := e.mustTask("Minha tarefa")
	if _, err := e.taskSvc.LinkExternalItem(mine.ID.String(), e.it.ID.String(), "Manual01", "https://trello.com/c/Manual01"); err != nil {
		t.Fatalf("link: %v", err)
	}

	sum := e.sync(issuesync.Full)
	if sum.Created != 0 || e.tasksCount() != 1 {
		t.Errorf("summary = %+v, %d tasks; the card already has a task", sum, e.tasksCount())
	}
	got := e.taskFor("Manual01")
	if got.ID != mine.ID || got.Name != "Do Trello" || got.Description != "corpo" || !eq(labelNames(got), []string{"Bug"}) {
		t.Errorf("adopted task = %q / %q / %v", got.Name, got.Description, labelNames(got))
	}
	// Colar o endereço do cartão no lugar do link curto também serve.
	other := e.mustTask("Por endereço")
	e.fake.AddCard(testutil.TrelloCard{ShortLink: "PorUrl01", Name: "Pelo endereço"})
	if _, err := e.taskSvc.LinkExternalItem(other.ID.String(), e.it.ID.String(), "https://trello.com/c/PorUrl01/3-pelo-endereco", "https://trello.com/c/PorUrl01/3-pelo-endereco"); err != nil {
		t.Fatalf("link by url: %v", err)
	}
	e.sync(issuesync.Full)
	if e.tasksCount() != 2 || e.taskFor("PorUrl01").ID != other.ID {
		t.Errorf("a task linked by the address must be adopted too: %d tasks", e.tasksCount())
	}
}

// Postar uma tarefa cria o cartão na primeira lista do quadro, com o que o Trello guarda, e liga a tarefa a
// ele; a rodada seguinte não escreve nem importa o cartão de novo.
func TestTrello_PublishCreatesTheCard(t *testing.T) {
	e := newTrelloEnv(t, issuesync.Config{})
	e.fake.AddLabel(board, "Bug", "red")
	entrada := e.fake.AddList(board, "Entrada", 1)

	tk := e.mustTask("Tarefa daqui")
	bug, _ := e.tasks.FindOrCreateLabels(uuid.MustParse(e.project), []string{"Bug", "Só aqui"})
	ids := []string{bug[0].ID.String(), bug[1].ID.String()}
	deadline := due
	e.edit(tk, "Tarefa daqui", "descrição", &deadline, &ids)
	e.fake.Reset()

	res, err := e.syncer.Publish(context.Background(), tk.ID, e.it.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.Problem != "" {
		t.Errorf("problem = %q", res.Problem)
	}
	if len(res.Task.Links) != 1 || res.Task.Links[0].Integration == nil || res.Task.Links[0].Integration.ID != e.it.ID {
		t.Fatalf("published task = %+v", res.Task)
	}
	cards := e.fake.Cards(board)
	if len(cards) != 1 {
		t.Fatalf("%d cards on the board, want the one that was posted", len(cards))
	}
	c := cards[0]
	if c.ShortLink != res.Task.Links[0].ItemID || c.Name != "Tarefa daqui" || c.Desc != "descrição" || !c.Due.Equal(due) || c.List != entrada || len(c.Labels) != 2 {
		t.Errorf("card = %+v, want name, description, date, the first list and both labels (the missing one is created)", c)
	}
	e.fake.Reset()
	if sum := e.sync(issuesync.Full); sum.Created != 0 || sum.Pushed != 0 || e.fake.Writes() != 0 || e.tasksCount() != 1 {
		t.Errorf("the round after posting: %+v, %d writes, %d tasks; it must be settled", sum, e.fake.Writes(), e.tasksCount())
	}
	if _, err := e.syncer.Publish(context.Background(), tk.ID, e.it.ID); !errors.Is(err, issuesync.ErrAlreadyLinked) {
		t.Errorf("posting twice: %v", err)
	}

	// Sem lista aberta o Trello não cria o cartão, e a tarefa continua sem vínculo.
	e.fake.ClearLists(board)
	second := e.mustTask("Sem lista")
	if _, err := e.syncer.Publish(context.Background(), second.ID, e.it.ID); err == nil || !strings.Contains(err.Error(), "integration.trello_no_list") {
		t.Errorf("a board with no list: %v, want trello_no_list", err)
	}
	if got, _ := e.tasks.GetByID(second.ID.String()); len(got.Links) != 0 {
		t.Errorf("the task was linked although no card was created: %+v", got)
	}
}
