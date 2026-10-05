// Package customer cuida dos clientes da organização: quem contrata os projetos.
package customer

import (
	"time"

	"github.com/google/uuid"
)

type Customer struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	// Document é o CNPJ do cliente, sem máscara.
	Document     string    `json:"document"`
	ContactName  string    `json:"contact_name"`
	ContactEmail string    `json:"contact_email"`
	ContactPhone string    `json:"contact_phone"`
	ProjectCount int       `json:"project_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// Input são os campos que a API aceita. Na criação, nil vale como vazio; na
// edição, nil mantém o valor atual e texto vazio apaga.
type Input struct {
	Name         *string `json:"name"`
	Document     *string `json:"document"`
	ContactName  *string `json:"contact_name"`
	ContactEmail *string `json:"contact_email"`
	ContactPhone *string `json:"contact_phone"`
}
