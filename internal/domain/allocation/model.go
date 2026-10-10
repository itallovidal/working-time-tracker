// Package allocation cuida do vínculo de uma pessoa com um projeto e do valor
// que a organização paga a ela por hora nesse projeto.
package allocation

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/validate"
)

// MaxRateCents é o teto de um valor por hora: 1.000.000,00. É validate.MaxCents, que fica num lugar só.
const MaxRateCents = validate.MaxCents

type Person struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type Project struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Allocation struct {
	ProjectID    uuid.UUID `json:"project_id"`
	Project      *Project  `json:"project,omitempty"`
	PersonID     uuid.UUID `json:"person_id"`
	Person       *Person   `json:"person,omitempty"`
	PayRateCents int       `json:"pay_rate_cents"`
	// Preset é o grupo de permissões da pessoa neste projeto, e Permissions, a lista que
	// ele deu (ver o pacote permission).
	Preset      string    `json:"preset"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}
