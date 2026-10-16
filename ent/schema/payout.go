package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Payout é a baixa de um período de pagamento de uma pessoa: o dono marcou que pagou. Os períodos são calculados da
// regra da pessoa (internal/domain/payment); a baixa guarda as datas do período que pagou e os números daquele momento,
// para que uma hora registrada depois, ou uma regra trocada, não reescreva o que foi pago.
type Payout struct {
	ent.Schema
}

func (Payout) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("person_id", uuid.UUID{}),
		// O período pago, em dias de calendário YYYY-MM-DD no fuso da organização, os dois inclusivos.
		field.String("period_start"),
		field.String("pay_date"),
		// As horas e o valor que o período tinha na hora da baixa, e o que o dono pagou de fato, que pode ser outro.
		field.Float("seconds").Min(0),
		field.Int("calculated_cents").NonNegative(),
		field.Int("amount_cents").NonNegative(),
		field.Time("paid_at").Default(time.Now),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Payout) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("payouts").Field("person_id").Unique().Required(),
	}
}

// Indexes garante uma baixa só por período de cada pessoa.
func (Payout) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id", "period_start", "pay_date").Unique(),
	}
}
