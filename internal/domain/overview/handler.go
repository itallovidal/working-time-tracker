package overview

import (
	"errors"
	"net/http"

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
