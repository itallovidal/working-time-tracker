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
	return c.Redirect(http.StatusSeeOther, "/projects/"+c.Param("projectId")+"/teams")
}

func (h *Handler) Teams(c *echo.Context) error {
	return h.projectPage(c, "project_teams", "Times", "teams")
}

func (h *Handler) ProjectSettings(c *echo.Context) error {
	return h.projectPage(c, "project_settings", "Configurações", "settings")
}
