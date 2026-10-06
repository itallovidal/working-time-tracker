package organization

import (
	"net/http"

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

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("orgId")
	org, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, org)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("orgId")
	var body UpdateInput
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	org, err := h.svc.Update(id, body)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, org)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("orgId")
	if err := h.svc.Delete(id); err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.NoContent(http.StatusNoContent)
}
