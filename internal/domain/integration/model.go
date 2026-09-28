package integration

import (
	"time"

	"github.com/google/uuid"
)

type Integration struct {
	ID          uuid.UUID              `json:"id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	Type        string                 `json:"type"`
	DisplayName string                 `json:"display_name"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Enabled     bool                   `json:"enabled"`
	// HasConfig diz se há credencial salva. É calculado antes de a config ser
	// removida da resposta, para a interface mostrar o estado sem ver o segredo.
	HasConfig bool      `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
