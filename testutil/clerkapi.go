package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
)

// ClerkAPI é um servidor HTTP que se passa pela Backend API do Clerk, para o teste do adapter: serve as chaves
// públicas (JWKS), os usuários e os convites, e assina tokens de sessão de verdade (RS256) com uma chave
// gerada no teste. Aponte o adapter para URL().
type ClerkAPI struct {
	srv *httptest.Server
	key *rsa.PrivateKey
	kid string

	mu          sync.Mutex
	users       map[string]map[string]any
	invitations []map[string]any
	calls       []string
	jwksFetches int
	fail        map[string]int
	lastSecret  string
}

// NewClerkAPI sobe o servidor falso; ele fecha junto com o teste.
func NewClerkAPI(t testing.TB) *ClerkAPI {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	a := &ClerkAPI{key: key, kid: "ins_" + strings.ReplaceAll(t.Name(), "/", "_"), users: map[string]map[string]any{}, fail: map[string]int{}}
	a.srv = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.srv.Close)
	return a
}

// URL é o endereço base que o adapter usa no lugar de api.clerk.com.
func (a *ClerkAPI) URL() string { return a.srv.URL }

// AddUser cadastra um usuário, com um e-mail primário.
func (a *ClerkAPI) AddUser(id, email string, verified bool, first, last string) {
	status := "unverified"
	if verified {
		status = "verified"
	}
	user := map[string]any{
		"object": "user", "id": id, "first_name": first, "last_name": last,
		"primary_email_address_id": "idn_" + id,
		"email_addresses": []any{map[string]any{
			"object": "email_address", "id": "idn_" + id, "email_address": email,
			"verification": map[string]any{"status": status, "strategy": "email_code"},
		}},
	}
	a.mu.Lock()
	a.users[id] = user
	a.mu.Unlock()
}

// SetUserField muda um campo do usuário (por exemplo "banned").
func (a *ClerkAPI) SetUserField(id, field string, value any) {
	a.mu.Lock()
	a.users[id][field] = value
	a.mu.Unlock()
}

// DeleteUser apaga o usuário: o servidor passa a responder 404 para ele.
func (a *ClerkAPI) DeleteUser(id string) {
	a.mu.Lock()
	delete(a.users, id)
	a.mu.Unlock()
}

// FailRequests faz as requisições a um caminho que contenha esse texto responderem com esse status (0 desfaz).
func (a *ClerkAPI) FailRequests(pathPart string, status int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if status == 0 {
		delete(a.fail, pathPart)
		return
	}
	a.fail[pathPart] = status
}

// Calls devolve as requisições recebidas, como "GET /v1/users/user_1".
func (a *ClerkAPI) Calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls...)
}

// JWKSFetches diz quantas vezes as chaves públicas foram pedidas.
func (a *ClerkAPI) JWKSFetches() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.jwksFetches
}

// LastSecret é a chave secreta que veio no cabeçalho da última requisição.
func (a *ClerkAPI) LastSecret() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastSecret
}

// Invitations devolve os corpos dos convites criados, na ordem.
func (a *ClerkAPI) Invitations() []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]map[string]any(nil), a.invitations...)
}

// TokenClaims são os campos de um token de sessão; o que vier vazio ganha um valor que dá certo.
type TokenClaims struct {
	Subject string
	Azp     string
	Issuer  string
	Expires time.Duration // a partir de agora; negativo é um token que já venceu
}

// Token assina um token de sessão com a chave do servidor falso (a que ele publica no JWKS).
func (a *ClerkAPI) Token(t testing.TB, c TokenClaims) string {
	t.Helper()
	return a.sign(t, a.key, a.kid, c)
}

// TokenSignedBy assina com outra chave, que o JWKS não publica: um token forjado.
func (a *ClerkAPI) TokenSignedBy(t testing.TB, c TokenClaims) string {
	t.Helper()
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return a.sign(t, other, a.kid, c)
}

// TokenWithKeyID assina com a chave certa, mas com um kid que o JWKS não tem.
func (a *ClerkAPI) TokenWithKeyID(t testing.TB, kid string, c TokenClaims) string {
	t.Helper()
	return a.sign(t, a.key, kid, c)
}

func (a *ClerkAPI) sign(t testing.TB, key *rsa.PrivateKey, kid string, c TokenClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer == "" {
		c.Issuer = "https://clerk.test"
	}
	if c.Expires == 0 {
		c.Expires = time.Minute
	}
	now := time.Now()
	claims := map[string]any{
		"iss": c.Issuer, "sub": c.Subject, "sid": "sess_1", "azp": c.Azp, "v": 2,
		"iat": now.Add(-time.Second).Unix(), "nbf": now.Add(-time.Second).Unix(), "exp": now.Add(c.Expires).Unix(),
	}
	token, err := jwt.Signed(signer).Claims(claims).CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (a *ClerkAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, r.Method+" "+r.URL.Path)
	a.lastSecret = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	for part, status := range a.fail {
		if strings.Contains(r.URL.Path, part) {
			clerkJSON(w, status, map[string]any{
				"errors":         []any{map[string]any{"code": "fake_failure", "message": "forced by the test"}},
				"clerk_trace_id": "trace_fake",
			})
			return
		}
	}

	switch {
	case r.Method == "GET" && r.URL.Path == "/v1/jwks":
		a.jwksFetches++
		pub := jose.JSONWebKey{Key: &a.key.PublicKey, KeyID: a.kid, Algorithm: "RS256", Use: "sig"}
		raw, _ := pub.MarshalJSON()
		clerkJSON(w, http.StatusOK, map[string]any{"keys": []json.RawMessage{raw}})

	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/users/"):
		if user, ok := a.users[strings.TrimPrefix(r.URL.Path, "/v1/users/")]; ok {
			clerkJSON(w, http.StatusOK, user)
			return
		}
		clerkNotFound(w)

	case r.Method == "POST" && r.URL.Path == "/v1/invitations":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		a.invitations = append(a.invitations, body)
		id := "inv_" + string(rune('a'+len(a.invitations)-1))
		clerkJSON(w, http.StatusOK, map[string]any{
			"object": "invitation", "id": id, "email_address": body["email_address"], "status": "pending",
			"url": "https://clerk.test/v1/tickets/accept?ticket=" + id,
		})

	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/invitations/") && strings.HasSuffix(r.URL.Path, "/revoke"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/invitations/"), "/revoke")
		if id == "inv_gone" {
			clerkNotFound(w)
			return
		}
		clerkJSON(w, http.StatusOK, map[string]any{"object": "invitation", "id": id, "status": "revoked", "revoked": true})

	default:
		clerkNotFound(w)
	}
}

func clerkNotFound(w http.ResponseWriter) {
	clerkJSON(w, http.StatusNotFound, map[string]any{
		"errors":         []any{map[string]any{"code": "resource_not_found", "message": "not found"}},
		"clerk_trace_id": "trace_404",
	})
}

func clerkJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
