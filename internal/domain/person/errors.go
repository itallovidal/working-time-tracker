package person

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound            = apperr.New("person.not_found", http.StatusNotFound)
	ErrEmailInUse          = apperr.New("person.email_in_use", http.StatusConflict)
	ErrInvalidEmail        = apperr.New("person.invalid_email", http.StatusBadRequest)
	ErrInvalidRole         = apperr.New("person.invalid_role", http.StatusBadRequest)
	ErrLastAdmin           = apperr.New("person.last_admin", http.StatusBadRequest)
	ErrOwnerRole           = apperr.New("person.owner_is_admin", http.StatusBadRequest)
	ErrInvalidWeekHours    = apperr.New("person.invalid_week_hours", http.StatusBadRequest)
	ErrNameRequired        = apperr.New("person.name_required", http.StatusBadRequest)
	ErrEmailRequired       = apperr.New("person.email_required", http.StatusBadRequest)
	ErrInvalidOrganization = apperr.New("person.invalid_organization", http.StatusBadRequest)
)
