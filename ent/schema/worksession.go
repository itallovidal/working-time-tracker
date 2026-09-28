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
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("person_id", uuid.UUID{}),
		field.Time("start_at"),
		field.Time("end_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (WorkSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", Task.Type).Ref("work_sessions").Field("task_id").Unique().Required(),
		edge.From("person", Person.Type).Ref("work_sessions").Field("person_id").Unique().Required(),
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
