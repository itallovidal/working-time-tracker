package integration_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"working-time-tracker/ent"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

// platformCalls conta as requisições que chegam às plataformas fake: é como os testes
// sabem se uma edição consultou a plataforma de novo.
var platformCalls atomic.Int32

// As chamadas às plataformas vão para servidores fake (testutil), que rejeitam
// apenas testutil.InvalidToken.
func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()

	serve := func(next http.Handler) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			platformCalls.Add(1)
			next.ServeHTTP(w, r)
		}))
	}
	github, gitlab, trello := serve(testutil.FakeGitHub()), serve(testutil.FakeGitLab()), serve(testutil.FakeTrello())
	adapter.Register("github", func() adapter.Integration {
		return &adapter.GitHubIntegration{BaseURL: github.URL}
	})
	adapter.Register("gitlab", func() adapter.Integration {
		return &adapter.GitLabIntegration{BaseURL: gitlab.URL}
	})
	adapter.Register("trello", func() adapter.Integration {
		return &adapter.TrelloIntegration{BaseURL: trello.URL}
	})

	code := m.Run()
	github.Close()
	gitlab.Close()
	trello.Close()
	os.Exit(code)
}
