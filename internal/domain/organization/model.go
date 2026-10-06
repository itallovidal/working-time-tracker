package organization

import (
	"time"

	"github.com/google/uuid"
)

const (
	DefaultTimezone = "America/Sao_Paulo"
	DefaultCurrency = "BRL"
)

type Organization struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`

	Summary     string `json:"summary"`
	Description string `json:"description"`
	Industry    string `json:"industry"`
	FoundedYear *int   `json:"founded_year"`
	Size        string `json:"size"`

	Website      string `json:"website"`
	ContactEmail string `json:"contact_email"`
	Phone        string `json:"phone"`
	LinkedinURL  string `json:"linkedin_url"`
	InstagramURL string `json:"instagram_url"`

	LegalName    string `json:"legal_name"`
	CNPJ         string `json:"cnpj"`
	AddressLine1 string `json:"address_line1"`
	AddressLine2 string `json:"address_line2"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`

	// WorkMode é o regime de trabalho: remote, hybrid, onsite ou vazio.
	WorkMode string `json:"work_mode"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`

	CreatedAt time.Time `json:"created_at"`
}

// UpdateInput são os campos que o PATCH da organização pode alterar. Campo
// ausente (nil) mantém o valor atual; texto vazio ou número zero apaga. Fuso e
// moeda nunca ficam sem valor: vazios, voltam para o padrão.
type UpdateInput struct {
	Name *string `json:"name"`

	Summary     *string `json:"summary"`
	Description *string `json:"description"`
	Industry    *string `json:"industry"`
	FoundedYear *int    `json:"founded_year"`
	Size        *string `json:"size"`

	Website      *string `json:"website"`
	ContactEmail *string `json:"contact_email"`
	Phone        *string `json:"phone"`
	LinkedinURL  *string `json:"linkedin_url"`
	InstagramURL *string `json:"instagram_url"`

	LegalName    *string `json:"legal_name"`
	CNPJ         *string `json:"cnpj"`
	AddressLine1 *string `json:"address_line1"`
	AddressLine2 *string `json:"address_line2"`
	City         *string `json:"city"`
	State        *string `json:"state"`
	PostalCode   *string `json:"postal_code"`
	Country      *string `json:"country"`

	WorkMode *string `json:"work_mode"`
	Timezone *string `json:"timezone"`
	Currency *string `json:"currency"`
}
