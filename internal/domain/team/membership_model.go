package team

import (
	"time"

	"github.com/google/uuid"
)

type Person struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type TeamMembership struct {
	PersonID  uuid.UUID `json:"person_id"`
	Person    *Person   `json:"person,omitempty"`
	TeamID    uuid.UUID `json:"team_id"`
	CreatedAt time.Time `json:"created_at"`
}
