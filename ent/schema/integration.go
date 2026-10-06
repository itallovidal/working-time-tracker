package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Integration struct {
	ent.Schema
}

func (Integration) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("type"),
		field.String("display_name"),
		// credentials guarda o token criptografado; metadata, os campos próprios de
		// cada plataforma (repositório, quadro), em claro.
		field.JSON("credentials", map[string]interface{}{}).Optional().Sensitive(),
		field.JSON("metadata", map[string]interface{}{}).Optional(),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Integration) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("integrations").Field("project_id").Unique().Required(),
		edge.To("tasks", Task.Type),
	}
}
