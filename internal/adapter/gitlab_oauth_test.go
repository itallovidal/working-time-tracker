package adapter

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"working-time-tracker/testutil"
)

func gitlabOAuthFixture(t *testing.T) (*GitLabOAuth, *testutil.GitLab) {
	t.Helper()
	fake := testutil.NewGitLab()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return &GitLabOAuth{
		ClientID: testutil.GitLabClientID, ClientSecret: testutil.GitLabClientSecret,
		RedirectURL: "http://localhost:8081/integrations/gitlab/callback", SiteURL: srv.URL,
	}, fake
}

func TestGitLabOAuth_AuthorizeURL(t *testing.T) {
	o := &GitLabOAuth{ClientID: "id", ClientSecret: "secret", RedirectURL: "https://app.example/integrations/gitlab/callback"}
	if !o.Configured() || (&GitLabOAuth{ClientID: "id"}).Configured() || (*GitLabOAuth)(nil).Configured() {
		t.Error("configured needs the id, the secret and the return address")
	}
	u, err := url.Parse(o.AuthorizeURL("st4te"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "gitlab.com" || u.Path != "/oauth/authorize" || q.Get("client_id") != "id" || q.Get("response_type") != "code" ||
		q.Get("scope") != "api" || q.Get("state") != "st4te" || q.Get("redirect_uri") != o.RedirectURL {
		t.Errorf("authorize URL = %s", u)
	}
}

// A troca dá o par e o vencimento; a renovação gira o par, e o token de renovação gasto é recusado.
func TestGitLabOAuth_ExchangeAndRefresh(t *testing.T) {
	o, fake := gitlabOAuthFixture(t)
	cred, err := o.Exchange(testutil.GitLabOAuthCode)
	if err != nil || cred.Token != testutil.GitLabOAuthToken || cred.RefreshToken != testutil.GitLabRefreshToken {
		t.Fatalf("exchange = %+v, %v", cred, err)
	}
	if left := time.Until(cred.ExpiresAt); left < time.Hour+50*time.Minute || left > 2*time.Hour {
		t.Errorf("expires in %v, want about 2 hours", left)
	}
	if _, err := o.Exchange("wrong-code"); !errors.Is(err, ErrGitLabOAuthExchange) {
		t.Errorf("a wrong code: %v", err)
	}
	wrongApp := *o
	wrongApp.ClientSecret = "nope"
	if _, err := wrongApp.Exchange(testutil.GitLabOAuthCode); !errors.Is(err, ErrGitLabOAuthExchange) {
		t.Errorf("a wrong app secret: %v", err)
	}

	next, err := o.Refresh(cred.RefreshToken)
	if err != nil || next.Token == "" || next.Token == cred.Token || next.RefreshToken == cred.RefreshToken || next.ExpiresAt.IsZero() {
		t.Fatalf("refresh = %+v, %v", next, err)
	}
	if _, err := o.Refresh(cred.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("a spent refresh token: %v, want invalid_token", err)
	}
	if fake.Refreshes() != 1 {
		t.Errorf("%d refreshes", fake.Refreshes())
	}
	fake.Fail("POST", "/oauth/token", 503, 1)
	if _, err := o.Refresh(next.RefreshToken); !errors.Is(err, ErrProviderStatus) {
		t.Errorf("GitLab down: %v", err)
	}
	body := fake.Requests()[0].Body
	for _, want := range []string{"grant_type=authorization_code", "redirect_uri=", "client_secret="} {
		if !strings.Contains(body, want) {
			t.Errorf("token request body = %q, want %s", body, want)
		}
	}
}
