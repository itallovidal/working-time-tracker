package page

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
)

// integrationTypes vai no window.BOOT das páginas que mostram integrações: o que
// cada tipo pede (campos do metadata, rótulos). A tela se desenha a partir disso,
// então um tipo novo no adapter aparece sem mexer no JavaScript. Os textos vêm do
// catálogo, no idioma da requisição: integration_types.<tipo>.
func (h *Handler) integrationTypes(c *echo.Context) map[string]any {
	lang := h.deps.I18n.Lang(c)
	types := adapter.Descriptors()
	for i := range types {
		h.localizeDescriptor(&types[i], lang)
	}
	return map[string]any{"integration_types": types}
}

// localizeDescriptor preenche os textos de um tipo de integração. A descrição, a dica
// do token e os rótulos são obrigatórios (um teste confere); o placeholder e a dica de
// um campo só existem em alguns.
func (h *Handler) localizeDescriptor(d *adapter.Descriptor, lang string) {
	cat := h.deps.I18n
	base := "integration_types." + d.Type + "."
	d.Description = cat.T(lang, base+"about")
	d.TokenHint = cat.T(lang, base+"token_hint")
	d.ItemLabel = cat.T(lang, base+"item_label")
	if cat.Has(lang, base+"item_placeholder") {
		d.ItemPlaceholder = cat.T(lang, base+"item_placeholder")
	}
	for i := range d.Metadata {
		f := &d.Metadata[i]
		fb := base + "fields." + f.Key + "."
		f.Label = cat.T(lang, fb+"label")
		if cat.Has(lang, fb+"placeholder") {
			f.Placeholder = cat.T(lang, fb+"placeholder")
		}
		if cat.Has(lang, fb+"hint") {
			f.Hint = cat.T(lang, fb+"hint")
		}
	}
}

// projectPage monta a página de uma aba do projeto. O middleware de rota já
// garantiu que o projeto existe e é da organização de quem está logado.
func (h *Handler) projectPage(c *echo.Context, name, titleKey, tab string, props map[string]any) error {
	return h.renderProject(c, false, name, titleKey, tab, props)
}

// managementPage monta uma página da área de Gestão, só de admins: a barra de
// abas troca as abas do dia a dia pelas da Gestão.
func (h *Handler) managementPage(c *echo.Context, name, titleKey, tab string, props map[string]any) error {
	return h.renderProject(c, true, name, titleKey, tab, props)
}

func (h *Handler) renderProject(c *echo.Context, management bool, name, titleKey, tab string, props map[string]any) error {
	p, err := h.deps.Projects.Get(c.Param("projectId"))
	if err != nil {
		return h.NotFound(c)
	}
	return h.render(c, name, Data{
		TitleKey:    titleKey,
		TitleSuffix: p.Name,
		Section:     "projects",
		Project:     &Crumb{ID: p.ID.String(), Name: p.Name},
		Management:  management,
		Tab:         tab,
		Script:      "project",
		Props:       props,
	})
}

// Project leva para a página principal do projeto, a Visão geral de quem olha.
func (h *Handler) Project(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/projects/"+c.Param("projectId")+"/"+projectHome())
}

// Tasks é a lista de tarefas do projeto (S9.1).
func (h *Handler) Tasks(c *echo.Context) error {
	return h.projectPage(c, "project_tasks", "titles.tasks", "tasks", h.integrationTypes(c))
}

// TimeTracking é o ponto do projeto: o relógio e as tarefas da pessoa (S9.3, S29).
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
	return h.managementPage(c, "project_teams", "titles.teams", "teams", nil)
}

// Collaborators é a aba Colaboradores de fora da Gestão: as pessoas e os times
// do projeto, só para ler, igual para admin e membro. O admin edita na Gestão.
func (h *Handler) Collaborators(c *echo.Context) error {
	return h.projectPage(c, "project_teams", "titles.collaborators", "teams", map[string]any{"readonly": true})
}

// Integrations configura as integrações do projeto com GitHub, GitLab e Trello (S10.1, S23).
func (h *Handler) Integrations(c *echo.Context) error {
	return h.managementPage(c, "project_integrations", "titles.integrations", "integrations", h.integrationTypes(c))
}

func (h *Handler) ProjectSettings(c *echo.Context) error {
	return h.managementPage(c, "project_settings", "titles.project_settings", "settings", nil)
}
