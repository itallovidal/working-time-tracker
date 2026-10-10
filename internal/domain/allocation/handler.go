package allocation

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
		return apperr.Respond(c, http.StatusNotFound, ErrNotDefined)
	}
	return apperr.Respond(c, http.StatusBadRequest, err)
}

// ListByProject devolve os valores do projeto. Quem vê o valor dos outros (admins e quem
// tem essa permissão no projeto) recebe os de todo mundo; qualquer outra pessoa recebe
// só o dela, ou uma lista vazia. A lista de permissões de cada pessoa só vai para ela mesma, para o dono e para os
// admins.
func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	me := auth.CurrentPerson(c)
	if auth.ProjectPermissions(c).HasAny(permission.RatesView, permission.RatesManage) {
		all, err := h.svc.ListByProject(projectID)
		if err != nil {
			return fail(c, err)
		}
		// O grupo (preset) é o papel da pessoa e todo mundo o vê; a lista de permissões de cada um é só da própria
		// pessoa, do dono e dos admins.
		if me == nil || !me.IsAdmin() {
			for i := range all {
				if me == nil || all[i].PersonID != me.PersonID {
					all[i].Permissions = []string{}
				}
			}
		}
		return c.JSON(http.StatusOK, all)
	}
	mine := []Allocation{}
	if me != nil {
		a, err := h.svc.Get(projectID, me.PersonID.String())
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return apperr.Respond(c, http.StatusInternalServerError, err)
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
		return apperr.Respond(c, http.StatusForbidden, ErrOwnRatesOnly)
	}
	list, err := h.svc.ListByPerson(personID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

// Set põe a pessoa no projeto, troca o valor dela ou o grupo de permissões. O valor só é
// obrigatório para quem entra; o grupo, só se vier. Cada mudança pede a sua permissão:
// pôr alguém no projeto e trocar o grupo, collaborators.manage; definir o valor, rates.manage.
// Ninguém concede um grupo que tenha permissão que ele mesmo não tem.
func (h *Handler) Set(c *echo.Context) error {
	var body struct {
		PayRateCents *int   `json:"pay_rate_cents"`
		Preset       string `json:"preset"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	projectID, personID := c.Param("projectId"), c.Param("personId")
	can := auth.ProjectPermissions(c)

	current, err := h.svc.Get(projectID, personID)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	isNew := current == nil
	if body.PayRateCents == nil && (isNew || body.Preset == "") {
		return fail(c, ErrRateRequired.With("field", "pay_rate_cents"))
	}
	changesRate := body.PayRateCents != nil && (isNew || *body.PayRateCents != current.PayRateCents)
	changesPreset := body.Preset != "" && (isNew || body.Preset != current.Preset)
	if (isNew || changesPreset) && !can.Has(permission.CollaboratorsManage) ||
		changesRate && !can.Has(permission.RatesManage) {
		return apperr.Respond(c, http.StatusForbidden, auth.ErrPermissionRequired)
	}
	var preset permission.Preset
	if body.Preset != "" {
		var ok bool
		if preset, ok = permission.PresetByID(body.Preset); !ok {
			return fail(c, ErrInvalidPreset.With("field", "preset"))
		}
		for _, k := range preset.Permissions {
			if !can.Has(k) {
				return apperr.Respond(c, http.StatusForbidden, ErrAbovePermission.With("field", "preset"))
			}
		}
	}

	var a *Allocation
	if body.PayRateCents != nil {
		if a, err = h.svc.Set(projectID, personID, *body.PayRateCents); err != nil {
			return fail(c, err)
		}
	} else {
		a = current
	}
	if body.Preset != "" {
		if a, err = h.svc.SetPreset(projectID, personID, preset.ID); err != nil {
			return fail(c, err)
		}
	}
	return c.JSON(http.StatusOK, a)
}
