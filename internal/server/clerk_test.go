package server_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter/clerkfake"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

// clerkKey é uma chave pública no formato do Clerk: pk_test_ e o endereço do Frontend API em base64, com um $.
var clerkKey = "pk_test_" + base64.RawStdEncoding.EncodeToString([]byte("clerk.test$"))

func newClerkServer(t *testing.T) (*echo.Echo, *clerkfake.Fake) {
	t.Helper()
	testutil.Truncate(t, testDB)
	fake := clerkfake.New()
	e, err := server.New(testClient, server.Options{
		EncryptKey: "test-key", AuthRateLimit: 1000,
		Clerk: &server.ClerkOptions{Provider: fake, PublishableKey: clerkKey},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e, fake
}

// clerkDo faz uma requisição de quem acabou de entrar no Clerk: o token de sessão dele vai em Authorization.
func clerkDo(e *echo.Echo, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// Sem as chaves do Clerk as rotas dele não existem para quem chama, e as páginas ficam como eram.
func TestClerk_OffByDefault(t *testing.T) {
	e := newServer(t)
	rec := clerkDo(e, "/api/auth/clerk/login", `{}`, "qualquer")
	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "auth.clerk_disabled" {
		t.Errorf("login without the Clerk = %d %s, want 404 auth.clerk_disabled", rec.Code, rec.Body.String())
	}
	if body := do(e, "GET", "/login", "", "").Body.String(); strings.Contains(body, "publishableKey") {
		t.Error("the login page must not mention the Clerk when it is off")
	}
}

// Quem entra pelo Clerk cria a organização, sai e volta a entrar; a sessão é a do sistema, o cookie de sempre.
func TestClerk_SignupLogoutLogin(t *testing.T) {
	e, fake := newClerkServer(t)
	fake.AddUser("user_1", "ana@test.com", "Ana", true)

	rec := clerkDo(e, "/api/auth/clerk/signup", `{"organization_name":"Acme"}`, fake.Token("user_1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d: %s", rec.Code, rec.Body.String())
	}
	session := sessionFrom(t, rec)
	body := decode(t, rec)
	identity, _ := body["identity"].(map[string]any)
	if body["status"] != "ok" || identity["email"] != "ana@test.com" || identity["is_owner"] != true || identity["has_password"] != false {
		t.Errorf("body = %v, want ok for ana, the owner, without a password", body)
	}

	me := decode(t, do(e, "GET", "/api/auth/me", "", session))
	if me["email"] != "ana@test.com" || me["has_password"] != false {
		t.Errorf("me = %v, want ana without a password", me)
	}
	if rec := do(e, "POST", "/api/auth/logout", "", session); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := do(e, "GET", "/api/auth/me", "", session); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", rec.Code)
	}

	rec = clerkDo(e, "/api/auth/clerk/login", `{}`, fake.Token("user_1"))
	if rec.Code != http.StatusOK || decode(t, rec)["status"] != "ok" {
		t.Fatalf("login = %d: %s", rec.Code, rec.Body.String())
	}
	if again := sessionFrom(t, rec); again == session {
		t.Error("each login opens a new session")
	}
}

func TestClerk_RejectsBadTokens(t *testing.T) {
	e, fake := newClerkServer(t)
	for name, tc := range map[string]struct {
		token string
		code  string
		want  int
	}{
		"no token":      {"", "auth.clerk_token_invalid", http.StatusUnauthorized},
		"unknown token": {"tok_de_outro_app", "auth.clerk_token_invalid", http.StatusUnauthorized},
	} {
		rec := clerkDo(e, "/api/auth/clerk/login", `{}`, tc.token)
		if rec.Code != tc.want || errorCode(t, rec) != tc.code {
			t.Errorf("%s = %d %s, want %d %s", name, rec.Code, rec.Body.String(), tc.want, tc.code)
		}
	}

	fake.AddUser("user_2", "bia@test.com", "Bia", false)
	rec := clerkDo(e, "/api/auth/clerk/login", `{}`, fake.Token("user_2"))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "auth.clerk_email_unverified" {
		t.Errorf("unverified email = %d %s, want 403 auth.clerk_email_unverified", rec.Code, rec.Body.String())
	}

	fake.SetDown(true)
	rec = clerkDo(e, "/api/auth/clerk/login", `{}`, fake.Token("user_2"))
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "auth.clerk_unavailable" {
		t.Errorf("Clerk down = %d %s, want 502 auth.clerk_unavailable", rec.Code, rec.Body.String())
	}
	// Nenhuma dessas respostas abre sessão.
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			t.Errorf("a failed login set the session cookie")
		}
	}
}

// A conta antiga, com senha, é ligada ao Clerk com a senha, uma vez, e o login sem a senha não abre sessão.
func TestClerk_LinkingAPasswordAccount(t *testing.T) {
	e, fake := newClerkServer(t)
	old := signup(t, e, "Acme", "ana@test.com")
	fake.AddUser("user_1", "ana@test.com", "Ana", true)

	rec := clerkDo(e, "/api/auth/clerk/login", `{}`, fake.Token("user_1"))
	body := decode(t, rec)
	if rec.Code != http.StatusOK || body["status"] != "needs_password" || body["email"] != "ana@test.com" {
		t.Fatalf("without the password = %d %v, want 200 needs_password", rec.Code, body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			t.Fatal("needs_password must not open a session")
		}
	}

	rec = clerkDo(e, "/api/auth/clerk/login", `{"password":"errada-123"}`, fake.Token("user_1"))
	if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "auth.invalid_credentials" {
		t.Errorf("wrong password = %d %s, want 401 auth.invalid_credentials", rec.Code, rec.Body.String())
	}

	rec = clerkDo(e, "/api/auth/clerk/login", `{"password":"senha-forte-1"}`, fake.Token("user_1"))
	if rec.Code != http.StatusOK || decode(t, rec)["status"] != "ok" {
		t.Fatalf("right password = %d: %s", rec.Code, rec.Body.String())
	}
	me := decode(t, do(e, "GET", "/api/auth/me", "", sessionFrom(t, rec)))
	if me["id"] != old.id || me["has_password"] != true {
		t.Errorf("me = %v, want the old account (id %s), still with its password", me, old.id)
	}
	// O login por senha segue funcionando ao lado do Clerk.
	if rec := do(e, "POST", "/api/auth/login", `{"email":"ana@test.com","password":"senha-forte-1"}`, ""); rec.Code != http.StatusOK {
		t.Errorf("password login with the Clerk on = %d, want 200", rec.Code)
	}
}

// O convite do link do e-mail: o token entra direto; sem ele, quem não tem conta vê os convites e escolhe.
func TestClerk_InvitesFlow(t *testing.T) {
	e, fake := newClerkServer(t)
	admin := signup(t, e, "Acme", "ana@test.com")
	fake.AddUser("user_c", "caio@test.com", "Caio", true)

	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/invites", `{"email":"caio@test.com","role":"member"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite = %d: %s", rec.Code, rec.Body.String())
	}
	inviteID := decode(t, rec)["id"].(string)
	token := decode(t, rec)["token"].(string)

	// Sem o token: não há conta, e o convite aparece na lista.
	rec = clerkDo(e, "/api/auth/clerk/login", `{}`, fake.Token("user_c"))
	body := decode(t, rec)
	invites, _ := body["invites"].([]any)
	if rec.Code != http.StatusOK || body["status"] != "no_account" || len(invites) != 1 {
		t.Fatalf("no token = %d %v, want 200 no_account with one invite", rec.Code, body)
	}
	if first, _ := invites[0].(map[string]any); first["id"] != inviteID || first["organization_name"] != "Acme" || first["role"] != "member" {
		t.Errorf("invite option = %v, want Acme as member", invites[0])
	}

	// Aceita o da lista.
	rec = clerkDo(e, "/api/auth/clerk/join", `{"invite_id":"`+inviteID+`"}`, fake.Token("user_c"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("join = %d: %s", rec.Code, rec.Body.String())
	}
	me := decode(t, do(e, "GET", "/api/auth/me", "", sessionFrom(t, rec)))
	if me["organization_id"] != admin.orgID || me["role"] != "member" || me["email"] != "caio@test.com" {
		t.Errorf("me = %v, want caio, member of Acme", me)
	}

	// O mesmo convite não serve duas vezes.
	if rec := clerkDo(e, "/api/auth/clerk/login", `{"invite_token":"`+token+`"}`, fake.Token("user_c")); rec.Code != http.StatusOK {
		t.Errorf("an existing member logging in with a used token = %d, want a plain 200 login", rec.Code)
	}

	// Com o token, quem não tem conta entra direto, em 201.
	rec = do(e, "POST", "/api/orgs/"+admin.orgID+"/invites", `{"email":"duda@test.com","role":"admin"}`, admin.session)
	token2 := decode(t, rec)["token"].(string)
	fake.AddUser("user_d", "duda@test.com", "Duda", true)
	rec = clerkDo(e, "/api/auth/clerk/login", `{"invite_token":"`+token2+`"}`, fake.Token("user_d"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("login with the invite token = %d: %s", rec.Code, rec.Body.String())
	}
	if me := decode(t, do(e, "GET", "/api/auth/me", "", sessionFrom(t, rec))); me["role"] != "admin" || me["organization_id"] != admin.orgID {
		t.Errorf("me = %v, want an admin of Acme", me)
	}

	// Um e-mail que não é o do convite não entra.
	rec = do(e, "POST", "/api/orgs/"+admin.orgID+"/invites", `{"email":"eva@test.com"}`, admin.session)
	token3 := decode(t, rec)["token"].(string)
	fake.AddUser("user_x", "intruso@test.com", "Intruso", true)
	rec = clerkDo(e, "/api/auth/clerk/login", `{"invite_token":"`+token3+`"}`, fake.Token("user_x"))
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "auth.invite_email_mismatch" {
		t.Errorf("another email = %d %s, want 400 auth.invite_email_mismatch", rec.Code, rec.Body.String())
	}
}

func TestClerk_SignupValidation(t *testing.T) {
	e, fake := newClerkServer(t)
	fake.AddUser("user_1", "ana@test.com", "Ana", true)
	rec := clerkDo(e, "/api/auth/clerk/signup", `{"organization_name":"  "}`, fake.Token("user_1"))
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "auth.org_name_required" {
		t.Errorf("no organization name = %d %s, want 400 auth.org_name_required", rec.Code, rec.Body.String())
	}
	clerkDo(e, "/api/auth/clerk/signup", `{"organization_name":"Acme"}`, fake.Token("user_1"))
	rec = clerkDo(e, "/api/auth/clerk/signup", `{"organization_name":"Outra"}`, fake.Token("user_1"))
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "auth.account_exists" {
		t.Errorf("second signup = %d %s, want 409 auth.account_exists", rec.Code, rec.Body.String())
	}
	// O corpo precisa ser JSON como em toda a API.
	req := httptest.NewRequest("POST", "/api/auth/clerk/login", strings.NewReader("a=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+fake.Token("user_1"))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form body = %d, want 415", rec.Code)
	}
}
