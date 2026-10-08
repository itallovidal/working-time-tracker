package issuesync_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
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

// denv é um projeto com as duas plataformas ao mesmo tempo: uma integração do GitHub e uma do Trello,
// cada uma com o seu servidor falso, e um só Syncer. O gancho de mudança está ligado, como no servidor; o de
// criação não, e quem posta a tarefa é o teste (Publish).
type denv struct {
	t       *testing.T
	gh      *testutil.GitHub
	tr      *testutil.Trello
	integ   *integration.Service
	tasks   *task.Store
	taskSvc *task.Service
	rows    *issuesync.Store
	syncer  *issuesync.Syncer
	ghIt    *integration.Integration
	trIt    *integration.Integration
	project string
}

func newDualEnv(t *testing.T) *denv { return newDualEnvWith(t, issuesync.Config{}) }

func newDualEnvWith(t *testing.T, cfg issuesync.Config) *denv {
	t.Helper()
	testutil.Truncate(t, testDB)

	d := &denv{t: t, gh: testutil.NewGitHub(), tr: testutil.NewTrello()}
	d.gh.AddRepo(repo)
	d.tr.DeleteCard("H0TZyzbK")
	d.tr.DeleteCard("ArqUiv4d")
	ghSrv, trSrv := httptest.NewServer(d.gh), httptest.NewServer(d.tr)
	t.Cleanup(ghSrv.Close)
	t.Cleanup(trSrv.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: ghSrv.URL} })
	adapter.Register("trello", func() adapter.Integration { return &adapter.TrelloIntegration{BaseURL: trSrv.URL} })

	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	must(t, err)
	proj, err := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Projeto", "", 0, project.Routine{})
	must(t, err)
	d.project = proj.ID.String()

	d.integ = integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")
	yes := true
	d.ghIt, err = d.integ.Create(d.project, "github", "GitHub", "ghp_test", map[string]any{"repo": repo}, true)
	must(t, err)
	d.ghIt, err = d.integ.Edit(d.ghIt.ID.String(), integration.EditInput{SyncIssues: &yes})
	must(t, err)
	d.trIt, err = d.integ.Create(d.project, "trello", "Trello", "trello-token",
		map[string]any{"api_key": testutil.TrelloKey, "board_id": board}, true)
	must(t, err)
	d.trIt, err = d.integ.Edit(d.trIt.ID.String(), integration.EditInput{SyncIssues: &yes})
	must(t, err)

	d.tasks = task.NewStore(testClient)
	d.taskSvc = task.NewService(d.tasks, team.NewMembershipStore(testClient), d.integ)
	d.rows = issuesync.NewStore(testClient)
	d.syncer = issuesync.New(issuesync.Deps{
		Integrations: d.integ, Tasks: d.tasks, People: person.NewStore(testClient),
		Members: team.NewMembershipStore(testClient), Rows: d.rows,
	}, cfg)
	d.tasks.SetChangeHook(d.syncer.Notify)
	d.gh.Reset()
	d.tr.Reset()
	return d
}

func (d *denv) syncGH(mode issuesync.Mode) *issuesync.Summary {
	d.t.Helper()
	sum, err := d.syncer.Sync(context.Background(), d.ghIt.ID, mode)
	must(d.t, err)
	return sum
}

func (d *denv) syncTrello(mode issuesync.Mode) *issuesync.Summary {
	d.t.Helper()
	sum, err := d.syncer.Sync(context.Background(), d.trIt.ID, mode)
	must(d.t, err)
	return sum
}

// flush é o que o worker faz depois do intervalo de agregação: empurra o que está na fila.
func (d *denv) flush() { d.syncer.Flush(context.Background()) }

// posted cria uma tarefa e a posta nas duas plataformas.
func (d *denv) posted(name string) *task.Task {
	d.t.Helper()
	tk, err := d.taskSvc.CreateAs("", d.project, name, "descrição", "", nil, task.Attrs{})
	must(d.t, err)
	for _, it := range []*integration.Integration{d.ghIt, d.trIt} {
		_, err := d.syncer.Publish(context.Background(), tk.ID, it.ID)
		must(d.t, err)
	}
	got, err := d.tasks.GetByID(tk.ID.String())
	must(d.t, err)
	if len(got.Links) != 2 {
		d.t.Fatalf("links after posting to both = %+v, want 2", got.Links)
	}
	return got
}

// issue e card são o item de cada plataforma a que a tarefa está ligada.
func (d *denv) issue(tk *task.Task) (int, testutil.GitHubIssue) {
	d.t.Helper()
	link := tk.LinkFor(d.ghIt.ID.String())
	if link == nil {
		d.t.Fatalf("task %q has no GitHub link: %+v", tk.Name, tk.Links)
	}
	n, err := strconv.Atoi(link.ItemID)
	must(d.t, err)
	issue, ok := d.gh.Issue(repo, n)
	if !ok {
		d.t.Fatalf("issue #%d does not exist", n)
	}
	return n, issue
}

func (d *denv) card(tk *task.Task) testutil.TrelloCard {
	d.t.Helper()
	link := tk.LinkFor(d.trIt.ID.String())
	if link == nil {
		d.t.Fatalf("task %q has no Trello link: %+v", tk.Name, tk.Links)
	}
	c, ok := d.tr.Card(link.ItemID)
	if !ok {
		d.t.Fatalf("card %s does not exist", link.ItemID)
	}
	return c
}

func (d *denv) cardLabels(c testutil.TrelloCard) []string {
	byID := map[string]string{}
	for _, l := range d.tr.Labels(board) {
		byID[l.ID] = l.Name
	}
	var out []string
	for _, id := range c.Labels {
		out = append(out, byID[id])
	}
	return out
}

func (d *denv) reload(tk *task.Task) *task.Task {
	d.t.Helper()
	got, err := d.tasks.GetByID(tk.ID.String())
	must(d.t, err)
	return got
}

func (d *denv) row(tk *task.Task, it *integration.Integration) *issuesync.Row {
	d.t.Helper()
	row, err := d.rows.ByTaskIntegration(tk.ID, it.ID)
	must(d.t, err)
	return row
}

// A tarefa postada nas duas plataformas ao mesmo tempo: cada uma recebe o seu item, a tarefa guarda os dois
// vínculos, e as rodadas seguintes não escrevem nada nem trazem os itens de volta como tarefas novas.
func TestDual_PostedToBothPlatforms(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Nova daqui")

	_, issue := d.issue(tk)
	card := d.card(tk)
	if issue.Title != "Nova daqui" || issue.Body != "descrição" || card.Name != "Nova daqui" || card.Desc != "descrição" {
		t.Errorf("issue %+v and card %+v, want both to carry the task", issue, card)
	}
	for _, it := range []*integration.Integration{d.ghIt, d.trIt} {
		if _, err := d.syncer.Publish(context.Background(), tk.ID, it.ID); !errors.Is(err, issuesync.ErrAlreadyLinked) {
			t.Errorf("posting again to %s = %v, want task.already_linked", it.Type, err)
		}
	}

	d.gh.Reset()
	d.tr.Reset()
	d.flush()
	for name, sum := range map[string]*issuesync.Summary{"GitHub": d.syncGH(issuesync.Full), "Trello": d.syncTrello(issuesync.Full)} {
		if sum.Created != 0 || sum.Pushed != 0 || sum.Updated != 0 || sum.Errors != 0 {
			t.Errorf("%s round after posting = %+v, want it settled", name, sum)
		}
	}
	if d.gh.Writes() != 0 || d.tr.Writes() != 0 {
		t.Errorf("settled rounds wrote: GitHub %v, Trello %v", d.gh.Requests(), d.tr.Requests())
	}
	if n := len(testListTasks(t, d)); n != 1 {
		t.Errorf("%d tasks, want the one that was posted (the items must not come back as tasks)", n)
	}
}

func testListTasks(t *testing.T, d *denv) []task.Task {
	t.Helper()
	list, err := d.taskSvc.ListByProject(d.project, task.ListFilter{})
	must(t, err)
	return list
}

// O que muda no GitHub chega à tarefa na rodada e dela ao Trello pelo gancho; o que muda no Trello faz o
// caminho contrário. A tarefa é o centro, e nenhum lado volta a empurrar o que acabou de receber.
func TestDual_ChangesTravelThroughTheTask(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Original")
	n, _ := d.issue(tk)

	d.gh.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.Title = "Mudou no GitHub"; i.Body = "corpo novo" })
	sum := d.syncGH(issuesync.Full)
	if sum.Updated != 1 || d.syncer.Pending() != 1 {
		t.Fatalf("GitHub round = %+v, pending %d; want the task updated and queued for the other platform", sum, d.syncer.Pending())
	}
	if got := d.reload(tk); got.Name != "Mudou no GitHub" || got.Description != "corpo novo" {
		t.Errorf("task = %q / %q, want the GitHub's", got.Name, got.Description)
	}
	d.flush()
	if c := d.card(tk); c.Name != "Mudou no GitHub" || c.Desc != "corpo novo" {
		t.Errorf("card = %q / %q, want the change to reach Trello through the task", c.Name, c.Desc)
	}
	if d.syncer.Pending() != 0 {
		t.Errorf("%d tasks still queued after the flush; a change must not bounce", d.syncer.Pending())
	}

	d.tr.EditCard(d.reload(tk).LinkFor(d.trIt.ID.String()).ItemID, func(c *testutil.TrelloCard) { c.Name = "Mudou no Trello" })
	sum = d.syncTrello(issuesync.Full)
	if sum.Updated != 1 || d.syncer.Pending() != 1 {
		t.Fatalf("Trello round = %+v, pending %d; want the task updated and queued for the other platform", sum, d.syncer.Pending())
	}
	d.flush()
	if _, issue := d.issue(tk); issue.Title != "Mudou no Trello" {
		t.Errorf("issue title = %q, want the change to reach GitHub through the task", issue.Title)
	}

	// Assentado: nenhuma rodada escreve de novo.
	d.gh.Reset()
	d.tr.Reset()
	d.syncGH(issuesync.Full)
	d.syncTrello(issuesync.Full)
	d.flush()
	if d.gh.Writes() != 0 || d.tr.Writes() != 0 || d.syncer.Pending() != 0 {
		t.Errorf("after settling: GitHub %v, Trello %v, pending %d", d.gh.Requests(), d.tr.Requests(), d.syncer.Pending())
	}
}

// Fechar num lado fecha a tarefa e o outro item; reabrir faz o caminho de volta.
func TestDual_ClosingAndReopeningTravel(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Vai fechar")
	n, _ := d.issue(tk)

	d.gh.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.State = "closed"; i.StateReason = "completed" })
	d.syncGH(issuesync.Full)
	if got := d.reload(tk); got.Status != "closed" {
		t.Fatalf("task status = %q, want it closed with the issue", got.Status)
	}
	d.flush()
	if c := d.card(tk); !c.Closed {
		t.Errorf("card = %+v, want it archived when the task closed", c)
	}

	short := d.reload(tk).LinkFor(d.trIt.ID.String()).ItemID
	d.tr.EditCard(short, func(c *testutil.TrelloCard) { c.Closed = false; c.DueComplete = false })
	d.syncTrello(issuesync.Full)
	if got := d.reload(tk); got.Status == "closed" {
		t.Fatalf("task status = %q, want it reopened with the card", got.Status)
	}
	d.flush()
	if _, issue := d.issue(tk); issue.State != "open" {
		t.Errorf("issue state = %q, want it reopened through the task", issue.State)
	}
}

// Uma etiqueta posta num lado vai para o outro, e as duas listas terminam iguais.
func TestDual_LabelsTravel(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Com etiqueta")
	n, _ := d.issue(tk)

	d.gh.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.Labels = append(i.Labels, "urgente") })
	d.syncGH(issuesync.Full)
	d.flush()
	if got := d.cardLabels(d.card(tk)); !eq(got, []string{"urgente"}) {
		t.Errorf("card labels = %v, want the GitHub's label to reach Trello", got)
	}
	if got := d.reload(tk); !eq(labelNames(got), []string{"urgente"}) {
		t.Errorf("task labels = %v", labelNames(got))
	}
}

// Desligar a tarefa de um item deixa o outro sincronizando, e o item solto não é mexido nem volta como tarefa.
func TestDual_UnlinkingOneKeepsTheOther(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Duas casas")
	n, _ := d.issue(tk)

	if _, err := d.taskSvc.UnlinkExternalItem(tk.ID.String(), d.ghIt.ID.String()); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if row := d.row(tk, d.ghIt); row != nil {
		t.Errorf("the GitHub link is still bound to the task: %+v", row)
	}
	d.gh.Reset()
	d.tr.EditCard(d.reload(tk).LinkFor(d.trIt.ID.String()).ItemID, func(c *testutil.TrelloCard) { c.Name = "Só o Trello" })
	d.syncTrello(issuesync.Full)
	d.flush()
	if got := d.reload(tk); got.Name != "Só o Trello" || len(got.Links) != 1 {
		t.Fatalf("task = %q with %d links, want the Trello change applied and one link left", got.Name, len(got.Links))
	}
	if issue, _ := d.gh.Issue(repo, n); issue.Title != "Duas casas" || d.gh.Writes() != 0 {
		t.Errorf("the unlinked issue was touched: %q, %v", issue.Title, d.gh.Requests())
	}
	if sum := d.syncGH(issuesync.Full); sum.Created != 0 {
		t.Errorf("the unlinked issue came back as %d task(s)", sum.Created)
	}
}

// Excluir a tarefa solta os dois itens: nenhum volta.
func TestDual_DeletingTheTaskDiscardsBothItems(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Vai embora")
	must(t, d.taskSvc.Delete(tk.ID.String()))

	for name, sync := range map[string]func(issuesync.Mode) *issuesync.Summary{"GitHub": d.syncGH, "Trello": d.syncTrello} {
		if sum := sync(issuesync.Full); sum.Created != 0 {
			t.Errorf("%s: the item of a deleted task came back as %d task(s)", name, sum.Created)
		}
	}
	if n := len(testListTasks(t, d)); n != 0 {
		t.Errorf("%d tasks, want none", n)
	}
}

// A tarefa com um vínculo só nunca entra na fila entre plataformas: não há para onde levar.
func TestDual_ASingleLinkNeverQueues(t *testing.T) {
	d := newDualEnv(t)
	tk, err := d.taskSvc.CreateAs("", d.project, "Só no GitHub", "", "", nil, task.Attrs{})
	must(t, err)
	_, err = d.syncer.Publish(context.Background(), tk.ID, d.ghIt.ID)
	must(t, err)
	d.flush()

	d.gh.EditIssue(repo, 1, func(i *testutil.GitHubIssue) { i.Title = "Mudou lá" })
	if sum := d.syncGH(issuesync.Full); sum.Updated != 1 {
		t.Fatalf("round = %+v, want the task updated", sum)
	}
	if d.syncer.Pending() != 0 {
		t.Errorf("%d tasks queued; a task in one platform has no other to tell", d.syncer.Pending())
	}
}

// A issue ligada à mão que já está fechada não é adotada: o vínculo espera, a tarefa fica como está.
func TestDual_ClosedIssueIsNotAdopted(t *testing.T) {
	d := newDualEnv(t)
	n := d.gh.AddIssue(repo, testutil.GitHubIssue{Title: "Antiga", State: "closed"})
	tk, err := d.taskSvc.CreateAs("", d.project, "Minha", "", "", nil, task.Attrs{})
	must(t, err)
	_, err = d.taskSvc.LinkExternalItem(tk.ID.String(), d.ghIt.ID.String(), strconv.Itoa(n), "https://github.com/owner/repo/issues/"+strconv.Itoa(n))
	must(t, err)

	sum := d.syncGH(issuesync.Full)
	if sum.Created != 0 || sum.Updated != 0 || sum.Pushed != 0 {
		t.Errorf("round = %+v, want nothing for a closed issue", sum)
	}
	if got := d.reload(tk); got.Name != "Minha" || len(got.Links) != 1 {
		t.Errorf("task = %q with %d links, want it untouched and still linked", got.Name, len(got.Links))
	}
	if row := d.row(tk, d.ghIt); row == nil || row.State != "pending" {
		t.Errorf("row = %+v, want it still pending", row)
	}
	if d.gh.Writes() != 0 {
		t.Errorf("a closed issue was written to: %v", d.gh.Requests())
	}
}

// Duas postagens ao mesmo tempo na mesma integração criam um item só: a segunda vê o vínculo da primeira.
func TestDual_ConcurrentPostsMakeOneItem(t *testing.T) {
	d := newDualEnv(t)
	tk, err := d.taskSvc.CreateAs("", d.project, "Duas vezes", "", "", nil, task.Attrs{})
	must(t, err)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = d.syncer.Publish(context.Background(), tk.ID, d.ghIt.ID)
		}()
	}
	wg.Wait()
	posted, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			posted++
		case errors.Is(err, issuesync.ErrAlreadyLinked):
			refused++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if posted != 1 || refused != 1 {
		t.Errorf("%d posted and %d refused, want one of each", posted, refused)
	}
	if _, ok := d.gh.Issue(repo, 2); ok {
		t.Error("the repository got two issues for the same task")
	}
}

// Quem desligou a tarefa do item no meio de uma rodada não é desfeito: gravar o acordo velho não religa.
func TestDual_StaleSaveDoesNotResurrectAnUnlink(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Desligada no meio")
	stale := d.row(tk, d.ghIt)

	if _, err := d.taskSvc.UnlinkExternalItem(tk.ID.String(), d.ghIt.ID.String()); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	stale.Title = "acordo velho"
	_ = d.rows.Save(stale) // recusado: o vínculo já não é desta tarefa
	got, err := d.rows.ByID(stale.ID)
	must(t, err)
	if got == nil || got.TaskID != nil || got.Title == "acordo velho" {
		t.Errorf("row after a stale save = %+v, want it still released and untouched", got)
	}
}

// A postagem sozinha é por integração: a tarefa que o modal postou no GitHub também vira cartão, e a que já tem
// um item no Trello (ligado à mão) não ganha outro.
func TestDual_AutoPostIsPerIntegration(t *testing.T) {
	d := newDualEnv(t)
	d.tasks.SetCreateHook(d.syncer.NotifyCreated)

	tk, err := d.taskSvc.CreateAs("", d.project, "Nos dois", "", "", nil, task.Attrs{})
	must(t, err)
	_, err = d.syncer.Publish(context.Background(), tk.ID, d.ghIt.ID)
	must(t, err)
	d.flush()
	got := d.reload(tk)
	if len(got.Links) != 2 || got.LinkFor(d.trIt.ID.String()) == nil {
		t.Fatalf("links = %+v, want the GitHub issue and the card the background post made", got.Links)
	}
	if c := d.card(got); c.Name != "Nos dois" {
		t.Errorf("card = %+v", c)
	}

	// Um item ligado à mão no Trello: a postagem sozinha não cria um segundo.
	d.tr.AddCard(testutil.TrelloCard{ShortLink: "Manual01", Name: "Do Trello"})
	manual, err := d.taskSvc.CreateAs("", d.project, "Já tem cartão", "", "", nil, task.Attrs{})
	must(t, err)
	_, err = d.taskSvc.LinkExternalItem(manual.ID.String(), d.trIt.ID.String(), "Manual01", "")
	must(t, err)
	before := len(d.tr.Cards(board))
	d.flush()
	if n := len(d.tr.Cards(board)); n != before {
		t.Errorf("%d cards, want %d: a task with a card of this integration must not get another", n, before)
	}
}

// Desmarcar o Trello na criação (skip_publish) deixa a tarefa fora dele, sem cair para outra integração; um id
// que não é de nenhuma integração do projeto não impede nada.
func TestDual_UntickedIntegrationIsSkipped(t *testing.T) {
	d := newDualEnv(t)
	d.tasks.SetCreateHook(d.syncer.NotifyCreated)

	only, err := d.taskSvc.CreateAs("", d.project, "Só no GitHub", "", "", nil, task.Attrs{SkipPublish: []uuid.UUID{d.trIt.ID}})
	must(t, err)
	_, err = d.syncer.Publish(context.Background(), only.ID, d.ghIt.ID)
	must(t, err)
	d.flush()
	if got := d.reload(only); len(got.Links) != 1 || got.LinkFor(d.ghIt.ID.String()) == nil {
		t.Errorf("links = %+v, want only the GitHub issue", got.Links)
	}
	if n := len(d.tr.Cards(board)); n != 0 {
		t.Errorf("%d cards, want none for a task made with the Trello unticked", n)
	}

	unknown, err := d.taskSvc.CreateAs("", d.project, "Id desconhecido", "", "", nil, task.Attrs{SkipPublish: []uuid.UUID{uuid.New()}})
	must(t, err)
	d.flush()
	if got := d.reload(unknown); got.LinkFor(d.trIt.ID.String()) == nil {
		t.Errorf("links = %+v, want the card: an unknown id in skip_publish skips nothing", got.Links)
	}
}

// Uma issue que veio do GitHub pode ser espelhada no Trello depois (e um cartão do Trello, no GitHub): o item
// novo nasce com o que a tarefa tem, os dois ficam em acordo, e uma mudança num lado chega ao outro pela tarefa.
func TestDual_MirrorAnImportedItem(t *testing.T) {
	d := newDualEnv(t)
	ctx := context.Background()
	taskOf := func(it *integration.Integration, item string) *task.Task {
		t.Helper()
		rows, err := d.rows.ByIntegration(it.ID)
		must(t, err)
		row := rows[item]
		if row == nil || row.TaskID == nil {
			t.Fatalf("item %s has no task (row %+v)", item, row)
		}
		tk, err := d.tasks.GetByID(row.TaskID.String())
		must(t, err)
		return tk
	}

	// GitHub para o Trello.
	n := d.gh.AddIssue(repo, testutil.GitHubIssue{Title: "Veio do GitHub", Body: "corpo", Labels: []string{"bug"}})
	d.syncGH(issuesync.Full)
	tk := taskOf(d.ghIt, strconv.Itoa(n))
	if len(tk.Links) != 1 {
		t.Fatalf("links = %+v, want only the issue it came from", tk.Links)
	}
	if _, err := d.syncer.Publish(ctx, tk.ID, d.trIt.ID); err != nil {
		t.Fatalf("mirror on Trello: %v", err)
	}
	tk = d.reload(tk)
	card := d.card(tk)
	if len(tk.Links) != 2 || card.Name != "Veio do GitHub" || card.Desc != "corpo" || !eq(d.cardLabels(card), []string{"bug"}) {
		t.Fatalf("task %+v, card %+v (labels %v), want the card made from the issue", tk.Links, card, d.cardLabels(card))
	}
	d.gh.Reset()
	d.tr.Reset()
	if sum := d.syncTrello(issuesync.Full); sum.Created != 0 || sum.Pushed != 0 || d.tr.Writes() != 0 {
		t.Errorf("round after mirroring = %+v with %d writes, want it settled (the card must not come back as a task)", sum, d.tr.Writes())
	}
	if n := len(testListTasks(t, d)); n != 1 {
		t.Errorf("%d tasks, want one", n)
	}
	d.tr.EditCard(tk.LinkFor(d.trIt.ID.String()).ItemID, func(c *testutil.TrelloCard) { c.Name = "Mudou no cartão" })
	d.syncTrello(issuesync.Full)
	d.flush()
	if _, issue := d.issue(tk); issue.Title != "Mudou no cartão" {
		t.Errorf("issue title = %q, want the change on the mirrored card to reach it through the task", issue.Title)
	}

	// Trello para o GitHub.
	d.tr.AddCard(testutil.TrelloCard{ShortLink: "Veio0001", Name: "Veio do Trello", Desc: "descrição do cartão"})
	d.syncTrello(issuesync.Full)
	fromCard := taskOf(d.trIt, "Veio0001")
	if _, err := d.syncer.Publish(ctx, fromCard.ID, d.ghIt.ID); err != nil {
		t.Fatalf("mirror on GitHub: %v", err)
	}
	fromCard = d.reload(fromCard)
	_, issue := d.issue(fromCard)
	if len(fromCard.Links) != 2 || issue.Title != "Veio do Trello" || issue.Body != "descrição do cartão" {
		t.Errorf("task %+v, issue %+v, want the issue made from the card", fromCard.Links, issue)
	}
	if _, err := d.syncer.Publish(ctx, fromCard.ID, d.ghIt.ID); !errors.Is(err, issuesync.ErrAlreadyLinked) {
		t.Errorf("mirroring twice = %v, want task.already_linked", err)
	}
}

// Editar a tarefa que está nas duas plataformas leva a edição às duas, pelo gancho: é o que a pessoa espera de
// uma tarefa sincronizada com o GitHub e com o Trello.
func TestDual_EditingTheTaskPushesToBothPlatforms(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Tarefa nos dois")

	if _, err := d.taskSvc.UpdateAs("", tk.ID.String(), "Tarefa nos dois", "descrição nova", nil, nil, task.Attrs{}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	d.flush()
	if _, issue := d.issue(tk); issue.Body != "descrição nova" {
		t.Errorf("issue body = %q, want the edit to reach GitHub", issue.Body)
	}
	if c := d.card(tk); c.Desc != "descrição nova" {
		t.Errorf("card description = %q, want the edit to reach Trello", c.Desc)
	}
}

// Uma integração em espera (o token que o GitHub recusou, o limite de requisições) não faz a edição se perder: o
// que não pôde sair espera, e sai sozinho quando a integração se recupera, sem depender da rodada completa da hora.
func TestDual_AnEditWaitsOutABackedOffIntegration(t *testing.T) {
	var now atomic.Int64
	now.Store(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC).UnixNano())
	clock := func() time.Time { return time.Unix(0, now.Load()).UTC() }
	// A rotina de fundo olha só o Trello: o GitHub só recebe o que o gancho empurrar.
	d := newDualEnvWith(t, issuesync.Config{
		Now: clock, Debounce: 20 * time.Millisecond, Intervals: map[string]time.Duration{"github": 0, "trello": 50 * time.Millisecond},
	})
	tk := d.posted("Com o token recusado")

	// O GitHub recusa o token no empurrão: o Trello recebe a edição, o GitHub entra em espera.
	d.gh.Fail("PATCH", "/repos/"+repo+"/issues", http.StatusUnauthorized, 1)
	if _, err := d.taskSvc.UpdateAs("", tk.ID.String(), "Com o token recusado", "primeira edição", nil, nil, task.Attrs{}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	d.flush()
	if c := d.card(tk); c.Desc != "primeira edição" {
		t.Fatalf("card description = %q, want the first edit on Trello", c.Desc)
	}
	if _, issue := d.issue(tk); issue.Body != "descrição" {
		t.Fatalf("issue body = %q, want GitHub untouched (it refused the token)", issue.Body)
	}

	// Uma segunda edição com o GitHub em espera: nem tenta, e também não se perde.
	if _, err := d.taskSvc.UpdateAs("", tk.ID.String(), "Com o token recusado", "segunda edição", nil, nil, task.Attrs{}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	d.gh.Reset()
	d.flush()
	if d.gh.Writes() != 0 {
		t.Errorf("GitHub was written to while backed off: %v", d.gh.Requests())
	}
	if _, issue := d.issue(tk); issue.Body != "descrição" {
		t.Fatalf("issue body = %q, want GitHub still untouched", issue.Body)
	}

	// A espera passa, e a rotina de fundo devolve à fila o que ficou para depois: o GitHub recebe a edição mais
	// nova, sem rodada completa dele (a rotina não olha o GitHub).
	now.Add(int64(30 * time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { d.syncer.Run(ctx); close(stopped) }()
	defer func() {
		cancel()
		<-stopped
	}()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, issue := d.issue(tk); issue.Body == "segunda edição" {
			return
		}
		if time.Now().After(deadline) {
			_, issue := d.issue(tk)
			t.Fatalf("issue body = %q, want the edit that waited to reach GitHub once it recovered", issue.Body)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
