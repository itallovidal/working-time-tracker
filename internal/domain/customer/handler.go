package customer

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/database"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func fail(c *echo.Context, err error) error {
	if err == database.ErrNotFound {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "cliente não encontrado"})
	}
	return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (h *Handler) Create(c *echo.Context) error {
	var body Input
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "corpo da requisição inválido"})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "corpo da requisição inválido"})
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
