package testutil

import (
	"net/http"
)

// InvalidToken é o único token que as plataformas fake rejeitam.
const InvalidToken = "invalid-token"

// TrelloKey é a única chave de API que o Trello fake aceita.
const TrelloKey = "0123456789abcdef0123456789abcdef"

// O app OAuth e a conta que o GitHub fake (github.go) conhece, e o token em que cada código de
// retorno se troca. O segundo código existe para os testes de reconexão: o token novo lista
// outros repositórios, e é assim que se vê que ele substituiu o antigo.
const (
	GitHubClientID         = "test-client-id"
	GitHubClientSecret     = "test-client-secret"
	GitHubLogin            = "octocat"
	GitHubOAuthCode        = "good-code"
	GitHubOAuthToken       = "gho_oauth_token"
	GitHubSecondOAuthCode  = "second-code"
	GitHubSecondOAuthToken = "gho_second_token"
)

// Os quadros que o Trello fake (trello.go) conhece, pelo id e pelo link curto, o token em que a autorização
// se resolve, e o usuário que ele representa. O segundo token existe para os testes de reconexão.
const (
	TrelloBoardID             = "5abbe4b7ddc1b351ef961414"
	TrelloBoardShortLink      = "AbC123xy"
	TrelloOtherBoardID        = "aaaaaaaaaaaaaaaaaaaaaaaa"
	TrelloOtherBoardShortLink = "OtroBrd1"
	TrelloOAuthToken          = "trello_oauth_token"
	TrelloSecondOAuthToken    = "trello_second_token"
	TrelloUsername            = "octotrello"
)

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
