package work_session

import (
	"math"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ProjectID uuid.UUID `json:"project_id"`
}

type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type WorkSession struct {
	ID        uuid.UUID  `json:"id"`
	TaskID    uuid.UUID  `json:"task_id"`
	Task      *Task      `json:"task,omitempty"`
	PersonID  uuid.UUID  `json:"person_id"`
	Person    *Person    `json:"person,omitempty"`
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`

	// PayRateCents e BillRateCents são os valores por hora travados no clock-in.
	// Os dois Amount são o tempo da sessão vezes cada valor; numa sessão aberta,
	// contam até agora. Tudo fica nil quando a sessão não tem valor ou quando
	// quem pede não pode ver (o handler apaga antes de responder).
	// OwnerHours marca a sessão do dono da organização: o valor pago é zero, e a tela
	// mostra o valor cobrado como o que ele ganhou.
	OwnerHours      bool `json:"owner_hours"`
	PayRateCents    *int `json:"pay_rate_cents"`
	BillRateCents   *int `json:"bill_rate_cents"`
	PayAmountCents  *int `json:"pay_amount_cents"`
	BillAmountCents *int `json:"bill_amount_cents"`
}

// Seconds é a duração da sessão. Uma sessão aberta conta até now.
func (w *WorkSession) Seconds(now time.Time) float64 {
	end := now
	if w.EndAt != nil {
		end = *w.EndAt
	}
	return math.Max(0, end.Sub(w.StartAt).Seconds())
}

// fillAmounts calcula os valores da sessão a partir dos valores por hora.
func (w *WorkSession) fillAmounts(now time.Time) {
	w.PayAmountCents = amount(w.Seconds(now), w.PayRateCents)
	w.BillAmountCents = amount(w.Seconds(now), w.BillRateCents)
}

// amount é o valor de um tempo trabalhado, arredondado para o centavo mais
// próximo. O arredondamento é por sessão: os totais somam sessões já
// arredondadas, para a soma da tela bater com as linhas.
func amount(seconds float64, rateCents *int) *int {
	if rateCents == nil {
		return nil
	}
	v := int(math.Round(seconds * float64(*rateCents) / 3600))
	return &v
}
