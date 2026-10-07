package work_session

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// personFor decide de quem é o ponto. Com login, o padrão é a pessoa logada, e só
// um admin pode bater o ponto de outra pessoa. Sem login no contexto (handler
// montado fora do servidor, como nos testes), person_id continua obrigatório.
func personFor(c *echo.Context, requested string) (string, int, error) {
	me := auth.CurrentPerson(c)
	if me == nil {
		if requested == "" {
			return "", 400, ErrPersonRequired
		}
		return requested, 0, nil
	}
	if requested == "" || requested == me.PersonID.String() {
		return me.PersonID.String(), 0, nil
	}
	if !me.IsAdmin() {
		return "", 403, ErrOtherPersonAdminOnly
	}
	return requested, 0, nil
}

// visiblePerson decide de quem são as sessões que a lista e o total mostram. Um
// admin vê as de quem pedir ou, sem filtro, as de todos; quem não é admin só vê
// as próprias: sem person_id vale o dele, e o de outra pessoa é recusado. Sem
// login no contexto (handler montado fora do servidor, como nos testes), o
// filtro fica como veio.
func visiblePerson(c *echo.Context, requested string) (string, int, error) {
	if me := auth.CurrentPerson(c); me != nil && !me.IsAdmin() {
		return personFor(c, requested)
	}
	return requested, 0, nil
}

// redact apaga da sessão os valores que quem chama não pode ver. O valor cobrado
// do cliente é só de admins; o valor pago é dos admins e da própria pessoa. Sem
// login no contexto, nada é mostrado.
func redact(me *auth.Identity, s *WorkSession) {
	if s == nil || me.IsAdmin() {
		return
	}
	s.BillRateCents, s.BillAmountCents = nil, nil
	if me == nil || s.PersonID != me.PersonID {
		s.PayRateCents, s.PayAmountCents = nil, nil
	}
}

func (h *Handler) ClockIn(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		TaskID   string `json:"task_id"`
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	if body.TaskID == "" {
		return apperr.Respond(c, 400, ErrTaskRequired)
	}
	personID, status, perr := personFor(c, body.PersonID)
	if status != 0 {
		return apperr.Respond(c, status, perr)
	}
	session, err := h.svc.ClockIn(projectID, body.TaskID, personID)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	redact(auth.CurrentPerson(c), session)
	return c.JSON(201, session)
}

func (h *Handler) ClockOut(c *echo.Context) error {
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	personID, status, perr := personFor(c, body.PersonID)
	if status != 0 {
		return apperr.Respond(c, status, perr)
	}
	session, err := h.svc.ClockOut(c.Param("projectId"), personID)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	redact(auth.CurrentPerson(c), session)
	return c.JSON(200, session)
}

func (h *Handler) List(c *echo.Context) error {
	projectID := c.Param("projectId")
	taskID := c.QueryParam("task_id")
	personID, status, perr := visiblePerson(c, c.QueryParam("person_id"))
	if status != 0 {
		return apperr.Respond(c, status, perr)
	}

	var taskIDPtr, personIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	if personID != "" {
		personIDPtr = &personID
	}

	sessions, err := h.svc.ListByProject(projectID, taskIDPtr, personIDPtr)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	me := auth.CurrentPerson(c)
	for i := range sessions {
		redact(me, &sessions[i])
	}
	return c.JSON(200, sessions)
}

func (h *Handler) Total(c *echo.Context) error {
	projectID := c.Param("projectId")
	taskID := c.QueryParam("task_id")
	personID, status, perr := visiblePerson(c, c.QueryParam("person_id"))
	if status != 0 {
		return apperr.Respond(c, status, perr)
	}

	var taskIDPtr, personIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	if personID != "" {
		personIDPtr = &personID
	}

	total, err := h.svc.TotalTime(projectID, taskIDPtr, personIDPtr)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	// Os mesmos limites das sessões: um membro só vê quanto ganhou quando o
	// filtro é ele mesmo, e nunca o valor cobrado.
	if me := auth.CurrentPerson(c); !me.IsAdmin() {
		total.BillAmountCents = nil
		if me == nil || personID != me.PersonID.String() {
			total.PayAmountCents = nil
		}
	}
	return c.JSON(200, total)
}

// Active devolve a sessão aberta da pessoa logada, ou null.
func (h *Handler) Active(c *echo.Context) error {
	me := auth.CurrentPerson(c)
	if me == nil {
		return apperr.Respond(c, 401, auth.ErrUnauthenticated)
	}
	session, err := h.svc.Active(me.PersonID.String())
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	redact(me, session)
	return c.JSON(200, session)
}
