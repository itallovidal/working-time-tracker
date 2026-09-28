package organization

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

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("orgId")
	org, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "organização não encontrada"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, org)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("orgId")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "corpo da requisição inválido"})
	}
	org, err := h.svc.Update(id, body.Name)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, org)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("orgId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}
