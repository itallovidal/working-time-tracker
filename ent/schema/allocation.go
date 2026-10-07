package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Allocation é o vínculo de uma pessoa com um projeto e o valor que a
// organização paga a ela por hora nesse projeto.
type Allocation struct {
	ent.Schema
}

func (Allocation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("person_id", uuid.UUID{}),
		field.Int("pay_rate_cents").NonNegative(),
		// O que a pessoa pode fazer neste projeto, além do que todo colaborador faz: as
		// permissões do catálogo (internal/domain/permission) e o nome do grupo de onde
		// vieram, ou "custom" quando alguém mexeu numa avulsa.
		field.Strings("permissions").Optional(),
		field.String("preset").Default("member"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Allocation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("allocations").Field("project_id").Unique().Required(),
		edge.From("person", Person.Type).Ref("allocations").Field("person_id").Unique().Required(),
	}
}

// Indexes garante um valor só por pessoa em cada projeto.
func (Allocation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "person_id").Unique(),
	}
}
