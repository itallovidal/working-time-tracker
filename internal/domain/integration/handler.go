package integration

import (
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
)

// integrationResponse é a mesma para todos os tipos: o que muda de uma plataforma
// para outra está em metadata. O token nunca volta.
type integrationResponse struct {
	ID          uuid.UUID              `json:"id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	Type        string                 `json:"type"`
	DisplayName string                 `json:"display_name"`
	HasToken    bool                   `json:"has_token"`
	Metadata    map[string]interface{} `json:"metadata"`
	Enabled     bool                   `json:"enabled"`
	// A sincronização das issues com as tarefas (só o GitHub): se está ligada, quando terminou a última
	// rodada, o código do erro dela e quantas issues têm responsável sem correspondência aqui.
	SyncIssues    bool       `json:"sync_issues"`
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	LastSyncError string     `json:"last_sync_error"`
	SyncUnmatched int        `json:"sync_unmatched"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toIntegrationResponse(m *Integration) integrationResponse {
	return integrationResponse{
		ID:          m.ID,
		ProjectID:   m.ProjectID,
		Type:        m.Type,
		DisplayName: m.DisplayName,
		HasToken:    m.HasToken,
		Metadata:    m.Metadata,
		Enabled:     m.Enabled,

		SyncIssues:    m.SyncIssues,
		LastSyncedAt:  m.LastSyncedAt,
		LastSyncError: m.LastSyncError,
		SyncUnmatched: m.SyncUnmatched,
		CreatedAt:     m.CreatedAt,
	}
}

func toIntegrationResponseList(ms []Integration) []integrationResponse {
	resp := make([]integrationResponse, len(ms))
	for i, m := range ms {
		resp[i] = toIntegrationResponse(&m)
	}
	return resp
}

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		Type        string                 `json:"type"`
		DisplayName string                 `json:"display_name"`
		Token       string                 `json:"token"`
		Metadata    map[string]interface{} `json:"metadata"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	if body.Type == "" {
		return apperr.Respond(c, 400, ErrTypeRequired.With("field", "type"))
	}
	// Um tipo "em breve" não ganha integração nova pela API. Fica no handler e não no
	// service: as que já existem seguem funcionando, e reativar o tipo é virar a flag.
	if impl, err := adapter.GetIntegration(body.Type); err == nil && impl.Descriptor().ComingSoon {
		return apperr.Respond(c, 400, ErrTypeComingSoon.With("provider", impl.Descriptor().Label, "field", "type"))
	}
	// Sem enabled no corpo, a integração nasce ativa.
	enabled := body.Enabled == nil || *body.Enabled
	it, err := h.svc.Create(projectID, body.Type, body.DisplayName, body.Token, body.Metadata, enabled)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(201, toIntegrationResponse(it))
}

func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	integrations, err := h.svc.ListByProject(projectID)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, toIntegrationResponseList(integrations))
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("integrationId")
	it, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, toIntegrationResponse(it))
}

// Repositories lista o que a conexão da integração enxerga, para a tela oferecer a
// escolha. O token fica no servidor: a resposta traz só os nomes.
func (h *Handler) Repositories(c *echo.Context) error {
	repos, err := h.svc.Repositories(c.Param("integrationId"))
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, repos)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("integrationId")
	var body struct {
		DisplayName string                 `json:"display_name"`
		Token       string                 `json:"token"`
		Metadata    map[string]interface{} `json:"metadata"`
		Enabled     *bool                  `json:"enabled"`
		SyncIssues  *bool                  `json:"sync_issues"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	it, err := h.svc.Edit(id, EditInput{
		DisplayName: body.DisplayName, Token: body.Token, Metadata: body.Metadata,
		Enabled: body.Enabled, SyncIssues: body.SyncIssues,
	})
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, toIntegrationResponse(it))
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("integrationId")
	if err := h.svc.Delete(id); err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.NoContent(204)
}
