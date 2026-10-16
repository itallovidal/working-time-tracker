package payment

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O texto em cada idioma está em internal/i18n/locales, na chave errors.<código>.
var (
	ErrOwnOnly = apperr.New("payment.own_only", http.StatusForbidden)
	// ErrInvalidHistory é o ?history= que não é um número inteiro maior que zero.
	ErrInvalidHistory = apperr.New("payment.invalid_history", http.StatusBadRequest)
	// ErrOwnerNotPaid é a baixa pedida para o dono, que não é pago.
	ErrOwnerNotPaid = apperr.New("payment.owner_not_paid", http.StatusBadRequest)
	// ErrNoRule é a baixa de quem não tem regra de pagamento: sem regra não há período.
	ErrNoRule = apperr.New("payment.no_rule", http.StatusBadRequest)
	// ErrNotAPeriod são datas que não formam um período da regra da pessoa.
	ErrNotAPeriod = apperr.New("payment.not_a_period", http.StatusBadRequest)
	// ErrPeriodNotStarted é a baixa de um período que ainda não começou.
	ErrPeriodNotStarted = apperr.New("payment.period_not_started", http.StatusBadRequest)
	// ErrAlreadyPaid é o período que já tem baixa.
	ErrAlreadyPaid = apperr.New("payment.already_paid", http.StatusConflict)
	// ErrInvalidAmount é o valor pago que falta ou é negativo.
	ErrInvalidAmount = apperr.New("payment.invalid_amount", http.StatusBadRequest)
	// ErrInvalidBonus é o bônus negativo.
	ErrInvalidBonus = apperr.New("payment.invalid_bonus", http.StatusBadRequest)
	// ErrPayoutNotFound é a baixa que não existe na organização.
	ErrPayoutNotFound = apperr.New("payment.payout_not_found", http.StatusNotFound)
)
