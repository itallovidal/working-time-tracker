package person

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type Person struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
}
