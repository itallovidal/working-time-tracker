package task

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(c *echo.Context) error {
	projectID := c.Param("projectId")
	var body struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		AssigneeID  string     `json:"assignee_id"`
		Deadline    *time.Time `json:"deadline"`
		Priority    *string    `json:"priority"`
		LabelIDs    *[]string  `json:"label_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	task, err := h.svc.CreateAs(selfID(c), projectID, body.Name, body.Description, body.AssigneeID, body.Deadline,
		Attrs{Priority: body.Priority, LabelIDs: body.LabelIDs})
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(201, task)
}

// maxQueryLen limita o texto da busca por nome.
const maxQueryLen = 100

// parseListFilter lê os filtros da lista na query string: q, assignee_id,
// deadline_to, priority, status, label_id, page e per_page. Todos são opcionais.
// assignee_id=none lista só as tarefas sem responsável; priority, status e label_id
// aceitam vários valores separados por vírgula, e valem para a tarefa que tem
// qualquer um deles.
func parseListFilter(c *echo.Context) (ListFilter, error) {
	var f ListFilter

	f.Query = strings.TrimSpace(c.QueryParam("q"))
	if utf8.RuneCountInString(f.Query) > maxQueryLen {
		return f, ErrQueryTooLong.With("max", maxQueryLen)
	}
	if v := c.QueryParam("assignee_id"); v == "none" {
		f.Unassigned = true
	} else if v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, ErrInvalidAssigneeFilter
		}
		f.AssigneeID = &id
	}
	if v := c.QueryParam("deadline_to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, ErrInvalidDeadlineFilter
		}
		f.DeadlineTo = &t
	}
	if v := c.QueryParam("priority"); v != "" {
		for _, p := range strings.Split(v, ",") {
			if !validPriority(p) {
				return f, ErrInvalidPriorityFilter
			}
			f.Priorities = append(f.Priorities, p)
		}
	}
	if v := c.QueryParam("status"); v != "" {
		for _, st := range strings.Split(v, ",") {
			if !validStatus(st) {
				return f, ErrInvalidStatusFilter
			}
			f.Statuses = append(f.Statuses, st)
		}
	}
	if v := c.QueryParam("label_id"); v != "" {
		for _, l := range strings.Split(v, ",") {
			id, err := uuid.Parse(l)
			if err != nil {
				return f, ErrInvalidLabelFilter
			}
			f.LabelIDs = append(f.LabelIDs, id)
		}
	}
	if v := c.QueryParam("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, ErrInvalidPage
		}
		f.Page = n
	}
	if v := c.QueryParam("per_page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, ErrInvalidPerPage
		}
		f.PerPage = n
	}
	return f, nil
}

// ListByProject lista as tarefas do projeto. Sem page, devolve todas num
// array; com page, uma página com o total (per_page só vale junto de page).
func (h *Handler) ListByProject(c *echo.Context) error {
	projectID := c.Param("projectId")
	f, err := parseListFilter(c)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	if f.Page == 0 {
		tasks, err := h.svc.ListByProject(projectID, f)
		if err != nil {
			return apperr.Respond(c, 500, err)
		}
		return c.JSON(200, tasks)
	}
	page, err := h.svc.ListPage(projectID, f)
	if err != nil {
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, page)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("taskId")
	task, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 500, err)
	}
	return c.JSON(200, task)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("taskId")
	var body struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		AssigneeID  *string    `json:"assignee_id"`
		Deadline    *time.Time `json:"deadline"`
		Priority    *string    `json:"priority"`
		Status      *string    `json:"status"`
		LabelIDs    *[]string  `json:"label_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	task, err := h.svc.UpdateAs(selfID(c), id, body.Name, body.Description, body.AssigneeID, body.Deadline,
		Attrs{Priority: body.Priority, Status: body.Status, LabelIDs: body.LabelIDs})
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("taskId")
	if err := h.svc.Delete(id); err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.NoContent(204)
}

func (h *Handler) LinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	var body struct {
		IntegrationID   string `json:"integration_id"`
		ExternalItemID  string `json:"external_item_id"`
		ExternalItemURL string `json:"external_item_url"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	if body.IntegrationID == "" || body.ExternalItemID == "" || body.ExternalItemURL == "" {
		return apperr.Respond(c, 400, ErrLinkFieldsRequired)
	}
	task, err := h.svc.LinkExternalItem(taskID, body.IntegrationID, body.ExternalItemID, body.ExternalItemURL)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

func (h *Handler) UnlinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	task, err := h.svc.UnlinkExternalItem(taskID)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

func (h *Handler) GetExternalDetails(c *echo.Context) error {
	taskID := c.Param("taskId")
	result, err := h.svc.GetExternalDetails(taskID)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, 404, ErrNotFound)
		}
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, result)
}

// selfID é o id de quem está logado, ou vazio sem sessão.
func selfID(c *echo.Context) string {
	if me := auth.CurrentPerson(c); me != nil {
		return me.PersonID.String()
	}
	return ""
}

// labelFail responde ao erro de uma rota de etiqueta: um projeto ou uma etiqueta
// que não existe é 404; os erros de validação, 400.
func labelFail(c *echo.Context, err error) error {
	if err == database.ErrNotFound {
		return apperr.Respond(c, 404, ErrLabelNotFound)
	}
	if _, ok := err.(*apperr.Error); ok {
		return apperr.Respond(c, 400, err)
	}
	return apperr.Respond(c, 500, err)
}

// ListLabels lista as etiquetas do projeto. Todos do projeto leem.
func (h *Handler) ListLabels(c *echo.Context) error {
	labels, err := h.svc.ListLabels(c.Param("projectId"))
	if err != nil {
		return labelFail(c, err)
	}
	return c.JSON(200, labels)
}

// CreateLabel cria uma etiqueta no projeto. A rota é só de admins.
func (h *Handler) CreateLabel(c *echo.Context) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	label, err := h.svc.CreateLabel(c.Param("projectId"), body.Name)
	if err != nil {
		return labelFail(c, err)
	}
	return c.JSON(201, label)
}

// RenameLabel troca o nome de uma etiqueta. A rota é só de admins.
func (h *Handler) RenameLabel(c *echo.Context) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.ErrInvalidBody)
	}
	label, err := h.svc.RenameLabel(c.Param("projectId"), c.Param("labelId"), body.Name)
	if err != nil {
		return labelFail(c, err)
	}
	return c.JSON(200, label)
}

// DeleteLabel exclui uma etiqueta; as tarefas só a perdem. A rota é só de admins.
func (h *Handler) DeleteLabel(c *echo.Context) error {
	if err := h.svc.DeleteLabel(c.Param("projectId"), c.Param("labelId")); err != nil {
		return labelFail(c, err)
	}
	return c.NoContent(204)
}
