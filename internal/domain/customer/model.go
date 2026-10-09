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
	// Country é o país do cliente: o código ISO 3166-1 (BR, US, DE...). Começa no país da organização e decide a regra
	// do documento.
	Country string `json:"country"`
	// Document é o documento fiscal do cliente. No Brasil (CNPJ) e nos EUA (EIN) é conferido e fica sem máscara; nos
	// outros países é texto livre, sem conferência.
	Document     string    `json:"document"`
	ContactName  string    `json:"contact_name"`
	ContactEmail string    `json:"contact_email"`
	ContactPhone string    `json:"contact_phone"`
	ProjectCount int       `json:"project_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// Input são os campos que a API aceita. Na criação, nil vale como vazio; na
// edição, nil mantém o valor atual e texto vazio apaga. O país é a exceção: nunca fica
// sem valor. Ausente (ou vazio, na criação), é o país da organização.
type Input struct {
	Name         *string `json:"name"`
	Country      *string `json:"country"`
	Document     *string `json:"document"`
	ContactName  *string `json:"contact_name"`
	ContactEmail *string `json:"contact_email"`
	ContactPhone *string `json:"contact_phone"`
}
