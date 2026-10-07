package task

import (
	"time"

	"github.com/google/uuid"
)

type Person struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type Integration struct {
	ID   uuid.UUID `json:"id"`
	Type string    `json:"type"`
}

// Priorities são as prioridades de uma tarefa, da mais para a menos urgente.
// "none" é a tarefa sem prioridade.
var Priorities = []string{"urgent", "high", "medium", "low", "none"}

func validPriority(p string) bool {
	for _, v := range Priorities {
		if v == p {
			return true
		}
	}
	return false
}

// Label é uma etiqueta de tarefa; cada projeto tem as suas.
type Label struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Attrs são a prioridade e as etiquetas que uma tarefa recebe ao ser criada ou
// editada. Nil mantém o que a tarefa já tem (ou, na criação, o padrão: sem
// prioridade e sem etiquetas); uma lista vazia de etiquetas as tira.
type Attrs struct {
	Priority *string
	LabelIDs *[]string
}

type Task struct {
	ID                    uuid.UUID    `json:"id"`
	ProjectID             uuid.UUID    `json:"project_id"`
	Name                  string       `json:"name"`
	Description           string       `json:"description"`
	Priority              string       `json:"priority"`
	Labels                []Label      `json:"labels"`
	AssigneeID            *uuid.UUID   `json:"assignee_id"`
	Assignee              *Person      `json:"assignee,omitempty"`
	Deadline              time.Time    `json:"deadline"`
	ExternalIntegrationID *uuid.UUID   `json:"external_integration_id,omitempty"`
	ExternalIntegration   *Integration `json:"external_integration,omitempty"`
	ExternalItemID        *string      `json:"external_item_id,omitempty"`
	ExternalItemURL       *string      `json:"external_item_url,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
}

// ListFilter restringe a lista de tarefas de um projeto. O valor zero lista tudo.
type ListFilter struct {
	Query      string      // trecho do nome, sem diferenciar maiúsculas
	AssigneeID *uuid.UUID  // só as tarefas desta pessoa
	Unassigned bool        // só as tarefas sem responsável; vale no lugar de AssigneeID
	DeadlineTo *time.Time  // só as com prazo até este instante, inclusive
	Priorities []string    // só as com uma destas prioridades
	LabelIDs   []uuid.UUID // só as que têm alguma destas etiquetas
	Page       int         // a partir de 1; zero é a lista inteira
	PerPage    int
}

// Page é uma página da lista de tarefas. Total conta tudo o que passa pelos
// filtros, e Assignees traz quem é responsável por alguma tarefa do projeto,
// para o filtro alcançar também quem já saiu dos times.
type Page struct {
	Items     []Task   `json:"items"`
	Total     int      `json:"total"`
	Page      int      `json:"page"`
	PerPage   int      `json:"per_page"`
	Assignees []Person `json:"assignees"`
}
