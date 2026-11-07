package receivable

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O texto em cada idioma está em internal/i18n/locales, na chave errors.<código>. Os que dizem
// qual campo falhou levam o parâmetro field (a chave JSON do corpo) quando são devolvidos, para a tela mostrar o erro
// embaixo do campo certo; o texto não usa o rótulo, então o código não o declara (como payment.invalid_amount).
var (
	// ErrNotFound é o recebível que não existe na organização.
	ErrNotFound = apperr.New("receivable.not_found", http.StatusNotFound)
	// ErrNotAPeriod são datas que não formam um período do cliente: fora do ciclo do closing_day dele, ou, no mensal,
	// fora do contrato (antes do início, depois do fim ou além do período seguinte ao corrente).
	ErrNotAPeriod = apperr.New("receivable.not_a_period", http.StatusBadRequest)
	// ErrPeriodOpen é a confirmação de um período do cliente por hora que ainda não fechou: só se cobra o que já
	// terminou. O mensal não tem essa regra.
	ErrPeriodOpen = apperr.New("receivable.period_open", http.StatusConflict)
	// ErrAlreadyConfirmed é o período do cliente que já tem recebível.
	ErrAlreadyConfirmed = apperr.New("receivable.already_confirmed", http.StatusConflict)
	// ErrAlreadyReceived é o recebível que já foi recebido: não se edita, não se recebe de novo e a confirmação não
	// se desfaz antes de desfazer o recebimento.
	ErrAlreadyReceived = apperr.New("receivable.already_received", http.StatusConflict)
	// ErrChanged é o recebimento de um recebível cujo valor foi editado depois de a tela o ler (duas abas): nada é
	// gravado, para o dono não receber um valor que não viu. Ele confere e tenta de novo.
	ErrChanged = apperr.New("receivable.changed", http.StatusConflict)
	// ErrNotReceived é o desfazer do recebimento de um recebível que ainda não foi recebido.
	ErrNotReceived = apperr.New("receivable.not_received", http.StatusConflict)
	// ErrInvalidAmount é um valor que falta, é zero ou negativo, ou passa do teto. field diz qual: amount_cents ou
	// received_cents.
	ErrInvalidAmount = apperr.New("receivable.invalid_amount", http.StatusBadRequest)
	// ErrInvalidCharge é a multa ou os juros negativos, ou acima do teto. field diz qual.
	ErrInvalidCharge = apperr.New("receivable.invalid_charge", http.StatusBadRequest)
	// ErrChargesNotAllowed é a multa ou os juros num recebível por hora: só o contrato mensal os tem.
	ErrChargesNotAllowed = apperr.New("receivable.charges_not_allowed", http.StatusBadRequest)
	// ErrReceivedMustMatch é o valor recebido diferente do confirmado num recebível por hora: lá o que entra é o valor
	// da confirmação.
	ErrReceivedMustMatch = apperr.New("receivable.received_must_match", http.StatusBadRequest)
	// ErrChargesExceedReceived é a multa e os juros somados maiores que o valor recebido, de que são partes.
	ErrChargesExceedReceived = apperr.New("receivable.charges_exceed_received", http.StatusBadRequest)
	// ErrReceivedInFuture é o recebimento com data depois de hoje.
	ErrReceivedInFuture = apperr.New("receivable.received_in_future", http.StatusBadRequest)
)
