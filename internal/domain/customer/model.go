// Package customer cuida dos clientes da organização: quem contrata os projetos.
package customer

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/validate"
)

// Os modos de cobrança de um cliente: por hora (as horas dos projetos vezes o valor cobrado de cada um) ou mensal (um
// valor fixo por mês, o contrato). Um ou outro, nunca os dois. O mensal só vale em organização solo.
const (
	BillingHourly  = "hourly"
	BillingMonthly = "monthly"
)

// DefaultClosingDay é o dia de fechamento do cliente sem termos: 31, o último dia de cada mês.
const DefaultClosingDay = 31

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
	// DefaultRateCents, só hourly: o valor por hora que preenche o campo ao criar um projeto do cliente (o projeto
	// guarda o seu). Quando existe, vale o piso de validate.MinRateCents.
	DefaultRateCents *int `json:"default_rate_cents"`
	// MonthlyFeeCents, ContractStart e ContractEnd, só monthly: a mensalidade em centavos (obrigatória, maior que
	// zero) e a vigência do contrato em dias de calendário YYYY-MM-DD, no fuso da organização. O início é obrigatório
	// e o fim, quando existe, vem depois dele; sem fim, o contrato segue por tempo indeterminado.
	MonthlyFeeCents *int    `json:"monthly_fee_cents"`
	ContractStart   *string `json:"contract_start"`
	ContractEnd     *string `json:"contract_end"`
	// ClosingDay é o dia do mês em que o período fecha (1 a 31; 31 é o último dia, e o mês curto fecha no último dia
	// dele), e PaymentDays, os dias entre o fechamento e o vencimento. Valem nos dois modos.
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
	BillingMode         *string                `json:"billing_mode"`
	DefaultRateCents    validate.Optional[int] `json:"default_rate_cents"`
	MonthlyFeeCents     validate.Optional[int] `json:"monthly_fee_cents"`
	ContractStart       *string                `json:"contract_start"`
	ContractEnd         *string                `json:"contract_end"`
	ClosingDay          *int                   `json:"closing_day"`
	PaymentDays         *int                   `json:"payment_days"`
	LateFeeBP           *int                   `json:"late_fee_bp"`
	LateInterestBPMonth *int                   `json:"late_interest_bp_month"`
	GraceDays           *int                   `json:"grace_days"`
}
