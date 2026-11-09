package server_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

// O OAuth do servidor MCP de ponta a ponta: a descoberta, a tela de consentimento, a troca do código, a renovação, a
// revogação e o /mcp com o token de acesso. O emissor dos testes é https://wtt.test; os apps são os do Claude, com a
// cópia embutida do documento de identidade (os testes não falam com a internet).

const (
	oauthIssuer    = "https://wtt.test"
	oauthResource  = oauthIssuer + "/mcp"
	claudeID       = "https://claude.ai/oauth/mcp-oauth-client-metadata"
	claudeCallback = "https://claude.ai/api/mcp/auth_callback"
	claudeCodeID   = "https://claude.ai/oauth/claude-code-client-metadata"
)

func newOAuthServer(t *testing.T, opts ...func(*server.Options)) *echo.Echo {
	t.Helper()
	testutil.Truncate(t, testDB)
	o := server.Options{EncryptKey: "test-key", AuthRateLimit: 1000, PublicURL: oauthIssuer, OAuth: true}
	for _, f := range opts {
		f(&o)
	}
	e, err := server.New(testClient, o)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e
}

type pkcePair struct{ verifier, challenge string }

func newPKCE(t *testing.T) pkcePair {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	v := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(v))
	return pkcePair{verifier: v, challenge: base64.RawURLEncoding.EncodeToString(sum[:])}
}

// authorizeQuery é o pedido de autorização que o Claude manda.
func authorizeQuery(client, redirect, state string, p pkcePair) url.Values {
	return url.Values{
		"response_type": {"code"}, "client_id": {client}, "redirect_uri": {redirect}, "state": {state},
		"code_challenge": {p.challenge}, "code_challenge_method": {"S256"},
		"resource": {oauthResource}, "scope": {"mcp"},
	}
}

// consentBody é o que a tela de consentimento devolve para o pedido.
func consentBody(q url.Values, approve bool) string {
	body := map[string]any{"approve": approve}
	for _, k := range []string{"client_id", "redirect_uri", "state", "response_type", "code_challenge", "code_challenge_method", "resource", "scope"} {
		body[k] = q.Get(k)
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// consentRedirect manda a resposta da tela e devolve o endereço de volta ao app.
func consentRedirect(t *testing.T, e *echo.Echo, acct account, q url.Values, approve bool) *url.URL {
	t.Helper()
	rec := do(e, "POST", "/api/oauth/consent", consentBody(q, approve), acct.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("consent = %d: %s", rec.Code, rec.Body.String())
	}
	u, err := url.Parse(decode(t, rec)["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// authorizeCode passa pela tela (que tem de abrir) e pelo "Permitir", e devolve o código.
func authorizeCode(t *testing.T, e *echo.Echo, acct account, client, redirect string) (string, pkcePair) {
	t.Helper()
	p := newPKCE(t)
	q := authorizeQuery(client, redirect, "st-"+p.verifier[:6], p)
	if rec := do(e, "GET", "/oauth/authorize?"+q.Encode(), "", acct.session); rec.Code != http.StatusOK {
		t.Fatalf("authorize = %d: %.300s", rec.Code, rec.Body.String())
	}
	u := consentRedirect(t, e, acct, q, true)
	code := u.Query().Get("code")
	if code == "" {
		t.Fatalf("consent redirect %s has no code", u)
	}
	return code, p
}

func tokenRequest(e *echo.Echo, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func exchangeForm(code string, p pkcePair, client, redirect string) url.Values {
	return url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {p.verifier},
		"client_id": {client}, "redirect_uri": {redirect}, "resource": {oauthResource},
	}
}

func refreshForm(refresh, client string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {client}}
}

// tokenOK confere uma resposta de sucesso do /oauth/token e devolve o access_token e o refresh_token.
func tokenOK(t *testing.T, rec *httptest.ResponseRecorder) (access, refresh string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("token = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	access, _ = body["access_token"].(string)
	refresh, _ = body["refresh_token"].(string)
	if !strings.HasPrefix(access, "wtta_") || !strings.HasPrefix(refresh, "wttr_") || body["token_type"] != "Bearer" ||
		body["expires_in"] != float64(3600) || body["scope"] != "mcp" {
		t.Fatalf("token response %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Pragma") != "no-cache" {
		t.Errorf("token response cache headers %q %q, want no-store", rec.Header().Get("Cache-Control"), rec.Header().Get("Pragma"))
	}
	return access, refresh
}

// tokenError confere o erro do /oauth/token.
func tokenError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("token = %d %s, want %d %s", rec.Code, rec.Body.String(), status, code)
	}
	if got := decode(t, rec)["error"]; got != code {
		t.Errorf("token error %v, want %s (%s)", got, code, rec.Body.String())
	}
}

// connectClaude conecta o claude.ai pela pessoa e devolve os tokens.
func connectClaude(t *testing.T, e *echo.Echo, acct account) (access, refresh string) {
	t.Helper()
	code, p := authorizeCode(t, e, acct, claudeID, claudeCallback)
	return tokenOK(t, tokenRequest(e, exchangeForm(code, p, claudeID, claudeCallback)))
}

// mcpStatus é o status de um initialize no /mcp com o token.
func mcpStatus(e *echo.Echo, token string) int {
	return rawMCP(e, map[string]string{"Authorization": "Bearer " + token}, "").Code
}

func TestOAuth_DiscoveryDocuments(t *testing.T) {
	e := newOAuthServer(t)

	rec := do(e, "GET", "/.well-known/oauth-protected-resource/mcp", "", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("resource metadata = %d (CORS %q): %s", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"), rec.Body.String())
	}
	prm := decode(t, rec)
	if prm["resource"] != oauthResource || fmt.Sprint(prm["authorization_servers"]) != "["+oauthIssuer+"]" ||
		fmt.Sprint(prm["scopes_supported"]) != "[mcp]" || fmt.Sprint(prm["bearer_methods_supported"]) != "[header]" {
		t.Errorf("resource metadata %s", rec.Body.String())
	}

	rec = do(e, "GET", "/.well-known/oauth-authorization-server", "", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("server metadata = %d: %s", rec.Code, rec.Body.String())
	}
	want := map[string]any{
		"issuer":                                         oauthIssuer,
		"authorization_endpoint":                         oauthIssuer + "/oauth/authorize",
		"token_endpoint":                                 oauthIssuer + "/oauth/token",
		"revocation_endpoint":                            oauthIssuer + "/oauth/revoke",
		"response_types_supported":                       "[code]",
		"response_modes_supported":                       "[query]",
		"grant_types_supported":                          "[authorization_code refresh_token]",
		"code_challenge_methods_supported":               "[S256]",
		"token_endpoint_auth_methods_supported":          "[none]",
		"revocation_endpoint_auth_methods_supported":     "[none]",
		"scopes_supported":                               "[mcp]",
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
	}
	got := decode(t, rec)
	for k, v := range want {
		g := got[k]
		if _, isList := g.([]any); isList {
			g = fmt.Sprint(g)
		}
		if g != v {
			t.Errorf("server metadata %s = %v, want %v", k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("server metadata has %s, which is not expected (no DCR, no jwks)", k)
		}
	}

	// O preflight do CORS do token responde sem cookie nem corpo.
	rec = do(e, "OPTIONS", "/oauth/token", "", "")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("token preflight = %d (CORS %q)", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// O 401 do /mcp aponta para os metadados do recurso só com o OAuth ligado.
func TestOAuth_MCPChallengePointsToTheMetadata(t *testing.T) {
	e := newOAuthServer(t)
	rec := rawMCP(e, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("mcp without token = %d", rec.Code)
	}
	want := `Bearer resource_metadata="` + oauthIssuer + `/.well-known/oauth-protected-resource/mcp", scope="mcp"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}

	off := newServer(t)
	if got := rawMCP(off, nil, "").Header().Get("WWW-Authenticate"); strings.Contains(got, "resource_metadata") {
		t.Errorf("WWW-Authenticate with OAuth off = %q, want no resource_metadata", got)
	}
}

// Um documento de descoberta que não existe é um 404 em JSON, para quem está logado ou não e com o OAuth ligado ou não:
// nunca o redirecionamento para o login nem a página de 404.
func TestOAuth_UnknownWellKnownIsJSON404(t *testing.T) {
	for name, e := range map[string]*echo.Echo{"on": newOAuthServer(t), "off": newServer(t)} {
		acct := signup(t, e, "Org "+name, name+"@test.com")
		paths := []string{"/.well-known/openid-configuration", "/.well-known/oauth-protected-resource"}
		if name == "off" {
			paths = append(paths, "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp")
		}
		for _, path := range paths {
			for _, session := range []string{"", acct.session} {
				rec := do(e, "GET", path, "", session)
				if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
					t.Errorf("OAuth %s, GET %s (logged in: %v) = %d %q", name, path, session != "", rec.Code, rec.Header().Get("Content-Type"))
				}
			}
		}
	}
}

// Com o OAuth ligado, as rotas dele existem; desligado, não.
func TestOAuth_RoutesOnlyWhenEnabled(t *testing.T) {
	routes := func(e *echo.Echo) []string {
		var out []string
		for _, r := range e.Router().Routes() {
			if strings.HasPrefix(r.Path, "/oauth") || strings.HasPrefix(r.Path, "/api/oauth") || strings.HasPrefix(r.Path, "/.well-known") {
				out = append(out, r.Method+" "+r.Path)
			}
		}
		slices.Sort(out)
		return out
	}
	on := routes(newOAuthServer(t))
	for _, want := range []string{
		"GET /.well-known/oauth-protected-resource/mcp", "GET /.well-known/oauth-authorization-server",
		"POST /oauth/token", "OPTIONS /oauth/token", "POST /oauth/revoke", "GET /oauth/authorize", "POST /api/oauth/consent",
	} {
		if !slices.Contains(on, want) {
			t.Errorf("OAuth on: route %s missing (have %v)", want, on)
		}
	}
	for _, r := range routes(newServer(t)) {
		if strings.Contains(r, "token") || strings.Contains(r, "consent") || strings.Contains(r, "oauth-authorization-server") {
			t.Errorf("OAuth off: route %s exists", r)
		}
	}
}

// Quem não está logado passa pelo login e volta com o pedido inteiro.
func TestOAuth_AuthorizeGoesThroughTheLogin(t *testing.T) {
	e := newOAuthServer(t)
	q := authorizeQuery(claudeID, claudeCallback, "xyz", newPKCE(t))
	uri := "/oauth/authorize?" + q.Encode()
	rec := do(e, "GET", uri, "", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("authorize without session = %d, want 303 to the login", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Path != "/login" || loc.Query().Get("next") != uri {
		t.Errorf("Location = %s, want /login?next=%s", rec.Header().Get("Location"), uri)
	}
	if rec := do(e, "GET", loc.String(), "", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"next":"/oauth/authorize?`) {
		t.Errorf("login page with next = %d, want the authorize request in BOOT.next", rec.Code)
	}
}

// A tela: os cabeçalhos que a protegem, quem está autorizando, para onde volta e sem a barra superior.
func TestOAuth_ConsentPage(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org Alfa", "ana@test.com")
	q := authorizeQuery(claudeID, claudeCallback, "xyz", newPKCE(t))
	rec := do(e, "GET", "/oauth/authorize?"+q.Encode(), "", acct.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorize = %d: %.300s", rec.Code, rec.Body.String())
	}
	for header, want := range map[string]string{
		"X-Frame-Options": "DENY", "Content-Security-Policy": "frame-ancestors 'none'",
		"Referrer-Policy": "no-referrer", "Cache-Control": "no-store",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	body := rec.Body.String()
	for _, want := range []string{"Claude quer acessar a sua conta", "ana@test.com", "Org Alfa", "claude.ai", "Permitir", "Cancelar"} {
		if !strings.Contains(body, want) {
			t.Errorf("consent page does not have %q", want)
		}
	}
	if strings.Contains(body, `class="topbar"`) || strings.Contains(body, "computador") {
		t.Error("consent page has the top bar or the loopback warning")
	}

	// O Claude Code volta para uma porta qualquer do próprio computador: a tela avisa.
	q = authorizeQuery(claudeCodeID, "http://localhost:54321/callback", "xyz", newPKCE(t))
	rec = do(e, "GET", "/oauth/authorize?"+q.Encode(), "", acct.session)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Claude Code quer acessar") ||
		!strings.Contains(rec.Body.String(), "programa neste computador") || !strings.Contains(rec.Body.String(), "localhost:54321") {
		t.Errorf("Claude Code consent = %d, want the loopback warning: %.200s", rec.Code, rec.Body.String())
	}

	// Em inglês, nada de português.
	body = getPage(e, "/oauth/authorize?"+q.Encode(), acct.session, "en", "").Body.String()
	if !strings.Contains(body, "Claude Code wants to access your account") {
		t.Error("consent page is not in English")
	}
	for _, word := range []string{"Permitir", "Cancelar", "quer acessar", "Entrar com outra conta"} {
		if strings.Contains(body, word) {
			t.Errorf("English consent page still has %q", word)
		}
	}
}

// Um app desconhecido, ou um endereço de volta que não é dele, para na página de erro, sem redirecionar: o erro não
// pode ir para um endereço que não se sabe de quem é.
func TestOAuth_AuthorizeRefusesUnknownAppOrRedirect(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	p := newPKCE(t)
	cases := map[string]url.Values{
		"unknown app":        authorizeQuery("https://evil.test/meta", claudeCallback, "x", p),
		"other redirect":     authorizeQuery(claudeID, "https://evil.test/cb", "x", p),
		"no client":          authorizeQuery("", claudeCallback, "x", p),
		"loopback lookalike": authorizeQuery(claudeCodeID, "http://localhost.evil.test:5000/callback", "x", p),
		"loopback on https":  authorizeQuery(claudeCodeID, "https://localhost:5000/callback", "x", p),
		"code app on claude": authorizeQuery(claudeCodeID, claudeCallback, "x", p),
	}
	two := authorizeQuery(claudeID, claudeCallback, "x", p)
	two.Add("redirect_uri", "https://evil.test/cb")
	cases["two redirects"] = two
	for name, q := range cases {
		rec := do(e, "GET", "/oauth/authorize?"+q.Encode(), "", acct.session)
		if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" || !strings.Contains(rec.Body.String(), "Não deu para conectar") {
			t.Errorf("%s: authorize = %d (Location %q), want the error page", name, rec.Code, rec.Header().Get("Location"))
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: error page can be framed", name)
		}
	}
}

// Os outros erros voltam ao app, com o state e o iss.
func TestOAuth_AuthorizeErrorsGoBackToTheApp(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	for name, c := range map[string]struct {
		change func(url.Values)
		code   string
	}{
		"no PKCE":        {func(q url.Values) { q.Del("code_challenge") }, "invalid_request"},
		"plain PKCE":     {func(q url.Values) { q.Set("code_challenge_method", "plain") }, "invalid_request"},
		"implicit":       {func(q url.Values) { q.Set("response_type", "token") }, "unsupported_response_type"},
		"other resource": {func(q url.Values) { q.Set("resource", "https://evil.test/mcp") }, "invalid_target"},
	} {
		q := authorizeQuery(claudeID, claudeCallback, "xyz", newPKCE(t))
		c.change(q)
		rec := do(e, "GET", "/oauth/authorize?"+q.Encode(), "", acct.session)
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusFound || loc == nil || loc.Scheme+"://"+loc.Host+loc.Path != claudeCallback ||
			loc.Query().Get("error") != c.code || loc.Query().Get("state") != "xyz" || loc.Query().Get("iss") != oauthIssuer {
			t.Errorf("%s: authorize = %d %s, want a redirect with error=%s", name, rec.Code, rec.Header().Get("Location"), c.code)
		}
	}
}

// O "Permitir" volta com o código; o "Cancelar", com o access_denied e sem concessão.
func TestOAuth_ConsentAllowAndDeny(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	q := authorizeQuery(claudeID, claudeCallback, "xyz", newPKCE(t))

	denied := consentRedirect(t, e, acct, q, false)
	if denied.Query().Get("error") != "access_denied" || denied.Query().Get("code") != "" ||
		denied.Query().Get("state") != "xyz" || denied.Query().Get("iss") != oauthIssuer {
		t.Errorf("deny redirect %s", denied)
	}
	var n int
	if err := testDB.QueryRow(`SELECT count(*) FROM oauth_grants`).Scan(&n); err != nil || n != 0 {
		t.Errorf("grants after deny = %d (%v), want 0", n, err)
	}

	allowed := consentRedirect(t, e, acct, q, true)
	if allowed.Scheme+"://"+allowed.Host+allowed.Path != claudeCallback || allowed.Query().Get("code") == "" ||
		allowed.Query().Get("state") != "xyz" || allowed.Query().Get("iss") != oauthIssuer || allowed.Query().Get("error") != "" {
		t.Errorf("allow redirect %s", allowed)
	}
	var person string
	if err := testDB.QueryRow(`SELECT person_id FROM oauth_grants`).Scan(&person); err != nil || person != acct.id {
		t.Errorf("grant person = %q (%v), want the session's %s", person, err, acct.id)
	}
}

// O "Permitir" é só da tela daqui: de outro site, sem JSON ou sem sessão, é recusado; um pedido que não confere mais
// também.
func TestOAuth_ConsentProtections(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	q := authorizeQuery(claudeID, claudeCallback, "xyz", newPKCE(t))
	body := consentBody(q, true)

	send := func(headers map[string]string, session string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/oauth/consent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if session != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	if rec := send(map[string]string{"Sec-Fetch-Site": "cross-site"}, acct.session); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site consent = %d, want 403", rec.Code)
	}
	if rec := send(map[string]string{"Origin": "https://evil.test"}, acct.session); rec.Code != http.StatusForbidden {
		t.Errorf("consent from another origin = %d, want 403", rec.Code)
	}
	if rec := send(map[string]string{"Content-Type": "text/plain"}, acct.session); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("consent as text/plain = %d, want 415", rec.Code)
	}
	if rec := send(nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("consent without session = %d, want 401", rec.Code)
	}
	q.Set("redirect_uri", "https://evil.test/cb")
	rec := do(e, "POST", "/api/oauth/consent", consentBody(q, true), acct.session)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "oauth.request_invalid" {
		t.Errorf("consent with another redirect = %d %s, want 400 oauth.request_invalid", rec.Code, rec.Body.String())
	}
	if rec := send(map[string]string{"Sec-Fetch-Site": "same-origin"}, acct.session); rec.Code != http.StatusOK {
		t.Errorf("same-origin consent = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

// A troca do código dá os tokens; o token de acesso abre o /mcp, e só ele: a API continua só com o cookie.
func TestOAuth_ExchangeAndUse(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	access, _ := connectClaude(t, e, acct)

	if got := mcpStatus(e, access); got != http.StatusOK {
		t.Errorf("mcp with the access token = %d, want 200", got)
	}
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("/api with the access token = %d, want 401", rec.Code)
	}

	// Vencido, ou de outro recurso, o token não entra.
	if _, err := testDB.Exec(`UPDATE oauth_grants SET resource = 'https://other.test/mcp'`); err != nil {
		t.Fatal(err)
	}
	if got := mcpStatus(e, access); got != http.StatusUnauthorized {
		t.Errorf("mcp with a token of another resource = %d, want 401", got)
	}
	if _, err := testDB.Exec(`UPDATE oauth_grants SET resource = $1`, oauthResource); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Exec(`UPDATE oauth_access_tokens SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if got := mcpStatus(e, access); got != http.StatusUnauthorized {
		t.Errorf("mcp with an expired token = %d, want 401", got)
	}

	// Com o OAuth desligado, um token de acesso não entra.
	_, _ = testDB.Exec(`UPDATE oauth_access_tokens SET expires_at = now() + interval '1 hour'`)
	if got := mcpStatus(newServerKeepingData(t), access); got != http.StatusUnauthorized {
		t.Errorf("mcp with OAuth off = %d, want 401", got)
	}
}

// newServerKeepingData é um servidor com o OAuth desligado sobre o mesmo banco, sem limpar.
func newServerKeepingData(t *testing.T) *echo.Echo {
	t.Helper()
	e, err := server.New(testClient, server.Options{EncryptKey: "test-key", AuthRateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// A troca do código só vale com o mesmo app, o mesmo endereço de volta, o verificador certo e o /mcp; o código se gasta
// na primeira tentativa, mesmo errada.
func TestOAuth_ExchangeRefusals(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")

	for name, c := range map[string]struct {
		spoil  func(url.Values)
		status int
		code   string
	}{
		"wrong verifier":  {func(f url.Values) { f.Set("code_verifier", newPKCE(t).verifier) }, 400, "invalid_grant"},
		"wrong redirect":  {func(f url.Values) { f.Set("redirect_uri", claudeCallback+"x") }, 400, "invalid_grant"},
		"another app":     {func(f url.Values) { f.Set("client_id", claudeCodeID) }, 400, "invalid_grant"},
		"other resource":  {func(f url.Values) { f.Set("resource", "https://evil.test/mcp") }, 400, "invalid_target"},
		"unknown app":     {func(f url.Values) { f.Set("client_id", "https://evil.test/meta") }, 401, "invalid_client"},
		"no verifier":     {func(f url.Values) { f.Del("code_verifier") }, 400, "invalid_request"},
		"no grant type":   {func(f url.Values) { f.Del("grant_type") }, 400, "invalid_request"},
		"password grant":  {func(f url.Values) { f.Set("grant_type", "password") }, 400, "unsupported_grant_type"},
		"repeated code":   {func(f url.Values) { f.Add("code", "x") }, 400, "invalid_request"},
		"two resources":   {func(f url.Values) { f.Add("resource", oauthResource) }, 400, "invalid_target"},
		"resource+query":  {func(f url.Values) { f.Set("resource", oauthResource+"?x=1") }, 400, "invalid_target"},
		"canonical match": {func(f url.Values) { f.Set("resource", "https://WTT.test:443/mcp/") }, 200, ""},
	} {
		t.Run(name, func(t *testing.T) {
			code, p := authorizeCode(t, e, acct, claudeID, claudeCallback)
			form := exchangeForm(code, p, claudeID, claudeCallback)
			c.spoil(form)
			rec := tokenRequest(e, form)
			if c.status == 200 {
				tokenOK(t, rec)
				return
			}
			tokenError(t, rec, c.status, c.code)
			if c.code == "invalid_grant" {
				// O código se gastou: nem a troca certa vale mais.
				tokenError(t, tokenRequest(e, exchangeForm(code, p, claudeID, claudeCallback)), 400, "invalid_grant")
			}
		})
	}

	// Um corpo que não é formulário, ou os parâmetros na URL, não valem.
	code, p := authorizeCode(t, e, acct, claudeID, claudeCallback)
	form := exchangeForm(code, p, claudeID, claudeCallback)
	rec := do(e, "POST", "/oauth/token", `{"grant_type":"authorization_code"}`, "")
	tokenError(t, rec, 400, "invalid_request")
	req := httptest.NewRequest("POST", "/oauth/token?"+form.Encode(), nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	tokenError(t, rec, 401, "invalid_client")

	// O código vence em dois minutos.
	if _, err := testDB.Exec(`UPDATE oauth_grants SET code_expires_at = now() - interval '1 second' WHERE refresh_hash IS NULL`); err != nil {
		t.Fatal(err)
	}
	tokenError(t, tokenRequest(e, form), 400, "invalid_grant")
}

// Um código usado duas vezes derruba a concessão: o token que a primeira troca deu deixa de valer.
func TestOAuth_CodeReuseRevokesTheGrant(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	code, p := authorizeCode(t, e, acct, claudeID, claudeCallback)
	access, refresh := tokenOK(t, tokenRequest(e, exchangeForm(code, p, claudeID, claudeCallback)))
	tokenError(t, tokenRequest(e, exchangeForm(code, p, claudeID, claudeCallback)), 400, "invalid_grant")
	if got := mcpStatus(e, access); got != http.StatusUnauthorized {
		t.Errorf("mcp after the code was reused = %d, want 401", got)
	}
	tokenError(t, tokenRequest(e, refreshForm(refresh, claudeID)), 400, "invalid_grant")
}

// A renovação gira o token e empurra a validade da conexão. O token anterior, dentro de um minuto, recebe o mesmo
// sucessor (dois processos renovando juntos); depois disso, é reuso e derruba a conexão.
func TestOAuth_RefreshRotates(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	access1, refresh1 := connectClaude(t, e, acct)
	if _, err := testDB.Exec(`UPDATE oauth_grants SET refresh_expires_at = now() + interval '1 day'`); err != nil {
		t.Fatal(err)
	}

	access2, refresh2 := tokenOK(t, tokenRequest(e, refreshForm(refresh1, claudeID)))
	if refresh2 == refresh1 || access2 == access1 {
		t.Fatal("the refresh did not rotate the tokens")
	}
	var expires time.Time
	if err := testDB.QueryRow(`SELECT refresh_expires_at FROM oauth_grants`).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if time.Until(expires) < 29*24*time.Hour {
		t.Errorf("connection expires at %v, want 30 days from now", expires)
	}
	for _, tok := range []string{access1, access2} {
		if got := mcpStatus(e, tok); got != http.StatusOK {
			t.Errorf("mcp with %s… = %d, want 200 (an access token lasts until it expires)", tok[:9], got)
		}
	}

	// Dentro da janela, o token anterior recebe o mesmo sucessor e um token de acesso novo.
	access3, replay := tokenOK(t, tokenRequest(e, refreshForm(refresh1, claudeID)))
	if replay != refresh2 || access3 == access2 {
		t.Errorf("replay inside the window gave refresh %s… (want the successor %s…)", replay[:9], refresh2[:9])
	}
	if got := mcpStatus(e, access3); got != http.StatusOK {
		t.Errorf("mcp with the replayed access token = %d", got)
	}

	// Fora da janela, o token anterior é reuso: a conexão cai inteira.
	if _, err := testDB.Exec(`UPDATE oauth_grants SET rotated_at = now() - interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	tokenError(t, tokenRequest(e, refreshForm(refresh1, claudeID)), 400, "invalid_grant")
	if got := mcpStatus(e, access2); got != http.StatusUnauthorized {
		t.Errorf("mcp after the reuse = %d, want 401", got)
	}
	tokenError(t, tokenRequest(e, refreshForm(refresh2, claudeID)), 400, "invalid_grant")
}

// A renovação só vale para o mesmo app e o /mcp; um token de renovação vencido derruba a conexão.
func TestOAuth_RefreshRefusals(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	_, refresh := connectClaude(t, e, acct)

	tokenError(t, tokenRequest(e, refreshForm(refresh, claudeCodeID)), 400, "invalid_grant")
	other := refreshForm(refresh, claudeID)
	other.Set("resource", "https://evil.test/mcp")
	tokenError(t, tokenRequest(e, other), 400, "invalid_target")
	tokenError(t, tokenRequest(e, refreshForm("wttr_nope", claudeID)), 400, "invalid_grant")
	tokenError(t, tokenRequest(e, refreshForm("", claudeID)), 400, "invalid_request")
	// Nada disso derrubou a conexão.
	_, refresh = tokenOK(t, tokenRequest(e, refreshForm(refresh, claudeID)))

	if _, err := testDB.Exec(`UPDATE oauth_grants SET refresh_expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	tokenError(t, tokenRequest(e, refreshForm(refresh, claudeID)), 400, "invalid_grant")
	var n int
	if err := testDB.QueryRow(`SELECT count(*) FROM oauth_grants`).Scan(&n); err != nil || n != 0 {
		t.Errorf("grants after an expired refresh = %d (%v), want 0", n, err)
	}
}

// O /oauth/token tem o próprio limite por IP, com o erro do OAuth.
func TestOAuth_TokenRateLimit(t *testing.T) {
	e := newOAuthServer(t, func(o *server.Options) { o.OAuthRateLimit = 0.001 })
	tokenError(t, tokenRequest(e, refreshForm("wttr_nope", claudeID)), 400, "invalid_grant")
	tokenError(t, tokenRequest(e, refreshForm("wttr_nope", claudeID)), 429, "temporarily_unavailable")
}

// Desconectar no perfil, revogar pelo /oauth/revoke ou trocar a senha corta o acesso na hora. O token pessoal continua
// valendo com o OAuth ligado.
func TestOAuth_RevocationCutsAccess(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	other := invite(t, e, acct, "bia@test.com", "member")

	access, refresh := connectClaude(t, e, acct)
	list := decodeList(t, do(e, "GET", "/api/auth/connections", "", acct.session))
	if len(list) != 1 || list[0]["name"] != "Claude" || list[0]["client_id"] != claudeID || list[0]["expires_at"] == nil {
		t.Fatalf("connections = %v", list)
	}
	id := list[0]["id"].(string)
	if rec := do(e, "DELETE", "/api/auth/connections/"+id, "", other.session); rec.Code != http.StatusNotFound || errorCode(t, rec) != "auth.connection_not_found" {
		t.Errorf("disconnect someone else's app = %d %s, want 404", rec.Code, rec.Body.String())
	}
	if got := decodeList(t, do(e, "GET", "/api/auth/connections", "", other.session)); len(got) != 0 {
		t.Errorf("the other person sees %v", got)
	}
	if rec := do(e, "DELETE", "/api/auth/connections/"+id, "", acct.session); rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect = %d", rec.Code)
	}
	if got := mcpStatus(e, access); got != http.StatusUnauthorized {
		t.Errorf("mcp after disconnecting = %d, want 401", got)
	}
	tokenError(t, tokenRequest(e, refreshForm(refresh, claudeID)), 400, "invalid_grant")

	// O /oauth/revoke: o token de acesso sai sozinho; o de renovação leva a conexão.
	access, refresh = connectClaude(t, e, acct)
	revoke := func(token string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/oauth/revoke", strings.NewReader(url.Values{"token": {token}, "client_id": {claudeID}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("revoke = %d %s", rec.Code, rec.Body.String())
		}
	}
	revoke("wtta_does-not-exist")
	revoke(access)
	if got := mcpStatus(e, access); got != http.StatusUnauthorized {
		t.Errorf("mcp after revoking the access token = %d, want 401", got)
	}
	access, _ = tokenOK(t, tokenRequest(e, refreshForm(refresh, claudeID)))
	if got := mcpStatus(e, access); got != http.StatusOK {
		t.Errorf("mcp with a new access token = %d, want 200", got)
	}
	_, refresh2 := connectClaude(t, e, acct)
	revoke(refresh2)
	if got := len(decodeList(t, do(e, "GET", "/api/auth/connections", "", acct.session))); got != 1 {
		t.Errorf("connections after revoking a refresh token = %d, want 1", got)
	}

	// O token pessoal continua.
	_, personal := createToken(t, e, acct, "Claude Code")
	if got := mcpStatus(e, personal); got != http.StatusOK {
		t.Errorf("mcp with the personal token = %d, want 200", got)
	}

	// Trocar a senha desconecta tudo.
	if rec := do(e, "POST", "/api/auth/password", `{"current_password":"senha-forte-1","new_password":"senha-nova-123"}`, acct.session); rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("change password = %d %s", rec.Code, rec.Body.String())
	}
	if got := mcpStatus(e, access); got != http.StatusUnauthorized {
		t.Errorf("mcp after the password change = %d, want 401", got)
	}
	if got := len(decodeList(t, do(e, "GET", "/api/auth/connections", "", acct.session))); got != 0 {
		t.Errorf("connections after the password change = %d, want 0", got)
	}
}

// A lista mostra só as conexões de verdade (a autorização que o app ainda não trocou fica de fora), e cada pessoa tem
// no máximo dez: a que está há mais tempo sem uso sai.
func TestOAuth_ConnectionsListAndLimit(t *testing.T) {
	e := newOAuthServer(t)
	acct := signup(t, e, "Org", "ana@test.com")
	connectClaude(t, e, acct)
	code, p := authorizeCode(t, e, acct, claudeCodeID, "http://127.0.0.1:3118/callback")
	tokenOK(t, tokenRequest(e, exchangeForm(code, p, claudeCodeID, "http://127.0.0.1:3118/callback")))
	authorizeCode(t, e, acct, claudeID, claudeCallback) // autorizado, não trocado

	list := decodeList(t, do(e, "GET", "/api/auth/connections", "", acct.session))
	var names []string
	for _, c := range list {
		names = append(names, c["name"].(string))
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "Claude,Claude Code" {
		t.Errorf("connections = %v, want Claude and Claude Code", names)
	}

	// A mais antiga sem uso é a primeira do Claude; ela sai quando a pessoa passa de dez.
	var oldest string
	if err := testDB.QueryRow(`SELECT id FROM oauth_grants WHERE client_id = $1 AND refresh_hash IS NOT NULL`, claudeID).Scan(&oldest); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Exec(`UPDATE oauth_grants SET last_used_at = now() - interval '10 days' WHERE id = $1`, oldest); err != nil {
		t.Fatal(err)
	}
	for range 9 {
		connectClaude(t, e, acct)
	}
	var total, kept int
	if err := testDB.QueryRow(`SELECT count(*), count(*) FILTER (WHERE id = $1) FROM oauth_grants`, oldest).Scan(&total, &kept); err != nil {
		t.Fatal(err)
	}
	if total != 10 || kept != 0 {
		t.Errorf("grants = %d (oldest kept: %d), want 10 without the oldest", total, kept)
	}
}

// De ponta a ponta com o cliente OAuth do próprio SDK do MCP, como um cliente de verdade: o 401 do /mcp, a descoberta,
// a identidade publicada do Claude Code, a tela de consentimento, o iss, a troca com PKCE e a ferramenta chamada com o
// token de acesso.
func TestOAuth_EndToEndWithTheSDKClient(t *testing.T) {
	var e *echo.Echo
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	e = newOAuthServer(t, func(o *server.Options) { o.PublicURL = srv.URL })
	acct := signup(t, e, "Org", "ana@test.com")
	cookie := &http.Cookie{Name: auth.CookieName, Value: acct.session}

	var screens int
	handler, err := mcpauth.NewAuthorizationCodeHandler(&mcpauth.AuthorizationCodeHandlerConfig{
		ClientIDMetadataDocumentConfig: &mcpauth.ClientIDMetadataDocumentConfig{URL: claudeCodeID},
		RedirectURL:                    "http://localhost:43210/callback",
		AuthorizationCodeFetcher: func(ctx context.Context, args *mcpauth.AuthorizationArgs) (*mcpauth.AuthorizationResult, error) {
			// O navegador: abre a tela com a sessão e clica em Permitir.
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
			req.AddCookie(cookie)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			page, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK || !strings.Contains(string(page), "Claude Code") {
				return nil, fmt.Errorf("consent page = %d", res.StatusCode)
			}
			screens++
			u, _ := url.Parse(args.URL)
			req, _ = http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/oauth/consent", strings.NewReader(consentBody(u.Query(), true)))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			res, err = http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer func() { _ = res.Body.Close() }()
			var out struct{ Redirect string }
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil || res.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("consent = %d: %v", res.StatusCode, err)
			}
			back, err := url.Parse(out.Redirect)
			if err != nil || back.Host != "localhost:43210" {
				return nil, fmt.Errorf("redirect %q", out.Redirect)
			}
			return &mcpauth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state"), Iss: back.Query().Get("iss")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", OAuthHandler: handler, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	me := callTool(t, cs, "whoami", nil)
	if fmt.Sprint(me["email"]) != "ana@test.com" && !strings.Contains(fmt.Sprint(me), "ana@test.com") {
		t.Errorf("whoami = %v, want ana", me)
	}
	if screens != 1 {
		t.Errorf("consent screens = %d, want 1", screens)
	}
	if list := decodeList(t, do(e, "GET", "/api/auth/connections", "", acct.session)); len(list) != 1 || list[0]["name"] != "Claude Code" {
		t.Errorf("connections = %v, want Claude Code", list)
	}
}
