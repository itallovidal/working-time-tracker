package model

import (
	"time"

	"github.com/google/uuid"
)

type Person struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name           string    `gorm:"not null"`
	Email          string    `gorm:"not null"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index"`
	Organization   Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE"`
	CreatedAt      time.Time
}
