package handlers

import (
	"working-time-tracker/internal/service"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type ProjectHandler struct {
	svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) Create(c *echo.Context) error {
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

func (h *ProjectHandler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	projects, err := h.svc.ListByOrg(orgID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, projects)
}

func (h *ProjectHandler) Get(c *echo.Context) error {
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

func (h *ProjectHandler) Update(c *echo.Context) error {
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

func (h *ProjectHandler) Delete(c *echo.Context) error {
	id := c.Param("projectId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}
