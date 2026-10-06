package team

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound       = apperr.New("team.not_found", http.StatusNotFound)
	ErrNameRequired   = apperr.New("team.name_required", http.StatusBadRequest)
	ErrPersonRequired = apperr.New("team.person_required", http.StatusBadRequest)
	ErrPersonNotInOrg = apperr.New("team.person_not_in_org", http.StatusBadRequest)
	ErrAlreadyMember  = apperr.New("team.already_member", http.StatusBadRequest)
)
