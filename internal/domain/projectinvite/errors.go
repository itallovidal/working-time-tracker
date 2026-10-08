package projectinvite

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrTeamNotInProject = apperr.New("projectinvite.team_not_in_project", http.StatusBadRequest)
)
