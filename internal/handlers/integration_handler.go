package handlers

import (
	"time"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
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

func toIntegrationResponse(m *model.Integration) integrationResponse {
	return integrationResponse{
		ID:          m.ID,
		ProjectID:   m.ProjectID,
		Type:        m.Type,
		DisplayName: m.DisplayName,
		HasConfig:   m.Enabled,
		Enabled:     m.Enabled,
		CreatedAt:   m.CreatedAt,
	}
}

func toIntegrationResponseList(ms []model.Integration) []integrationResponse {
	resp := make([]integrationResponse, len(ms))
	for i, m := range ms {
		resp[i] = toIntegrationResponse(&m)
	}
	return resp
}

type IntegrationHandler struct {
	svc *service.IntegrationService
}

func NewIntegrationHandler(svc *service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
}

func (h *IntegrationHandler) Create(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		Type        string                 `json:"type"`
		DisplayName string                 `json:"display_name"`
		Config      map[string]interface{} `json:"config"`
		Enabled     bool                   `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.Type == "" {
		return c.JSON(400, map[string]string{"error": "type is required"})
	}
	integration, err := h.svc.Create(projectID, body.Type, body.DisplayName, body.Config, body.Enabled)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, toIntegrationResponse(integration))
}

func (h *IntegrationHandler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	integrations, err := h.svc.ListByProject(projectID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, toIntegrationResponseList(integrations))
}

func (h *IntegrationHandler) Get(c *echo.Context) error {
	id := c.Param("integrationId")
	integration, err := h.svc.Get(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"error": "integration not found"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, toIntegrationResponse(integration))
}

func (h *IntegrationHandler) Update(c *echo.Context) error {
	id := c.Param("integrationId")
	var body struct {
		DisplayName string                 `json:"display_name"`
		Config      map[string]interface{} `json:"config,omitempty"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	integration, err := h.svc.Update(id, body.DisplayName, body.Config, body.Enabled)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, toIntegrationResponse(integration))
}

func (h *IntegrationHandler) Delete(c *echo.Context) error {
	id := c.Param("integrationId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}
