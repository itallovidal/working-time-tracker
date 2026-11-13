package personaccount_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	pa "working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/testutil"
)

type env struct {
	svc            *pa.Service
	org, otherOrg  uuid.UUID
	ana, bia, cris uuid.UUID // ana e bia na org; cris na outra
}

func newEnv(t *testing.T) env {
	t.Helper()
	testutil.Truncate(t, testDB)
	ctx := context.Background()
	org := testClient.Organization.Create().SetName("Org").SaveX(ctx)
	other := testClient.Organization.Create().SetName("Outra").SaveX(ctx)
	person := func(orgID uuid.UUID, email string) uuid.UUID {
		return testClient.Person.Create().SetName(email).SetEmail(email).SetOrganizationID(orgID).SaveX(ctx).ID
	}
	return env{
		svc: pa.NewService(pa.NewStore(testClient)), org: org.ID, otherOrg: other.ID,
		ana: person(org.ID, "ana@test.com"), bia: person(org.ID, "bia@test.com"), cris: person(other.ID, "cris@test.com"),
	}
}

func (e env) link(t *testing.T, person, org uuid.UUID, login, source string) (*pa.Account, error) {
	t.Helper()
	return e.svc.Link(pa.Account{PersonID: person, OrganizationID: org, Platform: pa.GitHub, Username: login, Source: source})
}

func (e env) username(t *testing.T, person uuid.UUID) string {
	t.Helper()
	a, err := e.svc.Store().Get(person, pa.GitHub)
	if err != nil {
		return ""
	}
	return a.Username
}

// A conta autorizada pela pessoa vence o vínculo de um admin ou do e-mail, e outra pessoa não autoriza o mesmo login.
func TestLink_OAuthWins(t *testing.T) {
	e := newEnv(t)
	if _, err := e.link(t, e.bia, e.org, "monalisa", pa.SourceAdmin); err != nil {
		t.Fatalf("admin link: %v", err)
	}
	if _, err := e.link(t, e.ana, e.org, "MonaLisa", pa.SourceOAuth); err != nil {
		t.Fatalf("oauth link: %v", err)
	}
	if got := e.username(t, e.ana); got != "MonaLisa" {
		t.Errorf("ana = %q, want MonaLisa", got)
	}
	if got := e.username(t, e.bia); got != "" {
		t.Errorf("bia = %q, want the admin link gone", got)
	}
	if _, err := e.link(t, e.bia, e.org, "monalisa", pa.SourceOAuth); !errors.Is(err, pa.ErrLoginTaken) {
		t.Errorf("a second oauth of the login = %v, want ErrLoginTaken", err)
	}
	// Em outra organização o mesmo login é de outra pessoa, sem conflito.
	if _, err := e.link(t, e.cris, e.otherOrg, "monalisa", pa.SourceOAuth); err != nil {
		t.Errorf("the same login in another organization: %v", err)
	}
}

// O admin não tira o login de quem o autorizou nem troca a conta autorizada da pessoa.
func TestLink_AdminRules(t *testing.T) {
	e := newEnv(t)
	if _, err := e.link(t, e.ana, e.org, "ana-dev", pa.SourceOAuth); err != nil {
		t.Fatal(err)
	}
	if _, err := e.link(t, e.bia, e.org, "ana-dev", pa.SourceAdmin); !errors.Is(err, pa.ErrLoginTaken) {
		t.Errorf("admin taking a verified login = %v, want ErrLoginTaken", err)
	}
	if _, err := e.link(t, e.ana, e.org, "other-login", pa.SourceAdmin); !errors.Is(err, pa.ErrPersonLinked) {
		t.Errorf("admin replacing a verified account = %v, want ErrPersonLinked", err)
	}
	if _, err := e.link(t, e.ana, e.org, "ANA-DEV", pa.SourceAdmin); err != nil {
		t.Errorf("admin confirming the same login: %v", err)
	}
	// Um admin pode passar um login de uma pessoa para outra quando nenhuma o autorizou.
	if _, err := e.link(t, e.bia, e.org, "bia-dev", pa.SourceAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.link(t, e.bia, e.org, "bia-2", pa.SourceAdmin); err != nil {
		t.Errorf("admin replacing an admin link: %v", err)
	}
	if got := e.username(t, e.bia); got != "bia-2" {
		t.Errorf("bia = %q, want bia-2", got)
	}
}

// A integração e o e-mail só entram onde não há conta, e não tiram o login de ninguém.
func TestLink_IntegrationAndEmailOnlyFill(t *testing.T) {
	e := newEnv(t)
	if _, err := e.link(t, e.ana, e.org, "ana-dev", pa.SourceOAuth); err != nil {
		t.Fatal(err)
	}
	if a, err := e.link(t, e.ana, e.org, "robot", pa.SourceIntegration); err != nil || a != nil {
		t.Errorf("integration over an account = %v, %v, want nothing", a, err)
	}
	if a, err := e.link(t, e.bia, e.org, "ana-dev", pa.SourceEmail); err != nil || a != nil {
		t.Errorf("email over someone else's login = %v, %v, want nothing", a, err)
	}
	if got := e.username(t, e.ana); got != "ana-dev" {
		t.Errorf("ana = %q", got)
	}
	// O "não tenho conta" conta como nada: a integração entra.
	if _, err := e.svc.Link(pa.Account{PersonID: e.bia, OrganizationID: e.org, Platform: pa.GitHub, Source: pa.SourceSelf}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.link(t, e.bia, e.org, "bia-dev", pa.SourceIntegration); err != nil {
		t.Fatal(err)
	}
	a, _ := e.svc.Store().Get(e.bia, pa.GitHub)
	if a.Username != "bia-dev" || a.NoAccount {
		t.Errorf("bia = %+v, want bia-dev and no longer no-account", a)
	}
}

// "Não tenho conta" não apaga um login ligado.
func TestLink_SelfKeepsALinkedLogin(t *testing.T) {
	e := newEnv(t)
	if _, err := e.link(t, e.ana, e.org, "ana-dev", pa.SourceOAuth); err != nil {
		t.Fatal(err)
	}
	a, err := e.svc.Link(pa.Account{PersonID: e.ana, OrganizationID: e.org, Platform: pa.GitHub, Source: pa.SourceSelf})
	if err != nil || a.Username != "ana-dev" || a.NoAccount {
		t.Errorf("self over a login = %+v, %v, want ana-dev kept", a, err)
	}
}

// Missing diz o que falta à pessoa; o "não tenho conta" não é conta.
func TestMissing(t *testing.T) {
	e := newEnv(t)
	if _, err := e.link(t, e.ana, e.org, "ana-dev", pa.SourceOAuth); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Link(pa.Account{PersonID: e.bia, OrganizationID: e.org, Platform: pa.GitHub, Source: pa.SourceSelf}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Missing(e.ana, []string{pa.GitHub, pa.GitLab}); len(got) != 1 || got[0] != pa.GitLab {
		t.Errorf("ana missing = %v, want [gitlab]", got)
	}
	if got, _ := e.svc.Missing(e.bia, []string{pa.GitHub}); len(got) != 1 {
		t.Errorf("bia missing = %v, want [github]", got)
	}
	if got, _ := e.svc.Missing(e.bia, nil); len(got) != 0 {
		t.Errorf("nothing required = %v", got)
	}
}

// O login tem o formato das plataformas.
func TestLink_InvalidLogin(t *testing.T) {
	e := newEnv(t)
	for _, l := range []string{"", "a b", "../x", "-x"} {
		if _, err := e.link(t, e.ana, e.org, l, pa.SourceAdmin); !errors.Is(err, pa.ErrInvalidLogin) {
			t.Errorf("login %q = %v, want ErrInvalidLogin", l, err)
		}
	}
}
