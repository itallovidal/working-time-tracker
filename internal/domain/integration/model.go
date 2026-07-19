package integration

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/project"
)

type Integration struct {
	ID          uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ProjectID   uuid.UUID              `gorm:"type:uuid;not null;index" json:"project_id"`
	Project     project.Project        `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"project,omitempty"`
	Type        string                 `gorm:"not null" json:"type"`
	DisplayName string                 `gorm:"not null" json:"display_name"`
	Config      map[string]interface{} `gorm:"serializer:json" json:"config,omitempty"`
	Enabled     bool                   `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time              `json:"created_at"`
}
