package team

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/project"
)

type Team struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string          `gorm:"not null" json:"name"`
	ProjectID uuid.UUID       `gorm:"type:uuid;not null;index" json:"project_id"`
	Project   project.Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"project,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}
