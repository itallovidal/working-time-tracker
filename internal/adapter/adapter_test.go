package adapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"working-time-tracker/testutil"
)

func TestDescriptors_OrderAndShape(t *testing.T) {
	var types []string
	for _, d := range Descriptors() {
		types = append(types, d.Type)
		if d.Label == "" {
			t.Errorf("%s: descriptor without the platform name: %+v", d.Type, d)
		}
		if len(d.Metadata) == 0 {
			t.Errorf("%s: descriptor declares no metadata field", d.Type)
		}
		for _, f := range d.Metadata {
			if f.Key == "" {
				t.Errorf("%s: metadata field without key: %+v", d.Type, f)
			}
		}
	}
	if want := []string{"github", "gitlab", "trello"}; !reflect.DeepEqual(types, want) {
		t.Errorf("types = %v, want %v", types, want)
	}
}

// CheckMetadata não fala com a plataforma: confere os campos do tipo e devolve o que
// deve ser guardado.
func TestCheckMetadata(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		raw     map[string]any
		want    map[string]any
		wantErr string
	}{
		{name: "github plain", typ: "github", raw: map[string]any{"repo": " owner/repo "}, want: map[string]any{"repo": "owner/repo"}},
		{name: "github url", typ: "github", raw: map[string]any{"repo": "https://github.com/owner/repo.git"}, want: map[string]any{"repo": "owner/repo"}},
		{name: "github drops unknown keys", typ: "github", raw: map[string]any{"repo": "owner/repo", "token": "ghp_leak", "extra": 1},
			want: map[string]any{"repo": "owner/repo"}},
		{name: "github missing repo", typ: "github", raw: map[string]any{}, wantErr: "integration.field_required"},
		{name: "github nil metadata", typ: "github", raw: nil, wantErr: "integration.field_required"},
		{name: "github blank repo", typ: "github", raw: map[string]any{"repo": "   "}, wantErr: "integration.field_required"},
		{name: "github repo is not a text", typ: "github", raw: map[string]any{"repo": 42}, wantErr: "integration.field_not_text"},
		{name: "github repo without owner", typ: "github", raw: map[string]any{"repo": "repo"}, wantErr: "integration.github_invalid_repo"},
		{name: "github repo climbing the path", typ: "github", raw: map[string]any{"repo": "owner/.."}, wantErr: "integration.github_invalid_repo"},
		{name: "github repo with extra path", typ: "github", raw: map[string]any{"repo": "owner/repo/issues"}, wantErr: "integration.github_invalid_repo"},

		{name: "gitlab plain", typ: "gitlab", raw: map[string]any{"project_url": "group/sub/project"}, want: map[string]any{"project_url": "group/sub/project"}},
		{name: "gitlab url", typ: "gitlab", raw: map[string]any{"project_url": "https://gitlab.com/group/project/-/issues"},
			want: map[string]any{"project_url": "group/project"}},
		{name: "gitlab missing project", typ: "gitlab", raw: map[string]any{"repo": "group/project"}, wantErr: "integration.field_required"},
		{name: "gitlab project without group", typ: "gitlab", raw: map[string]any{"project_url": "project"}, wantErr: "integration.gitlab_invalid_project"},

		{name: "trello ids", typ: "trello", raw: map[string]any{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardID},
			want: map[string]any{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardID}},
		{name: "trello board url", typ: "trello",
			raw:  map[string]any{"api_key": testutil.TrelloKey, "board_id": "https://trello.com/b/AbC123xy/app-do-cliente"},
			want: map[string]any{"api_key": testutil.TrelloKey, "board_id": "AbC123xy"}},
		{name: "trello missing key", typ: "trello", raw: map[string]any{"board_id": "AbC123xy"}, wantErr: "integration.field_required"},
		{name: "trello missing board", typ: "trello", raw: map[string]any{"api_key": testutil.TrelloKey}, wantErr: "integration.field_required"},
		{name: "trello key breaking the header", typ: "trello", raw: map[string]any{"api_key": `abc", oauth_token="x`, "board_id": "AbC123xy"},
			wantErr: "integration.trello_invalid_key"},
		{name: "trello board climbing the path", typ: "trello", raw: map[string]any{"api_key": testutil.TrelloKey, "board_id": "../members/me"},
			wantErr: "integration.trello_invalid_board"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			impl, err := GetIntegration(tc.typ)
			if err != nil {
				t.Fatalf("get integration: %v", err)
			}
			got, err := impl.CheckMetadata(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("metadata = %v, want %v", got, tc.want)
			}
		})
	}
}

// counted conta as requisições que chegam à plataforma fake.
func counted(next http.Handler) (http.Handler, *atomic.Int32) {
	var hits atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		next.ServeHTTP(w, r)
	}), &hits
}

func TestGitHub_ValidateAndFetch(t *testing.T) {
	handler, hits := counted(testutil.FakeGitHub())
	srv := httptest.NewServer(handler)
	defer srv.Close()
	g := &GitHubIntegration{BaseURL: srv.URL}
	conn := Connection{Token: "ghp_test", Metadata: map[string]any{"repo": "owner/repo"}}

	if err := g.Validate(conn); err != nil {
		t.Fatalf("validate: %v", err)
	}
	details, err := g.FetchItemDetails(conn, "#42")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if details.Title != "Corrigir login" || details.State != "open" {
		t.Errorf("details = %+v", details)
	}

	cases := []struct {
		name    string
		conn    Connection
		wantErr string
	}{
		{"invalid token", Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}, "integration.invalid_token"},
		{"unknown repository", Connection{Token: "ghp_test", Metadata: map[string]any{"repo": "owner/missing"}}, "integration.github_repo_not_found"},
	}
	for _, tc := range cases {
		if err := g.Validate(tc.conn); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}

	// O que não chega a virar requisição: sem token, sem metadata, e um "número de
	// issue" que tentaria sair do repositório configurado.
	before := hits.Load()
	if err := g.Validate(Connection{Metadata: conn.Metadata}); err == nil || !strings.Contains(err.Error(), "integration.token_required") {
		t.Errorf("validate without token: %v", err)
	}
	if err := g.Validate(Connection{Token: "ghp_test"}); err == nil || !strings.Contains(err.Error(), "integration.field_required") {
		t.Errorf("validate without metadata: %v", err)
	}
	if _, err := g.FetchItemDetails(conn, "../../../owner/other/issues/1"); err == nil || !strings.Contains(err.Error(), "integration.invalid_issue_number") {
		t.Errorf("fetch with a path as the issue number: %v", err)
	}
	if after := hits.Load(); after != before {
		t.Errorf("%d request(s) reached the platform for input that should be refused before it", after-before)
	}
}

// A conexão OAuth: o endereço de autorização leva o que o GitHub precisa, o código da volta
// vira um token, e o token diz quem é e quais repositórios enxerga.
func TestGitHubOAuth_AuthorizeAndExchange(t *testing.T) {
	srv := httptest.NewServer(testutil.FakeGitHub())
	defer srv.Close()
	oauth := &GitHubOAuth{
		ClientID: testutil.GitHubClientID, ClientSecret: testutil.GitHubClientSecret,
		RedirectURL: "http://wtt.test/integrations/github/callback", SiteURL: srv.URL,
	}
	if !oauth.Configured() {
		t.Fatal("an app with id, secret and redirect URL is not configured")
	}
	for name, o := range map[string]*GitHubOAuth{
		"nil":          nil,
		"no id":        {ClientSecret: "s", RedirectURL: "http://x/cb"},
		"no secret":    {ClientID: "i", RedirectURL: "http://x/cb"},
		"no redirect":  {ClientID: "i", ClientSecret: "s"},
		"empty struct": {},
	} {
		if o.Configured() {
			t.Errorf("%s: Configured() = true", name)
		}
	}

	u, err := url.Parse(oauth.AuthorizeURL("the-state"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Path != "/login/oauth/authorize" || q.Get("client_id") != testutil.GitHubClientID ||
		q.Get("redirect_uri") != oauth.RedirectURL || q.Get("scope") != "repo" || q.Get("state") != "the-state" {
		t.Errorf("authorize URL = %s", u)
	}

	token, err := oauth.Exchange(testutil.GitHubOAuthCode)
	if err != nil || token != testutil.GitHubOAuthToken {
		t.Errorf("exchange = %q, %v, want the fake's token", token, err)
	}
	// O GitHub responde 200 também quando recusa: o motivo vem no corpo.
	if _, err := oauth.Exchange("expired-code"); err == nil || !strings.Contains(err.Error(), "integration.github_oauth_exchange") {
		t.Errorf("exchange with a bad code: %v", err)
	}
	wrong := *oauth
	wrong.ClientSecret = "wrong"
	if _, err := wrong.Exchange(testutil.GitHubOAuthCode); err == nil || !strings.Contains(err.Error(), "integration.github_oauth_exchange") {
		t.Errorf("exchange with a wrong secret: %v", err)
	}
	srv.Close()
	if _, err := oauth.Exchange(testutil.GitHubOAuthCode); err == nil || !strings.Contains(err.Error(), "integration.provider_unreachable") {
		t.Errorf("exchange with the site down: %v", err)
	}
}

func TestGitHub_AccountAndRepositories(t *testing.T) {
	srv := httptest.NewServer(testutil.FakeGitHub())
	defer srv.Close()
	g := &GitHubIntegration{BaseURL: srv.URL}
	conn := Connection{Token: testutil.GitHubOAuthToken}

	login, err := g.Account(conn)
	if err != nil || login != testutil.GitHubLogin {
		t.Errorf("account = %q, %v, want %q", login, err, testutil.GitHubLogin)
	}
	repos, err := g.ListRepositories(conn)
	want := []Repository{{FullName: "owner/repo", Private: true}, {FullName: "owner/other"}}
	if err != nil || !reflect.DeepEqual(repos, want) {
		t.Errorf("repositories = %v, %v, want %v", repos, err, want)
	}

	bad := Connection{Token: testutil.InvalidToken}
	if _, err := g.Account(bad); err == nil || !strings.Contains(err.Error(), "integration.invalid_token") {
		t.Errorf("account with an invalid token: %v", err)
	}
	if _, err := g.ListRepositories(bad); err == nil || !strings.Contains(err.Error(), "integration.invalid_token") {
		t.Errorf("repositories with an invalid token: %v", err)
	}
	if _, err := g.Account(Connection{}); err == nil || !strings.Contains(err.Error(), "integration.token_required") {
		t.Errorf("account without a token: %v", err)
	}

	// O GitHub é o único tipo que se autoriza no site e que sabe listar; GitLab e Trello não.
	var _ AccountLookup = g
	var _ RepositoryLister = g
	if d := g.Descriptor(); d.Auth != AuthOAuth || d.ComingSoon {
		t.Errorf("github descriptor: auth %q, coming soon %v", d.Auth, d.ComingSoon)
	}
	for _, other := range []Integration{&GitLabIntegration{}, &TrelloIntegration{}} {
		if _, ok := other.(AccountLookup); ok {
			t.Errorf("%s looks up accounts", other.Descriptor().Type)
		}
		if _, ok := other.(RepositoryLister); ok {
			t.Errorf("%s lists repositories", other.Descriptor().Type)
		}
		if d := other.Descriptor(); d.Auth == AuthOAuth || !d.ComingSoon {
			t.Errorf("%s descriptor: auth %q, coming soon %v", d.Type, d.Auth, d.ComingSoon)
		}
	}
}

func TestGitLab_ValidateAndFetch(t *testing.T) {
	srv := httptest.NewServer(testutil.FakeGitLab())
	defer srv.Close()
	g := &GitLabIntegration{BaseURL: srv.URL}
	conn := Connection{Token: "glpat-test", Metadata: map[string]any{"project_url": "https://gitlab.com/group/project"}}

	if err := g.Validate(conn); err != nil {
		t.Fatalf("validate: %v", err)
	}
	details, err := g.FetchItemDetails(conn, "7")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if details.Title != "Ajustar relatório" || details.State != "opened" {
		t.Errorf("details = %+v", details)
	}

	if err := g.Validate(Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}); err == nil || !strings.Contains(err.Error(), "integration.invalid_token") {
		t.Errorf("invalid token: %v", err)
	}
	missing := Connection{Token: "glpat-test", Metadata: map[string]any{"project_url": "group/missing"}}
	if err := g.Validate(missing); err == nil || !strings.Contains(err.Error(), "integration.gitlab_project_not_found") {
		t.Errorf("unknown project: %v", err)
	}
	if _, err := g.FetchItemDetails(conn, "999"); err == nil || !strings.Contains(err.Error(), "integration.item_not_found") {
		t.Errorf("unknown issue: %v", err)
	}
}

func TestTrello_Validate(t *testing.T) {
	srv := httptest.NewServer(testutil.FakeTrello())
	defer srv.Close()
	tr := &TrelloIntegration{BaseURL: srv.URL}
	meta := func(board string) map[string]any {
		return map[string]any{"api_key": testutil.TrelloKey, "board_id": board}
	}

	for _, board := range []string{testutil.TrelloBoardID, testutil.TrelloBoardShortLink, "https://trello.com/b/AbC123xy/app"} {
		if err := tr.Validate(Connection{Token: "trello-token", Metadata: meta(board)}); err != nil {
			t.Errorf("validate with board %q: %v", board, err)
		}
	}

	cases := []struct {
		name    string
		conn    Connection
		wantErr string
	}{
		{"invalid token", Connection{Token: testutil.InvalidToken, Metadata: meta(testutil.TrelloBoardID)}, "integration.trello_no_access_board"},
		{"wrong key", Connection{Token: "trello-token", Metadata: map[string]any{"api_key": "outrachave", "board_id": testutil.TrelloBoardID}}, "integration.trello_no_access_board"},
		{"unknown board", Connection{Token: "trello-token", Metadata: meta("ZzZ999zz")}, "integration.trello_board_not_found"},
		{"no token", Connection{Metadata: meta(testutil.TrelloBoardID)}, "integration.token_required"},
		{"token breaking the header", Connection{Token: `x", extra="y`, Metadata: meta(testutil.TrelloBoardID)}, "integration.invalid_token"},
		{"no metadata", Connection{Token: "trello-token"}, "integration.field_required"},
	}
	for _, tc := range cases {
		if err := tr.Validate(tc.conn); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestTrello_FetchItemDetails(t *testing.T) {
	srv := httptest.NewServer(testutil.FakeTrello())
	defer srv.Close()
	tr := &TrelloIntegration{BaseURL: srv.URL}
	// O quadro pode ter sido configurado pelo id ou pelo link curto: o cartão vale nos dois.
	for _, board := range []string{testutil.TrelloBoardID, testutil.TrelloBoardShortLink} {
		conn := Connection{Token: "trello-token", Metadata: map[string]any{"api_key": testutil.TrelloKey, "board_id": board}}

		for _, item := range []string{"H0TZyzbK", " https://trello.com/c/H0TZyzbK/12-corrigir-login "} {
			details, err := tr.FetchItemDetails(conn, item)
			if err != nil {
				t.Fatalf("board %q, item %q: %v", board, item, err)
			}
			want := ItemDetails{Title: "Corrigir login", State: "Em andamento", URL: "https://trello.com/c/H0TZyzbK"}
			if *details != want {
				t.Errorf("board %q, item %q: details = %+v, want %+v", board, item, *details, want)
			}
		}

		archived, err := tr.FetchItemDetails(conn, "ArqUiv4d")
		if err != nil {
			t.Fatalf("archived card: %v", err)
		}
		if archived.State != "arquivado" {
			t.Errorf("archived card state = %q, want %q", archived.State, "arquivado")
		}

		cases := []struct{ item, wantErr string }{
			{"OutroQdr", "integration.trello_card_other_board"},
			{"NaoExist", "integration.item_not_found"},
			{"../boards/" + testutil.TrelloBoardID, "integration.trello_invalid_card"},
			{"", "integration.trello_invalid_card"},
		}
		for _, tc := range cases {
			if _, err := tr.FetchItemDetails(conn, tc.item); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("board %q, item %q: error = %v, want one containing %q", board, tc.item, err, tc.wantErr)
			}
		}
	}
}

func TestStopsSync(t *testing.T) {
	stops := []error{
		context.Canceled, context.DeadlineExceeded,
		ErrInvalidToken.With("provider", "GitHub"), ErrProviderUnreachable.With("provider", "Trello"),
		ErrGitHubRepoNotFound, ErrTrelloBoardMissing, ErrTrelloNoAccessBoard,
		ErrRateLimited.With("provider", "Trello", "until", int64(0)),
	}
	for _, err := range stops {
		if !StopsSync(err) {
			t.Errorf("StopsSync(%v) = false, want true", err)
		}
	}
	keeps := []error{nil, ErrIssueGone, ErrForbidden, ErrListTooLong, ErrProviderStatus.With("provider", "GitHub", "status", 422), errors.New("boom")}
	for _, err := range keeps {
		if StopsSync(err) {
			t.Errorf("StopsSync(%v) = true, want false", err)
		}
	}
}

func TestDescriptor_SummaryKey(t *testing.T) {
	for typ, want := range map[string]string{"github": "repo", "trello": "board_id"} {
		impl, err := GetIntegration(typ)
		if err != nil {
			t.Fatalf("get %s: %v", typ, err)
		}
		if got := impl.Descriptor().SummaryKey(); got != want {
			t.Errorf("%s summary key = %q, want %q", typ, got, want)
		}
	}
	if got := (Descriptor{Metadata: []Field{{Key: "x"}}}).SummaryKey(); got != "" {
		t.Errorf("a type with no summary field has key %q, want none", got)
	}
}

func TestGitHubDescriptor_SyncCaps(t *testing.T) {
	caps := githubDescriptor.Caps
	if !caps.Assignee || !caps.ServerSince || caps.Deadline || caps.AutoPublish {
		t.Errorf("github caps = %+v, want assignee and server-side since only", caps)
	}
}
