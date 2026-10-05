package page

import "github.com/labstack/echo/v5"

// Org é a página inicial da organização: a lista de projetos (S8.4).
func (h *Handler) Org(c *echo.Context) error {
	return h.render(c, "org_projects", Data{Title: "Projetos", Section: "projects", Script: "org"})
}

// orgPage monta a página de uma aba da organização. O middleware de rota já
// garantiu que quem está logado é admin dela.
func (h *Handler) orgPage(c *echo.Context, name, title, tab string) error {
	return h.render(c, name, Data{Title: title, Section: "organization", Tab: tab, Script: "org"})
}

// OrgSettings é a aba Geral: os dados da organização e a exclusão dela (S8.2).
// O perfil e a senha de quem está logado ficam em Profile.
func (h *Handler) OrgSettings(c *echo.Context) error {
	return h.orgPage(c, "org_settings", "Organização", "general")
}

// OrgProjects é a aba Projetos: a mesma lista da página inicial, dentro da
// organização.
func (h *Handler) OrgProjects(c *echo.Context) error {
	return h.orgPage(c, "org_projects", "Projetos · Organização", "projects")
}

// People é a aba Pessoas: as pessoas da organização, os papéis e os convites (S8.3).
func (h *Handler) People(c *echo.Context) error {
	return h.orgPage(c, "org_people", "Pessoas · Organização", "people")
}
