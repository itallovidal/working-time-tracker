package person

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
)

type Handler struct {
	svc   *Service
	scope func(c *echo.Context) (*uuid.UUID, error)
	// seesPayment diz se quem pede pode ver a regra de pagamento da pessoa; nil não esconde nada.
	seesPayment func(c *echo.Context, personID uuid.UUID) bool
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetScope liga a regra de quem vê quem: dado o pedido, devolve nil quando quem pede vê todas as pessoas da
// organização, ou o id dele quando só vê a si mesmo e quem divide projeto com ele. Quem monta é o servidor, que
// conhece a sessão e os projetos; sem isso todo mundo vê todo mundo.
func (h *Handler) SetScope(scope func(c *echo.Context) (*uuid.UUID, error)) {
	h.scope = scope
}

// SetPaymentGuard liga a regra de quem vê a regra de pagamento dos outros: a própria pessoa, admins e quem cuida de
// pessoas. Para os demais (colegas de projeto) a lista e o detalhe trazem payment: null.
func (h *Handler) SetPaymentGuard(f func(c *echo.Context, personID uuid.UUID) bool) {
	h.seesPayment = f
}

func (h *Handler) redactPayment(c *echo.Context, p *Person) {
	if h.seesPayment != nil && !h.seesPayment(c, p.ID) {
		p.Payment = nil
	}
}

// viewer devolve de quem a lista é vista, ou nil quando quem pede vê todas as pessoas.
func (h *Handler) viewer(c *echo.Context) (*uuid.UUID, error) {
	if h.scope == nil {
		return nil, nil
	}
	return h.scope(c)
}

// ListByOrg lista as pessoas da organização que quem pede pode ver: todas, para os admins e para quem cuida de
// pessoas (ou de colaboradores de algum projeto); para os outros, só eles mesmos e quem está em algum projeto deles.
func (h *Handler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	viewer, err := h.viewer(c)
	if err != nil {
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	persons, err := h.svc.ListVisible(orgID, viewer)
	if err != nil {
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	for i := range persons {
		h.redactPayment(c, &persons[i])
	}
	return c.JSON(http.StatusOK, persons)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("personId")
	viewer, err := h.viewer(c)
	if err != nil {
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	person, err := h.svc.GetVisible(id, viewer)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	h.redactPayment(c, person)
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.Update(id, body.Name, body.Email)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) SetRole(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.SetRole(id, body.Role)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}

// SetPermissions define as permissões da organização de quem não é admin. A rota é só
// do dono.
func (h *Handler) SetPermissions(c *echo.Context) error {
	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.SetPermissions(c.Param("personId"), body.Permissions)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}

// SetWeeklyHours define a jornada semanal da pessoa. A rota é só de admins: a
// jornada é o que a organização combinou com ela.
func (h *Handler) SetWeeklyHours(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		WeeklyHours *int `json:"weekly_hours"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.SetWeeklyHours(id, body.WeeklyHours)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}

// SetPayment define a regra de pagamento da pessoa (mensal, quinzenal ou nenhuma). Quem pode é quem pode
// mudar a jornada: a regra é o que a organização combinou com ela.
func (h *Handler) SetPayment(c *echo.Context) error {
	var body struct {
		Frequency string `json:"frequency"`
		Day       int    `json:"day"`
		Start     string `json:"start"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.SetPayment(c.Param("personId"), PaymentRule{Frequency: body.Frequency, Day: body.Day, Start: body.Start})
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}
