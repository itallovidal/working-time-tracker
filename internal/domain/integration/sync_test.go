package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"working-time-tracker/ent/issuesync"
	"working-time-tracker/internal/domain/integration"
)

func yes() *bool { v := true; return &v }
func no() *bool  { v := false; return &v }

// githubReady cria uma integração do GitHub ativa, com o repositório, pronta para ligar a sincronização.
func githubReady(t *testing.T) (*integration.Service, string, *integration.Integration) {
	t.Helper()
	svc, projectID := setup(t)
	it, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, projectID, it
}

// A sincronização nasce desligada e liga pela edição.
func TestService_SyncIssues_TurnOnAndOff(t *testing.T) {
	svc, _, it := githubReady(t)
	id := it.ID.String()
	if it.SyncIssues || it.LastSyncedAt != nil || it.LastSyncError != "" {
		t.Fatalf("a new integration must have the sync off: %+v", it)
	}

	on, err := svc.Edit(id, integration.EditInput{SyncIssues: yes()})
	if err != nil || !on.SyncIssues {
		t.Fatalf("turn on: %+v, %v", on, err)
	}
	// Um Update comum (o do resto da tela) não desliga a sincronização.
	if kept, err := svc.Update(id, "Outro nome", "", nil, nil); err != nil || !kept.SyncIssues {
		t.Errorf("renaming turned the sync off: %+v, %v", kept, err)
	}
	if listed, _ := svc.ListByProject(it.ProjectID.String()); len(listed) != 1 || !listed[0].SyncIssues {
		t.Errorf("list = %+v", listed)
	}

	off, err := svc.Edit(id, integration.EditInput{SyncIssues: no()})
	if err != nil || off.SyncIssues {
		t.Fatalf("turn off: %+v, %v", off, err)
	}
}

func TestService_SyncIssues_Requirements(t *testing.T) {
	svc, projectID := setup(t)

	// GitLab e Trello não leem nem escrevem issues desta forma.
	for typ, meta := range map[string]map[string]interface{}{
		"gitlab": {"project_url": "group/project"},
	} {
		it, err := svc.Create(projectID, typ, typ, "glpat-test", meta, true)
		if err != nil {
			t.Fatalf("create %s: %v", typ, err)
		}
		if _, err := svc.Edit(it.ID.String(), integration.EditInput{SyncIssues: yes()}); !errors.Is(err, integration.ErrSyncUnsupported) {
			t.Errorf("%s: err = %v, want sync_unsupported", typ, err)
		}
	}

	// Uma conexão nova ainda não tem repositório: não há o que sincronizar.
	connected, err := svc.Connect(projectID, "github", "gho_oauth_token")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := svc.Edit(connected.ID.String(), integration.EditInput{Enabled: yes(), SyncIssues: yes()}); !errors.Is(err, integration.ErrSyncNeedsRepo) {
		t.Errorf("without a repository: err = %v, want sync_needs_repo", err)
	}
	// Escolher o repositório, ativar e ligar de uma vez só (o que a tela faz na volta da conexão).
	done, err := svc.Edit(connected.ID.String(), integration.EditInput{Metadata: githubMetadata(), Enabled: yes(), SyncIssues: yes()})
	if err != nil || !done.SyncIssues || !done.Enabled {
		t.Fatalf("finish the connection with the sync on: %+v, %v", done, err)
	}

	// Desativada, não liga.
	idle, _ := svc.Create(projectID, "github", "Parada", "ghp_test", githubMetadata(), false)
	if _, err := svc.Edit(idle.ID.String(), integration.EditInput{SyncIssues: yes()}); !errors.Is(err, integration.ErrSyncNeedsEnabled) {
		t.Errorf("disabled: err = %v, want sync_needs_enabled", err)
	}
	// Sem credencial (uma integração de antes), também não.
	bare := testClient.Integration.Create().SetProjectID(idle.ProjectID).SetType("github").SetDisplayName("Sem token").
		SetMetadata(githubMetadata()).SaveX(context.Background())
	if _, err := svc.Edit(bare.ID.String(), integration.EditInput{SyncIssues: yes()}); !errors.Is(err, integration.ErrNoCredential) {
		t.Errorf("no credential: err = %v, want no_credential", err)
	}
	// E nenhuma recusa deixa a sincronização ligada pela metade.
	if got, _ := svc.Get(idle.ID.String()); got.SyncIssues {
		t.Error("a refused edit left the sync on")
	}
}

// Com a sincronização ligada o repositório não troca; desligada, troca e o vínculo das issues do
// repositório antigo é esquecido.
func TestService_SyncIssues_RepositoryLock(t *testing.T) {
	svc, _, it := githubReady(t)
	id := it.ID.String()
	other := map[string]interface{}{"repo": "owner/other"}
	if _, err := svc.Edit(id, integration.EditInput{SyncIssues: yes()}); err != nil {
		t.Fatalf("turn on: %v", err)
	}
	cursor := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	uid := it.ID
	if err := svc.RecordSync(uid, integration.SyncResult{At: time.Now(), Cursor: &cursor}); err != nil {
		t.Fatalf("record: %v", err)
	}
	testClient.IssueSync.Create().SetIntegrationID(uid).SetIssueNumber(7).SaveX(context.Background())

	if _, err := svc.Edit(id, integration.EditInput{Metadata: other}); !errors.Is(err, integration.ErrSyncRepoLocked) {
		t.Fatalf("changing the repository with the sync on: err = %v, want sync_repo_locked", err)
	}
	if _, err := svc.Update(id, "", "", other, nil); !errors.Is(err, integration.ErrSyncRepoLocked) {
		t.Errorf("Update must refuse it too: %v", err)
	}
	if got, _ := svc.Get(id); got.Metadata["repo"] != "owner/repo" {
		t.Errorf("a refused change moved the repository to %v", got.Metadata["repo"])
	}
	// Mandar o mesmo repositório, ou trocar só o token, não é trocar.
	if _, err := svc.Edit(id, integration.EditInput{Metadata: githubMetadata(), Token: "ghp_new"}); err != nil {
		t.Errorf("same repository with a new token: %v", err)
	}
	if testClient.IssueSync.Query().CountX(context.Background()) != 1 {
		t.Error("an edit that kept the repository dropped the links")
	}

	// Desligar e trocar na mesma edição vale, e esquece o vínculo e o cursor.
	moved, err := svc.Edit(id, integration.EditInput{Metadata: other, SyncIssues: no()})
	if err != nil || moved.SyncIssues || moved.Metadata["repo"] != "owner/other" {
		t.Fatalf("turn off and move: %+v, %v", moved, err)
	}
	if n := testClient.IssueSync.Query().CountX(context.Background()); n != 0 {
		t.Errorf("%d link(s) of the old repository survived", n)
	}
	row := testClient.Integration.GetX(context.Background(), uid)
	if row.SyncCursor != nil || row.LastSyncedAt != nil {
		t.Errorf("the old cursor survived: %v / %v", row.SyncCursor, row.LastSyncedAt)
	}
}

func TestService_Connection(t *testing.T) {
	svc, _, it := githubReady(t)

	conn, got, err := svc.Connection(it.ID.String())
	if err != nil || conn.Token != "ghp_test" || conn.Metadata["repo"] != "owner/repo" || got.Credentials != nil {
		t.Fatalf("connection = %+v, %+v, %v", conn, got, err)
	}
	bare := testClient.Integration.Create().SetProjectID(it.ProjectID).SetType("github").SetDisplayName("Sem token").SaveX(context.Background())
	if _, _, err := svc.Connection(bare.ID.String()); !errors.Is(err, integration.ErrNoCredential) {
		t.Errorf("connection without a credential: err = %v", err)
	}
}

func TestService_ListSyncing(t *testing.T) {
	svc, projectID, a := githubReady(t)
	b, _ := svc.Create(projectID, "github", "B", "ghp_test", githubMetadata(), true)
	off, _ := svc.Create(projectID, "github", "Desligada", "ghp_test", githubMetadata(), true)
	svc.Edit(a.ID.String(), integration.EditInput{SyncIssues: yes()})
	svc.Edit(b.ID.String(), integration.EditInput{SyncIssues: yes()})
	svc.Edit(b.ID.String(), integration.EditInput{Enabled: no()}) // desativada: a rodada a pula
	_ = off

	list, err := svc.ListSyncing()
	if err != nil || len(list) != 1 || list[0].ID != a.ID || list[0].Credentials != nil {
		t.Errorf("syncing = %+v, %v, want only the active one with the sync on", list, err)
	}
}

// A rodada deixa o resultado na integração, e uma edição não o desfaz.
func TestService_RecordSync(t *testing.T) {
	svc, _, it := githubReady(t)
	id := it.ID.String()
	svc.Edit(id, integration.EditInput{SyncIssues: yes()})

	at := time.Now().UTC().Truncate(time.Second)
	cursor := at.Add(-2 * time.Minute)
	if err := svc.RecordSync(it.ID, integration.SyncResult{At: at, Cursor: &cursor}); err != nil {
		t.Fatalf("record: %v", err)
	}
	// Uma rodada que parou no meio não avança o cursor, mas deixa o erro.
	if err := svc.RecordSync(it.ID, integration.SyncResult{At: at.Add(time.Minute), Err: "integration.rate_limited"}); err != nil {
		t.Fatalf("record error: %v", err)
	}
	got, _ := svc.Get(id)
	if got.LastSyncError != "integration.rate_limited" || got.LastSyncedAt == nil || !got.LastSyncedAt.Equal(at.Add(time.Minute)) {
		t.Errorf("after the failed round = %+v", got)
	}
	if row := testClient.Integration.GetX(context.Background(), it.ID); row.SyncCursor == nil || !row.SyncCursor.Equal(cursor) {
		t.Errorf("a round that stopped moved the cursor to %v, want %v", row.SyncCursor, cursor)
	}

	// Editar a integração (nome, token) não toca no resultado da rodada.
	if _, err := svc.Update(id, "Novo nome", "ghp_new", nil, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	if row := testClient.Integration.GetX(context.Background(), it.ID); row.SyncCursor == nil || !row.SyncCursor.Equal(cursor) || row.LastSyncError != "integration.rate_limited" {
		t.Errorf("an edit changed the sync result: cursor %v, error %q", row.SyncCursor, row.LastSyncError)
	}
	svc.RecordSync(it.ID, integration.SyncResult{At: at.Add(2 * time.Minute)})
	if got, _ = svc.Get(id); got.LastSyncError != "" {
		t.Errorf("a good round must clear the error, got %q", got.LastSyncError)
	}
}

// "N responsáveis sem correspondência": issues abertas, com tarefa, que têm responsável no GitHub e
// nenhum login ligado a alguém daqui.
func TestService_SyncUnmatched(t *testing.T) {
	svc, _, it := githubReady(t)
	svc.Edit(it.ID.String(), integration.EditInput{SyncIssues: yes()})
	ctx := context.Background()
	mk := func(n int, state issuesync.State, logins []string, mapped string, withTask bool) {
		q := testClient.IssueSync.Create().SetIntegrationID(it.ID).SetIssueNumber(n).SetState(state).
			SetAssigneeLogins(logins).SetMappedLogin(mapped)
		if withTask {
			tk := testClient.Task.Create().SetProjectID(it.ProjectID).SetName("t").SaveX(ctx)
			q = q.SetTaskID(tk.ID)
		}
		q.SaveX(ctx)
	}
	mk(1, issuesync.StateOpen, []string{"bob"}, "", true)           // conta: responsável sem correspondência
	mk(2, issuesync.StateOpen, []string{"bob", "ana"}, "ana", true) // não: um dos logins se liga a alguém
	mk(3, issuesync.StateOpen, nil, "", true)                       // não: sem responsável no GitHub
	mk(4, issuesync.StateClosed, []string{"bob"}, "", true)         // não: fechada
	mk(5, issuesync.StateOpen, []string{"bob"}, "", false)          // não: descartada, sem tarefa
	mk(6, issuesync.StateOpen, []string{"carol", "dave"}, "", true) // conta uma vez, não por login

	got, err := svc.Get(it.ID.String())
	if err != nil || got.SyncUnmatched != 2 {
		t.Errorf("unmatched = %d, %v, want 2", got.SyncUnmatched, err)
	}
	// Com a sincronização desligada a contagem não é calculada.
	svc.Edit(it.ID.String(), integration.EditInput{SyncIssues: no()})
	if got, _ = svc.Get(it.ID.String()); got.SyncUnmatched != 0 {
		t.Errorf("unmatched with the sync off = %d, want 0", got.SyncUnmatched)
	}
}
