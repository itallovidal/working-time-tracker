package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

const (
	trelloCallback = "/integrations/trello/callback"
	trelloTokenURL = "/integrations/trello/token"
)

// trelloServer sobe o servidor com a autorização do Trello apontada para o Trello fake: o mesmo endereço serve o
// site (a tela de autorização) e a API. configured falso é o servidor sem a chave do app.
func trelloServer(t *testing.T, configured bool) (*echo.Echo, *testutil.Trello) {
	t.Helper()
	fake := testutil.NewTrello()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	adapter.Register("trello", func() adapter.Integration { return &adapter.TrelloIntegration{BaseURL: srv.URL} })
	t.Cleanup(func() {
		adapter.Register("trello", func() adapter.Integration { return &adapter.TrelloIntegration{} })
	})

	testutil.Truncate(t, testDB)
	opts := server.Options{EncryptKey: "test-key", AuthRateLimit: 1000}
	if configured {
		opts.TrelloAuth = &adapter.TrelloAuth{
			APIKey: testutil.TrelloKey, AppName: "WTT Test", RedirectURL: "http://wtt.test" + trelloCallback, SiteURL: srv.URL,
		}
	}
	e, err := server.New(testClient, opts)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e, fake
}

// startTrelloConnect é o clique em Conectar com o Trello: devolve o cookie da conexão, o state e o endereço da
// autorização para onde a pessoa foi.
func startTrelloConnect(t *testing.T, e *echo.Echo, session, projectID, query string) (*http.Cookie, string, *url.URL) {
	t.Helper()
	rec := doCookies(e, "/projects/"+projectID+"/management/integrations/trello/connect"+query, sessionCookie(session))
	if rec.Code != http.StatusFound {
		t.Fatalf("connect = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	cookie := setCookie(rec, oauthCookieName)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("connect did not set the connection cookie")
	}
	to, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("connect redirected to %q", rec.Header().Get("Location"))
	}
	back, err := url.Parse(to.Query().Get("return_url"))
	if err != nil || back.Query().Get("state") == "" {
		t.Fatalf("the authorization URL %s has no return_url carrying a state", to)
	}
	return cookie, back.Query().Get("state"), to
}

// authorizeOnTrello é a pessoa clicando em Permitir no Trello: segue a ida até o Trello fake e devolve o que
// ele põe no fragmento do endereço de volta.
func authorizeOnTrello(t *testing.T, to *url.URL) (state, token string) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// O endereço da ida é do servidor de teste do Trello fake.
	res, err := client.Get(to.String())
	if err != nil {
		t.Fatalf("authorize on Trello: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("Trello authorization = %d, want a redirect back", res.StatusCode)
	}
	back, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatalf("Trello sent the person to %q", res.Header.Get("Location"))
	}
	frag, _ := url.ParseQuery(back.Fragment)
	return back.Query().Get("state"), frag.Get("token")
}

// postTrelloToken é a página de retorno entregando o token: um POST JSON com os cookies que o navegador mandaria.
func postTrelloToken(e *echo.Echo, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", trelloTokenURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func tokenBody(state, token string) string {
	raw, _ := json.Marshal(map[string]string{"state": state, "token": token})
	return string(raw)
}

// redirectOf lê o endereço para onde a resposta manda a página de retorno.
func redirectOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("token endpoint = %d, want 200 with a redirect: %s", rec.Code, rec.Body.String())
	}
	var out struct{ Redirect string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("token endpoint answered %q", rec.Body.String())
	}
	return out.Redirect
}

// connectTrello faz o caminho inteiro e devolve o id da integração criada.
func connectTrello(t *testing.T, e *echo.Echo, session, prj, query string) string {
	t.Helper()
	cookie, _, to := startTrelloConnect(t, e, session, prj, query)
	state, token := authorizeOnTrello(t, to)
	redirect := redirectOf(t, postTrelloToken(e, tokenBody(state, token), sessionCookie(session), cookie))
	_, id, ok := strings.Cut(redirect, "?trello=")
	if !ok {
		t.Fatalf("the connection ended at %q, want the tab with ?trello=<id>", redirect)
	}
	return id
}

// O caminho feliz, de ponta a ponta: o clique leva ao Trello com o que ele precisa, o token volta no
// fragmento, a página o entrega e o servidor guarda uma integração desativada e sem quadro; a pessoa a
// termina escolhendo um quadro da lista.
func TestTrelloOAuth_ConnectAndPickTheBoard(t *testing.T) {
	e, _ := trelloServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	cookie, state, to := startTrelloConnect(t, e, admin.session, prj, "")
	q := to.Query()
	if to.Path != "/1/authorize" || q.Get("key") != testutil.TrelloKey || q.Get("name") != "WTT Test" || q.Get("scope") != "read,write" ||
		q.Get("expiration") != "never" || q.Get("response_type") != "token" || q.Get("callback_method") != "fragment" {
		t.Errorf("authorization URL = %s", to)
	}
	if q.Get("return_url") != "http://wtt.test"+trelloCallback+"?state="+url.QueryEscape(state) {
		t.Errorf("return_url = %q, want the callback carrying the state", q.Get("return_url"))
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/integrations/trello" || cookie.MaxAge <= 0 {
		t.Errorf("connection cookie = %+v, want HttpOnly, SameSite=Lax, limited to the Trello return path and with a lifetime", cookie)
	}
	if strings.Contains(cookie.Value, state) {
		t.Error("the state is readable in the cookie: it must be sealed")
	}

	// A volta do Trello é uma página; o token está no fragmento e não passa pelo servidor.
	gotState, token := authorizeOnTrello(t, to)
	if gotState != state || token != testutil.TrelloOAuthToken {
		t.Fatalf("Trello returned state %q and token %q", gotState, token)
	}
	page := doCookies(e, trelloCallback+"?state="+url.QueryEscape(state), sessionCookie(admin.session))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "/static/pages/trello_callback.js") ||
		page.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(page.Header().Get("Cache-Control"), "no-store") {
		t.Errorf("callback page = %d, referrer %q, cache %q", page.Code, page.Header().Get("Referrer-Policy"), page.Header().Get("Cache-Control"))
	}

	rec := postTrelloToken(e, tokenBody(state, token), sessionCookie(admin.session), cookie)
	list := integrationsOf(t, e, admin.session, prj)
	if len(list) != 1 {
		t.Fatalf("the connection left %d integration(s), want 1", len(list))
	}
	it := list[0]
	id := it["id"].(string)
	if got := redirectOf(t, rec); got != "/projects/"+prj+"/management/integrations?trello="+id {
		t.Errorf("redirect = %q, want the tab with ?trello=<id>", got)
	}
	if cleared := setCookie(rec, oauthCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Errorf("the connection cookie was not cleared: %+v", cleared)
	}
	meta := it["metadata"].(map[string]any)
	if it["type"] != "trello" || it["enabled"] != false || it["has_token"] != true || it["display_name"] != "Trello · @"+testutil.TrelloUsername ||
		len(meta) != 1 || meta["api_key"] != testutil.TrelloKey {
		t.Errorf("connected integration = %v, want a disabled one with the app key and no board", it)
	}
	if body := do(e, "GET", "/api/integrations/"+id, "", admin.session).Body.String(); strings.Contains(body, testutil.TrelloOAuthToken) {
		t.Errorf("the API leaks the Trello token: %s", body)
	}

	// A lista que a tela oferece vem do servidor, que usa o token guardado: os quadros com o espaço de trabalho.
	rec = do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session)
	boards := decodeList(t, rec)
	if rec.Code != http.StatusOK || len(boards) != 2 || boards[0]["id"] != testutil.TrelloBoardID || boards[0]["full_name"] != "Acme / App" {
		t.Errorf("boards = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), testutil.TrelloOAuthToken) {
		t.Error("the boards answer leaks the Trello token")
	}

	// Escolher o quadro ativa a integração, validada com o token guardado. A chave do app não muda: quem edita
	// manda outra, e o servidor segue com a guardada.
	rec = do(e, "PATCH", "/api/integrations/"+id, `{"display_name":"App","enabled":true,"metadata":{"api_key":"outrachave","board_id":"`+testutil.TrelloBoardID+`","board_name":"Acme / App"}}`, admin.session)
	got := decode(t, rec)
	meta, _ = got["metadata"].(map[string]any)
	if rec.Code != http.StatusOK || got["enabled"] != true || got["display_name"] != "App" || meta["board_id"] != testutil.TrelloBoardID ||
		meta["api_key"] != testutil.TrelloKey || meta["board_name"] != "Acme / App" {
		t.Errorf("pick the board = %d %s", rec.Code, rec.Body.String())
	}
	// O nome do quadro é do quadro escolhido: trocar o quadro sem mandar outro nome apaga o antigo.
	rec = do(e, "PATCH", "/api/integrations/"+id, `{"metadata":{"board_id":"`+testutil.TrelloOtherBoardID+`","board_name":"Acme / App"}}`, admin.session)
	got = decode(t, rec)
	meta, _ = got["metadata"].(map[string]any)
	if rec.Code != http.StatusOK || meta["board_id"] != testutil.TrelloOtherBoardID || hasKey(meta, "board_name") {
		t.Errorf("changing the board kept the old name: %d %s", rec.Code, rec.Body.String())
	}
	do(e, "PATCH", "/api/integrations/"+id, `{"metadata":{"board_id":"`+testutil.TrelloBoardID+`","board_name":"Acme / App"}}`, admin.session)

	// Com a sincronização ligada o quadro não troca.
	if rec = do(e, "PATCH", "/api/integrations/"+id, `{"sync_issues":true}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("turn the sync on = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "PATCH", "/api/integrations/"+id, `{"metadata":{"board_id":"`+testutil.TrelloOtherBoardID+`"}}`, admin.session)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "integration.sync_repo_locked") {
		t.Errorf("changing the board with the sync on = %d %s, want it locked", rec.Code, rec.Body.String())
	}

	// A aba abre com o que o JavaScript precisa: o Trello se autoriza, está configurado, esconde a chave e
	// escolhe o quadro numa lista.
	tab := do(e, "GET", "/projects/"+prj+"/management/integrations?trello="+id, "", admin.session)
	body := tab.Body.String()
	i := strings.Index(body, `"type":"trello"`)
	if tab.Code != http.StatusOK || i < 0 {
		t.Fatalf("the tab = %d, want 200 with the trello type", tab.Code)
	}
	for _, want := range []string{`"auth":"oauth"`, `"configured":true`, `"internal":true`, `"picker":"select"`, `"coming_soon":false`} {
		if !strings.Contains(body[i:], want) {
			t.Errorf("the trello type lacks %s", want)
		}
	}
}

// Tudo o que não bate com a conexão que o servidor começou volta para a aba com o motivo, e nada é guardado.
// Sem conexão em andamento, de outra plataforma ou de outra pessoa, não há para onde voltar.
func TestTrelloOAuth_TokenRefusals(t *testing.T) {
	e, _ := trelloServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	back := "/projects/" + prj + "/management/integrations?trello_error="
	session := sessionCookie(admin.session)

	cases := []struct {
		name string
		body func(state string) string
		want string
	}{
		{"another state", func(string) string { return tokenBody("forged", testutil.TrelloOAuthToken) }, back + "integration.trello_oauth_state"},
		{"no state", func(string) string { return tokenBody("", testutil.TrelloOAuthToken) }, back + "integration.trello_oauth_state"},
		{"a body that is not JSON", func(string) string { return "not json" }, back + "integration.trello_oauth_state"},
		{"denied on Trello (no token)", func(s string) string { return tokenBody(s, "") }, back + "integration.trello_oauth_denied"},
		{"a token Trello rejects", func(s string) string { return tokenBody(s, testutil.InvalidToken) }, back + "integration.invalid_token"},
		{"a token that breaks the header", func(s string) string { return tokenBody(s, `x", y="z`) }, back + "integration.invalid_token"},
	}
	for _, tc := range cases {
		cookie, state, _ := startTrelloConnect(t, e, admin.session, prj, "")
		rec := postTrelloToken(e, tc.body(state), session, cookie)
		if got := redirectOf(t, rec); got != tc.want {
			t.Errorf("%s: redirect %q, want %q", tc.name, got, tc.want)
		}
		if cleared := setCookie(rec, oauthCookieName); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("%s: the connection cookie survived", tc.name)
		}
	}

	// Sem uma conexão em andamento (cookie ausente, adulterado, vencido ou de outra plataforma) não há projeto
	// para onde voltar: vai para o início, e o token que veio não é guardado.
	cookie, state, _ := startTrelloConnect(t, e, admin.session, prj, "")
	valid := tokenBody(state, testutil.TrelloOAuthToken)
	expired, err := adapter.EncryptConfig(map[string]interface{}{
		"type": "trello", "state": state, "person_id": admin.id, "project_id": prj, "integration_id": "",
		"expires": time.Now().Add(-time.Minute).Unix(),
	}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	github, err := adapter.EncryptConfig(map[string]interface{}{
		"type": "github", "state": state, "person_id": admin.id, "project_id": prj, "integration_id": "",
		"expires": time.Now().Add(time.Minute).Unix(),
	}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for name, cookies := range map[string][]*http.Cookie{
		"no cookie":       {session},
		"tampered":        {session, {Name: oauthCookieName, Value: cookie.Value[:len(cookie.Value)-4] + "AAAA"}},
		"garbage":         {session, {Name: oauthCookieName, Value: "nope"}},
		"expired":         {session, {Name: oauthCookieName, Value: expired["encrypted_data"].(string)}},
		"from the GitHub": {session, {Name: oauthCookieName, Value: github["encrypted_data"].(string)}},
	} {
		if got := redirectOf(t, postTrelloToken(e, valid, cookies...)); got != "/" {
			t.Errorf("%s: redirect %q, want /", name, got)
		}
	}

	// A conexão é de quem a começou: outra pessoa com o mesmo cookie não a conclui.
	other := invite(t, e, admin, "bia@test.com", "admin")
	if got := redirectOf(t, postTrelloToken(e, valid, sessionCookie(other.session), cookie)); got != "/" {
		t.Errorf("another person's token: redirect %q, want /", got)
	}
	// Sem sessão a página de retorno vai para o login, e um formulário (que não é JSON) é recusado.
	if rec := postTrelloToken(e, valid, cookie); rec.Code == http.StatusOK {
		t.Errorf("a token without a session = %d, want it refused", rec.Code)
	}
	req := httptest.NewRequest("POST", trelloTokenURL, strings.NewReader("state="+state+"&token=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a form post = %d, want 415", rec.Code)
	}

	if got := integrationsOf(t, e, admin.session, prj); len(got) != 0 {
		t.Errorf("refused tokens left %d integration(s) behind", len(got))
	}
}

// Quem pode começar é quem pode gerenciar integrações no projeto, e a permissão é conferida de novo na volta.
func TestTrelloOAuth_Permissions(t *testing.T) {
	e, _ := trelloServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	withPreset(t, e, admin, prj, manager.id, "manager")
	withPreset(t, e, admin, prj, plain.id, "member")
	connect := "/projects/" + prj + "/management/integrations/trello/connect"

	if rec := doCookies(e, connect, sessionCookie(plain.session)); rec.Code != http.StatusNotFound {
		t.Errorf("a collaborator without the permission starting a connection = %d, want 404", rec.Code)
	}
	if rec := doCookies(e, connect); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("starting a connection without a session = %d to %q, want the login", rec.Code, rec.Header().Get("Location"))
	}
	if rec := doCookies(e, trelloCallback); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("the return page without a session = %d to %q, want the login", rec.Code, rec.Header().Get("Location"))
	}

	cookie, state, to := startTrelloConnect(t, e, manager.session, prj, "")
	_, token := authorizeOnTrello(t, to)
	withPreset(t, e, admin, prj, manager.id, "member") // a permissão sai enquanto ele está no Trello
	if got := redirectOf(t, postTrelloToken(e, tokenBody(state, token), sessionCookie(manager.session), cookie)); got != "/" {
		t.Errorf("token after losing the permission: redirect %q, want /", got)
	}
	if got := integrationsOf(t, e, admin.session, prj); len(got) != 0 {
		t.Errorf("a token without permission left %d integration(s)", len(got))
	}

	// Com a permissão, o gerente conecta, e listar os quadros também é de quem gerencia.
	withPreset(t, e, admin, prj, manager.id, "manager")
	id := connectTrello(t, e, manager.session, prj, "")
	if got := status(e, "GET", "/api/integrations/"+id+"/repositories", "", plain.session); got != http.StatusForbidden {
		t.Errorf("a collaborator without the permission listing boards = %d, want 403", got)
	}
}

// Reconectar (?integration=<id>) troca o acesso da integração que já existe, sem criar outra e sem mexer no
// resto; só vale para uma integração do Trello do próprio projeto, e um token que não enxerga o quadro não troca.
func TestTrelloOAuth_Reconnect(t *testing.T) {
	e, fake := trelloServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	elsewhere := createProject(t, e, admin, "Beta")

	id := connectTrello(t, e, admin.session, prj, "")
	do(e, "PATCH", "/api/integrations/"+id, `{"display_name":"App","enabled":true,"metadata":{"board_id":"`+testutil.TrelloBoardID+`"}}`, admin.session)

	// O token antigo é revogado no Trello; reconectar põe o novo no lugar, e a lista volta a funcionar.
	fake.RevokeToken(testutil.TrelloOAuthToken)
	if got := status(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session); got == http.StatusOK {
		t.Fatalf("listing boards with a revoked token = %d, want a failure", got)
	}
	cookie, state, to := startTrelloConnect(t, e, admin.session, prj, "?integration="+id)
	_, _ = authorizeOnTrello(t, to)
	redirect := redirectOf(t, postTrelloToken(e, tokenBody(state, testutil.TrelloSecondOAuthToken), sessionCookie(admin.session), cookie))
	if redirect != "/projects/"+prj+"/management/integrations?trello="+id {
		t.Errorf("reconnect redirect = %q", redirect)
	}
	list := integrationsOf(t, e, admin.session, prj)
	meta, _ := list[0]["metadata"].(map[string]any)
	if len(list) != 1 || list[0]["display_name"] != "App" || list[0]["enabled"] != true || meta["board_id"] != testutil.TrelloBoardID || meta["api_key"] != testutil.TrelloKey {
		t.Errorf("after reconnecting: %v, want the same integration, untouched", list)
	}
	if got := status(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session); got != http.StatusOK {
		t.Errorf("listing boards with the new token = %d, want 200", got)
	}

	// Um token que não enxerga o quadro não troca o acesso.
	fake.SetVisible(testutil.TrelloBoardID, false)
	cookie, state, _ = startTrelloConnect(t, e, admin.session, prj, "?integration="+id)
	redirect = redirectOf(t, postTrelloToken(e, tokenBody(state, "a-third-valid-token"), sessionCookie(admin.session), cookie))
	if !strings.HasSuffix(redirect, "?trello_error=integration.trello_no_access_board") {
		t.Errorf("reconnect with a token that cannot see the board: %q", redirect)
	}
	fake.SetVisible(testutil.TrelloBoardID, true)
	if got := status(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session); got != http.StatusOK {
		t.Errorf("after the refused reconnect the new token must still work, got %d", got)
	}

	// Uma integração de outro projeto, ou que não existe, ou de outro tipo, não se reconecta por este.
	for name, path := range map[string]string{
		"from another project": "/projects/" + elsewhere + "/management/integrations/trello/connect?integration=" + id,
		"that does not exist":  "/projects/" + prj + "/management/integrations/trello/connect?integration=00000000-0000-0000-0000-000000000000",
	} {
		rec := doCookies(e, path, sessionCookie(admin.session))
		if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?trello_error=integration.not_found") {
			t.Errorf("reconnect %s: %d to %q, want the not_found error", name, rec.Code, rec.Header().Get("Location"))
		}
		if setCookie(rec, oauthCookieName) != nil {
			t.Errorf("reconnect %s started a connection", name)
		}
	}
	gh := decode(t, do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"GH","token":"ghp_x","metadata":{"repo":"owner/repo"}}`, admin.session))
	if ghID, _ := gh["id"].(string); ghID != "" {
		rec := doCookies(e, "/projects/"+prj+"/management/integrations/trello/connect?integration="+ghID, sessionCookie(admin.session))
		if !strings.HasSuffix(rec.Header().Get("Location"), "?trello_error=integration.not_found") {
			t.Errorf("reconnecting a GitHub integration through Trello: %q", rec.Header().Get("Location"))
		}
	}
}

// Sem a chave do app no servidor não há como conectar, e a aba avisa.
func TestTrelloOAuth_NotConfigured(t *testing.T) {
	e, _ := trelloServer(t, false)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	rec := doCookies(e, "/projects/"+prj+"/management/integrations/trello/connect", sessionCookie(admin.session))
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?trello_error=integration.trello_oauth_not_configured") {
		t.Errorf("connect without the key: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if setCookie(rec, oauthCookieName) != nil {
		t.Error("a connection started without the app key configured")
	}
	body := do(e, "GET", "/projects/"+prj+"/management/integrations", "", admin.session).Body.String()
	i := strings.Index(body, `"type":"trello"`)
	if i < 0 || !strings.Contains(body[i:], `"configured":false`) {
		t.Error("the tab does not tell the Trello type that the app is not configured")
	}
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }
