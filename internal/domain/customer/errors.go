package customer

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound        = apperr.New("customer.not_found", http.StatusNotFound)
	ErrNameRequired    = apperr.New("customer.name_required", http.StatusBadRequest)
	ErrNameTooLong     = apperr.New("customer.name_too_long", http.StatusBadRequest)
	ErrContactTooLong  = apperr.New("customer.contact_too_long", http.StatusBadRequest)
	ErrInvalidDocument = apperr.New("customer.invalid_document", http.StatusBadRequest)
	ErrInvalidCountry  = apperr.New("customer.invalid_country", http.StatusBadRequest)
	// ErrInvalidEmail não é mais devolvido: o e-mail do contato responde request.field_invalid e
	// request.field_too_long, com field contact_email. O código segue declarado para o contrato não mudar.
	ErrInvalidEmail        = apperr.New("customer.invalid_email", http.StatusBadRequest)
	ErrInvalidPhone        = apperr.New("customer.invalid_phone", http.StatusBadRequest)
	ErrHasProjects         = apperr.New("customer.has_projects", http.StatusConflict)
	ErrInvalidOrganization = apperr.New("customer.invalid_organization", http.StatusBadRequest)

	// Os termos comerciais (modo solo). Todos dizem o campo (a chave do corpo), menos os conflitos de estado.
	// ErrInvalidBillingMode recusa um modo que não é hourly nem monthly. O field é billing_mode.
	ErrInvalidBillingMode = apperr.New("customer.invalid_billing_mode", http.StatusBadRequest)
	// ErrMonthlyNeedsSolo recusa o modo monthly numa organização que não é solo. O field é billing_mode.
	ErrMonthlyNeedsSolo = apperr.New("customer.monthly_needs_solo", http.StatusBadRequest)
	// ErrModeLocked recusa trocar o modo de um cliente que já tem projetos (issue #25). O field é billing_mode.
	ErrModeLocked = apperr.New("customer.mode_locked", http.StatusConflict)
	// ErrHasReceivables recusa excluir o cliente que tem recebíveis: quem os desfaz é o dono, na página A receber.
	ErrHasReceivables = apperr.New("customer.has_receivables", http.StatusConflict)
	// ErrClosingDayLocked recusa trocar o dia de fechamento de um cliente que já tem recebíveis: os períodos novos se
	// sobreporiam aos já confirmados e as mesmas horas poderiam ser cobradas duas vezes. O field é closing_day.
	ErrClosingDayLocked = apperr.New("customer.closing_day_locked", http.StatusConflict)
	// ErrCycleLocked recusa trocar o ciclo ou a âncora de um cliente que já tem recebíveis, pelo mesmo motivo. Passar
	// para o ciclo de 14 dias com a âncora no fim do último recebível, ou depois, é aceito: os períodos novos começam
	// depois dos confirmados. O field é billing_cycle ou cycle_anchor.
	ErrCycleLocked = apperr.New("customer.cycle_locked", http.StatusConflict)
	// ErrInvalidBillingCycle recusa um ciclo que não é monthly nem biweekly. O field é billing_cycle.
	ErrInvalidBillingCycle = apperr.New("customer.invalid_billing_cycle", http.StatusBadRequest)
	// ErrInvalidCycleAnchor recusa uma âncora depois de hoje: ela é o fim de um período que já fechou. O field é
	// cycle_anchor.
	ErrInvalidCycleAnchor = apperr.New("customer.invalid_cycle_anchor", http.StatusBadRequest)
	// ErrFieldBiweeklyOnly recusa a âncora num cliente de ciclo mensal; o field diz qual.
	ErrFieldBiweeklyOnly = apperr.New("customer.field_biweekly_only", http.StatusBadRequest, "field")
	// ErrFieldHourlyOnly e ErrFieldMonthlyOnly recusam o campo do outro modo; o field diz qual.
	ErrFieldHourlyOnly  = apperr.New("customer.field_hourly_only", http.StatusBadRequest, "field")
	ErrFieldMonthlyOnly = apperr.New("customer.field_monthly_only", http.StatusBadRequest, "field")
	// Os valores fora da faixa, cada um com o campo e a faixa no texto.
	ErrInvalidHourlyRate  = apperr.New("customer.invalid_hourly_rate", http.StatusBadRequest)
	ErrInvalidMonthlyFee  = apperr.New("customer.invalid_monthly_fee", http.StatusBadRequest)
	ErrInvalidContractEnd = apperr.New("customer.invalid_contract_end", http.StatusBadRequest)
	// ErrInvalidEstimatedHours recusa uma estimativa de horas por semana fora de 1 a validate.MaxWeeklyHours.
	ErrInvalidEstimatedHours = apperr.New("customer.invalid_estimated_hours", http.StatusBadRequest)
	ErrInvalidClosingDay     = apperr.New("customer.invalid_closing_day", http.StatusBadRequest)
	ErrInvalidPaymentDays    = apperr.New("customer.invalid_payment_days", http.StatusBadRequest)
	ErrInvalidLateFee        = apperr.New("customer.invalid_late_fee", http.StatusBadRequest)
	ErrInvalidLateInterest   = apperr.New("customer.invalid_late_interest", http.StatusBadRequest)
	ErrInvalidGraceDays      = apperr.New("customer.invalid_grace_days", http.StatusBadRequest)
)
