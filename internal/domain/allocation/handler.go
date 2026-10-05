package allocation

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
		return c.JSON(http.StatusNotFound, map[string]string{"error": "esta pessoa não tem valor definido neste projeto"})
	}
	return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
}

// ListByProject devolve os valores do projeto. Um admin recebe os de todo
// mundo; qualquer outra pessoa recebe só o dela, ou uma lista vazia.
func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	me := auth.CurrentPerson(c)
	if me.IsAdmin() {
		all, err := h.svc.ListByProject(projectID)
		if err != nil {
			return fail(c, err)
		}
		return c.JSON(http.StatusOK, all)
	}
	mine := []Allocation{}
	if me != nil {
		a, err := h.svc.Get(projectID, me.PersonID.String())
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		if a != nil {
			mine = append(mine, *a)
		}
	}
	return c.JSON(http.StatusOK, mine)
}

// ListByPerson devolve os valores de uma pessoa em todos os projetos. Só ela
// mesma e os admins podem ver.
func (h *Handler) ListByPerson(c *echo.Context) error {
	personID := c.Param("personId")
	me := auth.CurrentPerson(c)
	if !me.IsAdmin() && (me == nil || me.PersonID.String() != personID) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "você só pode ver os seus próprios valores"})
	}
	list, err := h.svc.ListByPerson(personID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

func (h *Handler) Set(c *echo.Context) error {
	var body struct {
		PayRateCents *int `json:"pay_rate_cents"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "corpo da requisição inválido"})
	}
	if body.PayRateCents == nil {
		return fail(c, ErrRateRequired)
	}
	a, err := h.svc.Set(c.Param("projectId"), c.Param("personId"), *body.PayRateCents)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, a)
}

func (h *Handler) Remove(c *echo.Context) error {
	if err := h.svc.Remove(c.Param("projectId"), c.Param("personId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
