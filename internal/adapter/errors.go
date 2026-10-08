package adapter

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros que as integrações devolvem. O provedor ("GitHub", "GitLab", "Trello") e
// os valores vão nos parâmetros; o texto em cada idioma está em errors.<código>
// nos catálogos. Os campos do metadata chegam como "field", com o nome do campo na API.
var (
	ErrUnsupportedType     = apperr.New("integration.unsupported_type", http.StatusBadRequest, "type")
	ErrProviderUnreachable = apperr.New("integration.provider_unreachable", http.StatusBadRequest, "provider")
	ErrInvalidToken        = apperr.New("integration.invalid_token", http.StatusBadRequest, "provider")
	ErrProviderStatus      = apperr.New("integration.provider_status", http.StatusBadRequest, "provider", "status")
	ErrItemNotFound        = apperr.New("integration.item_not_found", http.StatusBadRequest, "item")
	ErrUnexpectedResponse  = apperr.New("integration.unexpected_response", http.StatusBadRequest, "provider")
	ErrTokenRequired       = apperr.New("integration.token_required", http.StatusBadRequest, "provider")
	ErrFieldRequired       = apperr.New("integration.field_required", http.StatusBadRequest, "field", "provider")
	ErrFieldNotText        = apperr.New("integration.field_not_text", http.StatusBadRequest, "field", "provider")
	ErrInvalidIssueNumber  = apperr.New("integration.invalid_issue_number", http.StatusBadRequest)

	// ErrIssueGone é a issue que a plataforma não tem mais (apagada) ou que mudou de repositório.
	ErrIssueGone = apperr.New("integration.issue_gone", http.StatusNotFound, "item")
	// ErrIssuesDisabled é o repositório que desligou as issues: não há onde criar uma.
	ErrIssuesDisabled = apperr.New("integration.issues_disabled", http.StatusBadRequest, "provider")
	// ErrRateLimited é o limite de requisições da plataforma; "until" é o instante (segundos Unix) em
	// que ele acaba, quando a plataforma o informa.
	ErrRateLimited = apperr.New("integration.rate_limited", http.StatusTooManyRequests, "provider")
	// ErrForbidden é o token sem permissão para o que se pediu.
	ErrForbidden = apperr.New("integration.forbidden", http.StatusForbidden, "provider")
	// ErrListTooLong é a listagem que passou do teto de páginas.
	ErrListTooLong = apperr.New("integration.list_too_long", http.StatusBadRequest, "provider")

	ErrGitHubInvalidRepo  = apperr.New("integration.github_invalid_repo", http.StatusBadRequest)
	ErrGitHubRepoNotFound = apperr.New("integration.github_repo_not_found", http.StatusBadRequest)

	// A conexão com o GitHub por OAuth: o servidor sem app cadastrado, a pessoa que
	// cancelou no GitHub, o retorno que não bate com o que foi pedido e o código que o
	// GitHub não aceitou.
	ErrGitHubOAuthNotConfigured = apperr.New("integration.github_oauth_not_configured", http.StatusBadRequest)
	ErrGitHubOAuthDenied        = apperr.New("integration.github_oauth_denied", http.StatusBadRequest)
	ErrGitHubOAuthState         = apperr.New("integration.github_oauth_state", http.StatusBadRequest)
	ErrGitHubOAuthExchange      = apperr.New("integration.github_oauth_exchange", http.StatusBadRequest)

	ErrGitLabInvalidProject = apperr.New("integration.gitlab_invalid_project", http.StatusBadRequest)
	ErrGitLabForbidden      = apperr.New("integration.gitlab_forbidden", http.StatusBadRequest)
	ErrGitLabProjectMissing = apperr.New("integration.gitlab_project_not_found", http.StatusBadRequest)

	ErrTrelloInvalidKey     = apperr.New("integration.trello_invalid_key", http.StatusBadRequest)
	ErrTrelloInvalidBoard   = apperr.New("integration.trello_invalid_board", http.StatusBadRequest)
	ErrTrelloNoAccessBoard  = apperr.New("integration.trello_no_access_board", http.StatusBadRequest)
	ErrTrelloBoardMissing   = apperr.New("integration.trello_board_not_found", http.StatusBadRequest)
	ErrTrelloInvalidCard    = apperr.New("integration.trello_invalid_card", http.StatusBadRequest)
	ErrTrelloNoAccessCard   = apperr.New("integration.trello_no_access_card", http.StatusBadRequest)
	ErrTrelloCardOtherBoard = apperr.New("integration.trello_card_other_board", http.StatusBadRequest, "card")
)
