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

type Task struct {
	ID                    uuid.UUID    `json:"id"`
	ProjectID             uuid.UUID    `json:"project_id"`
	Name                  string       `json:"name"`
	Description           string       `json:"description"`
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
	Query      string     // trecho do nome, sem diferenciar maiúsculas
	AssigneeID *uuid.UUID // só as tarefas desta pessoa
	Unassigned bool       // só as tarefas sem responsável; vale no lugar de AssigneeID
	DeadlineTo *time.Time // só as com prazo até este instante, inclusive
	Page       int        // a partir de 1; zero é a lista inteira
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
