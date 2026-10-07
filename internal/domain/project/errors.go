package project

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound              = apperr.New("project.not_found", http.StatusNotFound)
	ErrNameRequired          = apperr.New("project.name_required", http.StatusBadRequest)
	ErrInvalidSprint         = apperr.New("project.invalid_sprint", http.StatusBadRequest)
	ErrInvalidDailyTime      = apperr.New("project.invalid_daily_time", http.StatusBadRequest)
	ErrInvalidWeekday        = apperr.New("project.invalid_weekday", http.StatusBadRequest)
	ErrInvalidWeeklyTime     = apperr.New("project.invalid_weekly_time", http.StatusBadRequest)
	ErrWeeklyTimeWithoutDay  = apperr.New("project.weekly_time_without_day", http.StatusBadRequest)
	ErrInvalidMeetingTime    = apperr.New("project.invalid_customer_meeting_time", http.StatusBadRequest)
	ErrMeetingTimeWithoutDay = apperr.New("project.customer_meeting_time_without_day", http.StatusBadRequest)
	ErrInvalidBillRate       = apperr.New("project.invalid_bill_rate", http.StatusBadRequest)
	ErrCustomerNotFound      = apperr.New("project.customer_not_found", http.StatusBadRequest)
	ErrInvalidOrganization   = apperr.New("project.invalid_organization", http.StatusBadRequest)
	ErrInvalidPage           = apperr.New("project.invalid_page", http.StatusBadRequest)
	ErrInvalidPerPage        = apperr.New("project.invalid_per_page", http.StatusBadRequest)
)
