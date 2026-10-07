package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// WorkSessionTask é um intervalo em que uma tarefa esteve na sessão. Uma sessão tem uma ou
// mais, e elas podem se sobrepor (tarefas em paralelo); a mesma tarefa só volta à sessão
// depois de sair, sem sobrepor o intervalo anterior.
type WorkSessionTask struct {
	ent.Schema
}

func (WorkSessionTask) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("session_id", uuid.UUID{}),
		field.UUID("task_id", uuid.UUID{}),
		field.Time("from_at"),
		// Nulo é "até o fim da sessão" (ou até agora, se ela está aberta).
		field.Time("until_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (WorkSessionTask) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", WorkSession.Type).Ref("task_links").Field("session_id").Unique().Required(),
		edge.From("task", Task.Type).Ref("session_links").Field("task_id").Unique().Required(),
	}
}

func (WorkSessionTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("task_id"),
	}
}
