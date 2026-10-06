package integration_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"working-time-tracker/ent"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

// invalidGitHubToken é o único token que o GitHub fake rejeita.
const invalidGitHubToken = "invalid-token"

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()

	github := httptest.NewServer(fakeGitHub())
	adapter.Register("github", func() adapter.Integration {
		return &adapter.GitHubIntegration{BaseURL: github.URL}
	})

	code := m.Run()
	github.Close()
	os.Exit(code)
}

// fakeGitHub imita as duas chamadas que o adapter faz: validar o repositório e
// buscar uma issue. Conhece apenas o repositório owner/repo e a issue 42.
func fakeGitHub() http.Handler {
	mux := http.NewServeMux()
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "Bearer "+invalidGitHubToken {
				http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /repos/owner/repo", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"full_name":"owner/repo"}`))
	}))
	mux.HandleFunc("GET /repos/owner/repo/issues/42", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Corrigir login","state":"open","html_url":"https://github.com/owner/repo/issues/42"}`))
	}))
	return mux
}
