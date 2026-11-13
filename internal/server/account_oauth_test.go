package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

// accountServer sobe o servidor com os apps OAuth do GitHub e do GitLab apontados para os fakes, que ficam à mão para
// conferir o que o servidor pediu (a revogação do token).
func accountServer(t *testing.T) (*echo.Echo, *testutil.GitHub, *testutil.GitLab) {
	t.Helper()
	gh := testutil.NewGitHub()
	ghSrv := httptest.NewServer(gh)
	t.Cleanup(ghSrv.Close)
	gl := testutil.NewGitLab()
	glSrv := httptest.NewServer(gl)
	t.Cleanup(glSrv.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: ghSrv.URL} })
	adapter.Register("gitlab", func() adapter.Integration {
		return &adapter.GitLabIntegration{BaseURL: glSrv.URL + "/api/v4"}
	})
	t.Cleanup(func() {
		adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{} })
		adapter.Register("gitlab", func() adapter.Integration { return &adapter.GitLabIntegration{} })
	})

	testutil.Truncate(t, testDB)
	e, err := server.New(testClient, server.Options{
		EncryptKey: "test-key", AuthRateLimit: 1000,
		GitHubOAuth: &adapter.GitHubOAuth{
			ClientID: testutil.GitHubClientID, ClientSecret: testutil.GitHubClientSecret,
			RedirectURL: "http://wtt.test" + oauthCallback, SiteURL: ghSrv.URL, APIURL: ghSrv.URL,
		},
		GitLabOAuth: &adapter.GitLabOAuth{
			ClientID: testutil.GitLabClientID, ClientSecret: testutil.GitLabClientSecret,
			RedirectURL: "http://wtt.test/integrations/gitlab/callback", SiteURL: glSrv.URL,
		},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e, gh, gl
}

// startAccount é o clique em Conectar do perfil: devolve o cookie, o state e a URL de autorização.
func startAccount(t *testing.T, e *echo.Echo, session, platform, query string) (*http.Cookie, string, *url.URL) {
	t.Helper()
	rec := doCookies(e, "/profile/accounts/"+platform+"/connect"+query, sessionCookie(session))
	if rec.Code != http.StatusFound {
		t.Fatalf("account connect = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	cookie := setCookie(rec, oauthCookieName)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("account connect did not set the connection cookie")
	}
	to, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || to.Query().Get("state") == "" {
		t.Fatalf("account connect redirected to %q", rec.Header().Get("Location"))
	}
	return cookie, to.Query().Get("state"), to
}

func accountsOf(t *testing.T, e *echo.Echo, session string) []map[string]any {
	t.Helper()
	rec := do(e, "GET", "/api/auth/accounts", "", session)
	if rec.Code != http.StatusOK {
		t.Fatalf("list accounts = %d: %s", rec.Code, rec.Body.String())
	}
	return decodeList(t, rec)
}

// Um colaborador liga a conta do GitHub pelo perfil: a ida pede só user:email e volta pelo endereço da integração, a
// volta guarda o login e o e-mail principal, e o token é revogado (só ele, não a autorização inteira).
func TestAccountOAuth_ColleagueLinksGitHub(t *testing.T) {
	e, gh, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bruno := invite(t, e, admin, "bruno@test.com", "member")

	cookie, state, to := startAccount(t, e, bruno.session, "github", "")
	q := to.Query()
	if to.Path != "/login/oauth/authorize" || q.Get("scope") != adapter.GitHubAccountScope ||
		q.Get("redirect_uri") != "http://wtt.test"+oauthCallback {
		t.Errorf("authorization URL = %s, want the integration callback and only %s", to, adapter.GitHubAccountScope)
	}
	if cookie.Path != "/integrations/github" || !cookie.HttpOnly {
		t.Errorf("cookie = %+v, want it on the callback path, HttpOnly", cookie)
	}

	rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state="+url.QueryEscape(state),
		sessionCookie(bruno.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/profile?account=github" {
		t.Fatalf("callback = %d to %q, want 303 to /profile?account=github", rec.Code, rec.Header().Get("Location"))
	}
	list := accountsOf(t, e, bruno.session)
	if len(list) != 1 || list[0]["platform"] != "github" || list[0]["username"] != testutil.GitHubAccountLogin ||
		list[0]["email"] != testutil.GitHubAccountEmail || list[0]["source"] != "oauth" || list[0]["no_account"] != false {
		t.Errorf("accounts = %v, want monalisa with the primary e-mail, from oauth", list)
	}
	if got := accountsOf(t, e, admin.session); len(got) != 0 {
		t.Errorf("the admin's accounts = %v, want none: each one sees only their own", got)
	}
	revoked := false
	for _, r := range gh.Requests() {
		if r.Method == "DELETE" && strings.HasPrefix(r.Path, "/applications/") {
			revoked = r.Path == "/applications/"+testutil.GitHubClientID+"/token"
		}
	}
	if !revoked {
		t.Error("the token was not revoked with DELETE /applications/{client_id}/token")
	}
}

// A volta vai para o caminho que a ida pediu (a tarefa que a pessoa estava pegando), com os parâmetros dele; um
// endereço de outro site volta ao perfil.
func TestAccountOAuth_ReturnPath(t *testing.T) {
	e, _, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	cases := map[string]string{
		"/projects/p1/tasks/t1?claim=1": "/projects/p1/tasks/t1?claim=1&account=github",
		"//evil.test/x":                 "/profile?account=github",
		"https://evil.test/x":           "/profile?account=github",
		"/\\evil.test":                  "/profile?account=github",
	}
	for ret, want := range cases {
		cookie, state, _ := startAccount(t, e, admin.session, "github", "?return="+url.QueryEscape(ret))
		rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state="+url.QueryEscape(state),
			sessionCookie(admin.session), cookie)
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("return %q: back to %q, want %q", ret, got, want)
		}
	}
}

// A volta só vale para quem começou a ida, e a recusa no GitHub volta com o código do erro.
func TestAccountOAuth_Refusals(t *testing.T) {
	e, _, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bruno := invite(t, e, admin, "bruno@test.com", "member")

	cookie, state, _ := startAccount(t, e, admin.session, "github", "")
	rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state="+url.QueryEscape(state),
		sessionCookie(bruno.session), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("someone else's cookie = %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
	}
	if got := accountsOf(t, e, bruno.session); len(got) != 0 {
		t.Errorf("accounts = %v, want none", got)
	}

	cookie, state, _ = startAccount(t, e, admin.session, "github", "")
	rec = doCookies(e, oauthCallback+"?error=access_denied&state="+url.QueryEscape(state),
		sessionCookie(admin.session), cookie)
	if got := rec.Header().Get("Location"); got != "/profile?account_error=integration.github_oauth_denied" {
		t.Errorf("denied: back to %q", got)
	}

	cookie, _, _ = startAccount(t, e, admin.session, "github", "")
	rec = doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state=forged",
		sessionCookie(admin.session), cookie)
	if got := rec.Header().Get("Location"); got != "/profile?account_error=integration.github_oauth_state" {
		t.Errorf("wrong state: back to %q", got)
	}

	rec = doCookies(e, "/profile/accounts/trello/connect", sessionCookie(admin.session))
	if got := rec.Header().Get("Location"); got != "/profile?account_error=account.invalid_platform" {
		t.Errorf("trello: back to %q", got)
	}
}

// Sem o app do GitHub no servidor, Conectar volta avisando, sem cookie.
func TestAccountOAuth_NotConfigured(t *testing.T) {
	e := githubServer(t, false)
	admin := signup(t, e, "Org", "ana@test.com")
	rec := doCookies(e, "/profile/accounts/github/connect", sessionCookie(admin.session))
	if got := rec.Header().Get("Location"); got != "/profile?account_error=integration.github_oauth_not_configured" {
		t.Errorf("not configured: back to %q", got)
	}
	if setCookie(rec, oauthCookieName) != nil {
		t.Error("not configured set the connection cookie")
	}
}

// Um login autorizado por uma pessoa não vai para outra da mesma organização.
func TestAccountOAuth_LoginTaken(t *testing.T) {
	e, _, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bruno := invite(t, e, admin, "bruno@test.com", "member")

	for i, who := range []account{admin, bruno} {
		cookie, state, _ := startAccount(t, e, who.session, "github", "")
		rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state="+url.QueryEscape(state),
			sessionCookie(who.session), cookie)
		want := "/profile?account=github"
		if i == 1 {
			want = "/profile?account_error=account.login_taken&account_error_login=monalisa"
		}
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("person %d: back to %q, want %q", i, got, want)
		}
	}
}

// O GitLab: o mesmo vai e volta, com read_user, o e-mail do /user e a revogação no /oauth/revoke.
func TestAccountOAuth_GitLab(t *testing.T) {
	e, _, gl := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	rec := doCookies(e, "/profile/accounts/gitlab/connect", sessionCookie(admin.session))
	to, _ := url.Parse(rec.Header().Get("Location"))
	if to.Query().Get("scope") != adapter.GitLabAccountScope {
		t.Errorf("authorization URL = %s, want only %s", to, adapter.GitLabAccountScope)
	}
	cookie := setCookie(rec, oauthCookieName)
	rec = doCookies(e, "/integrations/gitlab/callback?code="+testutil.GitLabOAuthCode+"&state="+
		url.QueryEscape(to.Query().Get("state")), sessionCookie(admin.session), cookie)
	if got := rec.Header().Get("Location"); got != "/profile?account=gitlab" {
		t.Fatalf("callback: back to %q", got)
	}
	list := accountsOf(t, e, admin.session)
	if len(list) != 1 || list[0]["username"] != testutil.GitLabLogin || list[0]["email"] != testutil.GitLabEmail {
		t.Errorf("accounts = %v", list)
	}
	revoked := false
	for _, r := range gl.Requests() {
		revoked = revoked || (r.Method == "POST" && r.Path == "/oauth/revoke")
	}
	if !revoked {
		t.Error("the GitLab token was not revoked")
	}
}

// Quem conecta uma integração pelo OAuth fica com a conta com que autorizou, se ainda não tinha uma.
func TestAccountOAuth_ConnectorIsLinked(t *testing.T) {
	e, _, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	cookie, state := startConnect(t, e, admin.session, prj, "")
	rec := doCookies(e, oauthCallback+"?code="+testutil.GitHubOAuthCode+"&state="+url.QueryEscape(state),
		sessionCookie(admin.session), cookie)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "?github=") {
		t.Fatalf("integration callback = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	list := accountsOf(t, e, admin.session)
	if len(list) != 1 || list[0]["username"] != testutil.GitHubLogin || list[0]["source"] != "integration" {
		t.Errorf("accounts = %v, want octocat from the integration", list)
	}

	// Ligada a outra conta pelo perfil, reconectar a integração não a troca.
	cookie, state, _ = startAccount(t, e, admin.session, "github", "")
	doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state="+url.QueryEscape(state),
		sessionCookie(admin.session), cookie)
	cookie, state = startConnect(t, e, admin.session, prj, "")
	doCookies(e, oauthCallback+"?code="+testutil.GitHubOAuthCode+"&state="+url.QueryEscape(state),
		sessionCookie(admin.session), cookie)
	if list := accountsOf(t, e, admin.session); len(list) != 1 || list[0]["username"] != testutil.GitHubAccountLogin {
		t.Errorf("accounts = %v, want monalisa kept", list)
	}
}

// "Não tenho conta" e Desconectar, pela API.
func TestAccounts_NoAccountAndDisconnect(t *testing.T) {
	e, _, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	rec := do(e, "POST", "/api/auth/accounts/github/no-account", "", admin.session)
	if rec.Code != http.StatusOK || decode(t, rec)["no_account"] != true {
		t.Fatalf("no-account = %d: %s", rec.Code, rec.Body.String())
	}
	if list := accountsOf(t, e, admin.session); len(list) != 1 || list[0]["username"] != "" || list[0]["source"] != "self" {
		t.Errorf("accounts = %v, want the no-account answer", list)
	}
	if rec := do(e, "POST", "/api/auth/accounts/trello/no-account", "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("trello no-account = %d, want 404", rec.Code)
	}
	if rec := do(e, "DELETE", "/api/auth/accounts/github", "", admin.session); rec.Code != http.StatusNoContent {
		t.Errorf("disconnect = %d, want 204", rec.Code)
	}
	if rec := do(e, "DELETE", "/api/auth/accounts/github", "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("disconnect again = %d, want 404", rec.Code)
	}
	if rec := do(e, "GET", "/api/auth/accounts", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session = %d, want 401", rec.Code)
	}
}

// A trava "só quem tem conta no GitHub", pela API: a tarefa a guarda, pegar e bater o ponto recusam quem não tem a
// conta, e depois de conectar no perfil passa.
func TestRequiredAccounts_ClaimAndClockIn(t *testing.T) {
	e, _, _ := accountServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bruno := invite(t, e, admin, "bruno@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	if rec := do(e, "PUT", "/api/projects/"+prj+"/allocations/"+bruno.id, `{"pay_rate_cents":1000}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("allocate bruno = %d: %s", rec.Code, rec.Body.String())
	}
	rec := do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Dev","required_accounts":["github"]}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	task := decode(t, rec)
	if req, _ := task["required_accounts"].([]any); len(req) != 1 || req[0] != "github" {
		t.Fatalf("required_accounts = %v", task["required_accounts"])
	}
	id := task["id"].(string)

	rec = do(e, "POST", "/api/tasks/"+id+"/claim", "", bruno.session)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"task.account_required"`) {
		t.Errorf("claim without the account = %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "POST", "/api/projects/"+prj+"/work-sessions/clock-in", `{"task_id":"`+id+`"}`, bruno.session)
	if !strings.Contains(rec.Body.String(), `"task.account_required"`) {
		t.Errorf("clock-in without the account = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/work-sessions/open", "", bruno.session); strings.Contains(rec.Body.String(), prj) {
		t.Errorf("a refused clock-in left a session open: %s", rec.Body.String())
	}

	cookie, state, _ := startAccount(t, e, bruno.session, "github", "")
	doCookies(e, oauthCallback+"?code="+testutil.GitHubAccountCode+"&state="+url.QueryEscape(state),
		sessionCookie(bruno.session), cookie)
	if rec := do(e, "POST", "/api/tasks/"+id+"/claim", "", bruno.session); rec.Code != http.StatusOK {
		t.Errorf("claim with the account = %d: %s", rec.Code, rec.Body.String())
	}

	// Tirar a trava pelos detalhes: a lista vazia.
	rec = do(e, "PATCH", "/api/tasks/"+id+"/attributes", `{"required_accounts":[]}`, admin.session)
	if req, _ := decode(t, rec)["required_accounts"].([]any); rec.Code != http.StatusOK || len(req) != 0 {
		t.Errorf("turning it off = %d: %s", rec.Code, rec.Body.String())
	}
}
