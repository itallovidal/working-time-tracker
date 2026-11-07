package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Receivable é uma conta a receber: o dono confirmou o que vai cobrar de um cliente por um período. O período e a
// proposta são calculados (internal/domain/receivable) do modo de cobrança do cliente; o recebível guarda as datas do
// período e os números daquele momento, para que uma hora registrada depois, ou um contrato alterado, não reescreva o
// que foi cobrado. O sistema não emite nota: o número dela é só uma anotação.
type Receivable struct {
	ent.Schema
}

func (Receivable) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("organization_id", uuid.UUID{}),
		field.UUID("customer_id", uuid.UUID{}),
		// O modo de cobrança do cliente na confirmação: "hourly" ou "monthly". Validado em Go, como payment_frequency.
		field.String("kind"),
		// O período cobrado, em dias de calendário YYYY-MM-DD no fuso da organização, os dois inclusivos.
		field.String("period_start"),
		field.String("period_end"),
		// O tempo trabalhado no período, em todos os projetos do cliente; o valor que o sistema propôs na hora da
		// confirmação; e o valor que o dono confirmou de fato, que pode ser outro.
		field.Float("seconds").Min(0),
		field.Int("calculated_cents").NonNegative(),
		field.Int("amount_cents").Positive(),
		// A moeda da organização na confirmação; não há conversão.
		field.String("currency"),
		// O vencimento (YYYY-MM-DD): por padrão o fim do período mais os payment_days do cliente.
		field.String("due_date"),
		// O número da nota emitida fora do sistema, e uma observação; vazio quando não há.
		field.String("invoice_number").Optional(),
		field.String("note").Optional(),
		field.Time("confirmed_at").Default(time.Now),
		// O recebimento: nulo enquanto não entrou. received_cents é o que entrou, e a multa e os juros são partes
		// informativas dele (só no modo monthly); zero quando não houve.
		field.String("received_on").Optional().Nillable(),
		field.Int("received_cents").Optional().Nillable().Positive(),
		field.Int("late_fee_cents").NonNegative().Default(0),
		field.Int("interest_cents").NonNegative().Default(0),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Receivable) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("receivables").Field("organization_id").Unique().Required(),
		edge.From("customer", Customer.Type).Ref("receivables").Field("customer_id").Unique().Required(),
	}
}

// Indexes garante um recebível só por cliente e período: duas confirmações ao mesmo tempo calculam o mesmo período, e
// a segunda esbarra no índice em vez de cobrar o mesmo período duas vezes.
func (Receivable) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("customer_id", "period_start", "period_end").Unique(),
	}
}
