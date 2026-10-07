package collaborator

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/permission"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func fail(c *echo.Context, err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return apperr.Respond(c, http.StatusNotFound, ErrNotInProject)
	}
	return apperr.Respond(c, http.StatusInternalServerError, err)
}

// redact apaga o valor por hora que quem chama não pode ver: ele é da própria pessoa e de
// quem vê o valor dos outros (admins e quem tem essa permissão no projeto). Sem login no
// contexto, nada é mostrado.
func redact(set permission.Set, me *auth.Identity, c *Collaborator) {
	if set.HasAny(permission.RatesView, permission.RatesManage) {
		return
	}
	if me == nil || c.Person.ID != me.PersonID {
		c.PayRateCents = nil
	}
}

// ListByProject devolve quem está no projeto. Todos veem as pessoas e os times;
// o valor por hora dos colegas só vai para quem tem permissão de vê-lo.
func (h *Handler) ListByProject(c *echo.Context) error {
	list, err := h.svc.ListByProject(c.Param("projectId"))
	if err != nil {
		return fail(c, err)
	}
	me, set := auth.CurrentPerson(c), auth.ProjectPermissions(c)
	for i := range list {
		redact(set, me, &list[i])
	}
	return c.JSON(http.StatusOK, list)
}

func (h *Handler) Remove(c *echo.Context) error {
	if err := h.svc.Remove(c.Param("projectId"), c.Param("personId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
