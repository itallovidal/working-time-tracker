package project

import (
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(c *echo.Context) error {
	orgID := c.Param("orgId")
	var body struct {
		Name               string  `json:"name"`
		Description        string  `json:"description"`
		SprintDurationDays int     `json:"sprint_duration_days"`
		DailyTime          *string `json:"daily_time"`
		WeeklySyncDay      *string `json:"weekly_sync_day"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	project, err := h.svc.Create(orgID, body.Name, body.Description, body.SprintDurationDays, body.DailyTime, body.WeeklySyncDay)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, project)
}

func (h *Handler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	projects, err := h.svc.ListByOrg(orgID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, projects)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("projectId")
	project, err := h.svc.Get(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"error": "project not found"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, project)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("projectId")
	var body struct {
		Name               string  `json:"name"`
		Description        string  `json:"description"`
		SprintDurationDays int     `json:"sprint_duration_days"`
		DailyTime          *string `json:"daily_time"`
		WeeklySyncDay      *string `json:"weekly_sync_day"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	project, err := h.svc.Update(id, body.Name, body.Description, body.SprintDurationDays, body.DailyTime, body.WeeklySyncDay)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, project)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("projectId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}
