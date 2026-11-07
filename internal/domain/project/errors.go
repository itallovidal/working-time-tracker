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
	ErrCustomerNotFound     = apperr.New("project.customer_not_found", http.StatusBadRequest)
	ErrInvalidOrganization  = apperr.New("project.invalid_organization", http.StatusBadRequest)
	ErrInvalidPage          = apperr.New("project.invalid_page", http.StatusBadRequest)
	ErrInvalidPerPage       = apperr.New("project.invalid_per_page", http.StatusBadRequest)

	// O acréscimo do projeto sobre a cobrança do cliente (Decisão 21). O field é bill_extra_cents.
	// ErrInvalidBillExtra recusa o acréscimo negativo ou acima do teto (o do valor por hora no cliente por hora, o de
	// um valor em dinheiro no mensal).
	ErrInvalidBillExtra = apperr.New("project.invalid_bill_extra", http.StatusBadRequest)
	// ErrBillExtraNeedsCustomer recusa o acréscimo num projeto sem cliente: o projeto interno não é cobrado.
	ErrBillExtraNeedsCustomer = apperr.New("project.bill_extra_needs_customer", http.StatusBadRequest)
	// ErrCustomerWithoutRate recusa o cliente por hora que ainda não tem valor por hora (um cliente antigo, de antes da
	// Decisão 21): o projeto não teria o que cobrar. O dono informa o valor em Clientes. O field é customer_id.
	ErrCustomerWithoutRate = apperr.New("project.customer_without_rate", http.StatusBadRequest)

	// ErrCustomerLocked recusa tirar do cliente o projeto que tem sessões quando o cliente já tem recebíveis: as horas
	// iam para as propostas do cliente novo e o recebível do antigo continuaria cobrando as mesmas. O field é customer_id.
	ErrCustomerLocked = apperr.New("project.customer_locked", http.StatusConflict)
)
