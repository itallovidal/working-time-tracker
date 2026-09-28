package work_session

import (
	"github.com/labstack/echo/v5"

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
func personFor(c *echo.Context, requested string) (string, int, string) {
	me := auth.CurrentPerson(c)
	if me == nil {
		if requested == "" {
			return "", 400, "person_id is required"
		}
		return requested, 0, ""
	}
	if requested == "" || requested == me.PersonID.String() {
		return me.PersonID.String(), 0, ""
	}
	if !me.IsAdmin() {
		return "", 403, "só admins podem registrar o ponto de outra pessoa"
	}
	return requested, 0, ""
}

func (h *Handler) ClockIn(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		TaskID   string `json:"task_id"`
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	if body.TaskID == "" {
		return c.JSON(400, map[string]string{"error": "task_id is required"})
	}
	personID, status, msg := personFor(c, body.PersonID)
	if status != 0 {
		return c.JSON(status, map[string]string{"error": msg})
	}
	session, err := h.svc.ClockIn(projectID, body.TaskID, personID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, session)
}

func (h *Handler) ClockOut(c *echo.Context) error {
	var body struct {
		PersonID string `json:"person_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request body"})
	}
	personID, status, msg := personFor(c, body.PersonID)
	if status != 0 {
		return c.JSON(status, map[string]string{"error": msg})
	}
	session, err := h.svc.ClockOut(personID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, session)
}

func (h *Handler) List(c *echo.Context) error {
	projectID := c.Param("projectId")
	taskID := c.QueryParam("task_id")
	personID := c.QueryParam("person_id")

	var taskIDPtr, personIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	if personID != "" {
		personIDPtr = &personID
	}

	sessions, err := h.svc.ListByProject(projectID, taskIDPtr, personIDPtr)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, sessions)
}

func (h *Handler) Total(c *echo.Context) error {
	projectID := c.Param("projectId")
	taskID := c.QueryParam("task_id")
	personID := c.QueryParam("person_id")

	var taskIDPtr, personIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	if personID != "" {
		personIDPtr = &personID
	}

	total, err := h.svc.TotalTime(projectID, taskIDPtr, personIDPtr)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, total)
}

// Active devolve a sessão aberta da pessoa logada, ou null.
func (h *Handler) Active(c *echo.Context) error {
	me := auth.CurrentPerson(c)
	if me == nil {
		return c.JSON(401, map[string]string{"error": auth.ErrUnauthenticated.Error()})
	}
	session, err := h.svc.Active(me.PersonID.String())
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, session)
}
