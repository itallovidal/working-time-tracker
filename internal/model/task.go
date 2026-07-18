package model

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ProjectID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Project              Project    `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Name                 string     `gorm:"not null"`
	Description          string
	AssigneeID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	Assignee             Person     `gorm:"foreignKey:AssigneeID;constraint:OnDelete:CASCADE"`
	Deadline             time.Time
	ExternalIntegrationID *uuid.UUID `gorm:"type:uuid"`
	ExternalIntegration   *Integration `gorm:"foreignKey:ExternalIntegrationID;constraint:OnDelete:SET NULL"`
	ExternalItemID       *string
	ExternalItemURL      *string
	CreatedAt            time.Time
}
