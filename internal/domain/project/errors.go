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
	// ErrMeetingNeedsCustomer recusa a reunião com o cliente num projeto sem cliente. O field é customer_meeting_day.
	ErrMeetingNeedsCustomer = apperr.New("project.meeting_needs_customer", http.StatusBadRequest)
	ErrInvalidBillRate      = apperr.New("project.invalid_bill_rate", http.StatusBadRequest)
	// ErrBillRateRequired recusa o projeto com cliente e sem valor cobrado. O field é bill_rate_cents.
	ErrBillRateRequired = apperr.New("project.bill_rate_required", http.StatusBadRequest)
	// ErrBillRateNeedsCustomer recusa o valor cobrado num projeto sem cliente. O field é bill_rate_cents.
	ErrBillRateNeedsCustomer = apperr.New("project.bill_rate_needs_customer", http.StatusBadRequest)
	ErrCustomerNotFound      = apperr.New("project.customer_not_found", http.StatusBadRequest)
	ErrInvalidOrganization   = apperr.New("project.invalid_organization", http.StatusBadRequest)
	ErrInvalidPage           = apperr.New("project.invalid_page", http.StatusBadRequest)
	ErrInvalidPerPage        = apperr.New("project.invalid_per_page", http.StatusBadRequest)

	// ErrBillRateNotAllowed recusa o valor cobrado num projeto de cliente mensal: o contrato é um valor por mês, e o
	// projeto não tem valor por hora. O field é bill_rate_cents.
	ErrBillRateNotAllowed = apperr.New("project.bill_rate_not_allowed", http.StatusBadRequest)

	// ErrCustomerLocked recusa tirar do cliente o projeto que tem sessões quando o cliente já tem recebíveis: as horas
	// iam para as propostas do cliente novo e o recebível do antigo continuaria cobrando as mesmas. O field é customer_id.
	ErrCustomerLocked = apperr.New("project.customer_locked", http.StatusConflict)
)
