package team

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/personaccount"
)

type Handler struct {
	svc           *Service
	membershipSvc *MembershipService
	// accounts dá, na lista de quem está no projeto, as plataformas em que cada um tem conta ligada: o select de
	// responsável desabilita quem não passa na trava da tarefa. Nulo, a lista vem sem elas.
	accounts *personaccount.Store
}

// SetAccounts liga as contas das pessoas à lista de quem está no projeto.
func (h *Handler) SetAccounts(accounts *personaccount.Store) { h.accounts = accounts }

// projectMember é a pessoa do projeto com as plataformas em que ela tem conta ligada ("github", "gitlab").
type projectMember struct {
	Person
	Accounts []string `json:"accounts"`
}

func NewHandler(svc *Service, membershipSvc *MembershipService) *Handler {
	return &Handler{svc: svc, membershipSvc: membershipSvc}
}

func (h *Handler) Create(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	team, err := h.svc.Create(projectID, body.Name)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(201, team)
}

func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	teams, err := h.svc.ListByProject(projectID)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, teams)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("teamId")
	team, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, team)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("teamId")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	team, err := h.svc.Update(id, body.Name)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, team)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("teamId")
	if err := h.svc.Delete(id); err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.NoContent(204)
}

func (h *Handler) AddMember(c *echo.Context) error {
	teamID := c.Param("teamId")
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	if body.PersonID == "" {
		return apperr.Respond(c, 400, ErrPersonRequired.With("field", "person_id"))
	}
	membership, err := h.membershipSvc.Add(teamID, body.PersonID)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(201, membership)
}

func (h *Handler) RemoveMember(c *echo.Context) error {
	teamID := c.Param("teamId")
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	if body.PersonID == "" {
		return apperr.Respond(c, 400, ErrPersonRequired.With("field", "person_id"))
	}
	if err := h.membershipSvc.Remove(teamID, body.PersonID); err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.NoContent(204)
}

func (h *Handler) ListMembers(c *echo.Context) error {
	teamID := c.Param("teamId")
	members, err := h.membershipSvc.ListByTeam(teamID)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, members)
}

// ListProjectMembers lista as pessoas que estão no projeto (têm valor por hora
// nele): são elas que podem ser responsáveis por tarefas.
func (h *Handler) ListProjectMembers(c *echo.Context) error {
	persons, err := h.membershipSvc.ListPersonsInProject(c.Param("projectId"))
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	linked := map[uuid.UUID][]string{}
	if h.accounts != nil {
		ids := make([]uuid.UUID, len(persons))
		for i, p := range persons {
			ids[i] = p.ID
		}
		if linked, err = h.accounts.LinkedPlatforms(ids); err != nil {
			return apperr.Respond(c, 500, err)
		}
	}
	out := make([]projectMember, len(persons))
	for i, p := range persons {
		out[i] = projectMember{Person: p, Accounts: linked[p.ID]}
		if out[i].Accounts == nil {
			out[i].Accounts = []string{}
		}
	}
	return c.JSON(200, out)
}
