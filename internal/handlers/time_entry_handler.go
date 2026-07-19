package handlers

import (
	"working-time-tracker/internal/service"

	"github.com/labstack/echo/v5"
)

type TimeEntryHandler struct {
	svc *service.TimeEntryService
}

func NewTimeEntryHandler(svc *service.TimeEntryService) *TimeEntryHandler {
	return &TimeEntryHandler{svc: svc}
}

func (h *TimeEntryHandler) ClockIn(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		TaskID   string `json:"task_id"`
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.TaskID == "" || body.PersonID == "" {
		return c.JSON(400, map[string]string{"error": "task_id and person_id are required"})
	}
	session, err := h.svc.ClockIn(projectID, body.TaskID, body.PersonID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, session)
}

func (h *TimeEntryHandler) ClockOut(c *echo.Context) error {
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.PersonID == "" {
		return c.JSON(400, map[string]string{"error": "person_id is required"})
	}
	session, err := h.svc.ClockOut(body.PersonID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, session)
}

func (h *TimeEntryHandler) List(c *echo.Context) error {
	projectID := c.Param("projectId")
	taskID := c.QueryParam("task_id")
	personID := c.QueryParam("person_id")

	var taskIDPtr, personIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	if personID != "" {
		personIDPtr = &personID
	}

	sessions, err := h.svc.ListByProject(projectID, taskIDPtr, personIDPtr)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, sessions)
}

func (h *TimeEntryHandler) Total(c *echo.Context) error {
	projectID := c.Param("projectId")
	taskID := c.QueryParam("task_id")
	personID := c.QueryParam("person_id")

	var taskIDPtr, personIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	if personID != "" {
		personIDPtr = &personID
	}

	total, err := h.svc.TotalTime(projectID, taskIDPtr, personIDPtr)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, total)
}
