package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Customer é o cliente que contrata a organização. Um cliente pode ter vários
// projetos. O nome não é Client porque o Ent reserva esse identificador.
type Customer struct {
	ent.Schema
}

func (Customer) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("organization_id", uuid.UUID{}),
		field.String("name"),
		// O país do cliente (código ISO 3166-1: BR, US, DE...), que começa no da organização. Decide a regra do documento: o
		// CNPJ no Brasil e o EIN nos EUA são conferidos; nos outros países o documento é texto livre. Quem decide é
		// internal/country.
		field.String("country").Default("BR"),
		// O documento fiscal do cliente, sem máscara quando o país tem regra (CNPJ, EIN).
		field.String("document").Optional(),
		field.String("contact_name").Optional(),
		field.String("contact_email").Optional(),
		field.String("contact_phone").Optional(),

		// Os termos comerciais. O modo de cobrança é "hourly" (as horas dos projetos vezes o valor cobrado) ou "monthly"
		// (um valor fixo por mês, o contrato): um ou outro, nunca os dois. Validado em Go, como payment_frequency; os
		// campos do outro modo são recusados lá, e o modo "monthly" só vale em organização solo.
		field.String("billing_mode").Default("hourly"),
		// Só hourly: o valor por hora que o cliente paga, o mesmo em todos os projetos dele (cada projeto pode somar um
		// acréscimo, projects.bill_extra_cents). Obrigatório no hourly, conferido em Go; nulo só num cliente antigo sem
		// projetos, de antes da migração billing_on_customer.
		field.Int("hourly_rate_cents").Optional().Nillable().NonNegative(),
		// Só monthly: o valor do contrato por mês e a vigência dele, em dias de calendário YYYY-MM-DD no fuso da
		// organização. Sem fim, o contrato segue por tempo indeterminado.
		field.Int("monthly_fee_cents").Optional().Nillable().NonNegative(),
		field.String("contract_start").Optional().Nillable(),
		field.String("contract_end").Optional().Nillable(),
		// Só monthly e opcional: as horas que o dono estima dedicar ao cliente em cada período do contrato, somando todos
		// os projetos dele. É só para acompanhar (as horas do período contra a estimativa): não muda a mensalidade nem
		// cobra excedente, e por isso não é a franquia de horas que a Decisão 20 deixou para depois (Decisão 26).
		field.Int("estimated_monthly_hours").Optional().Nillable().Positive(),
		// O dia do mês em que o período fecha (31 é o último dia; mês curto fecha no último dia dele) e os dias entre o
		// fechamento e o vencimento do recebível.
		field.Int("closing_day").Default(31).Min(1).Max(31),
		field.Int("payment_days").NonNegative().Default(0),
		// O ciclo do período: "monthly" (fecha no closing_day de cada mês) ou "biweekly" (a cada 14 dias, o bi-weekly dos
		// EUA). Validado em Go, como billing_mode; o de 14 dias só vale no modo hourly. cycle_anchor é o dia, YYYY-MM-DD no
		// fuso da organização, em que fechou o último período já pago fora do app: os períodos de 14 dias começam no dia
		// seguinte, e nada até ele é cobrado. Só existe no ciclo de 14 dias. No de 14 dias o closing_day fica guardado sem
		// uso e volta a valer quando o ciclo volta a ser mensal.
		field.String("billing_cycle").Default("monthly"),
		field.String("cycle_anchor").Optional().Nillable(),
		// Só monthly: a multa por atraso e os juros ao mês, em pontos-base (200 = 2%), e os dias de tolerância depois do
		// vencimento antes de cobrar os dois.
		field.Int("late_fee_bp").NonNegative().Default(0),
		field.Int("late_interest_bp_month").NonNegative().Default(0),
		field.Int("grace_days").NonNegative().Default(0),

		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Customer) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("customers").Field("organization_id").Unique().Required(),
		edge.To("projects", Project.Type),
		// Um cliente com recebíveis não é apagado (o serviço recusa): a chave estrangeira não leva os recebíveis junto.
		// Apagar a organização continua funcionando, porque os recebíveis também saem pela organização.
		edge.To("receivables", Receivable.Type).Annotations(entsql.OnDelete(entsql.NoAction)),
	}
}
