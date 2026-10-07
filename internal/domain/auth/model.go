package auth

import (
	"time"

	"github.com/google/uuid"
)

// Identity é a pessoa logada, como os handlers e as páginas enxergam.
type Identity struct {
	PersonID uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	// IsOwner marca o dono da organização, que é admin e é um só por organização.
	IsOwner          bool      `json:"is_owner"`
	OrganizationID   uuid.UUID `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	// OrganizationCurrency é a moeda dos valores da organização (BRL, USD ou EUR).
	OrganizationCurrency string `json:"organization_currency"`
}

func (i *Identity) IsAdmin() bool {
	return i != nil && i.Role == "admin"
}

type Invite struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Email          *string    `json:"email"`
	Role           string     `json:"role"`
	CreatedByName  string     `json:"created_by_name,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// InviteInfo é o que a página pública do convite mostra antes do aceite.
type InviteInfo struct {
	OrganizationName string    `json:"organization_name"`
	Email            *string   `json:"email"`
	Role             string    `json:"role"`
	ExpiresAt        time.Time `json:"expires_at"`
}
