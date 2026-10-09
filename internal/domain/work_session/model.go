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

// SessionTask é um intervalo em que uma tarefa esteve na sessão. As tarefas em paralelo se
// sobrepõem, e cada uma conta o tempo cheio do intervalo dela; a sessão conta uma vez só.
type SessionTask struct {
	ID     uuid.UUID `json:"id"`
	TaskID uuid.UUID `json:"task_id"`
	Task   *Task     `json:"task,omitempty"`
	FromAt time.Time `json:"from_at"`
	// UntilAt nulo é "até o fim da sessão", ou até agora quando ela está aberta.
	UntilAt *time.Time `json:"until_at"`

	// Seconds é o tempo do intervalo dentro da sessão, e os dois Amount são esse tempo vezes
	// os valores por hora da sessão. Ficam nil quando a sessão não tem valor ou quando quem
	// pede não pode ver (o handler apaga antes de responder).
	Seconds         float64 `json:"seconds"`
	PayAmountCents  *int    `json:"pay_amount_cents"`
	BillAmountCents *int    `json:"bill_amount_cents"`
}

type WorkSession struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	// Tasks são as tarefas trabalhadas na sessão, na ordem em que entraram. A sessão começa
	// com uma e ganha outras depois; só fica sem nenhuma se as tarefas forem excluídas.
	Tasks     []SessionTask `json:"tasks"`
	PersonID  uuid.UUID     `json:"person_id"`
	Person    *Person       `json:"person,omitempty"`
	StartAt   time.Time     `json:"start_at"`
	EndAt     *time.Time    `json:"end_at,omitempty"`
	CreatedAt time.Time     `json:"created_at"`

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

// Within é o que caiu da sessão de since até now: o tempo e os dois valores, arredondados ao
// centavo como os da sessão inteira (uma sessão toda dentro da janela dá os mesmos valores
// dela, e só a que atravessa a borda é cortada). Um since zerado é a sessão inteira. A sessão
// aberta conta até now.
func (w *WorkSession) Within(since, now time.Time) (seconds float64, pay, bill *int) {
	start, end := w.StartAt, now
	if w.EndAt != nil {
		end = *w.EndAt
	}
	if start.Before(since) {
		start = since
	}
	seconds = math.Max(0, end.Sub(start).Seconds())
	return seconds, amount(seconds, w.PayRateCents), amount(seconds, w.BillRateCents)
}

// WithinRange é o que caiu da sessão no intervalo [from, to): o tempo e os dois valores, arredondados ao centavo sobre
// o pedaço que caiu dentro (uma sessão que atravessa a fronteira de dois períodos é arredondada uma vez em cada
// pedaço, e a soma dos pedaços pode diferir em um centavo da sessão inteira). A sessão aberta conta até now.
func (w *WorkSession) WithinRange(from, to, now time.Time) (seconds float64, pay, bill *int) {
	start, end := w.StartAt, now
	if w.EndAt != nil {
		end = *w.EndAt
	}
	if start.Before(from) {
		start = from
	}
	if end.After(to) {
		end = to
	}
	seconds = math.Max(0, end.Sub(start).Seconds())
	return seconds, amount(seconds, w.PayRateCents), amount(seconds, w.BillRateCents)
}

// taskSeconds é o tempo do intervalo dentro da sessão: o intervalo recortado entre o
// início e o fim dela (ou now, se está aberta).
func (w *WorkSession) taskSeconds(l *SessionTask, now time.Time) float64 {
	end := now
	if w.EndAt != nil {
		end = *w.EndAt
	}
	if l.UntilAt != nil && l.UntilAt.Before(end) {
		end = *l.UntilAt
	}
	start := l.FromAt
	if start.Before(w.StartAt) {
		start = w.StartAt
	}
	return math.Max(0, end.Sub(start).Seconds())
}

// TaskSeconds é o tempo que a tarefa teve na sessão, somando os intervalos dela.
func (w *WorkSession) TaskSeconds(taskID uuid.UUID, now time.Time) float64 {
	var total float64
	for i := range w.Tasks {
		if w.Tasks[i].TaskID == taskID {
			total += w.taskSeconds(&w.Tasks[i], now)
		}
	}
	return total
}

// HasTask diz se a tarefa esteve na sessão.
func (w *WorkSession) HasTask(taskID uuid.UUID) bool {
	for i := range w.Tasks {
		if w.Tasks[i].TaskID == taskID {
			return true
		}
	}
	return false
}

// fillAmounts calcula os valores da sessão e os de cada tarefa dela a partir dos valores
// por hora.
func (w *WorkSession) fillAmounts(now time.Time) {
	w.PayAmountCents = amount(w.Seconds(now), w.PayRateCents)
	w.BillAmountCents = amount(w.Seconds(now), w.BillRateCents)
	for i := range w.Tasks {
		l := &w.Tasks[i]
		l.Seconds = w.taskSeconds(l, now)
		l.PayAmountCents = amount(l.Seconds, w.PayRateCents)
		l.BillAmountCents = amount(l.Seconds, w.BillRateCents)
	}
}

// amount é o valor de um tempo trabalhado, arredondado para o centavo mais
// próximo. O arredondamento é por sessão (e, na visão por tarefa, por intervalo): os
// totais somam valores já arredondados, para a soma da tela bater com as linhas.
func amount(seconds float64, rateCents *int) *int {
	if rateCents == nil {
		return nil
	}
	v := int(math.Round(seconds * float64(*rateCents) / 3600))
	return &v
}
