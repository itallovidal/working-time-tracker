package customer

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound            = apperr.New("customer.not_found", http.StatusNotFound)
	ErrNameRequired        = apperr.New("customer.name_required", http.StatusBadRequest)
	ErrNameTooLong         = apperr.New("customer.name_too_long", http.StatusBadRequest)
	ErrContactTooLong      = apperr.New("customer.contact_too_long", http.StatusBadRequest)
	ErrInvalidDocument     = apperr.New("customer.invalid_document", http.StatusBadRequest)
	ErrInvalidEmail        = apperr.New("customer.invalid_email", http.StatusBadRequest)
	ErrInvalidPhone        = apperr.New("customer.invalid_phone", http.StatusBadRequest)
	ErrHasProjects         = apperr.New("customer.has_projects", http.StatusBadRequest)
	ErrInvalidOrganization = apperr.New("customer.invalid_organization", http.StatusBadRequest)
)
