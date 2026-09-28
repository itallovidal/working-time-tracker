package page

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// projectPage monta a página de uma aba do projeto. O middleware de rota já
// garantiu que o projeto existe e é da organização de quem está logado.
func (h *Handler) projectPage(c *echo.Context, name, title, tab string) error {
	p, err := h.deps.Projects.Get(c.Param("projectId"))
	if err != nil {
		return h.NotFound(c)
	}
	return h.render(c, name, Data{
		Title:   title + " · " + p.Name,
		Section: "projects",
		Project: &Crumb{ID: p.ID.String(), Name: p.Name},
		Tab:     tab,
		Script:  "project",
	})
}

// Project leva para a aba principal do projeto.
func (h *Handler) Project(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/projects/"+c.Param("projectId")+"/tasks")
}

// Tasks é a lista de tarefas do projeto (S9.1).
func (h *Handler) Tasks(c *echo.Context) error {
	return h.projectPage(c, "project_tasks", "Tarefas", "tasks")
}

// TimeTracking é o ponto do projeto: clock-in/out, sessões e totais (S9.3, S9.4).
func (h *Handler) TimeTracking(c *echo.Context) error {
	return h.projectPage(c, "project_time", "Ponto", "time")
}

// TaskDetail edita uma tarefa e o vínculo com o item externo (S9.2).
func (h *Handler) TaskDetail(c *echo.Context) error {
	t, err := h.deps.Tasks.Get(c.Param("taskId"))
	if err != nil {
		return h.NotFound(c)
	}
	p, err := h.deps.Projects.Get(t.ProjectID.String())
	if err != nil {
		return h.NotFound(c)
	}
	return h.render(c, "task_detail", Data{
		Title:   t.Name + " · " + p.Name,
		Section: "projects",
		Project: &Crumb{ID: p.ID.String(), Name: p.Name},
		Tab:     "tasks",
		Script:  "project",
		Props:   map[string]any{"task_id": t.ID.String()},
	})
}

func (h *Handler) Teams(c *echo.Context) error {
	return h.projectPage(c, "project_teams", "Times", "teams")
}

func (h *Handler) ProjectSettings(c *echo.Context) error {
	return h.projectPage(c, "project_settings", "Configurações", "settings")
}
