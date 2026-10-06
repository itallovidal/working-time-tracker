package page

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
)

// integrationTypes vai no window.BOOT das páginas que mostram integrações: o que
// cada tipo pede (campos do metadata, rótulos). A tela se desenha a partir disso,
// então um tipo novo no adapter aparece sem mexer no JavaScript.
func integrationTypes() map[string]any {
	return map[string]any{"integration_types": adapter.Descriptors()}
}

// projectPage monta a página de uma aba do projeto. O middleware de rota já
// garantiu que o projeto existe e é da organização de quem está logado.
func (h *Handler) projectPage(c *echo.Context, name, titleKey, tab string, props map[string]any) error {
	p, err := h.deps.Projects.Get(c.Param("projectId"))
	if err != nil {
		return h.NotFound(c)
	}
	return h.render(c, name, Data{
		TitleKey:    titleKey,
		TitleSuffix: p.Name,
		Section:     "projects",
		Project:     &Crumb{ID: p.ID.String(), Name: p.Name},
		Tab:         tab,
		Script:      "project",
		Props:       props,
	})
}

// Project leva para a aba principal do projeto: a Visão geral para admins, as
// Tarefas para os demais.
func (h *Handler) Project(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/projects/"+c.Param("projectId")+"/"+projectHome(c))
}

// Tasks é a lista de tarefas do projeto (S9.1).
func (h *Handler) Tasks(c *echo.Context) error {
	return h.projectPage(c, "project_tasks", "titles.tasks", "tasks", integrationTypes())
}

// TimeTracking é o ponto do projeto: clock-in/out, sessões e totais (S9.3, S9.4).
func (h *Handler) TimeTracking(c *echo.Context) error {
	return h.projectPage(c, "project_time", "titles.time", "time", nil)
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
		Props:   map[string]any{"task_id": t.ID.String(), "integration_types": adapter.Descriptors()},
	})
}

// Teams é a aba Colaboradores: quem está no projeto, os times e, para admins,
// quanto cada pessoa recebe por hora (S17). A rota segue /teams, de quando a
// aba só tinha os times.
func (h *Handler) Teams(c *echo.Context) error {
	return h.projectPage(c, "project_teams", "titles.teams", "teams", nil)
}

// Integrations configura as integrações do projeto com GitHub, GitLab e Trello (S10.1, S23).
func (h *Handler) Integrations(c *echo.Context) error {
	return h.projectPage(c, "project_integrations", "titles.integrations", "integrations", integrationTypes())
}

func (h *Handler) ProjectSettings(c *echo.Context) error {
	return h.projectPage(c, "project_settings", "titles.project_settings", "settings", nil)
}
