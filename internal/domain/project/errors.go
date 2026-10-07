package project

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound            = apperr.New("project.not_found", http.StatusNotFound)
	ErrNameRequired        = apperr.New("project.name_required", http.StatusBadRequest)
	ErrInvalidSprint       = apperr.New("project.invalid_sprint", http.StatusBadRequest)
	ErrInvalidDailyTime    = apperr.New("project.invalid_daily_time", http.StatusBadRequest)
	ErrInvalidWeekday      = apperr.New("project.invalid_weekday", http.StatusBadRequest)
	ErrInvalidBillRate     = apperr.New("project.invalid_bill_rate", http.StatusBadRequest)
	ErrCustomerNotFound    = apperr.New("project.customer_not_found", http.StatusBadRequest)
	ErrInvalidOrganization = apperr.New("project.invalid_organization", http.StatusBadRequest)
)
