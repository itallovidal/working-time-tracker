package project

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
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
		Name               string `json:"name"`
		Description        string `json:"description"`
		SprintDurationDays int    `json:"sprint_duration_days"`
		Routine
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	project, err := h.svc.Create(orgID, body.Name, body.Description, body.SprintDurationDays, body.Routine)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(201, project)
}

func (h *Handler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	projects, err := h.svc.ListByOrg(orgID)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, projects)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("projectId")
	project, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, project)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("projectId")
	var body struct {
		Name               string `json:"name"`
		Description        string `json:"description"`
		SprintDurationDays int    `json:"sprint_duration_days"`
		Routine
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	project, err := h.svc.Update(id, body.Name, body.Description, body.SprintDurationDays, body.Routine)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, project)
}

// GetBilling devolve o cliente e o valor cobrado do projeto. A rota é só de admins.
func (h *Handler) GetBilling(c *echo.Context) error {
	billing, err := h.svc.Billing(c.Param("projectId"))
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, billing)
}

// SetBilling substitui o cliente e o valor cobrado: o que vier null é apagado.
func (h *Handler) SetBilling(c *echo.Context) error {
	var body struct {
		CustomerID    *string `json:"customer_id"`
		BillRateCents *int    `json:"bill_rate_cents"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	billing, err := h.svc.SetBilling(c.Param("projectId"), body.CustomerID, body.BillRateCents)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, billing)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("projectId")
	if err := h.svc.Delete(id); err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.NoContent(204)
}
