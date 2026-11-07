package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

const gitlabCallback = "/integrations/gitlab/callback"

// gitlabServer sobe o servidor com o app OAuth do GitLab apontado para o GitLab fake: o mesmo endereço serve o
// OAuth (na raiz) e a API (em /api/v4). configured falso é o servidor sem o app cadastrado.
func gitlabServer(t *testing.T, configured bool) (*echo.Echo, *testutil.GitLab) {
	t.Helper()
	fake := testutil.NewGitLab()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	adapter.Register("gitlab", func() adapter.Integration { return &adapter.GitLabIntegration{BaseURL: srv.URL + "/api/v4"} })
	t.Cleanup(func() {
		adapter.Register("gitlab", func() adapter.Integration { return &adapter.GitLabIntegration{} })
	})

	testutil.Truncate(t, testDB)
	opts := server.Options{EncryptKey: "test-key", AuthRateLimit: 1000}
	if configured {
		opts.GitLabOAuth = &adapter.GitLabOAuth{
			ClientID: testutil.GitLabClientID, ClientSecret: testutil.GitLabClientSecret,
			RedirectURL: "http://wtt.test" + gitlabCallback, SiteURL: srv.URL,
		}
	}
	e, err := server.New(testClient, opts)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e, fake
}

// startGitLabConnect é o clique em Conectar com o GitLab: devolve o cookie da conexão e o state que foi para lá.
func startGitLabConnect(t *testing.T, e *echo.Echo, session, projectID, query string) (*http.Cookie, string) {
	t.Helper()
	rec := doCookies(e, "/projects/"+projectID+"/management/integrations/gitlab/connect"+query, sessionCookie(session))
	if rec.Code != http.StatusFound {
		t.Fatalf("connect = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	cookie := setCookie(rec, oauthCookieName)
	u, err := url.Parse(rec.Header().Get("Location"))
	if cookie == nil || err != nil || u.Query().Get("state") == "" {
		t.Fatalf("connect redirected to %q with cookie %v", rec.Header().Get("Location"), cookie)
	}
	return cookie, u.Query().Get("state")
}

// connectGitLab faz o vai e volta inteiro com o código dado e devolve o id da integração.
func connectGitLab(t *testing.T, e *echo.Echo, session, projectID, code string) string {
	t.Helper()
	cookie, state := startGitLabConnect(t, e, session, projectID, "")
	rec := doCookies(e, gitlabCallback+"?code="+code+"&state="+url.QueryEscape(state), sessionCookie(session), cookie)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "?gitlab=") {
		t.Fatalf("callback = %d to %q", rec.Code, loc)
	}
	return loc[strings.Index(loc, "?gitlab=")+len("?gitlab="):]
}

// O caminho feliz, de ponta a ponta: o clique leva ao GitLab com o que ele precisa, a volta guarda uma
// integração desativada, sem projeto e com o par de tokens, a pessoa escolhe o projeto da lista e liga a
// sincronização, e a primeira rodada traz as issues.
func TestGitLabOAuth_ConnectPickAndSync(t *testing.T) {
	e, _ := gitlabServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	rec := doCookies(e, "/projects/"+prj+"/management/integrations/gitlab/connect", sessionCookie(admin.session))
	to, _ := url.Parse(rec.Header().Get("Location"))
	q := to.Query()
	if rec.Code != http.StatusFound || to.Path != "/oauth/authorize" || q.Get("client_id") != testutil.GitLabClientID ||
		q.Get("redirect_uri") != "http://wtt.test"+gitlabCallback || q.Get("scope") != "api" || q.Get("response_type") != "code" || q.Get("state") == "" {
		t.Errorf("authorization = %d %s", rec.Code, to)
	}
	cookie := setCookie(rec, oauthCookieName)
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/integrations/gitlab" {
		t.Errorf("connection cookie = %+v, want HttpOnly, SameSite=Lax and limited to the GitLab callback", cookie)
	}

	rec = doCookies(e, gitlabCallback+"?code="+testutil.GitLabOAuthCode+"&state="+url.QueryEscape(q.Get("state")), sessionCookie(admin.session), cookie)
	list := integrationsOf(t, e, admin.session, prj)
	if len(list) != 1 {
		t.Fatalf("the callback left %d integration(s), want 1", len(list))
	}
	it := list[0]
	id := it["id"].(string)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/projects/"+prj+"/management/integrations?gitlab="+id {
		t.Errorf("callback = %d to %q, want 303 back to the tab with ?gitlab=<id>", rec.Code, rec.Header().Get("Location"))
	}
	if it["type"] != "gitlab" || it["enabled"] != false || it["has_token"] != true || it["display_name"] != "GitLab · @"+testutil.GitLabLogin {
		t.Errorf("connected integration = %v", it)
	}
	row := testClient.Integration.GetX(context.Background(), uuid.MustParse(id))
	secrets, err := adapter.DecryptConfig(row.Credentials, "test-key")
	if err != nil || secrets["token"] != testutil.GitLabOAuthToken || secrets["refresh_token"] != testutil.GitLabRefreshToken || secrets["expires_at"] == nil {
		t.Errorf("stored credential = %v (%v), want the pair and the expiry", secrets, err)
	}
	body := do(e, "GET", "/api/integrations/"+id, "", admin.session).Body.String()
	if strings.Contains(body, testutil.GitLabOAuthToken) || strings.Contains(body, testutil.GitLabRefreshToken) {
		t.Errorf("the API leaks a token: %s", body)
	}

	rec = do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session)
	repos := decodeList(t, rec)
	if rec.Code != http.StatusOK || len(repos) != 2 || repos[1]["full_name"] != "group/project" || repos[1]["private"] != true {
		t.Errorf("projects = %d %s", rec.Code, rec.Body.String())
	}

	// Escolher o projeto ativa a integração, e a sincronização liga junto, como a tela faz.
	rec = do(e, "PATCH", "/api/integrations/"+id, `{"display_name":"App","enabled":true,"sync_issues":true,"metadata":{"project_url":"https://gitlab.com/group/project"}}`, admin.session)
	got := decode(t, rec)
	if rec.Code != http.StatusOK || got["enabled"] != true || got["sync_issues"] != true || got["metadata"].(map[string]any)["project_url"] != "group/project" {
		t.Fatalf("pick the project = %d %s", rec.Code, rec.Body.String())
	}

	// Sincronizar agora traz a issue 7 do fake como tarefa.
	rec = do(e, "POST", "/api/integrations/"+id+"/sync", "", admin.session)
	if sum := decode(t, rec); rec.Code != http.StatusOK || sum["created"] != float64(1) {
		t.Errorf("sync now = %d %s", rec.Code, rec.Body.String())
	}

	// A aba abre com o GitLab como OAuth configurado e fora do "em breve".
	page := do(e, "GET", "/projects/"+prj+"/management/integrations?gitlab="+id, "", admin.session).Body.String()
	if d := snippet(page, `"type":"gitlab"`); !strings.Contains(d, `"coming_soon":false`) || !strings.Contains(d, `"sync":true`) ||
		!strings.Contains(d, `"auth":"oauth"`) || !strings.Contains(d, `"configured":true`) {
		t.Errorf("the tab does not offer GitLab: %s", snippet(page, `"type":"gitlab"`))
	}
}

// snippet devolve o texto de onde aparece o que se procura até o próximo tipo de integração (ou 2000
// caracteres): o descritor de um tipo, para conferir e para a mensagem de erro.
func snippet(s, near string) string {
	i := strings.Index(s, near)
	if i < 0 {
		return "(" + near + " not found)"
	}
	rest := s[i+len(near):]
	if j := strings.Index(rest, `{"type":`); j >= 0 {
		return near + rest[:j]
	}
	return s[i:min(len(s), i+2000)]
}

// O token que vence é renovado pelo servidor sozinho, na hora de usar, e o par novo fica guardado.
func TestGitLabOAuth_TheServerRefreshesTheToken(t *testing.T) {
	e, fake := gitlabServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	id := connectGitLab(t, e, admin.session, prj, testutil.GitLabOAuthCode)

	// Falta um minuto para o token vencer.
	row := testClient.Integration.GetX(context.Background(), uuid.MustParse(id))
	secrets, err := adapter.DecryptConfig(row.Credentials, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	secrets["expires_at"] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	sealed, err := adapter.EncryptConfig(secrets, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	testClient.Integration.UpdateOne(row).SetCredentials(sealed).ExecX(context.Background())

	if rec := do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session); rec.Code != http.StatusOK {
		t.Fatalf("projects with an expiring token = %d %s", rec.Code, rec.Body.String())
	}
	if fake.Refreshes() != 1 {
		t.Errorf("%d refreshes, want one", fake.Refreshes())
	}
	after, _ := adapter.DecryptConfig(testClient.Integration.GetX(context.Background(), row.ID).Credentials, "test-key")
	if after["token"] == testutil.GitLabOAuthToken || after["refresh_token"] == testutil.GitLabRefreshToken {
		t.Errorf("stored after the refresh = %v, want the new pair", after)
	}

	// A autorização tirada no GitLab: o token recusado aparece, e a pessoa reconecta.
	fake.RevokeTokens()
	if rec := do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "integration.invalid_token") {
		t.Errorf("projects after the revoke = %d %s", rec.Code, rec.Body.String())
	}
}

// Tudo o que não bate com a conexão que o servidor começou volta para a aba com o motivo, e nada é guardado.
// O cookie de uma plataforma não serve à volta da outra.
func TestGitLabOAuth_CallbackRefusals(t *testing.T) {
	e, _ := gitlabServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	back := "/projects/" + prj + "/management/integrations?gitlab_error="
	good := "code=" + testutil.GitLabOAuthCode

	cases := []struct {
		name   string
		query  func(state string) string
		wantTo string
	}{
		{"another state", func(string) string { return good + "&state=forged" }, back + "integration.gitlab_oauth_state"},
		{"cancelled on GitLab", func(s string) string { return "error=access_denied&state=" + s }, back + "integration.gitlab_oauth_denied"},
		{"another error from GitLab", func(s string) string { return "error=invalid_scope&state=" + s }, back + "integration.gitlab_oauth_exchange"},
		{"no code", func(s string) string { return "state=" + s }, back + "integration.gitlab_oauth_exchange"},
		{"a code GitLab does not accept", func(s string) string { return "code=expired&state=" + s }, back + "integration.gitlab_oauth_exchange"},
	}
	for _, tc := range cases {
		cookie, state := startGitLabConnect(t, e, admin.session, prj, "")
		rec := doCookies(e, gitlabCallback+"?"+tc.query(url.QueryEscape(state)), sessionCookie(admin.session), cookie)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.wantTo {
			t.Errorf("%s: %d to %q, want 303 to %q", tc.name, rec.Code, rec.Header().Get("Location"), tc.wantTo)
		}
	}

	// O cookie de uma conexão com o GitHub não conclui a do GitLab.
	githubCookie := doCookies(e, "/projects/"+prj+"/management/integrations/github/connect", sessionCookie(admin.session))
	if c := setCookie(githubCookie, oauthCookieName); c != nil {
		rec := doCookies(e, gitlabCallback+"?"+good+"&state=x", sessionCookie(admin.session), c)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
			t.Errorf("a GitHub cookie on the GitLab callback: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
		}
	}

	// Outra pessoa com o mesmo cookie não conclui a conexão.
	cookie, state := startGitLabConnect(t, e, admin.session, prj, "")
	other := invite(t, e, admin, "bia@test.com", "admin")
	rec := doCookies(e, gitlabCallback+"?"+good+"&state="+url.QueryEscape(state), sessionCookie(other.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("another person's callback: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
	}
	if got := integrationsOf(t, e, admin.session, prj); len(got) != 0 {
		t.Errorf("refused callbacks left %d integration(s) behind", len(got))
	}
}

// Quem pode começar é quem gerencia integrações no projeto, e a permissão é conferida de novo na volta.
func TestGitLabOAuth_Permissions(t *testing.T) {
	e, _ := gitlabServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	withPreset(t, e, admin, prj, manager.id, "manager")
	withPreset(t, e, admin, prj, plain.id, "member")

	if rec := doCookies(e, "/projects/"+prj+"/management/integrations/gitlab/connect", sessionCookie(plain.session)); rec.Code != http.StatusNotFound {
		t.Errorf("a collaborator without the permission starting a connection = %d, want 404", rec.Code)
	}
	cookie, state := startGitLabConnect(t, e, manager.session, prj, "")
	withPreset(t, e, admin, prj, manager.id, "member") // a permissão sai enquanto ele está no GitLab
	rec := doCookies(e, gitlabCallback+"?code="+testutil.GitLabOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(manager.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("callback after losing the permission: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
	}
	if got := integrationsOf(t, e, admin.session, prj); len(got) != 0 {
		t.Errorf("a callback without permission left %d integration(s)", len(got))
	}
}

// Reconectar troca o par de tokens da integração que já existe, sem criar outra e sem mexer no resto. A conta
// nova que não enxerga o projeto escolhido é recusada, e a integração continua com o par antigo.
func TestGitLabOAuth_Reconnect(t *testing.T) {
	e, fake := gitlabServer(t, true)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	id := connectGitLab(t, e, admin.session, prj, testutil.GitLabOAuthCode)
	do(e, "PATCH", "/api/integrations/"+id, `{"display_name":"App","enabled":true,"metadata":{"project_url":"group/project"}}`, admin.session)

	cookie, state := startGitLabConnect(t, e, admin.session, prj, "?integration="+id)
	rec := doCookies(e, gitlabCallback+"?code="+testutil.GitLabSecondOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(admin.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/projects/"+prj+"/management/integrations?gitlab="+id {
		t.Errorf("reconnect callback: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	list := integrationsOf(t, e, admin.session, prj)
	if len(list) != 1 || list[0]["display_name"] != "App" || list[0]["enabled"] != true {
		t.Errorf("after reconnecting: %v, want the same integration, untouched", list)
	}
	if repos := decodeList(t, do(e, "GET", "/api/integrations/"+id+"/repositories", "", admin.session)); len(repos) != 1 || repos[0]["full_name"] != "group/second" {
		t.Errorf("projects after reconnecting = %v, want the ones of the new account", repos)
	}
	secrets, _ := adapter.DecryptConfig(testClient.Integration.GetX(context.Background(), uuid.MustParse(id)).Credentials, "test-key")
	if secrets["refresh_token"] != testutil.GitLabSecondRefreshToken {
		t.Errorf("stored after reconnecting = %v, want the new pair", secrets)
	}

	// Uma conta que não enxerga o projeto: o GitLab responde 404 por ele, e a reconexão é recusada.
	fake.Fail("GET", "/api/v4/projects/group%2Fproject", http.StatusNotFound, 1)
	cookie, state = startGitLabConnect(t, e, admin.session, prj, "?integration="+id)
	rec = doCookies(e, gitlabCallback+"?code="+testutil.GitLabOAuthCode+"&state="+url.QueryEscape(state), sessionCookie(admin.session), cookie)
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?gitlab_error=integration.gitlab_project_not_found") {
		t.Errorf("reconnect with an account that does not see the project: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	secrets, _ = adapter.DecryptConfig(testClient.Integration.GetX(context.Background(), uuid.MustParse(id)).Credentials, "test-key")
	if secrets["refresh_token"] != testutil.GitLabSecondRefreshToken {
		t.Errorf("a refused reconnect changed the credential: %v", secrets)
	}

	// Uma integração do GitHub não se reconecta pelo GitLab.
	gh := testClient.Integration.Create().SetProjectID(uuid.MustParse(prj)).SetType("github").SetDisplayName("GitHub").SaveX(context.Background())
	rec = doCookies(e, "/projects/"+prj+"/management/integrations/gitlab/connect?integration="+gh.ID.String(), sessionCookie(admin.session))
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?gitlab_error=integration.not_found") {
		t.Errorf("reconnect a GitHub integration through GitLab: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

// Sem o app cadastrado no servidor não há como conectar, e a aba avisa.
func TestGitLabOAuth_NotConfigured(t *testing.T) {
	e, _ := gitlabServer(t, false)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	rec := doCookies(e, "/projects/"+prj+"/management/integrations/gitlab/connect", sessionCookie(admin.session))
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?gitlab_error=integration.gitlab_oauth_not_configured") {
		t.Errorf("connect without the app: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if setCookie(rec, oauthCookieName) != nil {
		t.Error("a connection started without the app configured")
	}
	page := do(e, "GET", "/projects/"+prj+"/management/integrations", "", admin.session).Body.String()
	if !strings.Contains(snippet(page, `"type":"gitlab"`), `"configured":false`) {
		t.Errorf("the tab does not tell that the GitLab app is not configured: %s", snippet(page, `"type":"gitlab"`))
	}
}
