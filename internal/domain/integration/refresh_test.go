package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/testutil"
)

// gitlabOAuth é um projeto com o GitLab fake (com o OAuth) e o service que renova o token do GitLab com o app
// dele. A integração vem conectada pelo OAuth, com o projeto group/project escolhido.
type gitlabOAuth struct {
	svc   *integration.Service
	fake  *testutil.GitLab
	oauth *adapter.GitLabOAuth
	it    *integration.Integration
}

func newGitLabOAuth(t *testing.T) *gitlabOAuth {
	t.Helper()
	svc, projectID := setup(t)
	fake := testutil.NewGitLab()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	orig, err := adapter.GetIntegration("gitlab")
	if err != nil {
		t.Fatal(err)
	}
	adapter.Register("gitlab", func() adapter.Integration { return &adapter.GitLabIntegration{BaseURL: srv.URL + "/api/v4"} })
	t.Cleanup(func() { adapter.Register("gitlab", func() adapter.Integration { return orig }) })

	g := &gitlabOAuth{svc: svc, fake: fake, oauth: &adapter.GitLabOAuth{
		ClientID: testutil.GitLabClientID, ClientSecret: testutil.GitLabClientSecret,
		RedirectURL: "http://localhost/integrations/gitlab/callback", SiteURL: srv.URL,
	}}
	svc.SetRefresher("gitlab", g.oauth)

	cred, err := g.oauth.Exchange(testutil.GitLabOAuthCode)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if cred.Token != testutil.GitLabOAuthToken || cred.RefreshToken != testutil.GitLabRefreshToken || cred.ExpiresAt.IsZero() {
		t.Fatalf("credential = %+v", cred)
	}
	g.it, err = svc.ConnectCredential(projectID, "gitlab", cred)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	yes := true
	g.it, err = svc.Edit(g.it.ID.String(), integration.EditInput{Enabled: &yes, Metadata: map[string]interface{}{"project_url": "group/project"}})
	if err != nil {
		t.Fatalf("pick the project: %v", err)
	}
	return g
}

// expireSoon adianta o vencimento guardado para daqui a um minuto, menos que a margem da renovação: é o token
// que vence logo. O GitLab fake continua aceitando o token, como o de verdade aceitaria até vencer.
func (g *gitlabOAuth) expireSoon(t *testing.T) {
	t.Helper()
	secrets := stored(t, g.it)
	secrets["expires_at"] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	sealed, err := adapter.EncryptConfig(secrets, testKey)
	if err != nil {
		t.Fatal(err)
	}
	testClient.Integration.UpdateOneID(g.it.ID).SetCredentials(sealed).ExecX(context.Background())
}

// token é o token de acesso que o service entrega à sincronização agora.
func (g *gitlabOAuth) token(t *testing.T) string {
	t.Helper()
	conn, _, err := g.svc.Connection(g.it.ID.String())
	if err != nil {
		t.Fatalf("connection: %v", err)
	}
	return conn.Token
}

// stored lê, com a chave, a credencial guardada no banco.
func stored(t *testing.T, it *integration.Integration) map[string]interface{} {
	t.Helper()
	row := testClient.Integration.GetX(context.Background(), it.ID)
	secrets, err := adapter.DecryptConfig(row.Credentials, testKey)
	if err != nil {
		t.Fatal(err)
	}
	return secrets
}

// A conexão pelo OAuth guarda o par e o vencimento; um token que ainda vale por muito tempo não é renovado.
func TestRefresh_ValidTokenIsNotRefreshed(t *testing.T) {
	g := newGitLabOAuth(t)
	if got := g.token(t); got != testutil.GitLabOAuthToken {
		t.Errorf("token = %q", got)
	}
	secrets := stored(t, g.it)
	if secrets["refresh_token"] != testutil.GitLabRefreshToken || secrets["expires_at"] == nil {
		t.Errorf("stored credential = %v, want the refresh token and the expiry", secrets)
	}
	if g.fake.Refreshes() != 0 {
		t.Errorf("%d refreshes, want none", g.fake.Refreshes())
	}
	if g.it.DisplayName != "GitLab · @"+testutil.GitLabLogin || !g.it.HasToken {
		t.Errorf("integration = %+v", g.it)
	}
}

// O token que vence logo é renovado antes de sair, o par novo fica guardado, e a plataforma aceita o novo.
func TestRefresh_ExpiringTokenIsRefreshed(t *testing.T) {
	g := newGitLabOAuth(t)
	g.expireSoon(t)
	first := g.token(t)
	if first == testutil.GitLabOAuthToken || g.fake.Refreshes() != 1 {
		t.Fatalf("token = %q after %d refreshes; want a new one", first, g.fake.Refreshes())
	}
	secrets := stored(t, g.it)
	if secrets["token"] != first || secrets["refresh_token"] == testutil.GitLabRefreshToken {
		t.Errorf("stored = %v; the new pair must be saved", secrets)
	}
	// O novo vale na plataforma, e o antigo não.
	impl, _ := adapter.GetIntegration("gitlab")
	if _, err := impl.(adapter.AccountLookup).Account(adapter.Connection{Token: first}); err != nil {
		t.Errorf("the refreshed token: %v", err)
	}
	if _, err := impl.(adapter.AccountLookup).Account(adapter.Connection{Token: testutil.GitLabOAuthToken}); !errors.Is(err, adapter.ErrInvalidToken) {
		t.Errorf("the old token still works: %v", err)
	}
	// O que se vê de fora nunca traz o token.
	got, err := g.svc.Get(g.it.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), first) || strings.Contains(string(raw), "glrt") || got.Credentials != nil {
		t.Errorf("the integration as the API sees it carries a token: %s", raw)
	}
}

// Dez chamadas ao mesmo tempo renovam exatamente uma vez: a renovação gira o par, e uma segunda usaria o token de
// renovação que a primeira já gastou. A que espera a vez relê a credencial e acha o par novo.
func TestRefresh_ConcurrentCallsRefreshExactlyOnce(t *testing.T) {
	g := newGitLabOAuth(t)
	g.expireSoon(t)
	var wg sync.WaitGroup
	tokens := make([]string, 10)
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, _, err := g.svc.Connection(g.it.ID.String())
			if err != nil {
				t.Errorf("call %d: %v", i, err)
			}
			tokens[i] = conn.Token
		}(i)
	}
	wg.Wait()
	if n := g.fake.Refreshes(); n != 1 {
		t.Errorf("%d refreshes, want exactly one", n)
	}
	for i, tok := range tokens {
		if tok != tokens[0] || tok == testutil.GitLabOAuthToken {
			t.Errorf("call %d got %q, want the one refreshed token %q", i, tok, tokens[0])
		}
	}
}

// A autorização tirada no GitLab: a renovação é recusada, e o motivo é o token recusado, que pede Reconectar.
func TestRefresh_RevokedIsInvalidToken(t *testing.T) {
	g := newGitLabOAuth(t)
	g.expireSoon(t)
	g.fake.RevokeTokens()
	if _, _, err := g.svc.Connection(g.it.ID.String()); !errors.Is(err, adapter.ErrInvalidToken) {
		t.Errorf("err = %v, want integration.invalid_token", err)
	}
	res, err := g.svc.FetchItemDetails(g.it.ID.String(), "7")
	if err != nil || res.Details != nil || res.Error == nil || res.Error.Code != "integration.invalid_token" {
		t.Errorf("details = %+v, %v; want the reason and no details", res, err)
	}
}

// O GitLab fora do ar na renovação não estraga o token que ainda vale: ele é entregue, e a renovação fica para a
// próxima chamada.
func TestRefresh_UnreachableKeepsTheValidToken(t *testing.T) {
	g := newGitLabOAuth(t)
	g.expireSoon(t)
	g.fake.Fail("POST", "/oauth/token", 502, 1)
	if got := g.token(t); got != testutil.GitLabOAuthToken {
		t.Errorf("token = %q, want the stored one while it is still valid", got)
	}
	if got := g.token(t); got == testutil.GitLabOAuthToken {
		t.Error("the next call must refresh")
	}
}

// O token colado (um token pessoal) nunca é renovado, mesmo com o app configurado.
func TestRefresh_PastedTokenIsNeverRefreshed(t *testing.T) {
	g := newGitLabOAuth(t)
	g.expireSoon(t)
	pasted, err := g.svc.Edit(g.it.ID.String(), integration.EditInput{Token: "glpat-pessoal"})
	if err != nil {
		t.Fatalf("paste a token: %v", err)
	}
	if secrets := stored(t, pasted); secrets["refresh_token"] != nil || secrets["expires_at"] != nil {
		t.Errorf("stored = %v; a pasted token has nothing to refresh", secrets)
	}
	for range 3 {
		if got := g.token(t); got != "glpat-pessoal" {
			t.Errorf("token = %q", got)
		}
	}
	if g.fake.Refreshes() != 0 {
		t.Errorf("%d refreshes of a pasted token", g.fake.Refreshes())
	}
}

// A gravação da integração (a edição) não leva a credencial junto: a cópia lida no começo da edição tem o par
// antigo, e gravá-la desfaria uma renovação feita no meio.
func TestRefresh_UpdateDoesNotUndoARefresh(t *testing.T) {
	g := newGitLabOAuth(t)
	store := integration.NewStore(testClient)
	stale, err := store.GetByID(g.it.ID.String()) // a edição lê
	if err != nil {
		t.Fatal(err)
	}

	// No meio, a rotina de fundo renova.
	fresh, err := g.oauth.Refresh(testutil.GitLabRefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RefreshCredentials(g.it.ID, func(map[string]interface{}) (map[string]interface{}, error) {
		return adapter.EncryptConfig(map[string]interface{}{"token": fresh.Token, "refresh_token": fresh.RefreshToken}, testKey)
	}); err != nil {
		t.Fatal(err)
	}

	// A edição grava o que leu, com outro nome.
	stale.DisplayName = "Outro nome"
	if err := store.Update(stale, false); err != nil {
		t.Fatal(err)
	}
	if after := stored(t, g.it); after["token"] != fresh.Token {
		t.Errorf("stored token = %v after the edit, want the refreshed %q", after["token"], fresh.Token)
	}
	if got, _ := g.svc.Get(g.it.ID.String()); got.DisplayName != "Outro nome" {
		t.Errorf("name = %q", got.DisplayName)
	}
	// E renomear pelo service também não toca na credencial.
	if _, err := g.svc.Edit(g.it.ID.String(), integration.EditInput{DisplayName: "Mais um", Metadata: map[string]interface{}{"project_url": "group/project"}}); err != nil {
		t.Fatal(err)
	}
	if after := stored(t, g.it); after["token"] != fresh.Token || after["refresh_token"] != fresh.RefreshToken {
		t.Errorf("stored = %v after a rename, want the refreshed pair", after)
	}
}
