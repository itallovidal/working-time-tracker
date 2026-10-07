package server_test

import (
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
	oauthCookieName = "wtt_oauth"
	oauthCallback   = "/integrations/github/callback"
)

// githubServer sobe o servidor com o app OAuth do GitHub apontado para o GitHub fake: o
// mesmo endereço serve o site (login/oauth) e a API. configured falso é o servidor sem o
// app cadastrado.
func githubServer(t *testing.T, configured bool) *echo.Echo {
	t.Helper()
	fake := httptest.NewServer(testutil.FakeGitHub())
	t.Cleanup(fake.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: fake.URL} })
	t.Cleanup(func() {
		adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{} })
	})

	testutil.Truncate(t, testDB)
	opts := server.Options{EncryptKey: "test-key", AuthRateLimit: 1000}
	if configured {
		opts.GitHubOAuth = &adapter.GitHubOAuth{
			ClientID: testutil.GitHubClientID, ClientSecret: testutil.GitHubClientSecret,
			RedirectURL: "http://wtt.test" + oauthCallback, SiteURL: fake.URL,
		}
	}
	e, err := server.New(testClient, opts)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e
}

// doCookies é o do() com os cookies que o navegador mandaria: o de sessão e o da conexão.
func doCookies(e *echo.Echo, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(session string) *http.Cookie {
	return &http.Cookie{Name: "wtt_session", Value: session}
}

// setCookie devolve o cookie de nome name que a resposta mandou gravar.
func setCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// startConnect é o clique em Conectar: devolve o cookie da conexão e o state que foi para o
// GitHub.
func startConnect(t *testing.T, e *echo.Echo, session, projectID, query string) (*http.Cookie, string) {
	t.Helper()
	rec := doCookies(e, "/projects/"+projectID+"/management/integrations/github/connect"+query, sessionCookie(session))
	if rec.Code != http.StatusFound {
		t.Fatalf("connect = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	cookie := setCookie(rec, oauthCookieName)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("connect did not set the connection cookie")
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || u.Query().Get("state") == "" {
		t.Fatalf("connect redirected to %q, want the GitHub authorization with a state", rec.Header().Get("Location"))
	}
	return cookie, u.Query().Get("state")
}

func integrationsOf(t *testing.T, e *echo.Echo, session, projectID string) []map[string]any {
	t.Helper()
	rec := do(e, "GET", "/api/projects/"+projectID+"/integrations", "", session)
	if rec.Code != http.StatusOK {
		t.Fatalf("list integrations = %d: %s", rec.Code, rec.Body.String())
	}
	return decodeList(t, rec)
}

// O caminho feliz, de ponta a ponta: o clique leva ao GitHub com o que ele precisa, a volta
// guarda uma integração desativada e sem repositório, e a pessoa a termina escolhendo um
// repositório da lista.
func TestGitHubOAuth_ConnectAndPickTheRepository(t *testing.T) {
	e := githubServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	rec := doCookies(e, "/projects/"+prj+"/management/integrations/github/connect", sessionCookie(admin.session))
	if rec.Code != http.StatusFound {
		t.Fatalf("connect = %d: %s", rec.Code, rec.Body.String())
	}
	to, _ := url.Parse(rec.Header().Get("Location"))
	q := to.Query()
	if to.Path != "/login/oauth/authorize" || q.Get("client_id") != testutil.GitHubClientID ||
		q.Get("redirect_uri") != "http://wtt.test"+oauthCallback || q.Get("scope") != "repo" || q.Get("state") == "" {
		t.Errorf("authorization URL = %s", to)
	}
	cookie := setCookie(rec, oauthCookieName)
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/integrations/github" || cookie.MaxAge <= 0 {
		t.Errorf("connection cookie = %+v, want HttpOnly, SameSite=Lax, limited to the callback path and with a lifetime", cookie)
	}
	if strings.Contains(cookie.Value, q.Get("state")) {
		t.Error("the state is readable in the cookie: it must be sealed")
	}

	rec = doCookies(e, oauthCallback+"?code="+testutil.GitHubOAuthCode+"&state="+url.QueryEscape(q.Get("state")),
		sessionCookie(admin.session), cookie)
	list := integrationsOf(t, e, admin.session, prj)
	if len(list) != 1 {
		t.Fatalf("the callback left %d integration(s), want 1", len(list))
	}
	it := list[0]
	id := it["id"].(string)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/projects/"+prj+"/management/integrations?github="+id {
		t.Errorf("callback = %d to %q, want 303 back to the tab with ?github=<id>", rec.Code, rec.Header().Get("Location"))
	}
	if cleared := setCookie(rec, oauthCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Errorf("the connection cookie was not cleared after the callback: %+v", cleared)
	}
	if it["type"] != "github" || it["enabled"] != false || it["has_token"] != true ||
		it["display_name"] != "GitHub · @"+testutil.GitHubLogin || len(it["metadata"].(map[string]any)) != 0 {
		t.Errorf("connected integration = %v", it)
	}
	if body := do(e, "GET", "/api/integrations/"+id, "", admin.session).Body.String(); strings.Contains(body, testutil.GitHubOAuthToken) {
		t.Errorf("the API leaks the OAuth token: %s", body)
	}

	// A lista que a tela oferece vem do servidor, que usa o token guardado.
	rec = do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session)
	repos := decodeList(t, rec)
	if rec.Code != http.StatusOK || len(repos) != 2 || repos[0]["full_name"] != "owner/repo" || repos[0]["private"] != true {
		t.Errorf("repositories = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), testutil.GitHubOAuthToken) {
		t.Error("the repositories answer leaks the OAuth token")
	}

	// Escolher o repositório ativa a integração, validada com o token guardado.
	rec = do(e, "PATCH", "/api/integrations/"+id, `{"display_name":"App","enabled":true,"metadata":{"repo":"owner/repo"}}`, admin.session)
	got := decode(t, rec)
	if rec.Code != http.StatusOK || got["enabled"] != true || got["display_name"] != "App" || got["metadata"].(map[string]any)["repo"] != "owner/repo" {
		t.Errorf("pick the repository = %d %s", rec.Code, rec.Body.String())
	}

	// A aba abre com o que o JavaScript precisa para a volta.
	page := do(e, "GET", "/projects/"+prj+"/management/integrations?github="+id, "", admin.session)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"auth":"oauth"`) || !strings.Contains(page.Body.String(), `"configured":true`) {
		t.Errorf("the tab = %d, want 200 with the github type as oauth and configured", page.Code)
	}
}

// Tudo o que não bate com a conexão que o servidor começou volta para a aba com o motivo, e
// nada é guardado. Sem conexão em andamento, ou com outra pessoa, não há para onde voltar.
func TestGitHubOAuth_CallbackRefusals(t *testing.T) {
	e := githubServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	back := "/projects/" + prj + "/management/integrations?github_error="
	good := "code=" + testutil.GitHubOAuthCode

	cases := []struct {
		name   string
		query  func(state string) string
		wantTo string
	}{
		{"another state", func(string) string { return good + "&state=forged" }, back + "integration.github_oauth_state"},
		{"no state", func(string) string { return good }, back + "integration.github_oauth_state"},
		{"cancelled on GitHub", func(s string) string { return "error=access_denied&state=" + s }, back + "integration.github_oauth_denied"},
		{"another error from GitHub", func(s string) string { return "error=redirect_uri_mismatch&state=" + s }, back + "integration.github_oauth_exchange"},
		{"no code", func(s string) string { return "state=" + s }, back + "integration.github_oauth_exchange"},
		{"a code GitHub does not accept", func(s string) string { return "code=expired&state=" + s }, back + "integration.github_oauth_exchange"},
	}
	for _, tc := range cases {
		cookie, state := startConnect(t, e, admin.session, prj, "")
		rec := doCookies(e, oauthCallback+"?"+tc.query(url.QueryEscape(state)), sessionCookie(admin.session), cookie)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.wantTo {
			t.Errorf("%s: %d to %q, want 303 to %q", tc.name, rec.Code, rec.Header().Get("Location"), tc.wantTo)
		}
		if cleared := setCookie(rec, oauthCookieName); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("%s: the connection cookie survived", tc.name)
		}
	}

	// Sem uma conexão em andamento (cookie ausente, adulterado ou vencido) não há projeto para
	// onde voltar: vai para o início, e o código que veio na URL não é trocado.
	cookie, state := startConnect(t, e, admin.session, prj, "")
	valid := oauthCallback + "?" + good + "&state=" + url.QueryEscape(state)
	expired, err := adapter.EncryptConfig(map[string]interface{}{
		"state": state, "person_id": admin.id, "project_id": prj, "integration_id": "",
		"expires": time.Now().Add(-time.Minute).Unix(),
	}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for name, cookies := range map[string][]*http.Cookie{
		"no cookie": {sessionCookie(admin.session)},
		"tampered":  {sessionCookie(admin.session), {Name: oauthCookieName, Value: cookie.Value[:len(cookie.Value)-4] + "AAAA"}},
		"garbage":   {sessionCookie(admin.session), {Name: oauthCookieName, Value: "nope"}},
		"expired":   {sessionCookie(admin.session), {Name: oauthCookieName, Value: expired["encrypted_data"].(string)}},
	} {
		rec := doCookies(e, valid, cookies...)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
			t.Errorf("%s: %d to %q, want 303 to /", name, rec.Code, rec.Header().Get("Location"))
		}
	}

	// A conexão é de quem a começou: outra pessoa com o mesmo cookie não a conclui.
	other := invite(t, e, admin, "bia@test.com", "admin")
	rec := doCookies(e, valid, sessionCookie(other.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("another person's callback: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
	}

	if got := integrationsOf(t, e, admin.session, prj); len(got) != 0 {
		t.Errorf("refused callbacks left %d integration(s) behind", len(got))
	}
}

// Quem pode começar é quem pode gerenciar integrações no projeto, e a permissão é conferida de
// novo na volta: os minutos no GitHub podem tê-la tirado.
func TestGitHubOAuth_Permissions(t *testing.T) {
	e := githubServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	withPreset(t, e, admin, prj, manager.id, "manager")
	withPreset(t, e, admin, prj, plain.id, "member")
	connect := "/projects/" + prj + "/management/integrations/github/connect"

	if rec := doCookies(e, connect, sessionCookie(plain.session)); rec.Code != http.StatusNotFound {
		t.Errorf("a collaborator without the permission starting a connection = %d, want 404", rec.Code)
	}
	if rec := doCookies(e, connect); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("starting a connection without a session = %d to %q, want the login", rec.Code, rec.Header().Get("Location"))
	}

	cookie, state := startConnect(t, e, manager.session, prj, "")
	withPreset(t, e, admin, prj, manager.id, "member") // a permissão sai enquanto ele está no GitHub
	rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(manager.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("callback after losing the permission: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
	}
	if got := integrationsOf(t, e, admin.session, prj); len(got) != 0 {
		t.Errorf("a callback without permission left %d integration(s)", len(got))
	}

	// Com a permissão, o gerente conecta.
	withPreset(t, e, admin, prj, manager.id, "manager")
	cookie, state = startConnect(t, e, manager.session, prj, "")
	rec = doCookies(e, oauthCallback+"?code="+testutil.GitHubOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(manager.session), cookie)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "?github=") {
		t.Errorf("a manager's callback: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	// Ler os repositórios de uma integração também é de quem gerencia.
	id := integrationsOf(t, e, admin.session, prj)[0]["id"].(string)
	if got := status(e, "GET", "/api/integrations/"+id+"/repositories", "", plain.session); got != http.StatusForbidden {
		t.Errorf("a collaborator without the permission listing repositories = %d, want 403", got)
	}
}

// Reconectar (?integration=<id>) troca o acesso da integração que já existe, sem criar outra
// e sem mexer no resto; só vale para uma integração GitHub do próprio projeto.
func TestGitHubOAuth_Reconnect(t *testing.T) {
	e := githubServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	elsewhere := createProject(t, e, admin, "Beta")

	cookie, state := startConnect(t, e, admin.session, prj, "")
	doCookies(e, oauthCallback+"?code="+testutil.GitHubOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(admin.session), cookie)
	id := integrationsOf(t, e, admin.session, prj)[0]["id"].(string)
	do(e, "PATCH", "/api/integrations/"+id, `{"display_name":"App","enabled":true,"metadata":{"repo":"owner/repo"}}`, admin.session)

	cookie, state = startConnect(t, e, admin.session, prj, "?integration="+id)
	rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubSecondOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(admin.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/projects/"+prj+"/management/integrations?github="+id {
		t.Errorf("reconnect callback: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	list := integrationsOf(t, e, admin.session, prj)
	if len(list) != 1 || list[0]["display_name"] != "App" || list[0]["enabled"] != true || list[0]["metadata"].(map[string]any)["repo"] != "owner/repo" {
		t.Errorf("after reconnecting: %v, want the same integration, untouched", list)
	}
	// O token novo é o que lista (o fake responde outro repositório para ele).
	if repos := decodeList(t, do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session)); len(repos) != 1 || repos[0]["full_name"] != "owner/second" {
		t.Errorf("repositories after reconnecting = %v, want the ones of the new token", repos)
	}

	// Uma integração de outro projeto, ou que não existe, não se reconecta por este.
	for name, path := range map[string]string{
		"from another project": "/projects/" + elsewhere + "/management/integrations/github/connect?integration=" + id,
		"that does not exist":  "/projects/" + prj + "/management/integrations/github/connect?integration=00000000-0000-0000-0000-000000000000",
	} {
		rec := doCookies(e, path, sessionCookie(admin.session))
		if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?github_error=integration.not_found") {
			t.Errorf("reconnect %s: %d to %q, want the not_found error", name, rec.Code, rec.Header().Get("Location"))
		}
		if setCookie(rec, oauthCookieName) != nil {
			t.Errorf("reconnect %s started a connection", name)
		}
	}
}

// Sem o app cadastrado no servidor não há como conectar, e a aba avisa.
func TestGitHubOAuth_NotConfigured(t *testing.T) {
	e := githubServer(t, false)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	rec := doCookies(e, "/projects/"+prj+"/management/integrations/github/connect", sessionCookie(admin.session))
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?github_error=integration.github_oauth_not_configured") {
		t.Errorf("connect without the app: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if setCookie(rec, oauthCookieName) != nil {
		t.Error("a connection started without the app configured")
	}
	if page := do(e, "GET", "/projects/"+prj+"/management/integrations", "", admin.session).Body.String(); !strings.Contains(page, `"configured":false`) {
		t.Error("the tab does not tell the types that the GitHub app is not configured")
	}
}
