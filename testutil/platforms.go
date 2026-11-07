package testutil

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

// O app OAuth e a conta que o GitLab fake (gitlab.go) conhece, e o par de tokens em que cada código de retorno
// se troca. O segundo código existe para os testes de reconexão: o token novo lista outros projetos. A renovação
// gira o par, e os tokens renovados são outros (gloas-1, glrt-1, ...).
const (
	GitLabClientID           = "gitlab-client-id"
	GitLabClientSecret       = "gitlab-client-secret"
	GitLabLogin              = "octolab"
	GitLabUserID             = 1
	GitLabOAuthCode          = "gitlab-good-code"
	GitLabOAuthToken         = "gloas_oauth_token"
	GitLabRefreshToken       = "glrt_refresh_token"
	GitLabSecondOAuthCode    = "gitlab-second-code"
	GitLabSecondOAuthToken   = "gloas_second_token"
	GitLabSecondRefreshToken = "glrt_second_token"
)
