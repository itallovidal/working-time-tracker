package routes

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/page"
)

// RegisterPages monta as páginas HTML. As públicas mandam quem já está logado
// para o início; as outras mandam quem não está para o login.
func RegisterPages(e *echo.Echo, p *page.Handler, m *auth.Middleware) {
	// O idioma vale para todo mundo, logado ou não.
	e.GET("/lang/:code", p.SetLanguage)
	e.GET("/i18n/:file", p.I18nScript)

	e.GET("/login", p.Login, m.RedirectIfAuthenticated)
	e.GET("/signup", p.Signup, m.RedirectIfAuthenticated)
	e.GET("/invite/:token", p.Invite, m.RedirectIfAuthenticated)

	g := e.Group("", m.RequirePage)
	g.GET("/", p.Home)
	g.GET("/profile", p.Profile)

	org := m.RequireOrgPage(auth.KindOrganization, "orgId", p.NotFound)
	g.GET("/orgs/:orgId", p.Org, org)
	g.GET("/orgs/:orgId/about", p.About, org)
	// A tela de edição e as outras abas da organização são só de admins.
	admin := m.RequireAdminPage(p.NotFound)
	g.GET("/orgs/:orgId/settings", p.OrgSettings, org, admin)
	g.GET("/orgs/:orgId/people", p.People, org, admin)
	g.GET("/orgs/:orgId/customers", p.Customers, org, admin)
	g.GET("/orgs/:orgId/projects", p.OrgProjects, org, admin)

	prj := m.RequireOrgPage(auth.KindProject, "projectId", p.NotFound)
	g.GET("/projects/:projectId", p.Project, prj)
	g.GET("/projects/:projectId/overview", p.MyOverview, prj)
	g.GET("/projects/:projectId/tasks", p.Tasks, prj)
	// O Ponto foi para o Início do projeto; o endereço antigo continua levando para lá.
	g.GET("/projects/:projectId/time-tracking", p.ToHome, prj)
	g.GET("/projects/:projectId/collaborators", p.Collaborators, prj)
	// A Gestão é a área do projeto só de admins: quem não é recebe o 404.
	g.GET("/projects/:projectId/management", p.Management, prj, admin)
	g.GET("/projects/:projectId/management/overview", p.Overview, prj, admin)
	g.GET("/projects/:projectId/management/teams", p.Teams, prj, admin)
	g.GET("/projects/:projectId/management/integrations", p.Integrations, prj, admin)
	g.GET("/projects/:projectId/management/settings", p.ProjectSettings, prj, admin)
	// Os caminhos de antes da Gestão continuam levando às mesmas abas.
	g.GET("/projects/:projectId/teams", p.ToManagement("teams"), prj)
	g.GET("/projects/:projectId/integrations", p.ToManagement("integrations"), prj)
	g.GET("/projects/:projectId/settings", p.ToManagement("settings"), prj)

	g.GET("/tasks/:taskId", p.TaskDetail, m.RequireOrgPage(auth.KindTask, "taskId", p.NotFound))
}
