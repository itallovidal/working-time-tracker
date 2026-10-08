package auth

import (
	"slices"
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
	IsOwner bool `json:"is_owner"`
	// Permissions são as permissões da organização que o dono liberou a esta pessoa;
	// ficam vazias nos admins, que têm todas.
	Permissions      []string  `json:"permissions"`
	OrganizationID   uuid.UUID `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	// OrganizationCurrency é a moeda dos valores da organização (BRL, USD ou EUR).
	OrganizationCurrency string `json:"organization_currency"`
	// HasPassword diz se a conta tem senha do sistema. Quem entra só pelo Clerk não tem, e a troca de senha
	// não se aplica a essa pessoa.
	HasPassword bool `json:"has_password"`
}

func (i *Identity) IsAdmin() bool {
	return i != nil && i.Role == "admin"
}

// Can diz se a pessoa pode fazer o que a permissão da organização libera. Os admins
// podem tudo; os outros, o que o dono liberou a eles.
func (i *Identity) Can(key string) bool {
	return i.IsAdmin() || (i != nil && slices.Contains(i.Permissions, key))
}

// As formas de entrega de um convite (ver Invite.Delivery).
const (
	DeliveryEmail    = "email"
	DeliveryTerminal = "terminal"
	DeliveryLink     = "link"
)

// ProjectSetup é o que um convite leva para um projeto: quando a pessoa o aceita, entra nele com este valor por
// hora, este time (opcional) e este grupo de permissões (vazio é o grupo básico, de membro).
type ProjectSetup struct {
	ProjectID   uuid.UUID `json:"project_id"`
	ProjectName string    `json:"project_name,omitempty"`
	// PayRateCents é nulo quando a lista que devolve o convite não pode mostrar o valor a quem olha.
	PayRateCents *int       `json:"pay_rate_cents,omitempty"`
	TeamID       *uuid.UUID `json:"team_id,omitempty"`
	TeamName     string     `json:"team_name,omitempty"`
	Preset       string     `json:"preset,omitempty"`
}

type Invite struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          *string   `json:"email"`
	Role           string    `json:"role"`
	CreatedByName  string    `json:"created_by_name,omitempty"`
	// Delivery diz como o convite chega à pessoa e só vem na resposta que o cria: DeliveryEmail (o Clerk manda
	// o e-mail), DeliveryTerminal (não mandou: o link está no log do servidor) ou DeliveryLink (só o link, para
	// copiar). Não é guardado.
	Delivery string `json:"delivery,omitempty"`
	// Project é o projeto a que o convite leva a pessoa, quando leva (ver ProjectSetup).
	Project    *ProjectSetup `json:"project,omitempty"`
	ExpiresAt  time.Time     `json:"expires_at"`
	AcceptedAt *time.Time    `json:"accepted_at,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}

// InviteInfo é o que a página pública do convite mostra antes do aceite.
type InviteInfo struct {
	OrganizationName string    `json:"organization_name"`
	Email            *string   `json:"email"`
	Role             string    `json:"role"`
	ExpiresAt        time.Time `json:"expires_at"`
}
