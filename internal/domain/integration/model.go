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
	HasToken bool `json:"has_token"`
	// SyncIssues liga a sincronização das issues do repositório com as tarefas (só o GitHub).
	SyncIssues bool `json:"sync_issues"`
	// SyncCursor é de onde a próxima rodada incremental pede as issues mexidas; nulo é a primeira vez.
	SyncCursor *time.Time `json:"-"`
	// LastSyncedAt é o fim da última rodada que terminou, e LastSyncError o código do erro dela (vazio
	// se deu certo).
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	LastSyncError string     `json:"last_sync_error"`
	// SyncUnmatched é quantas issues abertas têm responsável no GitHub que não se liga a ninguém aqui.
	// Só é calculado quando a sincronização está ligada.
	SyncUnmatched int       `json:"sync_unmatched"`
	CreatedAt     time.Time `json:"created_at"`
}
