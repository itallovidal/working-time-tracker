package receivable

import (
	"time"

	"github.com/google/uuid"
)

// Os modos de cobrança do cliente, e por isso o kind do recebível. O texto é o de customers.billing_mode.
const (
	KindHourly  = "hourly"
	KindMonthly = "monthly"
)

const (
	// MaxHourlyProposals é quantos períodos fechados do cliente por hora de ciclo mensal se calculam, os mais
	// recentes: dois anos.
	MaxHourlyProposals = 24
	// MaxBiweeklyProposals é o mesmo teto no ciclo de 14 dias, também cerca de dois anos. Um ano da tela inicial tem
	// no máximo 26 períodos fechados desse ciclo, abaixo dele.
	MaxBiweeklyProposals = 52
	// MaxMonthlyProposals limita os períodos do contrato mensal que ainda não foram confirmados: um contrato que
	// começa muito atrás, ou uma data digitada errada, não enche a tela. Fica com os mais recentes.
	MaxMonthlyProposals = 60
	// MaxReceived é quantos recebidos a lista Recebidos traz, os mais recentes.
	MaxReceived = 50
	// interestMonthDays é o mês dos juros: o ao-mês é proporcional aos dias de atraso sobre 30.
	interestMonthDays = 30
)

// Ref é o cliente citado num recebível ou numa proposta.
type Ref struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Proposal é o que o sistema calcula cobrar de um cliente por um período: calculado na hora, nada gravado até o dono
// receber ou ajustar. Os dias são YYYY-MM-DD no fuso da organização, os dois inclusivos.
type Proposal struct {
	Customer    Ref    `json:"customer"`
	Kind        string `json:"kind"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	// Seconds é o tempo trabalhado no período em todos os projetos do cliente (até agora, no período corrente).
	Seconds float64 `json:"seconds"`
	// CalculatedCents é o valor proposto: no por hora, a soma das sessões cortadas no período, cada uma arredondada ao
	// centavo pelo valor cobrado congelado nela; no mensal, a mensalidade mais os acréscimos mensais dos projetos.
	CalculatedCents int `json:"calculated_cents"`
	// ExtraCents é a parte de CalculatedCents que vem dos acréscimos mensais dos projetos do cliente (só no mensal;
	// zero no por hora, em que o acréscimo já está no valor de cada sessão).
	ExtraCents int `json:"extra_cents"`
	// DueDate é o vencimento que a confirmação usa se o dono não mudar: o fim do período mais os payment_days.
	DueDate string `json:"due_date"`
	// Open diz que o período ainda não fechou: é a lista Em andamento da página.
	Open bool `json:"open"`
	// EffectiveRateCents é o valor por hora que a proposta rende: o valor dividido pelas horas. Nulo sem horas.
	// BelowMinRate avisa que ficou abaixo do piso de valor por hora (validate.MinRateCents); não é um erro.
	EffectiveRateCents *int `json:"effective_rate_cents"`
	BelowMinRate       bool `json:"below_min_rate"`
}

// Receivable é uma conta a receber: o dono confirmou o que vai cobrar de um cliente por um período.
type Receivable struct {
	ID       uuid.UUID `json:"id"`
	Customer Ref       `json:"customer"`
	// Kind é o modo de cobrança do cliente na confirmação: hourly ou monthly.
	Kind        string `json:"kind"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	// Seconds e CalculatedCents são o que o sistema propôs na confirmação; AmountCents, o que o dono confirmou.
	Seconds         float64 `json:"seconds"`
	CalculatedCents int     `json:"calculated_cents"`
	AmountCents     int     `json:"amount_cents"`
	Currency        string  `json:"currency"`
	DueDate         string  `json:"due_date"`
	// InvoiceNumber e Note são anotações: o sistema não emite nota.
	InvoiceNumber string    `json:"invoice_number"`
	Note          string    `json:"note"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
	// ReceivedOn é o dia do recebimento, nulo enquanto não entrou. ReceivedCents é o que entrou, e a multa e os juros
	// são partes dele (só no mensal).
	ReceivedOn    *string `json:"received_on"`
	ReceivedCents *int    `json:"received_cents"`
	LateFeeCents  int     `json:"late_fee_cents"`
	InterestCents int     `json:"interest_cents"`
	// Late é derivado: confirmado, sem recebimento e com hoje depois do vencimento. DaysLate são os dias de atraso
	// (zero quando não está atrasado).
	Late     bool `json:"late"`
	DaysLate int  `json:"days_late"`
	// EffectiveRateCents é o valor confirmado dividido pelas horas do período; nulo sem horas. BelowMinRate o compara
	// com o piso de valor por hora.
	EffectiveRateCents *int `json:"effective_rate_cents"`
	BelowMinRate       bool `json:"below_min_rate"`
}

// Received diz que o recebimento foi registrado.
func (r Receivable) Received() bool { return r.ReceivedOn != nil }

// Due é uma conta das listas Em andamento (o período ainda não fechou) e A receber (fechou e não entrou). Sem ID é o
// calculado, que nada grava até o dono receber ou ajustar; com ID (Adjusted) é o recebível gravado, que o dono ajustou,
// ajustou adiantado, ou recebeu e desfez o recebimento. O atraso vale para os dois: hoje depois do vencimento.
type Due struct {
	ID       *uuid.UUID `json:"id"`
	Adjusted bool       `json:"adjusted"`
	Customer Ref        `json:"customer"`
	Kind     string     `json:"kind"`
	// Os dias do período, YYYY-MM-DD, os dois inclusivos.
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	// Seconds é o tempo no período; CalculatedCents, o que o sistema calcula (no gravado, o que calculava ao gravar);
	// AmountCents, o que se vai receber (no calculado, o próprio CalculatedCents). ExtraCents é a parte dos acréscimos
	// mensais dos projetos, só no mensal calculado.
	Seconds         float64 `json:"seconds"`
	CalculatedCents int     `json:"calculated_cents"`
	AmountCents     int     `json:"amount_cents"`
	ExtraCents      int     `json:"extra_cents"`
	DueDate         string  `json:"due_date"`
	InvoiceNumber   string  `json:"invoice_number"`
	Note            string  `json:"note"`
	Late            bool    `json:"late"`
	DaysLate        int     `json:"days_late"`
	// EffectiveRateCents é AmountCents dividido pelas horas; nulo sem horas.
	EffectiveRateCents *int `json:"effective_rate_cents"`
	BelowMinRate       bool `json:"below_min_rate"`
}

// Totals são os cartões da página, em centavos da moeda da organização.
type Totals struct {
	// InProgress é o que os períodos ainda abertos somam: a mensalidade do mensal (ou o ajustado) e o valor até agora do
	// por hora.
	InProgressCount int `json:"in_progress_count"`
	InProgressCents int `json:"in_progress_cents"`
	// ToReceive é tudo o que fechou e ainda não entrou, atrasado incluído; Late é a parte que passou do vencimento.
	ToReceiveCount int `json:"to_receive_count"`
	ToReceiveCents int `json:"to_receive_cents"`
	LateCount      int `json:"late_count"`
	LateCents      int `json:"late_cents"`
	// ReceivedMonth é o que entrou no mês corrente (calendário, no fuso da organização): a soma de received_cents.
	ReceivedMonthCount int `json:"received_month_count"`
	ReceivedMonthCents int `json:"received_month_cents"`
}

// Summary é a resposta de GET /receivables: os cartões e as três listas da página.
type Summary struct {
	GeneratedAt time.Time `json:"generated_at"`
	Timezone    string    `json:"timezone"`
	Currency    string    `json:"currency"`
	// Today é o dia de hoje (YYYY-MM-DD) no fuso da organização, de onde saem o atraso e o mês dos cartões.
	Today  string `json:"today"`
	Totals Totals `json:"totals"`
	// InProgress são os períodos ainda abertos, calculados ou ajustados adiantado, o que fecha primeiro antes; ToReceive,
	// o que fechou e não entrou, do vencimento mais antigo (o mais atrasado) para o mais novo; Received, o que entrou, o
	// mais recente primeiro.
	InProgress []Due        `json:"in_progress"`
	ToReceive  []Due        `json:"to_receive"`
	Received   []Receivable `json:"received"`
}

// LateCharges é a sugestão de multa e juros para receber um recebível mensal numa data.
type LateCharges struct {
	// On é a data do recebimento da conta; DueDate e GraceDays, o vencimento e a tolerância do cliente.
	On          string `json:"on"`
	DueDate     string `json:"due_date"`
	GraceDays   int    `json:"grace_days"`
	AmountCents int    `json:"amount_cents"`
	// DaysLate são os dias entre o vencimento e On (zero se não passou), e Charged diz que a data passou da
	// tolerância: só então há multa e juros.
	DaysLate      int  `json:"days_late"`
	Charged       bool `json:"charged"`
	LateFeeCents  int  `json:"late_fee_cents"`
	InterestCents int  `json:"interest_cents"`
	// TotalCents é o valor confirmado mais a multa e os juros: o que se sugere receber.
	TotalCents int `json:"total_cents"`
}

// ConfirmInput é o pedido de confirmação: o cliente e o período, e o que o dono ajusta. O valor vem da tela,
// preenchido com o proposto. O vencimento vazio vale o padrão do cliente.
type ConfirmInput struct {
	CustomerID    string `json:"customer_id"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	AmountCents   *int   `json:"amount_cents"`
	DueDate       string `json:"due_date"`
	InvoiceNumber string `json:"invoice_number"`
	Note          string `json:"note"`
}

// UpdateInput são os campos que o PATCH pode alterar. Campo ausente (nil) mantém o valor; texto vazio apaga a nota e o
// número da nota.
type UpdateInput struct {
	AmountCents   *int    `json:"amount_cents"`
	DueDate       *string `json:"due_date"`
	InvoiceNumber *string `json:"invoice_number"`
	Note          *string `json:"note"`
}

// ReceiveInput é o pedido de recebimento. Sem received_on vale hoje. No por hora o valor recebido é o confirmado e
// multa e juros são recusados; no mensal o valor é livre, e sem ele vale o confirmado mais a multa e os juros.
// InvoiceNumber ausente (nil) mantém o número da nota; texto vazio o apaga.
type ReceiveInput struct {
	ReceivedOn    string  `json:"received_on"`
	ReceivedCents *int    `json:"received_cents"`
	LateFeeCents  *int    `json:"late_fee_cents"`
	InterestCents *int    `json:"interest_cents"`
	InvoiceNumber *string `json:"invoice_number"`
}

// ReceivePeriodInput é o recebimento de um período calculado, num passo só: o cliente e o período, o recebimento e a
// observação. O valor é o calculado e o vencimento é o padrão do cliente; para outros, o dono ajusta antes
// (POST /receivables).
type ReceivePeriodInput struct {
	CustomerID  string `json:"customer_id"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	ReceiveInput
	Note string `json:"note"`
}
