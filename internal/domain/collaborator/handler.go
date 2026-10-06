package collaborator

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func fail(c *echo.Context, err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "esta pessoa não está neste projeto"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// redact apaga o valor por hora que quem chama não pode ver: ele é dos admins
// e da própria pessoa. Sem login no contexto, nada é mostrado.
func redact(me *auth.Identity, c *Collaborator) {
	if me.IsAdmin() {
		return
	}
	if me == nil || c.Person.ID != me.PersonID {
		c.PayRateCents = nil
	}
}

// ListByProject devolve quem está no projeto. Todos veem as pessoas e os times;
// o valor por hora dos colegas só vai para admins.
func (h *Handler) ListByProject(c *echo.Context) error {
	list, err := h.svc.ListByProject(c.Param("projectId"))
	if err != nil {
		return fail(c, err)
	}
	me := auth.CurrentPerson(c)
	for i := range list {
		redact(me, &list[i])
	}
	return c.JSON(http.StatusOK, list)
}

func (h *Handler) Remove(c *echo.Context) error {
	if err := h.svc.Remove(c.Param("projectId"), c.Param("personId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
