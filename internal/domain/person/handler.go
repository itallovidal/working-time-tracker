package person

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

func (h *Handler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	persons, err := h.svc.ListByOrg(orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, persons)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("personId")
	person, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "pessoa não encontrada"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "corpo da requisição inválido"})
	}
	person, err := h.svc.Update(id, body.Name, body.Email)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) SetRole(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "corpo da requisição inválido"})
	}
	person, err := h.svc.SetRole(id, body.Role)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "pessoa não encontrada"})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, person)
}
