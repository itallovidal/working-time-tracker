package task

import (
	"time"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/database"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		AssigneeID  string     `json:"assignee_id"`
		Deadline    *time.Time `json:"deadline"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	task, err := h.svc.Create(projectID, body.Name, body.Description, body.AssigneeID, body.Deadline)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, task)
}

func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	tasks, err := h.svc.ListByProject(projectID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, tasks)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("taskId")
	task, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(404, map[string]string{"error": "task not found"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("taskId")
	var body struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		AssigneeID  *string    `json:"assignee_id"`
		Deadline    *time.Time `json:"deadline"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	task, err := h.svc.Update(id, body.Name, body.Description, body.AssigneeID, body.Deadline)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("taskId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}

func (h *Handler) LinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	var body struct {
		IntegrationID   string `json:"integration_id"`
		ExternalItemID  string `json:"external_item_id"`
		ExternalItemURL string `json:"external_item_url"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.IntegrationID == "" || body.ExternalItemID == "" || body.ExternalItemURL == "" {
		return c.JSON(400, map[string]string{"error": "integration_id, external_item_id, and external_item_url are required"})
	}
	task, err := h.svc.LinkExternalItem(taskID, body.IntegrationID, body.ExternalItemID, body.ExternalItemURL)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) UnlinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	task, err := h.svc.UnlinkExternalItem(taskID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) GetExternalDetails(c *echo.Context) error {
	taskID := c.Param("taskId")
	result, err := h.svc.GetExternalDetails(taskID)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(404, map[string]string{"error": "task not found"})
		}
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, result)
}
