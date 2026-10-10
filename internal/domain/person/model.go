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
	// WeeklyHours é a jornada semanal combinada com a pessoa, em horas; nil quando
	// não foi informada. Vale para a organização toda, e não por projeto.
	WeeklyHours *int `json:"weekly_hours"`
	// Payment é a regra de quando a pessoa é paga; nil quando não há regra.
	Payment   *PaymentRule `json:"payment"`
	CreatedAt time.Time    `json:"created_at"`
}

const (
	PaymentMonthly  = "monthly"
	PaymentBiweekly = "biweekly"
)

// PaymentRule é a regra de pagamento: mensal (Day, 1 a 31) ou quinzenal (Start, um dia de calendário
// YYYY-MM-DD, sem fuso, de onde se contam os períodos de 15 dias).
type PaymentRule struct {
	Frequency string `json:"frequency"`
	Day       int    `json:"day,omitempty"`
	Start     string `json:"start,omitempty"`
}
