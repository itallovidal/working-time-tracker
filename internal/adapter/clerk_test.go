package adapter

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"working-time-tracker/testutil"
)

const testOrigin = "http://localhost:8080"

func newTestClerk(t *testing.T) (*Clerk, *testutil.ClerkAPI) {
	t.Helper()
	api := testutil.NewClerkAPI(t)
	c := NewClerk(ClerkConfig{SecretKey: "sk_test_secret", APIURL: api.URL(), AuthorizedParties: []string{testOrigin}})
	return c, api
}

func TestClerkFrontendAPI(t *testing.T) {
	enc := func(s string) string { return base64.RawStdEncoding.EncodeToString([]byte(s)) }
	for key, want := range map[string]string{
		"pk_test_" + enc("tolerant-leopard-3444.clerk.accounts.dev$"): "tolerant-leopard-3444.clerk.accounts.dev",
		"pk_live_" + enc("clerk.example.com$"):                        "clerk.example.com",
		"pk_test_" + enc("sem-cifrao.clerk.dev"):                      "",
		"pk_test_" + enc("$"):                                         "",
		"pk_test_" + enc("evil.com/path$"):                            "",
		"pk_test_!!!":                                                 "",
		"pk_test":                                                     "",
		"":                                                            "",
	} {
		if got := ClerkFrontendAPI(key); got != want {
			t.Errorf("ClerkFrontendAPI(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestClerk_Verify(t *testing.T) {
	c, api := newTestClerk(t)
	api.AddUser("user_1", "Ana@Acme.com", true, "Ana", "Souza")
	ctx := context.Background()

	id, err := c.Verify(ctx, api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.UserID != "user_1" || id.Email != "ana@acme.com" || !id.EmailVerified || id.Name != "Ana Souza" || id.SessionID != "sess_1" {
		t.Errorf("identity = %+v, want user_1, ana@acme.com verified, Ana Souza", id)
	}
	if got := api.LastSecret(); got != "sk_test_secret" {
		t.Errorf("the secret key reached the API as %q, want the configured one", got)
	}

	// As chaves públicas ficam guardadas: a segunda verificação não pede o JWKS de novo.
	if _, err := c.Verify(ctx, api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin})); err != nil {
		t.Fatal(err)
	}
	if n := api.JWKSFetches(); n != 1 {
		t.Errorf("JWKS fetched %d times for two verifications, want 1", n)
	}
}

// O e-mail que vale é o primário, e só se o Clerk o verificou.
func TestClerk_Verify_ReportsTheVerificationOfThePrimaryEmail(t *testing.T) {
	c, api := newTestClerk(t)
	api.AddUser("user_2", "bia@acme.com", false, "", "")
	id, err := c.Verify(context.Background(), api.Token(t, testutil.TokenClaims{Subject: "user_2", Azp: testOrigin}))
	if err != nil {
		t.Fatal(err)
	}
	if id.EmailVerified || id.Email != "bia@acme.com" || id.Name != "" {
		t.Errorf("identity = %+v, want an unverified bia@acme.com without a name", id)
	}
}

func TestClerk_Verify_RefusesWhatIsNotAValidSession(t *testing.T) {
	c, api := newTestClerk(t)
	api.AddUser("user_1", "ana@acme.com", true, "Ana", "")
	api.AddUser("user_banned", "ban@acme.com", true, "Ban", "")
	api.SetUserField("user_banned", "banned", true)
	api.AddUser("user_locked", "lock@acme.com", true, "Lock", "")
	api.SetUserField("user_locked", "locked", true)
	ctx := context.Background()

	for name, token := range map[string]string{
		"forged signature":    api.TokenSignedBy(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin}),
		"expired":             api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin, Expires: -time.Minute}),
		"another origin":      api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: "https://outro-app.example"}),
		"no authorized party": api.Token(t, testutil.TokenClaims{Subject: "user_1"}),
		"not a Clerk issuer":  api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin, Issuer: "https://evil.example"}),
		"no subject":          api.Token(t, testutil.TokenClaims{Azp: testOrigin}),
		"unknown key id":      api.TokenWithKeyID(t, "ins_de_outra_instancia", testutil.TokenClaims{Subject: "user_1", Azp: testOrigin}),
		"deleted user":        api.Token(t, testutil.TokenClaims{Subject: "user_apagado", Azp: testOrigin}),
		"banned user":         api.Token(t, testutil.TokenClaims{Subject: "user_banned", Azp: testOrigin}),
		"locked user":         api.Token(t, testutil.TokenClaims{Subject: "user_locked", Azp: testOrigin}),
		"garbage":             "isto.nao.e-um-jwt",
		"empty":               "  ",
	} {
		if _, err := c.Verify(ctx, token); !errors.Is(err, ErrClerkTokenInvalid) {
			t.Errorf("%s: err = %v, want ErrClerkTokenInvalid", name, err)
		}
	}
}

// Uma chave desconhecida faz buscar o JWKS de novo, mas não a cada tentativa: um token com chave inventada não
// pode fazer o servidor bater no Clerk toda vez.
func TestClerk_Verify_UnknownKeyDoesNotHammerTheJWKS(t *testing.T) {
	c, api := newTestClerk(t)
	ctx := context.Background()
	bogus := api.TokenWithKeyID(t, "ins_inventada", testutil.TokenClaims{Subject: "user_1", Azp: testOrigin})
	for range 5 {
		if _, err := c.Verify(ctx, bogus); !errors.Is(err, ErrClerkTokenInvalid) {
			t.Fatalf("err = %v, want ErrClerkTokenInvalid", err)
		}
	}
	if n := api.JWKSFetches(); n != 1 {
		t.Errorf("JWKS fetched %d times for five bogus tokens, want 1", n)
	}
}

// Se o Clerk está fora do ar, a falha é de indisponibilidade, e não de token inválido: a pessoa pode tentar de novo.
func TestClerk_Verify_ClerkOutageIsNotAnInvalidToken(t *testing.T) {
	ctx := context.Background()

	c, api := newTestClerk(t)
	api.AddUser("user_1", "ana@acme.com", true, "Ana", "")
	api.FailRequests("/v1/jwks", http.StatusInternalServerError)
	if _, err := c.Verify(ctx, api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin})); !errors.Is(err, ErrClerkUnavailable) {
		t.Errorf("JWKS 500: err = %v, want ErrClerkUnavailable", err)
	}
	api.FailRequests("/v1/jwks", 0)

	api.FailRequests("/v1/users/", http.StatusServiceUnavailable)
	if _, err := c.Verify(ctx, api.Token(t, testutil.TokenClaims{Subject: "user_1", Azp: testOrigin})); !errors.Is(err, ErrClerkUnavailable) {
		t.Errorf("user lookup 503: err = %v, want ErrClerkUnavailable", err)
	}
}

func TestClerk_UserExists(t *testing.T) {
	c, api := newTestClerk(t)
	api.AddUser("user_1", "ana@acme.com", true, "Ana", "")
	ctx := context.Background()

	if ok, err := c.UserExists(ctx, "user_1"); err != nil || !ok {
		t.Errorf("existing user = %v, %v; want true", ok, err)
	}
	if ok, err := c.UserExists(ctx, "user_2"); err != nil || ok {
		t.Errorf("missing user = %v, %v; want false, no error", ok, err)
	}
	api.FailRequests("/v1/users/", http.StatusBadGateway)
	if _, err := c.UserExists(ctx, "user_1"); !errors.Is(err, ErrClerkUnavailable) {
		t.Errorf("Clerk failing: err = %v, want ErrClerkUnavailable", err)
	}
}

func TestClerk_CreateInvitation(t *testing.T) {
	c, api := newTestClerk(t)
	ctx := context.Background()

	inv, err := c.CreateInvitation(ctx, ClerkInviteParams{
		Email: "caio@acme.com", RedirectURL: testOrigin + "/invite/abc", Notify: false, ExpiresInDays: 7,
	})
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	if inv.ID == "" || inv.URL == "" {
		t.Errorf("invitation = %+v, want the id and the Clerk link even without notifying", inv)
	}
	body := api.Invitations()[0]
	if body["email_address"] != "caio@acme.com" || body["redirect_url"] != testOrigin+"/invite/abc" ||
		body["notify"] != false || body["ignore_existing"] != true || body["expires_in_days"] != float64(7) {
		t.Errorf("request body = %v, want the email, redirect, notify false, ignore_existing and 7 days", body)
	}

	if _, err := c.CreateInvitation(ctx, ClerkInviteParams{Email: "x@acme.com", Notify: true}); err != nil {
		t.Fatal(err)
	}
	if body := api.Invitations()[1]; body["notify"] != true {
		t.Errorf("notify = %v, want true", body["notify"])
	}
	if _, set := api.Invitations()[1]["expires_in_days"]; set {
		t.Error("without ExpiresInDays the Clerk's default applies")
	}

	for status, want := range map[int]error{
		http.StatusUnprocessableEntity: ErrClerkRejected,
		http.StatusBadRequest:          ErrClerkRejected,
		http.StatusTooManyRequests:     ErrClerkUnavailable,
		http.StatusInternalServerError: ErrClerkUnavailable,
	} {
		api.FailRequests("/v1/invitations", status)
		if _, err := c.CreateInvitation(ctx, ClerkInviteParams{Email: "y@acme.com"}); !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}
}

func TestClerk_RevokeInvitation(t *testing.T) {
	c, api := newTestClerk(t)
	ctx := context.Background()

	if err := c.RevokeInvitation(ctx, "inv_a"); err != nil {
		t.Errorf("revoke: %v", err)
	}
	// Um convite que já não existe conta como cancelado.
	if err := c.RevokeInvitation(ctx, "inv_gone"); err != nil {
		t.Errorf("revoke of a missing invitation: %v, want nil", err)
	}
	api.FailRequests("/revoke", http.StatusInternalServerError)
	if err := c.RevokeInvitation(ctx, "inv_a"); !errors.Is(err, ErrClerkUnavailable) {
		t.Errorf("Clerk failing: err = %v, want ErrClerkUnavailable", err)
	}
}

// O erro que sobe do adapter leva o status e o rastro do Clerk, e nunca a chave secreta.
func TestClerk_ErrorsCarryTheTraceButNotTheKey(t *testing.T) {
	c, api := newTestClerk(t)
	api.FailRequests("/v1/invitations", http.StatusUnprocessableEntity)
	_, err := c.CreateInvitation(context.Background(), ClerkInviteParams{Email: "y@acme.com"})
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if want := "trace_fake"; !strings.Contains(msg, want) || !strings.Contains(msg, "422") {
		t.Errorf("error %q must carry the status and the trace id", msg)
	}
	if strings.Contains(msg, "sk_test_secret") {
		t.Errorf("error %q leaks the secret key", msg)
	}
}
