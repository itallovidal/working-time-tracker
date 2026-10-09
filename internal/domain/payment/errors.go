package payment

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O texto em cada idioma está em internal/i18n/locales, na chave errors.<código>.
var (
	ErrOwnOnly = apperr.New("payment.own_only", http.StatusForbidden)
)
