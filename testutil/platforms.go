package testutil

import (
	"fmt"
	"net/http"
	"strings"
)

// InvalidToken é o único token que as plataformas fake rejeitam.
const InvalidToken = "invalid-token"

// TrelloKey é a única chave de API que o Trello fake aceita.
const TrelloKey = "0123456789abcdef0123456789abcdef"

// Os quadros que o Trello fake conhece, pelo id e pelo link curto.
const (
	TrelloBoardID        = "5abbe4b7ddc1b351ef961414"
	TrelloBoardShortLink = "AbC123xy"
)

// FakeGitHub imita as duas chamadas que o adapter faz: validar o repositório e
// buscar uma issue. Conhece os repositórios owner/repo e owner/other e a issue 42
// do primeiro.
func FakeGitHub() http.Handler {
	mux := http.NewServeMux()
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "Bearer "+InvalidToken {
				http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /repos/owner/repo", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"full_name":"owner/repo"}`))
	}))
	mux.HandleFunc("GET /repos/owner/other", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"full_name":"owner/other"}`))
	}))
	mux.HandleFunc("GET /repos/owner/repo/issues/42", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Corrigir login","state":"open","html_url":"https://github.com/owner/repo/issues/42"}`))
	}))
	return mux
}

// FakeGitLab conhece o projeto group/project e a issue 7 dele. O adapter manda o
// caminho do projeto escapado num segmento só, como a API do GitLab pede.
func FakeGitLab() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") == InvalidToken {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		switch r.URL.EscapedPath() {
		case "/projects/group%2Fproject":
			w.Write([]byte(`{"path_with_namespace":"group/project"}`))
		case "/projects/group%2Fproject/issues/7":
			w.Write([]byte(`{"title":"Ajustar relatório","state":"opened","web_url":"https://gitlab.com/group/project/-/issues/7"}`))
		default:
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
		}
	})
}

// FakeTrello imita o quadro e os cartões que o adapter lê. Só aceita a chave e o
// token no cabeçalho Authorization: com eles na URL, responde 400. Conhece um quadro
// (pelo id e pelo link curto) e três cartões: H0TZyzbK, aberto na lista "Em
// andamento"; ArqUiv4d, arquivado; e OutroQdr, que é de outro quadro.
func FakeTrello() http.Handler {
	const card = `{"id":"5abbe4b7ddc1b351ef961499","name":%q,"closed":%s,"shortUrl":"https://trello.com/c/%s","idBoard":%q,` +
		`"list":{"id":"5abbe4b7ddc1b351ef961477","name":"Em andamento"},"board":{"id":%q,"shortLink":%q}}`
	write := func(w http.ResponseWriter, name, closed, shortLink, boardID, boardShortLink string) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, card, name, closed, shortLink, boardID, boardID, boardShortLink)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Has("key") || query.Has("token") {
			http.Error(w, "credentials in the URL", http.StatusBadRequest)
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.Contains(header, `oauth_consumer_key="`+TrelloKey+`"`) {
			http.Error(w, "invalid key", http.StatusUnauthorized)
			return
		}
		if !strings.Contains(header, `oauth_token="`) || strings.Contains(header, `oauth_token="`+InvalidToken+`"`) {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/boards/" + TrelloBoardID, "/boards/" + TrelloBoardShortLink:
			w.Write([]byte(`{"id":"` + TrelloBoardID + `","name":"App"}`))
			return
		}
		// O adapter depende destes dois parâmetros para receber a lista e o quadro.
		if strings.HasPrefix(r.URL.Path, "/cards/") && (query.Get("list") != "true" || query.Get("board") != "true") {
			http.Error(w, "missing nested resources", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/cards/H0TZyzbK":
			write(w, "Corrigir login", "false", "H0TZyzbK", TrelloBoardID, TrelloBoardShortLink)
		case "/cards/ArqUiv4d":
			write(w, "Tela antiga", "true", "ArqUiv4d", TrelloBoardID, TrelloBoardShortLink)
		case "/cards/OutroQdr":
			write(w, "De outro quadro", "false", "OutroQdr", "aaaaaaaaaaaaaaaaaaaaaaaa", "ZzZ999zz")
		default:
			http.Error(w, "The requested resource was not found.", http.StatusNotFound)
		}
	})
}
