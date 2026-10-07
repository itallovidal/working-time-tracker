package allocation

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrRateRequired    = apperr.New("allocation.rate_required", http.StatusBadRequest)
	ErrInvalidRate     = apperr.New("allocation.invalid_rate", http.StatusBadRequest)
	ErrPersonNotInOrg  = apperr.New("allocation.person_not_in_org", http.StatusBadRequest)
	ErrNotDefined      = apperr.New("allocation.not_defined", http.StatusNotFound)
	ErrOwnRatesOnly    = apperr.New("allocation.own_rates_only", http.StatusForbidden)
	ErrInvalidPreset   = apperr.New("allocation.invalid_preset", http.StatusBadRequest)
	ErrAbovePermission = apperr.New("allocation.preset_above_yours", http.StatusForbidden)
)
