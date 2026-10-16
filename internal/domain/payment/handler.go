package payment

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ByPerson devolve os pagamentos de uma pessoa: o período corrente, os próximos e o histórico. Só ela mesma e o
// dono veem o dinheiro. Um admin que não é o dono abre o perfil de outra pessoa e recebe só as horas, sem a regra,
// os períodos nem os valores (HideMoney). ?history=N pede mais ou menos períodos fechados.
func (h *Handler) ByPerson(c *echo.Context) error {
	personID := c.Param("personId")
	me := auth.CurrentPerson(c)
	if me == nil || (!me.IsAdmin() && me.PersonID.String() != personID) {
		return apperr.Respond(c, http.StatusForbidden, ErrOwnOnly)
	}
	history := 0
	if raw := c.QueryParam("history"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return apperr.Respond(c, http.StatusBadRequest, ErrInvalidHistory.With("field", "history"))
		}
		history = n
	}
	sum, err := h.svc.Person(me.OrganizationID.String(), personID, history, me.IsOwner)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return apperr.Respond(c, http.StatusNotFound, person.ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	if !me.IsOwner && me.PersonID.String() != personID {
		sum.HideMoney()
	}
	return c.JSON(http.StatusOK, sum)
}

// Team devolve a visão do dono: quanto pagar, as horas de cada pessoa e as datas. A rota é só do dono; o admin e
// quem tem people.manage não veem dinheiro de pagamento.
func (h *Handler) Team(c *echo.Context) error {
	t, err := h.svc.Team(c.Param("orgId"))
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return apperr.Respond(c, http.StatusNotFound, organization.ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, t)
}

// MarkPaid dá baixa num período de uma pessoa (só o dono, pela rota). Responde 201 com a baixa.
func (h *Handler) MarkPaid(c *echo.Context) error {
	var body MarkInput
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	if body.PersonID == "" {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrFieldRequired.With("field", "person_id"))
	}
	po, err := h.svc.MarkPaid(c.Param("orgId"), body)
	if err != nil {
		var ae *apperr.Error
		switch {
		case errors.Is(err, database.ErrNotFound):
			return apperr.Respond(c, http.StatusNotFound, person.ErrNotFound)
		case errors.Is(err, ErrAlreadyPaid):
			return apperr.Respond(c, http.StatusConflict, err)
		case errors.As(err, &ae):
			return apperr.Respond(c, http.StatusBadRequest, err)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusCreated, po)
}

// Unmark desfaz uma baixa (só o dono, pela rota). Responde 204.
func (h *Handler) Unmark(c *echo.Context) error {
	if err := h.svc.Unmark(c.Param("orgId"), c.Param("payoutId")); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return apperr.Respond(c, http.StatusNotFound, ErrPayoutNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.NoContent(http.StatusNoContent)
}
