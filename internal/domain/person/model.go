package person

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/organization"
)

type Person struct {
	ID             uuid.UUID                 `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name           string                    `gorm:"not null" json:"name"`
	Email          string                    `gorm:"not null;uniqueIndex:idx_org_email" json:"email"`
	OrganizationID uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex:idx_org_email" json:"organization_id"`
	Organization   organization.Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"organization,omitempty"`
	CreatedAt      time.Time                 `json:"created_at"`
}
