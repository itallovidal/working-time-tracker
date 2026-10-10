package customer

import (
	"errors"
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

func fail(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
	case errors.Is(err, ErrHasProjects):
		// Conflito com o estado atual (o cliente ainda tem projetos), não um corpo mal formado.
		return apperr.Respond(c, http.StatusConflict, err)
	}
	return apperr.Respond(c, http.StatusBadRequest, err)
}

func (h *Handler) Create(c *echo.Context) error {
	var body Input
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	customer, err := h.svc.Create(c.Param("orgId"), body)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusCreated, customer)
}

func (h *Handler) ListByOrg(c *echo.Context) error {
	customers, err := h.svc.ListByOrg(c.Param("orgId"))
	if err != nil {
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, customers)
}

func (h *Handler) Get(c *echo.Context) error {
	customer, err := h.svc.Get(c.Param("customerId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, customer)
}

func (h *Handler) Update(c *echo.Context) error {
	var body Input
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	customer, err := h.svc.Update(c.Param("customerId"), body)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, customer)
}

func (h *Handler) Delete(c *echo.Context) error {
	if err := h.svc.Delete(c.Param("customerId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
