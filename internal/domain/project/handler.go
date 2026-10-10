package project

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
)

// OwnerEnroller põe uma pessoa num projeto como colaboradora. O dono da organização que
// cria um projeto já entra nele, para aparecer nas listas e poder ser responsável por tarefas.
type OwnerEnroller interface {
	Set(projectID, personID string, payRateCents int) (*allocation.Allocation, error)
}

type Handler struct {
	svc      *Service
	enroller OwnerEnroller
}

func NewHandler(svc *Service, enroller OwnerEnroller) *Handler {
	return &Handler{svc: svc, enroller: enroller}
}

func (h *Handler) Create(c *echo.Context) error {
	orgID := c.Param("orgId")
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		// SprintDurationDays ausente vale 14; um 0 enviado é recusado, igual na edição.
		SprintDurationDays *int `json:"sprint_duration_days"`
		// CustomerID é opcional: com ele o projeto já nasce com o cliente, e a reunião com o cliente vale.
		CustomerID string `json:"customer_id"`
		// BillRateCents é o valor cobrado por hora: obrigatório com customer_id e recusado sem ele.
		BillRateCents *int `json:"bill_rate_cents"`
		Routine
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	sprint := 0
	if body.SprintDurationDays != nil {
		if sprint = *body.SprintDurationDays; sprint == 0 {
			return apperr.Respond(c, 400, ErrInvalidSprint.With("field", "sprint_duration_days"))
		}
	}
	project, err := h.svc.CreateWithCustomer(orgID, CreateInput{
		Name: body.Name, Description: body.Description, SprintDurationDays: sprint,
		Routine: body.Routine, CustomerID: body.CustomerID, BillRateCents: body.BillRateCents,
	})
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	// O dono que cria um projeto já entra nele; o valor não importa, a alocação do dono sai sempre com zero. Quem
	// cria sem ser o dono é admin e vê todos os projetos, então não é matriculado.
	if me := auth.CurrentPerson(c); me != nil && me.IsOwner {
		if _, err := h.enroller.Set(project.ID.String(), me.PersonID.String(), 0); err != nil {
			return apperr.Respond(c, 500, err)
		}
	}
	return c.JSON(201, project)
}

// ListByOrg lista os projetos da organização. Sem page, devolve todos num array; com page,
// uma página com o total (per_page só vale junto de page). Os admins veem todos os projetos; quem
// não é admin vê só aqueles em que está (tem valor por hora ou está em algum time).
func (h *Handler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	page, perPage, err := pageParams(c)
	if err != nil {
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	var member *uuid.UUID
	if me := auth.CurrentPerson(c); !me.IsAdmin() {
		member = &me.PersonID
	}
	if page > 0 {
		out, err := h.svc.ListPage(orgID, member, page, perPage)
		if err != nil {
			return apperr.Respond(c, 500, err)
		}
		return c.JSON(200, out)
	}
	projects, err := h.svc.ListByOrg(orgID, member)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, projects)
}

// pageParams lê page e per_page; zero é "não veio". Um valor que não é um número a partir de 1
// é recusado, em vez de virar o padrão sem aviso.
func pageParams(c *echo.Context) (page, perPage int, err error) {
	if v := c.QueryParam("page"); v != "" {
		if page, err = strconv.Atoi(v); err != nil || page < 1 {
			return 0, 0, ErrInvalidPage
		}
	}
	if v := c.QueryParam("per_page"); v != "" {
		if perPage, err = strconv.Atoi(v); err != nil || perPage < 1 {
			return 0, 0, ErrInvalidPerPage
		}
	}
	return page, perPage, nil
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("projectId")
	project, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, project)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("projectId")
	var body struct {
		// Name, Description e SprintDurationDays ausentes mantêm o valor atual; Name "" é recusado e Description ""
		// apaga.
		Name               *string `json:"name"`
		Description        *string `json:"description"`
		SprintDurationDays *int    `json:"sprint_duration_days"`
		Routine
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	project, err := h.svc.Update(id, body.Name, body.Description, body.SprintDurationDays, body.Routine)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, project)
}

// GetBilling devolve o cliente e o valor cobrado do projeto. A rota é só de admins.
func (h *Handler) GetBilling(c *echo.Context) error {
	billing, err := h.svc.Billing(c.Param("projectId"))
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, billing)
}

// SetBilling substitui o cliente e o valor cobrado: o que vier null é apagado.
func (h *Handler) SetBilling(c *echo.Context) error {
	var body struct {
		CustomerID    *string `json:"customer_id"`
		BillRateCents *int    `json:"bill_rate_cents"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	billing, err := h.svc.SetBilling(c.Param("projectId"), body.CustomerID, body.BillRateCents)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, billing)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("projectId")
	if err := h.svc.Delete(id); err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.NoContent(204)
}
