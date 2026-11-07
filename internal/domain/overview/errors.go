package overview

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrInvalidTaskState = apperr.New("overview.invalid_task_state", http.StatusBadRequest)
	// ErrInvalidWindow é uma janela da tela inicial do modo solo que não é month, last_month nem year.
	ErrInvalidWindow = apperr.New("overview.invalid_window", http.StatusBadRequest)
)
