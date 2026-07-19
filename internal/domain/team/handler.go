package team

import (
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type Handler struct {
	svc           *Service
	membershipSvc *MembershipService
}

func NewHandler(svc *Service, membershipSvc *MembershipService) *Handler {
	return &Handler{svc: svc, membershipSvc: membershipSvc}
}

func (h *Handler) Create(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	team, err := h.svc.Create(projectID, body.Name)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, team)
}

func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	teams, err := h.svc.ListByProject(projectID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, teams)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("teamId")
	team, err := h.svc.Get(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"error": "team not found"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, team)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("teamId")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	team, err := h.svc.Update(id, body.Name)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, team)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("teamId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}

func (h *Handler) AddMember(c *echo.Context) error {
	teamID := c.Param("teamId")
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.PersonID == "" {
		return c.JSON(400, map[string]string{"error": "person_id is required"})
	}
	membership, err := h.membershipSvc.Add(teamID, body.PersonID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, membership)
}

func (h *Handler) RemoveMember(c *echo.Context) error {
	teamID := c.Param("teamId")
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.PersonID == "" {
		return c.JSON(400, map[string]string{"error": "person_id is required"})
	}
	if err := h.membershipSvc.Remove(teamID, body.PersonID); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.NoContent(204)
}

func (h *Handler) ListMembers(c *echo.Context) error {
	teamID := c.Param("teamId")
	members, err := h.membershipSvc.ListByTeam(teamID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, members)
}
