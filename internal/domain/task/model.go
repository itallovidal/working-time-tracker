package task

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
)

type Task struct {
	ID                    uuid.UUID                `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ProjectID             uuid.UUID                `gorm:"type:uuid;not null;index" json:"project_id"`
	Project               project.Project          `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"project,omitempty"`
	Name                  string                   `gorm:"not null" json:"name"`
	Description           string                   `json:"description"`
	AssigneeID            uuid.UUID                `gorm:"type:uuid;not null;index" json:"assignee_id"`
	Assignee              person.Person            `gorm:"foreignKey:AssigneeID;constraint:OnDelete:CASCADE" json:"assignee,omitempty"`
	Deadline              time.Time                `json:"deadline"`
	ExternalIntegrationID *uuid.UUID               `gorm:"type:uuid" json:"external_integration_id,omitempty"`
	ExternalIntegration   *integration.Integration `gorm:"foreignKey:ExternalIntegrationID;constraint:OnDelete:SET NULL" json:"external_integration,omitempty"`
	ExternalItemID        *string                  `json:"external_item_id,omitempty"`
	ExternalItemURL       *string                  `json:"external_item_url,omitempty"`
	CreatedAt             time.Time                `json:"created_at"`
}
