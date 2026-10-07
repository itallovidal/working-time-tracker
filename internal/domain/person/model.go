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
	// IsOwner marca o dono da organização: quem a criou. É admin, um só por organização,
	// e não perde o papel.
	IsOwner bool `json:"is_owner"`
	// Permissions são as permissões da organização liberadas a quem não é admin.
	Permissions []string `json:"permissions"`
	// WeeklyHours é a jornada semanal combinada com a pessoa, em horas; nil quando
	// não foi informada. Vale para a organização toda, e não por projeto.
	WeeklyHours *int      `json:"weekly_hours"`
	CreatedAt   time.Time `json:"created_at"`
}
