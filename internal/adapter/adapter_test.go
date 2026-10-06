package adapter

import (
	"net/http"
	"net/http/httptest"
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
		if d.Label == "" || d.ItemLabel == "" {
			t.Errorf("%s: descriptor without a label or an item label: %+v", d.Type, d)
		}
		if len(d.Metadata) == 0 {
			t.Errorf("%s: descriptor declares no metadata field", d.Type)
		}
		for _, f := range d.Metadata {
			if f.Key == "" || f.Label == "" {
				t.Errorf("%s: metadata field without key or label: %+v", d.Type, f)
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
		{name: "github missing repo", typ: "github", raw: map[string]any{}, wantErr: `informe o campo "Repositório" do GitHub`},
		{name: "github nil metadata", typ: "github", raw: nil, wantErr: `informe o campo "Repositório" do GitHub`},
		{name: "github blank repo", typ: "github", raw: map[string]any{"repo": "   "}, wantErr: `informe o campo "Repositório" do GitHub`},
		{name: "github repo is not a text", typ: "github", raw: map[string]any{"repo": 42}, wantErr: "precisa ser um texto"},
		{name: "github repo without owner", typ: "github", raw: map[string]any{"repo": "repo"}, wantErr: "repositório do GitHub inválido"},
		{name: "github repo climbing the path", typ: "github", raw: map[string]any{"repo": "owner/.."}, wantErr: "repositório do GitHub inválido"},
		{name: "github repo with extra path", typ: "github", raw: map[string]any{"repo": "owner/repo/issues"}, wantErr: "repositório do GitHub inválido"},

		{name: "gitlab plain", typ: "gitlab", raw: map[string]any{"project_url": "group/sub/project"}, want: map[string]any{"project_url": "group/sub/project"}},
		{name: "gitlab url", typ: "gitlab", raw: map[string]any{"project_url": "https://gitlab.com/group/project/-/issues"},
			want: map[string]any{"project_url": "group/project"}},
		{name: "gitlab missing project", typ: "gitlab", raw: map[string]any{"repo": "group/project"}, wantErr: `informe o campo "Projeto" do GitLab`},
		{name: "gitlab project without group", typ: "gitlab", raw: map[string]any{"project_url": "project"}, wantErr: "projeto do GitLab inválido"},

		{name: "trello ids", typ: "trello", raw: map[string]any{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardID},
			want: map[string]any{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardID}},
		{name: "trello board url", typ: "trello",
			raw:  map[string]any{"api_key": testutil.TrelloKey, "board_id": "https://trello.com/b/AbC123xy/app-do-cliente"},
			want: map[string]any{"api_key": testutil.TrelloKey, "board_id": "AbC123xy"}},
		{name: "trello missing key", typ: "trello", raw: map[string]any{"board_id": "AbC123xy"}, wantErr: `informe o campo "Chave da API" do Trello`},
		{name: "trello missing board", typ: "trello", raw: map[string]any{"api_key": testutil.TrelloKey}, wantErr: `informe o campo "Quadro" do Trello`},
		{name: "trello key breaking the header", typ: "trello", raw: map[string]any{"api_key": `abc", oauth_token="x`, "board_id": "AbC123xy"},
			wantErr: "chave da API do Trello inválida"},
		{name: "trello board climbing the path", typ: "trello", raw: map[string]any{"api_key": testutil.TrelloKey, "board_id": "../members/me"},
			wantErr: "quadro do Trello inválido"},
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
		{"invalid token", Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}, "token do GitHub inválido"},
		{"unknown repository", Connection{Token: "ghp_test", Metadata: map[string]any{"repo": "owner/missing"}}, "não encontrado ou o token não tem acesso"},
	}
	for _, tc := range cases {
		if err := g.Validate(tc.conn); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}

	// O que não chega a virar requisição: sem token, sem metadata, e um "número de
	// issue" que tentaria sair do repositório configurado.
	before := hits.Load()
	if err := g.Validate(Connection{Metadata: conn.Metadata}); err == nil || !strings.Contains(err.Error(), "informe o token do GitHub") {
		t.Errorf("validate without token: %v", err)
	}
	if err := g.Validate(Connection{Token: "ghp_test"}); err == nil || !strings.Contains(err.Error(), `informe o campo "Repositório"`) {
		t.Errorf("validate without metadata: %v", err)
	}
	if _, err := g.FetchItemDetails(conn, "../../../owner/other/issues/1"); err == nil || !strings.Contains(err.Error(), "só dígitos") {
		t.Errorf("fetch with a path as the issue number: %v", err)
	}
	if after := hits.Load(); after != before {
		t.Errorf("%d request(s) reached the platform for input that should be refused before it", after-before)
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

	if err := g.Validate(Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}); err == nil || !strings.Contains(err.Error(), "token do GitLab inválido") {
		t.Errorf("invalid token: %v", err)
	}
	missing := Connection{Token: "glpat-test", Metadata: map[string]any{"project_url": "group/missing"}}
	if err := g.Validate(missing); err == nil || !strings.Contains(err.Error(), "não encontrado ou o token não tem acesso") {
		t.Errorf("unknown project: %v", err)
	}
	if _, err := g.FetchItemDetails(conn, "999"); err == nil || !strings.Contains(err.Error(), "item 999 não encontrado") {
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
		{"invalid token", Connection{Token: testutil.InvalidToken, Metadata: meta(testutil.TrelloBoardID)}, "chave ou token do Trello inválido"},
		{"wrong key", Connection{Token: "trello-token", Metadata: map[string]any{"api_key": "outrachave", "board_id": testutil.TrelloBoardID}}, "chave ou token do Trello inválido"},
		{"unknown board", Connection{Token: "trello-token", Metadata: meta("ZzZ999zz")}, "quadro do Trello não encontrado"},
		{"no token", Connection{Metadata: meta(testutil.TrelloBoardID)}, "informe o token do Trello"},
		{"token breaking the header", Connection{Token: `x", extra="y`, Metadata: meta(testutil.TrelloBoardID)}, "token do Trello inválido"},
		{"no metadata", Connection{Token: "trello-token"}, `informe o campo "Chave da API" do Trello`},
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
			{"OutroQdr", "é de outro quadro"},
			{"NaoExist", "item NaoExist não encontrado"},
			{"../boards/" + testutil.TrelloBoardID, "cartão do Trello inválido"},
			{"", "cartão do Trello inválido"},
		}
		for _, tc := range cases {
			if _, err := tr.FetchItemDetails(conn, tc.item); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("board %q, item %q: error = %v, want one containing %q", board, tc.item, err, tc.wantErr)
			}
		}
	}
}
