package task

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/database"
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
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "corpo da requisição inválido"})
	}
	task, err := h.svc.Create(projectID, body.Name, body.Description, body.AssigneeID, body.Deadline)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, task)
}

// maxQueryLen limita o texto da busca por nome.
const maxQueryLen = 100

// parseListFilter lê os filtros da lista na query string: q, assignee_id,
// deadline_to, page e per_page. Todos são opcionais.
func parseListFilter(c *echo.Context) (ListFilter, error) {
	var f ListFilter

	f.Query = strings.TrimSpace(c.QueryParam("q"))
	if utf8.RuneCountInString(f.Query) > maxQueryLen {
		return f, errors.New("a busca aceita até 100 caracteres (q)")
	}
	if v := c.QueryParam("assignee_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, errors.New("responsável inválido (assignee_id)")
		}
		f.AssigneeID = &id
	}
	if v := c.QueryParam("deadline_to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, errors.New("prazo inválido (deadline_to): use data e hora, como 2026-10-12T23:59:59Z")
		}
		f.DeadlineTo = &t
	}
	if v := c.QueryParam("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, errors.New("página inválida (page): use um número a partir de 1")
		}
		f.Page = n
	}
	if v := c.QueryParam("per_page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, errors.New("tamanho de página inválido (per_page): use um número a partir de 1")
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
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	if f.Page == 0 {
		tasks, err := h.svc.ListByProject(projectID, f)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, tasks)
	}
	page, err := h.svc.ListPage(projectID, f)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, page)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("taskId")
	task, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(404, map[string]string{"error": "tarefa não encontrada"})
		}
		return c.JSON(500, map[string]string{"error": err.Error()})
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
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(400, map[string]string{"error": "corpo da requisição inválido"})
	}
	task, err := h.svc.Update(id, body.Name, body.Description, body.AssigneeID, body.Deadline)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) Delete(c *echo.Context) error {
	id := c.Param("taskId")
	if err := h.svc.Delete(id); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
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
		return c.JSON(400, map[string]string{"error": "corpo da requisição inválido"})
	}
	if body.IntegrationID == "" || body.ExternalItemID == "" || body.ExternalItemURL == "" {
		return c.JSON(400, map[string]string{"error": "informe a integração, o item e o link"})
	}
	task, err := h.svc.LinkExternalItem(taskID, body.IntegrationID, body.ExternalItemID, body.ExternalItemURL)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) UnlinkExternalItem(c *echo.Context) error {
	taskID := c.Param("taskId")
	task, err := h.svc.UnlinkExternalItem(taskID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, task)
}

func (h *Handler) GetExternalDetails(c *echo.Context) error {
	taskID := c.Param("taskId")
	result, err := h.svc.GetExternalDetails(taskID)
	if err != nil {
		if err == database.ErrNotFound {
			return c.JSON(404, map[string]string{"error": "tarefa não encontrada"})
		}
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, result)
}
