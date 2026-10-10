package work_session

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/permission"
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
			return "", 400, ErrPersonRequired.With("field", "person_id")
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

// visiblePerson decide de quem são as sessões que a lista e o total mostram. Quem tem
// alguma permissão no projeto (admins, e quem cuida dele) vê as de quem pedir ou, sem
// filtro, as de todos; os outros só veem as próprias: sem person_id vale o dele, e o de
// outra pessoa é recusado. Sem login no contexto (handler montado fora do servidor, como
// nos testes), o filtro fica como veio.
func visiblePerson(c *echo.Context, requested string) (string, int, error) {
	if me := auth.CurrentPerson(c); me != nil && !auth.ProjectPermissions(c).Manages() {
		return personFor(c, requested)
	}
	return requested, 0, nil
}

// redact apaga da sessão os valores que quem chama não pode ver. O valor cobrado do
// cliente é de quem vê o faturamento do projeto; o valor pago é de quem vê o valor dos
// outros e da própria pessoa. Os admins veem tudo. Sem login no contexto, nada é mostrado.
func redact(set permission.Set, me *auth.Identity, s *WorkSession) {
	if s == nil {
		return
	}
	if !set.Has(permission.BillingView) {
		s.BillRateCents, s.BillAmountCents = nil, nil
		for i := range s.Tasks {
			s.Tasks[i].BillAmountCents = nil
		}
	}
	if !set.HasAny(permission.RatesView, permission.RatesManage) && (me == nil || s.PersonID != me.PersonID) {
		s.PayRateCents, s.PayAmountCents = nil, nil
		for i := range s.Tasks {
			s.Tasks[i].PayAmountCents = nil
		}
	}
}

func (h *Handler) ClockIn(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		TaskID   string `json:"task_id"`
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	if body.TaskID == "" {
		return apperr.Respond(c, 400, ErrTaskRequired.With("field", "task_id"))
	}
	personID, status, perr := personFor(c, body.PersonID)
	if status != 0 {
		return apperr.Respond(c, status, perr)
	}
	session, err := h.svc.ClockIn(projectID, body.TaskID, personID)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	redact(auth.ProjectPermissions(c), auth.CurrentPerson(c), session)
	return c.JSON(201, session)
}

func (h *Handler) ClockOut(c *echo.Context) error {
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	personID, status, perr := personFor(c, body.PersonID)
	if status != 0 {
		return apperr.Respond(c, status, perr)
	}
	session, err := h.svc.ClockOut(c.Param("projectId"), personID)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	redact(auth.ProjectPermissions(c), auth.CurrentPerson(c), session)
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
		// Um filtro com id malformado é do pedido (400), como no total; o resto é falha nossa.
		var coded *apperr.Error
		if errors.As(err, &coded) {
			return apperr.Respond(c, 400, err)
		}
		return apperr.Respond(c, 500, err)
	}
	me, set := auth.CurrentPerson(c), auth.ProjectPermissions(c)
	for i := range sessions {
		redact(set, me, &sessions[i])
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
	// Os mesmos limites das sessões: quem não vê o valor dos outros só vê quanto ganhou
	// quando o filtro é ele mesmo, e o valor cobrado só vai para quem vê o faturamento.
	me, set := auth.CurrentPerson(c), auth.ProjectPermissions(c)
	if !set.Has(permission.BillingView) {
		total.BillAmountCents = nil
	}
	if !set.HasAny(permission.RatesView, permission.RatesManage) && (me == nil || personID != me.PersonID.String()) {
		total.PayAmountCents = nil
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
	redact(auth.ProjectPermissions(c), me, session)
	return c.JSON(200, session)
}

// optTime é um horário que pode vir ausente, nulo ou preenchido no corpo: o nulo de until_at
// quer dizer "até o fim da sessão", e não "não mexa".
type optTime struct {
	Set   bool
	Value *time.Time
}

func (o *optTime) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(b, &t); err != nil {
		// Um UnmarshalTypeError recebe o nome do campo do decodificador, e o servidor o devolve em invalid_body.
		return &json.UnmarshalTypeError{Value: "string", Type: reflect.TypeOf(t)}
	}
	o.Value = &t
	return nil
}

// respond responde um erro do serviço: a sessão ou o intervalo que não existem são 404, quem
// não pode mexer na sessão é 403, o resto é 400.
func respond(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrTaskLinkNotFound):
		return apperr.Respond(c, 404, err)
	case errors.Is(err, ErrSessionNotYours):
		return apperr.Respond(c, 403, err)
	}
	return apperr.Respond(c, 400, err)
}

// editable confere que a sessão da rota existe e que quem chama pode mexer nas tarefas dela: a
// própria pessoa e os admins. Mudar as tarefas não muda o tempo nem os valores da sessão,
// só como o tempo se divide entre elas, então não há permissão própria. Sem login no
// contexto (handler montado fora do servidor, como nos testes), vale para qualquer um.
// Devolve o erro sem responder: quem chama o passa a respond e para.
func (h *Handler) editable(c *echo.Context) error {
	session, err := h.svc.Get(c.Param("projectId"), c.Param("sessionId"))
	if err != nil {
		return err
	}
	if me := auth.CurrentPerson(c); me != nil && !me.IsAdmin() && session.PersonID != me.PersonID {
		return ErrSessionNotYours
	}
	return nil
}

// AddTask põe uma tarefa na sessão, aberta ou encerrada.
func (h *Handler) AddTask(c *echo.Context) error {
	var body struct {
		TaskID  string  `json:"task_id"`
		FromAt  optTime `json:"from_at"`
		UntilAt optTime `json:"until_at"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	if body.TaskID == "" {
		return apperr.Respond(c, 400, ErrTaskRequired.With("field", "task_id"))
	}
	if err := h.editable(c); err != nil {
		return respond(c, err)
	}
	session, err := h.svc.AddTask(c.Param("projectId"), c.Param("sessionId"), body.TaskID, body.FromAt.Value, body.UntilAt.Value)
	if err != nil {
		return respond(c, err)
	}
	redact(auth.ProjectPermissions(c), auth.CurrentPerson(c), session)
	return c.JSON(201, session)
}

// UpdateTask muda o intervalo de uma tarefa da sessão, ou o encerra agora com stop.
func (h *Handler) UpdateTask(c *echo.Context) error {
	var body struct {
		FromAt  optTime `json:"from_at"`
		UntilAt optTime `json:"until_at"`
		Stop    bool    `json:"stop"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	if err := h.editable(c); err != nil {
		return respond(c, err)
	}
	change := TaskChange{
		From:       body.FromAt.Value,
		Until:      body.UntilAt.Value,
		ClearUntil: body.UntilAt.Set && body.UntilAt.Value == nil,
		Stop:       body.Stop,
	}
	session, err := h.svc.UpdateTask(c.Param("projectId"), c.Param("sessionId"), c.Param("linkId"), change)
	if err != nil {
		return respond(c, err)
	}
	redact(auth.ProjectPermissions(c), auth.CurrentPerson(c), session)
	return c.JSON(200, session)
}

// RemoveTask tira uma tarefa da sessão e devolve a sessão como ficou.
func (h *Handler) RemoveTask(c *echo.Context) error {
	if err := h.editable(c); err != nil {
		return respond(c, err)
	}
	session, err := h.svc.RemoveTask(c.Param("projectId"), c.Param("sessionId"), c.Param("linkId"))
	if err != nil {
		return respond(c, err)
	}
	redact(auth.ProjectPermissions(c), auth.CurrentPerson(c), session)
	return c.JSON(200, session)
}
