package model

import (
	"time"

	"github.com/google/uuid"
)

type Team struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string    `gorm:"not null"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index"`
	Project   Project   `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	CreatedAt time.Time
}
