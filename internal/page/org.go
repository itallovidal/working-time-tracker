package page

import "github.com/labstack/echo/v5"

// Org é a página inicial da organização: a lista de projetos (S8.4).
func (h *Handler) Org(c *echo.Context) error {
	return h.render(c, "org_projects", Data{Title: "Projetos", Section: "projects", Script: "org"})
}

// People lista as pessoas da organização e, para admins, os convites (S8.3).
func (h *Handler) People(c *echo.Context) error {
	return h.render(c, "org_people", Data{Title: "Pessoas", Section: "people", Script: "org"})
}

// OrgSettings tem os dados da organização, o perfil e a senha da pessoa (S8.2).
func (h *Handler) OrgSettings(c *echo.Context) error {
	return h.render(c, "org_settings", Data{Title: "Configurações", Section: "settings", Script: "org"})
}
