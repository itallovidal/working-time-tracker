package issuesync_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/task"
)

// deleteTask exclui a tarefa pedindo para tirar o item das integrações dadas.
func (d *denv) deleteTask(tk *task.Task, removeIn ...*integration.Integration) []task.RemoteResult {
	d.t.Helper()
	ids := make([]uuid.UUID, 0, len(removeIn))
	for _, it := range removeIn {
		ids = append(ids, it.ID)
	}
	res, err := d.taskSvc.Delete(context.Background(), tk.ID.String(), ids)
	must(d.t, err)
	if res == nil {
		d.t.Fatal("remote results = nil, want a list (empty when nothing was asked)")
	}
	return res
}

func resultFor(t *testing.T, res []task.RemoteResult, it *integration.Integration) task.RemoteResult {
	t.Helper()
	for _, r := range res {
		if r.IntegrationID == it.ID {
			return r
		}
	}
	t.Fatalf("no result for %s in %+v", it.Type, res)
	return task.RemoteResult{}
}

// noProblem confere que o item foi tratado como se esperava, sem aviso.
func noProblem(t *testing.T, r task.RemoteResult, outcome string) {
	t.Helper()
	if r.Outcome != outcome || r.Problem != nil {
		t.Errorf("%s: result = %+v (problem %v), want %q with no problem", r.Provider, r, r.Problem, outcome)
	}
}

// Marcadas as duas caixas: a issue do GitHub é apagada, o cartão do Trello é arquivado (sem marcar o prazo como
// concluído), a tarefa sai, e nenhuma rodada traz nada de volta.
func TestRemove_DeletesTheIssueAndArchivesTheCard(t *testing.T) {
	d := newDualEnv(t)
	due := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	tk, err := d.taskSvc.CreateAs("", d.project, "Vai embora", "descrição", "", &due, task.Attrs{})
	must(t, err)
	for _, it := range []*integration.Integration{d.ghIt, d.trIt} {
		_, err := d.syncer.Publish(context.Background(), tk.ID, it.ID)
		must(t, err)
	}
	tk = d.reload(tk)
	n, _ := d.issue(tk)
	card := d.card(tk)
	if card.Due.IsZero() {
		t.Fatal("the card should have taken the deadline of the task")
	}

	d.gh.Reset()
	d.tr.Reset()
	res := d.deleteTask(tk, d.ghIt, d.trIt)

	if len(res) != 2 {
		t.Fatalf("results = %+v, want one for each platform", res)
	}
	gh, tr := resultFor(t, res, d.ghIt), resultFor(t, res, d.trIt)
	noProblem(t, gh, "deleted")
	noProblem(t, tr, "archived")
	if gh.Provider != "github" || tr.Provider != "trello" {
		t.Errorf("providers = %q and %q", gh.Provider, tr.Provider)
	}
	if issue, _ := d.gh.Issue(repo, n); !issue.Deleted {
		t.Errorf("issue = %+v, want it deleted", issue)
	}
	if c, _ := d.tr.Card(card.ID); !c.Closed || c.DueComplete {
		t.Errorf("card = %+v, want it archived with the deadline left alone", c)
	}
	if d.gh.Count("PATCH", "/repos/") != 0 {
		t.Error("a deleted issue must not also be patched")
	}
	if _, err := d.tasks.GetByID(tk.ID.String()); err == nil {
		t.Error("the task is still there")
	}

	for name, sync := range map[string]func(issuesync.Mode) *issuesync.Summary{"GitHub": d.syncGH, "Trello": d.syncTrello} {
		if sum := sync(issuesync.Full); sum.Created != 0 || sum.Updated != 0 || sum.Closed != 0 || sum.Errors != 0 {
			t.Errorf("%s: round after = %+v, want nothing (the task must not come back)", name, sum)
		}
	}
	if left := testListTasks(t, d); len(left) != 0 {
		t.Errorf("%d tasks after the rounds, want none", len(left))
	}
}

// Sem nenhuma caixa marcada só a tarefa sai: as plataformas não são tocadas, e o item solto não volta.
func TestRemove_NothingTickedLeavesThePlatformsAlone(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Só aqui")
	n, _ := d.issue(tk)
	card := d.card(tk)

	d.gh.Reset()
	d.tr.Reset()
	if res := d.deleteTask(tk); len(res) != 0 {
		t.Errorf("results = %+v, want none", res)
	}
	if d.gh.Writes() != 0 || d.tr.Writes() != 0 || len(d.gh.Requests()) != 0 || len(d.tr.Requests()) != 0 {
		t.Errorf("platforms were called: %v and %v", d.gh.Requests(), d.tr.Requests())
	}
	if issue, _ := d.gh.Issue(repo, n); issue.Deleted || issue.State != "open" {
		t.Errorf("issue = %+v, want it as it was", issue)
	}
	if c, _ := d.tr.Card(card.ID); c.Closed {
		t.Errorf("card = %+v, want it as it was", c)
	}
	if sum := d.syncGH(issuesync.Full); sum.Created != 0 {
		t.Errorf("the issue of a deleted task came back as %d task(s)", sum.Created)
	}
}

// Cada plataforma tem a sua caixa: marcar só uma não mexe na outra.
func TestRemove_OnlyTheTickedPlatform(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Metade")
	n, _ := d.issue(tk)
	card := d.card(tk)

	res := d.deleteTask(tk, d.trIt)
	if len(res) != 1 {
		t.Fatalf("results = %+v, want only the Trello one", res)
	}
	noProblem(t, resultFor(t, res, d.trIt), "archived")
	if c, _ := d.tr.Card(card.ID); !c.Closed {
		t.Errorf("card = %+v, want it archived", c)
	}
	if issue, _ := d.gh.Issue(repo, n); issue.Deleted || issue.State != "open" {
		t.Errorf("issue = %+v, want it untouched", issue)
	}
}

// Uma integração a que a tarefa não está ligada é ignorada, sem erro.
func TestRemove_IgnoresIntegrationsTheTaskIsNotIn(t *testing.T) {
	d := newDualEnv(t)
	tk, err := d.taskSvc.CreateAs("", d.project, "Só no GitHub", "", "", nil, task.Attrs{})
	must(t, err)
	_, err = d.syncer.Publish(context.Background(), tk.ID, d.ghIt.ID)
	must(t, err)
	d.tr.Reset()

	res := d.deleteTask(d.reload(tk), d.trIt, d.ghIt)
	if len(res) != 1 || resultFor(t, res, d.ghIt).Outcome != "deleted" {
		t.Errorf("results = %+v, want only the GitHub one, deleted", res)
	}
	if len(d.tr.Requests()) != 0 {
		t.Errorf("Trello was called for a task that is not in it: %v", d.tr.Requests())
	}
}

// A conta que não é admin do repositório não pode apagar a issue: ela é fechada como não planejada, e o resultado
// avisa. O cartão do Trello, na mesma exclusão, é arquivado do mesmo jeito.
func TestRemove_WithoutAdminTheIssueIsOnlyClosed(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Sem admin")
	n, _ := d.issue(tk)
	card := d.card(tk)
	d.gh.SetAdmin(repo, false)

	res := d.deleteTask(tk, d.ghIt, d.trIt)
	gh := resultFor(t, res, d.ghIt)
	if gh.Outcome != "closed" || gh.Problem == nil || gh.Problem.Code != issuesync.ErrRemoveOnlyClosed.Code {
		t.Errorf("GitHub result = %+v (problem %v), want closed with the only-closed warning", gh, gh.Problem)
	}
	if issue, _ := d.gh.Issue(repo, n); issue.Deleted || issue.State != "closed" || issue.StateReason != "not_planned" {
		t.Errorf("issue = %+v, want it closed as not planned and still there", issue)
	}
	noProblem(t, resultFor(t, res, d.trIt), "archived")
	if c, _ := d.tr.Card(card.ID); !c.Closed {
		t.Errorf("card = %+v, want it archived", c)
	}
	if left := testListTasks(t, d); len(left) != 0 {
		t.Errorf("%d tasks left, want none", len(left))
	}
}

// Um item que já não existe do outro lado não é falha: já está como se queria.
func TestRemove_AnItemThatIsAlreadyGone(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Já foi")
	n, _ := d.issue(tk)
	card := d.card(tk)
	d.gh.DeleteIssue(repo, n)
	d.tr.DeleteCard(card.ID)

	res := d.deleteTask(tk, d.ghIt, d.trIt)
	noProblem(t, resultFor(t, res, d.ghIt), "gone")
	noProblem(t, resultFor(t, res, d.trIt), "gone")
}

// Se uma plataforma falha, a tarefa sai mesmo assim, o resultado diz o motivo, e a outra plataforma é tratada.
func TestRemove_AFailureDoesNotStopTheOtherNorKeepTheTask(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Falha parcial")
	n, _ := d.issue(tk)
	card := d.card(tk)
	d.tr.Fail("PUT", "/cards/", 500, 1)

	res := d.deleteTask(tk, d.ghIt, d.trIt)
	noProblem(t, resultFor(t, res, d.ghIt), "deleted")
	tr := resultFor(t, res, d.trIt)
	if tr.Outcome != "" || tr.Problem == nil || tr.Problem.Code != adapter.ErrProviderStatus.Code {
		t.Errorf("Trello result = %+v (problem %v), want a provider-status problem and no outcome", tr, tr.Problem)
	}
	if issue, _ := d.gh.Issue(repo, n); !issue.Deleted {
		t.Errorf("issue = %+v, want it deleted despite the Trello failure", issue)
	}
	if c, _ := d.tr.Card(card.ID); c.Closed {
		t.Errorf("card = %+v, want it as it was (the PUT failed)", c)
	}
	if _, err := d.tasks.GetByID(tk.ID.String()); err == nil {
		t.Error("the task is still there after a failure on a platform")
	}
	// O cartão que ficou é item descartado: a rodada seguinte não o traz de volta como tarefa.
	if sum := d.syncTrello(issuesync.Full); sum.Created != 0 {
		t.Errorf("the card that stayed came back as %d task(s)", sum.Created)
	}
}

// Token que não escreve, integração com a sincronização desligada ou token recusado: o item fica, o motivo vai no
// resultado, a tarefa sai.
func TestRemove_RefusalsAreReportedPerPlatform(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Recusas")
	n, _ := d.issue(tk)
	card := d.card(tk)

	// GitHub sem escrita: o item continua lá.
	d.gh.SetPush(repo, false)
	// Trello com a sincronização desligada.
	no := false
	_, err := d.integ.Edit(d.trIt.ID.String(), integration.EditInput{SyncIssues: &no})
	must(t, err)

	res := d.deleteTask(tk, d.ghIt, d.trIt)
	gh, tr := resultFor(t, res, d.ghIt), resultFor(t, res, d.trIt)
	if gh.Outcome != "" || gh.Problem == nil || gh.Problem.Code != issuesync.ErrRemoveReadOnly.Code {
		t.Errorf("GitHub result = %+v (problem %v), want the read-only refusal", gh, gh.Problem)
	}
	if tr.Outcome != "" || tr.Problem == nil || tr.Problem.Code != issuesync.ErrSyncOff.Code {
		t.Errorf("Trello result = %+v (problem %v), want sync off", tr, tr.Problem)
	}
	if issue, _ := d.gh.Issue(repo, n); issue.Deleted || issue.State != "open" {
		t.Errorf("issue = %+v, want it untouched", issue)
	}
	if c, _ := d.tr.Card(card.ID); c.Closed {
		t.Errorf("card = %+v, want it untouched", c)
	}
	if _, err := d.tasks.GetByID(tk.ID.String()); err == nil {
		t.Error("the task is still there")
	}

	// Token recusado pelo GitHub: um erro da plataforma, com o nome dela.
	d2 := newDualEnv(t)
	tk2 := d2.posted("Token")
	d2.gh.Fail("GET", "/repos/"+repo, 401, 1)
	res = d2.deleteTask(tk2, d2.ghIt)
	if p := resultFor(t, res, d2.ghIt).Problem; p == nil || p.Code != adapter.ErrInvalidToken.Code || p.Params["provider"] != "GitHub" {
		t.Errorf("refused token: problem = %v, want invalid token naming GitHub", p)
	}
}

// Quem não tem Remover (o servidor sem sincronizador) só exclui a tarefa, mesmo com as caixas marcadas.
func TestRemove_WithoutARemoverOnlyTheTaskGoes(t *testing.T) {
	d := newDualEnv(t)
	tk := d.posted("Sem sincronizador")
	n, _ := d.issue(tk)
	d.taskSvc.SetRemover(nil)

	res, err := d.taskSvc.Delete(context.Background(), tk.ID.String(), []uuid.UUID{d.ghIt.ID})
	must(t, err)
	if len(res) != 0 {
		t.Errorf("results = %+v, want none", res)
	}
	if issue, _ := d.gh.Issue(repo, n); issue.Deleted {
		t.Error("the issue was deleted with no remover")
	}
}
