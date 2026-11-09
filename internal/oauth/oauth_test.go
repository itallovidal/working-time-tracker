package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"working-time-tracker/internal/domain/auth"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// O recurso que o app manda é comparado ao /mcp na forma canônica: esquema e nome em minúsculas, sem a porta padrão e
// sem barra no fim. Um endereço com query, fragmento ou usuário não é recurso.
func TestCanonicalResource(t *testing.T) {
	for raw, want := range map[string]string{
		"https://wtt.example.com/mcp":      "https://wtt.example.com/mcp",
		"https://WTT.Example.com/mcp/":     "https://wtt.example.com/mcp",
		"HTTPS://wtt.example.com:443/mcp":  "https://wtt.example.com/mcp",
		"https://wtt.example.com:8443/mcp": "https://wtt.example.com:8443/mcp",
		"http://localhost:80/mcp":          "http://localhost/mcp",
		"http://127.0.0.1:1243/mcp":        "http://127.0.0.1:1243/mcp",
		"http://[::1]:1243/mcp":            "http://[::1]:1243/mcp",
	} {
		got, err := CanonicalResource(raw)
		if err != nil || got != want {
			t.Errorf("CanonicalResource(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "/mcp", "ftp://wtt.example.com/mcp", "https://wtt.example.com/mcp?x=1",
		"https://wtt.example.com/mcp#a", "https://user@wtt.example.com/mcp", "https://wtt.example.com/mcp?"} {
		if got, err := CanonicalResource(raw); err == nil {
			t.Errorf("CanonicalResource(%q) = %q, want an error", raw, got)
		}
	}
}

// O endereço de volta tem de ser um dos do app, exato. No http do próprio computador (o Claude Code) a porta não conta,
// mas o nome, o caminho e a query, sim.
func TestMatchRedirect(t *testing.T) {
	claude := []string{"https://claude.ai/api/mcp/auth_callback"}
	code := []string{"http://localhost/callback", "http://127.0.0.1/callback"}
	cases := []struct {
		registered []string
		got        string
		want       bool
	}{
		{claude, "https://claude.ai/api/mcp/auth_callback", true},
		{claude, "https://claude.ai/api/mcp/auth_callback/", false},
		{claude, "https://claude.ai/api/mcp/auth_callback?x=1", false},
		{claude, "https://claude.ai:8443/api/mcp/auth_callback", false},
		{claude, "https://claude.ai.evil.com/api/mcp/auth_callback", false},
		{claude, "http://claude.ai/api/mcp/auth_callback", false},
		{claude, "https://claude.ai/api/mcp/auth_callback#frag", false},
		{code, "http://localhost/callback", true},
		{code, "http://localhost:54321/callback", true},
		{code, "http://127.0.0.1:3118/callback", true},
		{code, "http://[::1]:3118/callback", false}, // o documento não tem o [::1]
		{code, "http://localhost:54321/other", false},
		{code, "http://localhost:54321/callback?x=1", false},
		{code, "http://localhost.evil.com:54321/callback", false},
		{code, "https://localhost:54321/callback", false},
		{code, "http://user@localhost:54321/callback", false},
		{code, "", false},
	}
	for _, tc := range cases {
		if got := MatchRedirect(tc.registered, tc.got); got != tc.want {
			t.Errorf("MatchRedirect(%v, %q) = %v, want %v", tc.registered, tc.got, got, tc.want)
		}
	}
}

// O PKCE só aceita o S256: o verificador tem de 43 a 128 caracteres sem reserva, e o desafio é o sha256 dele em
// base64url sem preenchimento.
func TestPKCE(t *testing.T) {
	verifier := strings.Repeat("abc-._~XYZ", 5) // 50 caracteres
	challenge := s256(verifier)
	if !auth.ValidPKCEChallenge(challenge) {
		t.Fatalf("challenge %q is not valid", challenge)
	}
	if !auth.PKCEMatches(verifier, challenge) {
		t.Error("the right verifier does not match")
	}
	for _, v := range []string{verifier + "x", strings.Repeat("a", 42), strings.Repeat("a", 129), verifier[:43] + "+"} {
		if auth.PKCEMatches(v, s256(verifier)) {
			t.Errorf("verifier %q matches, want no", v)
		}
	}
	if auth.PKCEMatches(strings.Repeat("a", 42), s256(strings.Repeat("a", 42))) {
		t.Error("a 42-character verifier matches, want no (the minimum is 43)")
	}
	for _, c := range []string{"", challenge[:42], challenge + "A", strings.Repeat("=", 43)} {
		if auth.ValidPKCEChallenge(c) {
			t.Errorf("challenge %q is valid, want no", c)
		}
	}
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// O documento de identidade fica guardado pelo max-age, entre 5 minutos e 24 horas; sem max-age, uma hora.
func TestCacheTTL(t *testing.T) {
	for header, want := range map[string]time.Duration{
		"":                          time.Hour,
		"public, max-age=300":       5 * time.Minute,
		"max-age=60":                5 * time.Minute,
		"max-age=7200":              2 * time.Hour,
		"max-age=999999":            24 * time.Hour,
		"no-store":                  5 * time.Minute,
		"public, max-age=nonsense":  time.Hour,
		"Public, MAX-AGE=1800, foo": 30 * time.Minute,
	} {
		if got := cacheTTL(header); got != want {
			t.Errorf("cacheTTL(%q) = %v, want %v", header, got, want)
		}
	}
}

// O registro busca o documento de identidade do app e o guarda; o client_id do documento tem de ser o próprio endereço,
// e o app, público. Quando a busca falha (o servidor não responde, o documento não serve, um redirecionamento), vale o
// último documento que veio ou a cópia embutida. Um client_id fora da lista nem é buscado.
func TestRegistry_FetchesCachesAndFallsBack(t *testing.T) {
	var hits atomic.Int32
	var doc atomic.Value
	doc.Store(`{"client_id":"%s","client_name":"Claude (atual)","redirect_uris":["https://claude.example/cb"],"response_types":["code"],"grant_types":["authorization_code","refresh_token","urn:ietf:params:oauth:grant-type:jwt-bearer"],"token_endpoint_auth_method":"none"}`)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/meta", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=600")
		_, _ = io.WriteString(w, strings.ReplaceAll(doc.Load().(string), "%s", "https://"+r.Host+r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	id := srv.URL + "/meta"
	client := &http.Client{
		Transport:     srv.Client().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	pinned := Client{ID: id, Name: "Claude", RedirectURIs: []string{"https://claude.example/pinned"}}
	reg := NewRegistry([]Client{pinned, {ID: srv.URL + "/redirect", Name: "Outro", RedirectURIs: []string{"https://x.example/cb"}}}, client, quiet)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reg.now = func() time.Time { return now }
	ctx := context.Background()

	got, err := reg.Lookup(ctx, id)
	if err != nil || got.Name != "Claude (atual)" || got.RedirectURIs[0] != "https://claude.example/cb" {
		t.Fatalf("Lookup = %+v, %v; want the fetched document", got, err)
	}
	if _, err := reg.Lookup(ctx, id); err != nil || hits.Load() != 1 {
		t.Errorf("second Lookup fetched again (%d hits), want the cached copy", hits.Load())
	}

	// Venceu o max-age: busca de novo. Um documento que não serve (de um app com segredo) fica de fora, e vale o último.
	now = now.Add(11 * time.Minute)
	doc.Store(`{"client_id":"%s","client_name":"Mau","redirect_uris":["https://evil.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`)
	got, err = reg.Lookup(ctx, id)
	if err != nil || got.Name != "Claude (atual)" || hits.Load() != 2 {
		t.Errorf("Lookup with a bad document = %+v, %v (%d hits); want the last good copy", got, err, hits.Load())
	}

	// O redirecionamento não é seguido: vale a cópia embutida.
	got, err = reg.Lookup(ctx, srv.URL+"/redirect")
	if err != nil || got.Name != "Outro" || got.RedirectURIs[0] != "https://x.example/cb" {
		t.Errorf("Lookup through a redirect = %+v, %v; want the pinned copy", got, err)
	}

	if _, err := reg.Lookup(ctx, "https://evil.example/meta"); err != errUnknownClient {
		t.Errorf("Lookup of an unknown client = %v, want errUnknownClient", err)
	}
}

// Sem resposta do servidor do app, vale a cópia embutida.
func TestRegistry_PinnedCopyWhenUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // ninguém atende
	reg := NewRegistry([]Client{{ID: srv.URL + "/meta", Name: "Claude", RedirectURIs: []string{"https://claude.example/cb"}}}, nil, quiet)
	got, err := reg.Lookup(context.Background(), srv.URL+"/meta")
	if err != nil || got.Name != "Claude" || got.RedirectURIs[0] != "https://claude.example/cb" {
		t.Errorf("Lookup = %+v, %v; want the pinned copy", got, err)
	}
}

// O documento de identidade é conferido: o client_id é o endereço, o app é público e os endereços de volta são https
// ou http no próprio computador.
func TestValidDocument(t *testing.T) {
	id := "https://claude.ai/oauth/mcp-oauth-client-metadata"
	ok := clientDocument{ClientID: id, RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}, TokenEndpointAuthMethod: "none"}
	if err := validDocument(id, ok); err != nil {
		t.Errorf("valid document refused: %v", err)
	}
	bad := map[string]func(*clientDocument){
		"another client_id": func(d *clientDocument) { d.ClientID = "https://evil.example/meta" },
		"a secret":          func(d *clientDocument) { d.TokenEndpointAuthMethod = "client_secret_post" },
		"no code":           func(d *clientDocument) { d.ResponseTypes = []string{"token"} },
		"no redirect":       func(d *clientDocument) { d.RedirectURIs = nil },
		"http redirect":     func(d *clientDocument) { d.RedirectURIs = []string{"http://claude.ai/cb"} },
		"fragment":          func(d *clientDocument) { d.RedirectURIs = []string{"https://claude.ai/cb#x"} },
	}
	for name, spoil := range bad {
		d := ok
		d.RedirectURIs = append([]string(nil), ok.RedirectURIs...)
		spoil(&d)
		if err := validDocument(id, d); err == nil {
			t.Errorf("%s: document accepted, want refused", name)
		}
	}
}

// O pedido de autorização: o app e o endereço de volta decidem se o erro volta ao app (com state e iss) ou para na
// página; o resto pede response_type code, PKCE S256 e, se vier, o recurso do /mcp.
func TestParseAuthorize(t *testing.T) {
	s := New(Config{Issuer: "https://wtt.example.com", ResourcePath: "/mcp", Logger: quiet,
		Clients:    []Client{{ID: "https://claude.example/meta", Name: "Claude", RedirectURIs: []string{"https://claude.example/cb?app=1"}}},
		HTTPClient: &http.Client{Transport: failing{}}})
	challenge := s256(strings.Repeat("v", 43))
	base := func() url.Values {
		return url.Values{
			"response_type": {"code"}, "client_id": {"https://claude.example/meta"},
			"redirect_uri": {"https://claude.example/cb?app=1"}, "state": {"xyz"},
			"code_challenge": {challenge}, "code_challenge_method": {"S256"},
			"resource": {"https://WTT.example.com/mcp/"}, "scope": {"mcp offline_access"},
		}
	}

	req, failure := s.ParseAuthorize(context.Background(), base())
	if failure != nil {
		t.Fatalf("valid request refused: %+v", failure)
	}
	if req.Resource != "https://wtt.example.com/mcp" || req.Scope != "mcp" || req.ClientName != "Claude" || req.State != "xyz" {
		t.Errorf("request = %+v", req)
	}
	if req.RedirectHost() != "claude.example" || req.Loopback() {
		t.Errorf("redirect host %q loopback %v", req.RedirectHost(), req.Loopback())
	}
	noResource := base()
	noResource.Del("resource")
	noResource.Del("scope")
	if req, failure := s.ParseAuthorize(context.Background(), noResource); failure != nil || req.Resource != "https://wtt.example.com/mcp" {
		t.Errorf("request without resource = %+v, %+v; want the /mcp", req, failure)
	}

	// Sem volta para o app: a página mostra o erro.
	for name, c := range map[string]struct {
		change func(url.Values)
		reason string
	}{
		"no client":        {func(q url.Values) { q.Del("client_id") }, ReasonRequestInvalid},
		"two clients":      {func(q url.Values) { q.Add("client_id", "https://claude.example/meta") }, ReasonRequestInvalid},
		"unknown client":   {func(q url.Values) { q.Set("client_id", "https://evil.example/meta") }, ReasonClientUnknown},
		"no redirect":      {func(q url.Values) { q.Del("redirect_uri") }, ReasonRedirectInvalid},
		"other redirect":   {func(q url.Values) { q.Set("redirect_uri", "https://evil.example/cb") }, ReasonRedirectInvalid},
		"two redirects":    {func(q url.Values) { q.Add("redirect_uri", "https://claude.example/cb?app=1") }, ReasonRedirectInvalid},
		"redirect + query": {func(q url.Values) { q.Set("redirect_uri", "https://claude.example/cb?app=2") }, ReasonRedirectInvalid},
	} {
		q := base()
		c.change(q)
		_, failure := s.ParseAuthorize(context.Background(), q)
		if failure == nil || failure.Redirect != "" || failure.Reason != c.reason {
			t.Errorf("%s: failure = %+v, want the page with %s", name, failure, c.reason)
		}
	}

	// Com volta para o app: o erro vai no endereço de volta, que mantém a query dele, com o state e o iss.
	for name, c := range map[string]struct {
		change func(url.Values)
		code   string
	}{
		"implicit flow":   {func(q url.Values) { q.Set("response_type", "token") }, "unsupported_response_type"},
		"no PKCE":         {func(q url.Values) { q.Del("code_challenge") }, "invalid_request"},
		"plain PKCE":      {func(q url.Values) { q.Set("code_challenge_method", "plain") }, "invalid_request"},
		"no method":       {func(q url.Values) { q.Del("code_challenge_method") }, "invalid_request"},
		"bad challenge":   {func(q url.Values) { q.Set("code_challenge", "short") }, "invalid_request"},
		"two challenges":  {func(q url.Values) { q.Add("code_challenge", challenge) }, "invalid_request"},
		"other resource":  {func(q url.Values) { q.Set("resource", "https://evil.example/mcp") }, "invalid_target"},
		"two resources":   {func(q url.Values) { q.Add("resource", "https://wtt.example.com/mcp") }, "invalid_target"},
		"resource+query":  {func(q url.Values) { q.Set("resource", "https://wtt.example.com/mcp?x=1") }, "invalid_target"},
		"two states":      {func(q url.Values) { q.Add("state", "abc") }, "invalid_request"},
		"two scopes":      {func(q url.Values) { q.Add("scope", "mcp") }, "invalid_request"},
		"no responsetype": {func(q url.Values) { q.Del("response_type") }, "unsupported_response_type"},
	} {
		q := base()
		c.change(q)
		_, failure := s.ParseAuthorize(context.Background(), q)
		if failure == nil || failure.Redirect == "" {
			t.Errorf("%s: failure = %+v, want a redirect", name, failure)
			continue
		}
		u, _ := url.Parse(failure.Redirect)
		got := u.Query()
		if u.Host != "claude.example" || u.Path != "/cb" || got.Get("app") != "1" || got.Get("error") != c.code ||
			got.Get("iss") != "https://wtt.example.com" || got.Get("code") != "" {
			t.Errorf("%s: redirect %s, want error=%s with the app's query and iss", name, failure.Redirect, c.code)
		}
		if name != "two states" && got.Get("state") != "xyz" {
			t.Errorf("%s: state %q, want xyz", name, got.Get("state"))
		}
	}
}

// failing é um transporte que nunca alcança ninguém: o registro cai na cópia embutida.
type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}
