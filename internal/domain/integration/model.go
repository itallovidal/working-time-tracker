package integration

import (
	"time"

	"github.com/google/uuid"
)

type Integration struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Type        string    `json:"type"`
	DisplayName string    `json:"display_name"`
	// Credentials é o token criptografado. Não sai do service.
	Credentials map[string]interface{} `json:"-"`
	// Metadata são os campos próprios da plataforma (repositório, quadro). Não guarda
	// segredo, então volta nas respostas.
	Metadata map[string]interface{} `json:"metadata"`
	Enabled  bool                   `json:"enabled"`
	// HasToken diz se há credencial salva. É calculado antes de ela ser removida da
	// resposta, para a interface mostrar o estado sem ver o segredo.
	HasToken  bool      `json:"has_token"`
	CreatedAt time.Time `json:"created_at"`
}
