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
		return &adapter.GitLabIntegration{BaseURL: gitlab.URL + "/api/v4"}
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

// basicOnly é um tipo que só tem o básico de Integration, sem as capacidades opcionais (dizer quem o token
// representa, listar o que a conexão enxerga, sincronizar as issues).
type basicOnly struct{ adapter.Integration }

// withBasicOnly troca o tipo, durante o teste, pela versão só com o básico: é como se testa o que acontece com
// um tipo que não sabe essas coisas, agora que os três sabem.
func withBasicOnly(t *testing.T, typ string) {
	t.Helper()
	orig, err := adapter.GetIntegration(typ)
	if err != nil {
		t.Fatal(err)
	}
	adapter.Register(typ, func() adapter.Integration { return basicOnly{orig} })
	t.Cleanup(func() { adapter.Register(typ, func() adapter.Integration { return orig }) })
}
