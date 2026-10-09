package overview

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func fail(c *echo.Context, err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return apperr.Respond(c, http.StatusNotFound, project.ErrNotFound)
	}
	return apperr.Respond(c, http.StatusInternalServerError, err)
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

// Organization devolve a visão geral da organização de quem pede. A rota é só de admins: é o
// dinheiro de todos os projetos e o tempo de cada pessoa.
func (h *Handler) Organization(c *echo.Context) error {
	o, err := h.svc.Organization(c.Param("orgId"), auth.CurrentPerson(c).PersonID.String())
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return apperr.Respond(c, http.StatusNotFound, organization.ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, o)
}

// WorkingNow devolve quem está com o ponto aberto na organização e em que tarefas, sem tempo
// nem dinheiro. É só de admins, como a visão geral: mostra o que cada pessoa faz agora.
func (h *Handler) WorkingNow(c *echo.Context) error {
	list, err := h.svc.WorkingNow(c.Param("orgId"))
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return apperr.Respond(c, http.StatusNotFound, organization.ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, list)
}

// Mine devolve o painel de quem pede: as horas dele hoje e na semana, no fuso do ?tz=, e as tarefas
// dele. Qualquer pessoa da organização lê o seu, e só o seu.
func (h *Handler) Mine(c *echo.Context) error {
	me := auth.CurrentPerson(c)
	m, err := h.svc.Mine(c.Param("orgId"), me.PersonID.String(), c.QueryParam("tz"), !me.IsAdmin())
	if err != nil {
		return failOrg(c, err)
	}
	return c.JSON(http.StatusOK, m)
}

// MyTasks devolve uma página das tarefas de quem pede em todos os projetos: ?state=open (padrão) ou
// closed, ?page= e ?per_page=.
func (h *Handler) MyTasks(c *echo.Context) error {
	state := c.QueryParam("state")
	switch state {
	case "":
		state = StateOpen
	case StateOpen, StateClosed:
	default:
		return apperr.Respond(c, http.StatusBadRequest, ErrInvalidTaskState)
	}
	page, perPage, err := pageParams(c)
	if err != nil {
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	me := auth.CurrentPerson(c)
	list, err := h.svc.MyTasks(c.Param("orgId"), me.PersonID.String(), state, !me.IsAdmin(), page, perPage)
	if err != nil {
		return failOrg(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

// pageParams lê page e per_page; zero é "não veio". Um valor que não é um número a partir de 1 é
// recusado, com os mesmos códigos da lista de projetos.
func pageParams(c *echo.Context) (page, perPage int, err error) {
	if v := c.QueryParam("page"); v != "" {
		if page, err = strconv.Atoi(v); err != nil || page < 1 {
			return 0, 0, project.ErrInvalidPage
		}
	}
	if v := c.QueryParam("per_page"); v != "" {
		if perPage, err = strconv.Atoi(v); err != nil || perPage < 1 {
			return 0, 0, project.ErrInvalidPerPage
		}
	}
	return page, perPage, nil
}

// failOrg responde o erro de uma rota da organização: um id que não existe é "organização não encontrada".
func failOrg(c *echo.Context, err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return apperr.Respond(c, http.StatusNotFound, organization.ErrNotFound)
	}
	return apperr.Respond(c, http.StatusInternalServerError, err)
}
