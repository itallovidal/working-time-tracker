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

// Org é a página inicial da organização: para os admins, a visão geral (o tempo e o dinheiro de
// todos os projetos), e para todos, os projetos em cartões, uma página por vez.
func (h *Handler) Org(c *echo.Context) error {
	return h.render(c, "org_home", h.orgData(c, Data{TitleKey: "titles.home", Section: "home"}))
}

// orgPage monta a página de uma aba da organização. A aba Colaboradores tem o próprio item na
// barra superior; as outras ficam sob Organização.
func (h *Handler) orgPage(c *echo.Context, name, titleKey, tab string) error {
	section := "organization"
	if tab == "people" {
		section = "people"
	}
	return h.render(c, name, h.orgData(c, Data{TitleKey: titleKey, Section: section, Tab: tab}))
}

// About é a aba Sobre: o perfil da organização, que todos os membros leem (S11.4).
func (h *Handler) About(c *echo.Context) error {
	return h.orgPage(c, "org_about", "titles.org_about", "about")
}

// OrgSettings é a tela de edição da organização: os dados dela e a exclusão (S8.2).
// Abre pelo botão Editar da aba Sobre, que continua marcada. O perfil e a senha de
// quem está logado ficam em Profile. Esta tela e as abas seguintes são só de admins;
// o middleware de rota garante isso.
func (h *Handler) OrgSettings(c *echo.Context) error {
	return h.orgPage(c, "org_settings", "titles.org_settings", "about")
}

// OrgProjects é a aba Projetos: todos os projetos numa tabela de gestão, dentro da organização.
func (h *Handler) OrgProjects(c *echo.Context) error {
	return h.orgPage(c, "org_projects", "titles.org_projects", "projects")
}

// Customers é a aba Clientes: quem contrata os projetos da organização (S12.4).
func (h *Handler) Customers(c *echo.Context) error {
	return h.orgPage(c, "org_customers", "titles.org_customers", "customers")
}

// People é a aba Colaboradores: as pessoas da organização, os papéis e os convites (S8.3).
func (h *Handler) People(c *echo.Context) error {
	return h.orgPage(c, "org_people", "titles.org_people", "people")
}
