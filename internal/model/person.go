package model

import (
	"time"

	"github.com/google/uuid"
)

type Person struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name           string       `gorm:"not null"`
	Email          string       `gorm:"not null;uniqueIndex:idx_org_email"`
	OrganizationID uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:idx_org_email"`
	Organization   Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE"`
	CreatedAt      time.Time
}
