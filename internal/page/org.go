package page

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
)

// orgData monta os dados de uma página da organização de quem está logado. A
// organização vai para o template (resumo no cabeçalho) e para o JavaScript, em
// window.BOOT.org.
func (h *Handler) orgData(c *echo.Context, d Data) Data {
	d.Script = "org"
	me := auth.CurrentPerson(c)
	if me == nil {
		return d
	}
	if org, err := h.deps.Orgs.Get(me.OrganizationID.String()); err == nil {
		d.Org = org
		d.Props = map[string]any{"org": org}
	}
	return d
}

// Org é a página inicial da organização: a lista de projetos (S8.4).
func (h *Handler) Org(c *echo.Context) error {
	return h.render(c, "org_projects", h.orgData(c, Data{Title: "Projetos", Section: "projects"}))
}

// orgPage monta a página de uma aba da organização.
func (h *Handler) orgPage(c *echo.Context, name, title, tab string) error {
	return h.render(c, name, h.orgData(c, Data{Title: title, Section: "organization", Tab: tab}))
}

// About é a aba Sobre: o perfil da organização, que todos os membros leem (S11.4).
func (h *Handler) About(c *echo.Context) error {
	return h.orgPage(c, "org_about", "Sobre · Organização", "about")
}

// OrgSettings é a tela de edição da organização: os dados dela e a exclusão (S8.2).
// Abre pelo botão Editar da aba Sobre, que continua marcada. O perfil e a senha de
// quem está logado ficam em Profile. Esta tela e as abas seguintes são só de admins;
// o middleware de rota garante isso.
func (h *Handler) OrgSettings(c *echo.Context) error {
	return h.orgPage(c, "org_settings", "Editar · Organização", "about")
}

// OrgProjects é a aba Projetos: a mesma lista da página inicial, dentro da
// organização.
func (h *Handler) OrgProjects(c *echo.Context) error {
	return h.orgPage(c, "org_projects", "Projetos · Organização", "projects")
}

// Customers é a aba Clientes: quem contrata os projetos da organização (S12.4).
func (h *Handler) Customers(c *echo.Context) error {
	return h.orgPage(c, "org_customers", "Clientes · Organização", "customers")
}

// People é a aba Colaboradores: as pessoas da organização, os papéis e os convites (S8.3).
func (h *Handler) People(c *echo.Context) error {
	return h.orgPage(c, "org_people", "Colaboradores · Organização", "people")
}
