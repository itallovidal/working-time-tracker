package projectinvite

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/validate"
)

type Handler struct {
	invites *auth.Service
	teams   *team.Service
}

func NewHandler(invites *auth.Service, teams *team.Service) *Handler {
	return &Handler{invites: invites, teams: teams}
}

// Create convida para a organização alguém que ainda não está nela e já deixa pronto o projeto da rota: o valor
// por hora, o grupo de permissões e o time com que a pessoa entra quando aceitar. Pede o que pedem juntos pôr
// alguém no projeto, dar o valor, o grupo e o time (o PUT de allocations e o POST de membros do time), mais a
// permissão de pessoas da organização, que é a de quem convida. Ninguém concede um grupo que tenha permissão
// que ele mesmo não tem. O resto é do auth.CreateProjectInvite, igual ao convite da organização.
func (h *Handler) Create(c *echo.Context) error {
	var body struct {
		Email        string `json:"email"`
		PayRateCents *int   `json:"pay_rate_cents"`
		TeamID       string `json:"team_id"`
		Preset       string `json:"preset"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	projectID := c.Param("projectId")
	can := auth.ProjectPermissions(c)
	if !can.Has(permission.CollaboratorsManage) || !can.Has(permission.RatesManage) {
		return apperr.Respond(c, http.StatusForbidden, auth.ErrPermissionRequired)
	}
	if body.PayRateCents == nil {
		return apperr.Respond(c, http.StatusBadRequest, allocation.ErrRateRequired.With("field", "pay_rate_cents"))
	}
	if validate.Rate("pay_rate_cents", *body.PayRateCents) != nil {
		return apperr.Respond(c, http.StatusBadRequest, allocation.ErrInvalidRate.With("field", "pay_rate_cents"))
	}

	setup := auth.ProjectSetup{ProjectID: uuid.MustParse(projectID), PayRateCents: body.PayRateCents}
	if body.Preset != "" && body.Preset != permission.PresetMember {
		preset, ok := permission.PresetByID(body.Preset)
		if !ok {
			return apperr.Respond(c, http.StatusBadRequest, allocation.ErrInvalidPreset.With("field", "preset"))
		}
		for _, k := range preset.Permissions {
			if !can.Has(k) {
				return apperr.Respond(c, http.StatusForbidden, allocation.ErrAbovePermission.With("field", "preset"))
			}
		}
		setup.Preset = preset.ID
	}
	if body.TeamID != "" {
		if !can.Has(permission.TeamsManage) {
			return apperr.Respond(c, http.StatusForbidden, auth.ErrPermissionRequired)
		}
		teamID, err := uuid.Parse(body.TeamID)
		if err != nil {
			return apperr.Respond(c, http.StatusBadRequest, ErrTeamNotInProject.With("field", "team_id"))
		}
		t, err := h.teams.Get(body.TeamID)
		if err != nil || t.ProjectID.String() != projectID {
			return apperr.Respond(c, http.StatusBadRequest, ErrTeamNotInProject.With("field", "team_id"))
		}
		setup.TeamID = &teamID
	}

	inv, token, err := h.invites.CreateProjectInvite(c.Request().Context(), auth.CurrentPerson(c), body.Email, setup)
	if err != nil {
		return auth.Respond(c, err)
	}
	return c.JSON(http.StatusCreated, auth.NewInviteResponse(inv, token))
}

// List devolve os convites pendentes que levam a pessoa a este projeto. O valor por hora que cada um leva só
// aparece para quem vê os valores do projeto.
func (h *Handler) List(c *echo.Context) error {
	invites, err := h.invites.ProjectInvites(c.Param("projectId"))
	if err != nil {
		return auth.Respond(c, err)
	}
	if !auth.ProjectPermissions(c).HasAny(permission.RatesView, permission.RatesManage) {
		auth.HideRates(invites)
	}
	return c.JSON(http.StatusOK, invites)
}
