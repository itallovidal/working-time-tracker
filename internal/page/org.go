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
	d := h.orgData(c, Data{TitleKey: "titles.home", Section: "home"})
	if d.Props == nil {
		d.Props = map[string]any{}
	}
	// As boas-vindas do primeiro acesso têm um passo de convite, que fala em enviar por e-mail quando o Clerk está ligado.
	d.Props["emailInvites"] = h.deps.InviteByEmail
	return h.render(c, "org_home", d)
}

// Payments é a tela de pagamentos da equipe, só de admins: quanto pagar, a quem e quando. O que cada pessoa recebe
// fica no perfil dela.
func (h *Handler) Payments(c *echo.Context) error {
	d := h.orgData(c, Data{TitleKey: "titles.payments", Section: "payments"})
	d.Script = "payments"
	return h.render(c, "org_payments", d)
}

// orgPage monta uma página da organização. Cada uma tem o seu item na barra superior (section).
func (h *Handler) orgPage(c *echo.Context, name, titleKey, section string) error {
	return h.render(c, name, h.orgData(c, Data{TitleKey: titleKey, Section: section}))
}

// About é a página Configurações: o perfil da organização, que todos os membros leem (S11.4).
func (h *Handler) About(c *echo.Context) error {
	return h.orgPage(c, "org_about", "titles.org_about", "organization")
}

// OrgSettings é a tela de edição da organização: os dados dela e a exclusão (S8.2).
// Abre pelo botão Editar das Configurações, que continuam marcadas na barra. O perfil e a senha de
// quem está logado ficam em Profile. Esta tela é só de admins; o middleware de rota garante isso.
func (h *Handler) OrgSettings(c *echo.Context) error {
	return h.orgPage(c, "org_settings", "titles.org_settings", "organization")
}

// OrgProjects é a página Projetos: os projetos de quem olha (todos, para os admins), em cartões.
func (h *Handler) OrgProjects(c *echo.Context) error {
	return h.orgPage(c, "org_projects", "titles.org_projects", "projects")
}

// Customers é a página Clientes: quem contrata os projetos da organização (S12.4).
func (h *Handler) Customers(c *echo.Context) error {
	return h.orgPage(c, "org_customers", "titles.org_customers", "customers")
}

// People é a página Colaboradores: as pessoas da organização, os papéis e os convites (S8.3).
func (h *Handler) People(c *echo.Context) error {
	d := h.orgData(c, Data{TitleKey: "titles.org_people", Section: "people"})
	if d.Props == nil {
		d.Props = map[string]any{}
	}
	// Com o Clerk ligado o convite sai por e-mail: a tela fala em enviar, e não só em copiar o link.
	d.Props["emailInvites"] = h.deps.InviteByEmail
	return h.render(c, "org_people", d)
}

// Person é o perfil de um colaborador, aberto por um admin a partir de Colaboradores ou de Pagamentos: o mesmo
// conteúdo que a pessoa vê no próprio perfil (as horas, o valor e os pagamentos dela), com o botão de editar.
func (h *Handler) Person(c *echo.Context) error {
	p, err := h.deps.People.Get(c.Param("personId"))
	if err != nil {
		return h.NotFound(c)
	}
	d := h.orgData(c, Data{TitleKey: "titles.org_person", TitleSuffix: p.Name, Section: "people"})
	d.Person = &Crumb{ID: p.ID.String(), Name: p.Name}
	return h.render(c, "org_person", d)
}
