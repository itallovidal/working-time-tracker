package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type WorkSession struct {
	ent.Schema
}

func (WorkSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		// O projeto é da sessão: todas as tarefas dela são dele, e o valor por hora abaixo
		// é o da pessoa neste projeto.
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("person_id", uuid.UUID{}),
		field.Time("start_at"),
		field.Time("end_at").Optional().Nillable(),
		// Os valores por hora que valiam quando o ponto abriu: o que a pessoa
		// recebe e o que o cliente paga. Ficam na sessão para que uma mudança de
		// valor não reescreva as horas já trabalhadas. Sessões anteriores aos
		// valores, e projetos sem valor cobrado, ficam com nulo.
		field.Int("pay_rate_cents").Optional().Nillable().NonNegative(),
		field.Int("bill_rate_cents").Optional().Nillable().NonNegative(),
		// Sessão do dono da organização: o valor pago é zero, porque o que ele tira do
		// projeto é a margem, e a tela mostra o valor cobrado como o que ele ganhou.
		field.Bool("owner_hours").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (WorkSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("work_sessions").Field("project_id").Unique().Required(),
		edge.From("person", Person.Type).Ref("work_sessions").Field("person_id").Unique().Required(),
		// As tarefas da sessão, com o intervalo de cada uma. Excluir a sessão leva os intervalos.
		edge.To("task_links", WorkSessionTask.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

// Indexes garante no banco que cada pessoa tenha no máximo uma sessão aberta.
func (WorkSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id").
			Unique().
			StorageKey("one_active_session").
			Annotations(entsql.IndexWhere("end_at IS NULL")),
	}
}
