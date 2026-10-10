package task

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/validate"
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
		// SkipPublish são as integrações em que a tarefa nova não deve ser postada sozinha.
		SkipPublish []string `json:"skip_publish"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	skip, err := parseIntegrationIDs(body.SkipPublish)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	task, err := h.svc.CreateAs(selfID(c), projectID, body.Name, body.Description, body.AssigneeID, body.Deadline,
		Attrs{Priority: body.Priority, LabelIDs: body.LabelIDs, SkipPublish: skip})
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(201, task)
}

// maxIntegrationIDs é quantas integrações um pedido pode listar (as de skip_publish na criação, as de remove_in
// na exclusão).
const maxIntegrationIDs = 20

// parseIntegrationIDs lê uma lista de ids de integração. Um id que não é um UUID não é de nenhuma
// integração; um id que não existe, ou é de outro projeto, simplesmente não casa com nada.
func parseIntegrationIDs(raw []string) ([]uuid.UUID, error) {
	if len(raw) > maxIntegrationIDs {
		return nil, ErrIntegrationNotFound
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := uuid.Parse(r)
		if err != nil {
			return nil, ErrIntegrationNotFound
		}
		out = append(out, id)
	}
	return out, nil
}

// parseListFilter lê os filtros da lista na query string: q, assignee_id,
// deadline_to, priority, status, label_id, page e per_page. Todos são opcionais.
// assignee_id=none lista só as tarefas sem responsável, assignee_id=any, só as que têm, e
// assignee_id=others, só as que têm responsável e não é quem pede; priority, status e label_id
// aceitam vários valores separados por vírgula, e valem para a tarefa que tem
// qualquer um deles.
func parseListFilter(c *echo.Context) (ListFilter, error) {
	var f ListFilter

	f.Query = strings.TrimSpace(c.QueryParam("q"))
	if utf8.RuneCountInString(f.Query) > validate.MaxSearch {
		return f, ErrQueryTooLong.With("max", validate.MaxSearch, "field", "q")
	}
	if v := c.QueryParam("assignee_id"); v == "none" {
		f.Unassigned = true
	} else if v == "any" {
		f.Assigned = true
	} else if v == "others" {
		me, err := uuid.Parse(selfID(c))
		if err != nil {
			return f, ErrInvalidAssigneeFilter.With("field", "assignee_id")
		}
		f.OthersOf = &me
	} else if v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, ErrInvalidAssigneeFilter.With("field", "assignee_id")
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
				return f, ErrInvalidPriorityFilter.With("field", "priority")
			}
			f.Priorities = append(f.Priorities, p)
		}
	}
	if v := c.QueryParam("status"); v != "" {
		for _, st := range strings.Split(v, ",") {
			if !validStatus(st) {
				return f, ErrInvalidStatusFilter.With("field", "status")
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
	// A descrição omitida mantém a que a tarefa tem (só "" a esvazia), e o prazo distingue ausente (mantém) de
	// null (apaga).
	var body struct {
		Name        string                       `json:"name"`
		Description *string                      `json:"description"`
		AssigneeID  *string                      `json:"assignee_id"`
		Deadline    validate.Optional[time.Time] `json:"deadline"`
		Priority    *string                      `json:"priority"`
		Status      *string                      `json:"status"`
		LabelIDs    *[]string                    `json:"label_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	task, err := h.svc.PatchAs(selfID(c), id, body.Name, body.Description, body.AssigneeID, body.Deadline,
		Attrs{Priority: body.Priority, Status: body.Status, LabelIDs: body.LabelIDs})
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

// Claim é "pegar a tarefa": passa uma tarefa sem responsável para quem está logado, sem bater o ponto.
// Uma tarefa que já é de outra pessoa responde 409.
func (h *Handler) Claim(c *echo.Context) error {
	task, err := h.svc.Claim(selfID(c), c.Param("taskId"))
	if err != nil {
		if errors.Is(err, ErrAlreadyAssigned) {
			return apperr.Respond(c, http.StatusConflict, err)
		}
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

// UpdateAttrs é a edição dos detalhes da tarefa: prioridade, status, etiquetas, responsável e prazo, cada um
// opcional. O responsável vazio tira o atual, e o prazo null o apaga (ausente o mantém).
func (h *Handler) UpdateAttrs(c *echo.Context) error {
	var body struct {
		Priority   *string                      `json:"priority"`
		Status     *string                      `json:"status"`
		LabelIDs   *[]string                    `json:"label_ids"`
		AssigneeID *string                      `json:"assignee_id"`
		Deadline   validate.Optional[time.Time] `json:"deadline"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	task, err := h.svc.PatchAttrsAs(selfID(c), c.Param("taskId"),
		Attrs{Priority: body.Priority, Status: body.Status, LabelIDs: body.LabelIDs}, body.AssigneeID, body.Deadline)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

// Delete exclui a tarefa. remove_in, repetido na query, lista as integrações em que o item dela também deve sair
// (a issue do GitHub, o cartão do Trello); a resposta traz, para cada uma, o que se fez, e o que falhou lá não
// desfaz a exclusão.
func (h *Handler) Delete(c *echo.Context) error {
	removeIn, err := parseIntegrationIDs(c.QueryParams()["remove_in"])
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	remote, err := h.svc.Delete(c.Request().Context(), c.Param("taskId"), removeIn)
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, map[string]any{"remote": remote})
}

func (h *Handler) LinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	var body struct {
		IntegrationID   string `json:"integration_id"`
		ExternalItemID  string `json:"external_item_id"`
		ExternalItemURL string `json:"external_item_url"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	task, err := h.svc.LinkExternalItem(taskID, body.IntegrationID, body.ExternalItemID, body.ExternalItemURL)
	if err != nil {
		// Um item que já tem dono, ou uma tarefa que já tem item nesta integração, é conflito, não pedido ruim.
		if errors.Is(err, ErrItemTaken) || errors.Is(err, ErrAlreadyLinked) {
			return apperr.Respond(c, http.StatusConflict, err)
		}
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

// UnlinkExternalItem solta a tarefa do item da integração do query integration_id; sem ele, do único que tem.
func (h *Handler) UnlinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	task, err := h.svc.UnlinkExternalItem(taskID, c.QueryParam("integration_id"))
	if err != nil {
		return apperr.Respond(c, 400, err)
	}
	return c.JSON(200, task)
}

// GetExternalDetails lê o item da integração do query integration_id; sem ele, o único que a tarefa tem.
func (h *Handler) GetExternalDetails(c *echo.Context) error {
	taskID := c.Param("taskId")
	result, err := h.svc.GetExternalDetails(taskID, c.QueryParam("integration_id"))
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
// que não existe é 404; um nome que já existe, 409; os erros de validação, 400.
func labelFail(c *echo.Context, err error) error {
	if err == database.ErrNotFound {
		return apperr.Respond(c, 404, ErrLabelNotFound)
	}
	if errors.Is(err, ErrLabelNameTaken) {
		return apperr.Respond(c, http.StatusConflict, err)
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
		return apperr.Respond(c, 400, apperr.BindError(err))
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
		return apperr.Respond(c, 400, apperr.BindError(err))
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
