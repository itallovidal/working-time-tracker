package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/ent/apitoken"
	"working-time-tracker/internal/domain/auth"
)

// createToken gera um token do Claude para a conta e devolve o id e o token em claro.
func createToken(t *testing.T, e *echo.Echo, acc account, name string) (id, token string) {
	t.Helper()
	rec := do(e, "POST", "/api/auth/tokens", `{"name":"`+name+`"}`, acc.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	return body["id"].(string), body["token"].(string)
}

func TestAPITokens_CreateShowsTheTokenOnce(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")

	before := time.Now()
	rec := do(e, "POST", "/api/auth/tokens", `{"name":"  Claude Code no notebook  "}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	token := body["token"].(string)
	if !strings.HasPrefix(token, auth.APITokenPrefix) || len(token) < 40 {
		t.Fatalf("token = %q, want wtt_ and 43 random characters", token)
	}
	if body["name"] != "Claude Code no notebook" {
		t.Errorf("name = %v, want it trimmed", body["name"])
	}
	if body["prefix"] != token[:10] {
		t.Errorf("prefix = %v, want %q", body["prefix"], token[:10])
	}
	expires, err := time.Parse(time.RFC3339Nano, body["expires_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if d := expires.Sub(before.AddDate(0, 0, auth.APITokenDefaultDays)); d < 0 || d > time.Minute {
		t.Errorf("expires_at = %v, want 90 days from now", expires)
	}
	if body["last_used_at"] != nil {
		t.Errorf("last_used_at = %v, want null", body["last_used_at"])
	}

	// A lista nunca traz o token, nem o hash dele.
	rec = do(e, "GET", "/api/auth/tokens", "", owner.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), auth.HashToken(token)) {
		t.Fatalf("list shows the token: %s", rec.Body.String())
	}
	list := decodeList(t, rec)
	if len(list) != 1 || list[0]["id"] != body["id"] {
		t.Fatalf("list = %v, want the token just created", list)
	}
	if _, has := list[0]["token"]; has {
		t.Errorf("list item has a token field: %v", list[0])
	}

	// O banco também não guarda o token em claro.
	row, err := testClient.APIToken.Query().Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if row.TokenHash != auth.HashToken(token) {
		t.Errorf("token_hash is not the sha256 of the token")
	}
}

func TestAPITokens_Validation(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")

	cases := []struct {
		body, code string
		status     int
	}{
		{`{"name":"   "}`, "auth.token_name_required", http.StatusBadRequest},
		{`{"name":"` + strings.Repeat("a", 121) + `"}`, "request.field_too_long", http.StatusBadRequest},
		{`{"name":"x","days":7}`, "auth.token_days_invalid", http.StatusBadRequest},
		{`{"name":"x","days":-30}`, "auth.token_days_invalid", http.StatusBadRequest},
		{`{"name":"x","token":"wtt_escolhido"}`, "request.invalid_body", http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := do(e, "POST", "/api/auth/tokens", c.body, owner.session)
		if rec.Code != c.status || errorCode(t, rec) != c.code {
			t.Errorf("POST %s = %d %s, want %d %s", c.body, rec.Code, rec.Body.String(), c.status, c.code)
		}
	}
	for _, days := range auth.APITokenDays {
		rec := do(e, "POST", "/api/auth/tokens", fmt.Sprintf(`{"name":"x","days":%d}`, days), owner.session)
		if rec.Code != http.StatusCreated {
			t.Errorf("days %d = %d: %s", days, rec.Code, rec.Body.String())
		}
	}
}

func TestAPITokens_Limit(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	for i := 0; i < auth.APITokenMax; i++ {
		createToken(t, e, owner, fmt.Sprintf("t%d", i))
	}
	rec := do(e, "POST", "/api/auth/tokens", `{"name":"um a mais"}`, owner.session)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "auth.token_limit" {
		t.Fatalf("11th token = %d %s, want 409 auth.token_limit", rec.Code, rec.Body.String())
	}
	if max := decode(t, rec)["error"].(map[string]any)["params"].(map[string]any)["max"]; max != float64(auth.APITokenMax) {
		t.Errorf("params.max = %v, want %d", max, auth.APITokenMax)
	}

	// Um token vencido não conta: ele sai quando o próximo é criado.
	if _, err := testClient.APIToken.Update().
		Where(apitoken.NameEQ("t0")).
		SetExpiresAt(time.Now().Add(-time.Hour)).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	createToken(t, e, owner, "no lugar do vencido")
	if n := testClient.APIToken.Query().Where(apitoken.NameEQ("t0")).CountX(t.Context()); n != 0 {
		t.Errorf("expired token still there")
	}

	// O limite é de cada pessoa.
	member := invite(t, e, owner, "membro@example.com", "member")
	createToken(t, e, member, "do membro")
}

func TestAPITokens_Revoke(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	member := invite(t, e, owner, "membro@example.com", "member")
	ownerToken, _ := createToken(t, e, owner, "do dono")
	memberToken, _ := createToken(t, e, member, "do membro")

	// O de outra pessoa, mesmo o dono pedindo, é como um que não existe.
	for _, path := range []string{"/api/auth/tokens/" + memberToken, "/api/auth/tokens/" + uuid.NewString(), "/api/auth/tokens/nao-e-uuid"} {
		rec := do(e, "DELETE", path, "", owner.session)
		if rec.Code != http.StatusNotFound || errorCode(t, rec) != "auth.token_not_found" {
			t.Errorf("DELETE %s = %d %s, want 404 auth.token_not_found", path, rec.Code, rec.Body.String())
		}
	}
	if rec := do(e, "GET", "/api/auth/tokens", "", member.session); len(decodeList(t, rec)) != 1 {
		t.Fatalf("member's token was revoked by someone else")
	}

	rec := do(e, "DELETE", "/api/auth/tokens/"+ownerToken, "", owner.session)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE own = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/auth/tokens", "", owner.session); len(decodeList(t, rec)) != 0 {
		t.Errorf("token still listed after revoke: %s", rec.Body.String())
	}
}

func TestAPITokens_NeedTheSession(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	_, token := createToken(t, e, owner, "claude")

	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/auth/tokens", ""},
		{"POST", "/api/auth/tokens", `{"name":"x"}`},
		{"GET", "/api/auth/me", ""},
		{"GET", "/api/orgs/" + owner.orgID + "/projects", ""},
	} {
		if rec := do(e, r.method, r.path, r.body, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without session = %d, want 401", r.method, r.path, rec.Code)
		}
		// O token do Claude não abre a API: ele só vale no /mcp.
		if rec := doBearer(e, r.method, r.path, r.body, token); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with the Claude token = %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestAPITokens_PasswordChangeRevokesThem(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	createToken(t, e, owner, "um")
	createToken(t, e, owner, "outro")
	// Outra sessão da mesma pessoa, como a de outro navegador: a troca a encerra junto com os tokens.
	login := do(e, "POST", "/api/auth/login", `{"email":"dono@example.com","password":"senha-forte-1"}`, "")
	if login.Code != http.StatusOK {
		t.Fatalf("second login = %d: %s", login.Code, login.Body.String())
	}
	other := sessionFrom(t, login)

	rec := do(e, "POST", "/api/auth/password",
		`{"current_password":"senha-forte-1","new_password":"senha-forte-nova"}`, owner.session)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change password = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/auth/tokens", "", owner.session); len(decodeList(t, rec)) != 0 {
		t.Errorf("tokens survived the password change: %s", rec.Body.String())
	}
	if rec := do(e, "GET", "/api/auth/me", "", other); rec.Code != http.StatusUnauthorized {
		t.Errorf("the other session after the password change = %d, want 401", rec.Code)
	}
	if rec := do(e, "GET", "/api/auth/me", "", owner.session); rec.Code != http.StatusOK {
		t.Errorf("the session that changed the password = %d, want 200", rec.Code)
	}
}

// A pessoa só sai do banco com a organização; os tokens dela vão junto.
func TestAPITokens_GoWithThePerson(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	member := invite(t, e, owner, "membro@example.com", "member")
	createToken(t, e, member, "do membro")
	createToken(t, e, owner, "do dono")

	if rec := do(e, "DELETE", "/api/orgs/"+owner.orgID, "", owner.session); rec.Code != http.StatusNoContent {
		t.Fatalf("delete organization = %d: %s", rec.Code, rec.Body.String())
	}
	if n := testClient.APIToken.Query().CountX(t.Context()); n != 0 {
		t.Errorf("%d tokens left after the person was deleted", n)
	}
}

// doBearer faz uma requisição com o token em Authorization: Bearer, sem cookie.
func doBearer(e *echo.Echo, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}
