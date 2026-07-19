package handlers

import (
	"time"
	"working-time-tracker/internal/service"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

func (h *TaskHandler) Create(c *echo.Context) error {
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

func (h *TaskHandler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	tasks, err := h.svc.ListByProject(projectID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, tasks)
}

func (h *TaskHandler) Get(c *echo.Context) error {
	id := c.Param("taskId")
	task, err := h.svc.Get(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"error": "task not found"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *TaskHandler) Update(c *echo.Context) error {
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

func (h *TaskHandler) Delete(c *echo.Context) error {
	id := c.Param("taskId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}

func (h *TaskHandler) LinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	var body struct {
		IntegrationID  string `json:"integration_id"`
		ExternalItemID string `json:"external_item_id"`
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

func (h *TaskHandler) UnlinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	task, err := h.svc.UnlinkExternalItem(taskID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *TaskHandler) GetExternalDetails(c *echo.Context) error {
	taskID := c.Param("taskId")
	task, err := h.svc.Get(taskID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"error": "task not found"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	if task.ExternalIntegrationID == nil {
		return c.JSON(400, map[string]string{"error": "task has no linked external item"})
	}
	return c.JSON(501, map[string]string{"error": "external details fetching not yet implemented"})
}
