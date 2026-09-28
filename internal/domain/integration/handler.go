package integration

import (
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/database"
)

type integrationResponse struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Type        string    `json:"type"`
	DisplayName string    `json:"display_name"`
	HasConfig   bool      `json:"has_config"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

func toIntegrationResponse(m *Integration) integrationResponse {
	return integrationResponse{
		ID:          m.ID,
		ProjectID:   m.ProjectID,
		Type:        m.Type,
		DisplayName: m.DisplayName,
		HasConfig:   m.HasConfig,
		Enabled:     m.Enabled,
		CreatedAt:   m.CreatedAt,
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
		Config      map[string]interface{} `json:"config"`
		Enabled     bool                   `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "corpo da requisição inválido"})
	}
	if body.Type == "" {
		return c.JSON(400, map[string]string{"error": "informe o tipo da integração"})
	}
	it, err := h.svc.Create(projectID, body.Type, body.DisplayName, body.Config, body.Enabled)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, toIntegrationResponse(it))
}

func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	integrations, err := h.svc.ListByProject(projectID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, toIntegrationResponseList(integrations))
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("integrationId")
	it, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(404, map[string]string{"error": "integração não encontrada"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, toIntegrationResponse(it))
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("integrationId")
	var body struct {
		DisplayName string                 `json:"display_name"`
		Config      map[string]interface{} `json:"config,omitempty"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "corpo da requisição inválido"})
	}
	it, err := h.svc.Update(id, body.DisplayName, body.Config, body.Enabled)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, toIntegrationResponse(it))
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("integrationId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}
