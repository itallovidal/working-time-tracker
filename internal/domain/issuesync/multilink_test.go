package issuesync_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

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

func newDualEnv(t *testing.T) *denv {
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
	}, issuesync.Config{})
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
