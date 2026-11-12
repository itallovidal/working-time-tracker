// Package customer cuida dos clientes da organização: quem contrata os projetos.
package customer

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/validate"
)

// Os modos de cobrança de um cliente: por hora (as horas dos projetos vezes o valor por hora do cliente, mais o
// acréscimo de cada projeto) ou mensal (um valor fixo por mês, o contrato, mais o acréscimo mensal de cada projeto). Um
// ou outro, nunca os dois. O mensal só vale em organização solo. A cobrança mora no cliente (Decisão 21): o projeto só
// guarda o acréscimo.
const (
	BillingHourly  = "hourly"
	BillingMonthly = "monthly"
)

// BillRate é o valor cobrado por hora de um projeto: o valor por hora do cliente mais o acréscimo do projeto. É nil
// quando o projeto não é cobrado por hora: o projeto interno (mode vazio), o do cliente mensal (o contrato é por mês) e
// o de um cliente antigo ainda sem valor. É a única conta do valor cobrado, para o ponto, o billing do projeto e as
// margens darem o mesmo número.
func BillRate(mode string, hourlyRateCents *int, extraCents int) *int {
	if mode != BillingHourly || hourlyRateCents == nil {
		return nil
	}
	v := *hourlyRateCents + extraCents
	return &v
}

// DefaultClosingDay é o dia de fechamento do cliente sem termos: 31, o último dia de cada mês.
const DefaultClosingDay = 31

// Os ciclos do período do cliente: mensal (fecha no ClosingDay de cada mês, o padrão) ou a cada CycleDays dias a partir
// da âncora, o bi-weekly dos EUA. O de 14 dias só vale no modo hourly; o contrato mensal é sempre mensal. Não é a
// quinzena de 15 dias da regra de pagamento da pessoa.
const (
	CycleMonthly  = "monthly"
	CycleBiweekly = "biweekly"
	CycleDays     = 14
)

type Customer struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	// Country é o país do cliente: o código ISO 3166-1 (BR, US, DE...). Começa no país da organização e decide a regra
	// do documento.
	Country string `json:"country"`
	// Document é o documento fiscal do cliente. No Brasil (CNPJ) e nos EUA (EIN) é conferido e fica sem máscara; nos
	// outros países é texto livre, sem conferência.
	Document     string    `json:"document"`
	ContactName  string    `json:"contact_name"`
	ContactEmail string    `json:"contact_email"`
	ContactPhone string    `json:"contact_phone"`
	ProjectCount int       `json:"project_count"`
	CreatedAt    time.Time `json:"created_at"`

	// BillingMode é hourly (padrão) ou monthly. Os campos do outro modo ficam vazios: o servidor os recusa.
	BillingMode string `json:"billing_mode"`
	// HourlyRateCents, só hourly: o valor por hora do cliente, o mesmo em todos os projetos dele (cada projeto pode somar
	// um acréscimo). Obrigatório, com o piso de validate.MinRateCents; nulo só num cliente antigo sem projetos.
	HourlyRateCents *int `json:"hourly_rate_cents"`
	// MonthlyFeeCents, ContractStart e ContractEnd, só monthly: a mensalidade em centavos (obrigatória, maior que
	// zero) e a vigência do contrato em dias de calendário YYYY-MM-DD, no fuso da organização. O início é obrigatório
	// e o fim, quando existe, vem depois dele; sem fim, o contrato segue por tempo indeterminado.
	MonthlyFeeCents *int    `json:"monthly_fee_cents"`
	ContractStart   *string `json:"contract_start"`
	ContractEnd     *string `json:"contract_end"`
	// EstimatedWeeklyHours, só monthly e opcional: as horas que o dono estima dedicar ao cliente por semana, em todos os
	// projetos dele (1 a validate.MaxWeeklyHours). A meta do período do contrato sai dela (Decisão 29). Só
	// acompanhamento: não muda nenhum valor.
	EstimatedWeeklyHours *int `json:"estimated_weekly_hours"`
	// BillingCycle é monthly (padrão) ou biweekly, este só no modo hourly. CycleAnchor, só no biweekly, é o último dia
	// do último período já pago, YYYY-MM-DD: os períodos de 14 dias começam no dia seguinte e nada até ele é cobrado.
	BillingCycle string  `json:"billing_cycle"`
	CycleAnchor  *string `json:"cycle_anchor"`
	// ClosingDay é o dia do mês em que o período mensal fecha (1 a 31; 31 é o último dia, e o mês curto fecha no último
	// dia dele); no ciclo de 14 dias fica guardado sem uso. PaymentDays são os dias entre o fechamento e o vencimento,
	// em qualquer ciclo e nos dois modos.
	ClosingDay  int `json:"closing_day"`
	PaymentDays int `json:"payment_days"`
	// LateFeeBP (multa, 200 = 2%), LateInterestBPMonth (juros ao mês, em pontos-base) e GraceDays (tolerância depois do
	// vencimento, antes de multa e juros), só monthly.
	LateFeeBP           int `json:"late_fee_bp"`
	LateInterestBPMonth int `json:"late_interest_bp_month"`
	GraceDays           int `json:"grace_days"`
}

// Input são os campos que a API aceita. Na criação, nil vale como vazio; na
// edição, nil mantém o valor atual e texto vazio apaga. O país é a exceção: nunca fica
// sem valor. Ausente (ou vazio, na criação), é o país da organização.
type Input struct {
	Name         *string `json:"name"`
	Country      *string `json:"country"`
	Document     *string `json:"document"`
	ContactName  *string `json:"contact_name"`
	ContactEmail *string `json:"contact_email"`
	ContactPhone *string `json:"contact_phone"`

	// Os termos comerciais. Ausente mantém o valor atual (na criação, o padrão). Nos campos opcionais de dinheiro,
	// null apaga; nas datas, texto vazio apaga. Um campo do outro modo, com valor, é recusado. Trocar o modo apaga
	// os campos do modo que saiu.
	BillingMode     *string                `json:"billing_mode"`
	HourlyRateCents validate.Optional[int] `json:"hourly_rate_cents"`
	MonthlyFeeCents validate.Optional[int] `json:"monthly_fee_cents"`
	ContractStart   *string                `json:"contract_start"`
	ContractEnd     *string                `json:"contract_end"`
	// EstimatedWeeklyHours: null apaga a estimativa, como nos campos opcionais de dinheiro.
	EstimatedWeeklyHours validate.Optional[int] `json:"estimated_weekly_hours"`
	BillingCycle         *string                `json:"billing_cycle"`
	CycleAnchor          *string                `json:"cycle_anchor"`
	ClosingDay           *int                   `json:"closing_day"`
	PaymentDays          *int                   `json:"payment_days"`
	LateFeeBP            *int                   `json:"late_fee_bp"`
	LateInterestBPMonth  *int                   `json:"late_interest_bp_month"`
	GraceDays            *int                   `json:"grace_days"`
}
