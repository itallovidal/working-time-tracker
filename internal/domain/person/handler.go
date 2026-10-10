package person

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/validate"
)

// fail traduz um erro do domínio em resposta: pessoa que não existe é 404, e-mail já usado é 409 (conflito com
// outra conta), e o resto dos erros de validação é 400.
func fail(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
	case errors.Is(err, ErrEmailInUse):
		return apperr.Respond(c, http.StatusConflict, err)
	}
	return apperr.Respond(c, http.StatusBadRequest, err)
}

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
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	person, err := h.svc.Update(id, body.Name, body.Email)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) SetRole(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	person, err := h.svc.SetRole(id, body.Role)
	if err != nil {
		return fail(c, err)
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
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	person, err := h.svc.SetPermissions(c.Param("personId"), body.Permissions)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, person)
}

// SetWeeklyHours define a jornada semanal da pessoa. A rota é só de admins: a
// jornada é o que a organização combinou com ela.
func (h *Handler) SetWeeklyHours(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		WeeklyHours validate.Optional[int] `json:"weekly_hours"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	// Ausente não é "apagar": {} é um pedido sem nada, e só null (ou zero) apaga a jornada.
	if !body.WeeklyHours.Set {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrFieldRequired.With("field", "weekly_hours"))
	}
	person, err := h.svc.SetWeeklyHours(id, body.WeeklyHours.Value)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, person)
}

// SetPayment define a regra de pagamento da pessoa (mensal, quinzenal ou nenhuma). Quem pode é quem pode
// mudar a jornada: a regra é o que a organização combinou com ela.
func (h *Handler) SetPayment(c *echo.Context) error {
	var body struct {
		Frequency *string `json:"frequency"`
		Day       int     `json:"day"`
		Start     string  `json:"start"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	// {} não é "apagar a regra": quem apaga manda frequency vazio.
	if body.Frequency == nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrFieldRequired.With("field", "frequency"))
	}
	person, err := h.svc.SetPayment(c.Param("personId"), PaymentRule{Frequency: strings.TrimSpace(*body.Frequency), Day: body.Day, Start: body.Start})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, person)
}
