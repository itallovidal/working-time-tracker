package overview

import (
	"errors"
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
	if errors.Is(err, database.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "projeto não encontrado"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// Get devolve a visão geral do projeto. A rota é só de admins: quase tudo aqui
// é o dinheiro do projeto, então não há o que entregar a um membro.
func (h *Handler) Get(c *echo.Context) error {
	o, err := h.svc.Get(c.Param("projectId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, o)
}
