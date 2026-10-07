package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Label é uma etiqueta de tarefa. Cada projeto tem as suas, e uma tarefa pode
// ter várias.
type Label struct {
	ent.Schema
}

func (Label) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("name"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Label) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("labels").Field("project_id").Unique().Required(),
		edge.From("tasks", Task.Type).Ref("labels"),
	}
}

// Indexes garante um nome só por projeto. O service ainda confere sem diferenciar
// maiúsculas, o que o índice não faz.
func (Label) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "name").Unique(),
	}
}
