package organization

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound           = apperr.New("organization.not_found", http.StatusNotFound)
	ErrNameRequired       = apperr.New("organization.name_required", http.StatusBadRequest)
	ErrFieldTooLong       = apperr.New("organization.field_too_long", http.StatusBadRequest, "field", "max")
	ErrInvalidFoundedYear = apperr.New("organization.invalid_founded_year", http.StatusBadRequest)
	ErrInvalidSize        = apperr.New("organization.invalid_size", http.StatusBadRequest)
	ErrInvalidURL         = apperr.New("organization.invalid_url", http.StatusBadRequest)
	ErrInvalidEmail       = apperr.New("organization.invalid_email", http.StatusBadRequest)
	ErrInvalidPhone       = apperr.New("organization.invalid_phone", http.StatusBadRequest)
	ErrInvalidCNPJ        = apperr.New("organization.invalid_cnpj", http.StatusBadRequest)
	ErrInvalidWorkMode    = apperr.New("organization.invalid_work_mode", http.StatusBadRequest)
	ErrInvalidTimezone    = apperr.New("organization.invalid_timezone", http.StatusBadRequest)
	ErrInvalidCurrency    = apperr.New("organization.invalid_currency", http.StatusBadRequest)
	ErrHasProjects        = apperr.New("organization.has_projects", http.StatusBadRequest)
)
