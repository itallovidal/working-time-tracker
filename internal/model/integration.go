package model

import (
	"time"

	"github.com/google/uuid"
)

type Integration struct {
	ID          uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ProjectID   uuid.UUID              `gorm:"type:uuid;not null;index"`
	Project     Project                `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Type        string                 `gorm:"not null"`
	DisplayName string                 `gorm:"not null"`
	Config      map[string]interface{} `gorm:"serializer:json"`
	Enabled     bool                   `gorm:"default:true"`
	CreatedAt   time.Time
}
