package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

// trelloSyncServer sobe o servidor ligado a um Trello fake de quadro vazio (o TrelloBoardID, com uma lista), sem
// os cartões de sempre.
func trelloSyncServer(t *testing.T) (*server.App, *testutil.Trello) {
	t.Helper()
	fake := testutil.NewTrello()
	fake.DeleteCard("H0TZyzbK")
	fake.DeleteCard("ArqUiv4d")
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	adapter.Register("trello", func() adapter.Integration { return &adapter.TrelloIntegration{BaseURL: srv.URL} })
	t.Cleanup(func() {
		adapter.Register("trello", func() adapter.Integration { return &adapter.TrelloIntegration{} })
	})

	testutil.Truncate(t, testDB)
	app, err := server.Build(testClient, server.Options{EncryptKey: "test-key", AuthRateLimit: 1000})
	if err != nil {
		t.Fatalf("server.Build: %v", err)
	}
	return app, fake
}

// connectTrelloBoard cria a integração do Trello no projeto, com a sincronização ligada, e devolve o id dela.
func connectTrelloBoard(t *testing.T, app *server.App, session, prj string) string {
	t.Helper()
	e := app.Echo
	rec := do(e, "POST", "/api/projects/"+prj+"/integrations",
		`{"type":"trello","display_name":"Quadro","token":"tok","metadata":{"api_key":"`+testutil.TrelloKey+`","board_id":"`+testutil.TrelloBoardID+`"}}`, session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create the Trello integration = %d: %s", rec.Code, rec.Body.String())
	}
	id := decode(t, rec)["id"].(string)
	if rec = do(e, "PATCH", "/api/integrations/"+id, `{"sync_issues":true}`, session); rec.Code != http.StatusOK {
		t.Fatalf("turn the sync on = %d: %s", rec.Code, rec.Body.String())
	}
	return id
}

// A tarefa criada pela API vira um cartão sozinha, no worker; o que se faz nela depois (editar, fechar) vai para o
// cartão, e o que se faz no cartão (criar, arquivar) vem para cá pelo botão.
func TestTrelloSync_EndToEnd(t *testing.T) {
	app, fake := trelloSyncServer(t)
	e := app.Echo
	ctx := context.Background()
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	integ := connectTrelloBoard(t, app, admin.session, prj)
	fake.AddLabel(testutil.TrelloBoardID, "Bug", "red")

	labelID := decode(t, do(e, "POST", "/api/projects/"+prj+"/labels", `{"name":"Bug"}`, admin.session))["id"].(string)
	rec := do(e, "POST", "/api/projects/"+prj+"/tasks",
		`{"name":"Tarefa daqui","description":"descrição","deadline":"2026-11-03T20:59:00Z","label_ids":["`+labelID+`"]}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create the task = %d: %s", rec.Code, rec.Body.String())
	}
	taskID := decode(t, rec)["id"].(string)
	if fake.Writes() != 0 {
		t.Fatal("creating the task waited for Trello")
	}

	// O Flush posta a tarefa nova e ainda dá o passe sem diferença que o vínculo recém-feito agenda (2 itens).
	if got := app.Sync.Flush(ctx); got < 1 {
		t.Errorf("Flush touched %d, want at least the new task", got)
	}
	cards := fake.Cards(testutil.TrelloBoardID)
	if len(cards) != 1 || cards[0].Name != "Tarefa daqui" || cards[0].Desc != "descrição" || len(cards[0].Labels) != 1 ||
		!cards[0].Due.Equal(time.Date(2026, 11, 3, 20, 59, 0, 0, time.UTC)) {
		t.Fatalf("cards = %+v, want the posted task", cards)
	}
	card := cards[0]
	task := decode(t, do(e, "GET", "/api/tasks/"+taskID, "", admin.session))
	links := linksOf(task)
	if len(links) != 1 || links[0]["item_id"] != card.ShortLink {
		t.Fatalf("task = %v, want it linked to the card %s", task, card.ShortLink)
	}
	if integ, _ := links[0]["integration"].(map[string]any); integ["type"] != "trello" {
		t.Errorf("link = %v, want the Trello integration", links[0])
	}

	// Editar e fechar a tarefa levam o nome e o fechamento para o cartão, pelo gancho.
	if rec = do(e, "PATCH", "/api/tasks/"+taskID, `{"name":"Tarefa editada","description":"descrição","deadline":null}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("edit the task = %d: %s", rec.Code, rec.Body.String())
	}
	if rec = do(e, "PATCH", "/api/tasks/"+taskID+"/attributes", `{"status":"closed"}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("close the task = %d: %s", rec.Code, rec.Body.String())
	}
	app.Sync.Flush(ctx)
	if got, _ := fake.Card(card.ID); got.Name != "Tarefa editada" || !got.Closed || !got.DueComplete {
		t.Errorf("card after the edit and the close = %+v", got)
	}

	// Um cartão novo no Trello vira tarefa pelo botão, e o cartão arquivado fecha a tarefa.
	fake.AddCard(testutil.TrelloCard{ShortLink: "NovoCard", Name: "Veio do Trello", Desc: "texto"})
	rec = do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session)
	sum := decode(t, rec)
	if rec.Code != http.StatusOK || sum["created"] != 1.0 {
		t.Fatalf("sync = %d %v, want the new card imported", rec.Code, sum)
	}
	tasks := decodeList(t, do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session))
	if len(tasks) != 2 {
		t.Fatalf("%d tasks, want the posted one and the imported one", len(tasks))
	}
	fake.Reset()
	if rec = do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session); decode(t, rec)["pushed"] != 0.0 || fake.Writes() != 0 {
		t.Errorf("a settled round wrote to Trello: %v", fake.Requests())
	}
	fake.EditCard("NovoCard", func(c *testutil.TrelloCard) { c.Closed = true })
	do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session)
	for _, tk := range decodeList(t, do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session)) {
		if linkedItem(tk, "NovoCard") && tk["status"] != "closed" {
			t.Errorf("the archived card left its task %v", tk["status"])
		}
	}
}

// Uma integração do GitHub no mesmo projeto não muda isso: a tarefa que a etapa Integrações posta no GitHub já
// está ligada, e a postagem automática do Trello não a pega; e o Trello fora do ar não impede a tarefa de ser criada.
func TestTrelloSync_GitHubChoiceWinsAndTrelloDownDoesNotBlock(t *testing.T) {
	app, trello := trelloSyncServer(t)
	e := app.Echo
	ctx := context.Background()
	gh := testutil.NewGitHub()
	gh.AddRepo("owner/repo")
	srv := httptest.NewServer(gh)
	t.Cleanup(srv.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: srv.URL} })
	t.Cleanup(func() { adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{} }) })

	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	connectTrelloBoard(t, app, admin.session, prj)
	rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create the GitHub integration = %d: %s", rec.Code, rec.Body.String())
	}
	ghID := decode(t, rec)["id"].(string)
	do(e, "PATCH", "/api/integrations/"+ghID, `{"sync_issues":true}`, admin.session)

	// A tela cria a tarefa e posta no GitHub logo em seguida; o worker chega depois e vê a tarefa ligada.
	rec = do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Para o GitHub"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)
	if rec = do(e, "POST", "/api/tasks/"+taskID+"/publish", `{"integration_id":"`+ghID+`"}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("publish to GitHub = %d: %s", rec.Code, rec.Body.String())
	}
	app.Sync.Flush(ctx)
	if n := len(trello.Cards(testutil.TrelloBoardID)); n != 0 {
		t.Errorf("the task that went to GitHub also became %d card(s)", n)
	}
	if task := decode(t, do(e, "GET", "/api/tasks/"+taskID, "", admin.session)); !linkedItem(task, "1") {
		t.Errorf("task = %v, want it linked to the GitHub issue", task)
	}

	// Com o Trello fora do ar a tarefa é criada do mesmo jeito, sem cartão, e o aviso fica na integração.
	trello.RateLimitedFor(time.Hour)
	rec = do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Com o Trello fora"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with Trello down = %d: %s", rec.Code, rec.Body.String())
	}
	app.Sync.Flush(ctx)
	if n := len(trello.Cards(testutil.TrelloBoardID)); n != 0 {
		t.Errorf("%d cards while Trello was refusing", n)
	}
	for _, it := range integrationsOf(t, e, admin.session, prj) {
		if it["type"] == "trello" && it["last_sync_error"] != "integration.rate_limited" {
			t.Errorf("the Trello integration says %q, want the rate limit warning", it["last_sync_error"])
		}
	}
}
