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
	AssigneeID            uuid.UUID    `json:"assignee_id"`
	Assignee              *Person      `json:"assignee,omitempty"`
	Deadline              time.Time    `json:"deadline"`
	ExternalIntegrationID *uuid.UUID   `json:"external_integration_id,omitempty"`
	ExternalIntegration   *Integration `json:"external_integration,omitempty"`
	ExternalItemID        *string      `json:"external_item_id,omitempty"`
	ExternalItemURL       *string      `json:"external_item_url,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
}
