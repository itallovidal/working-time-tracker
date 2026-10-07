package routes

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/page"
)

// RegisterPages monta as páginas HTML. As públicas mandam quem já está logado
// para o início; as outras mandam quem não está para o login.
func RegisterPages(e *echo.Echo, p *page.Handler, m *auth.Middleware, oauth *integration.OAuthHandler) {
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
	// A tela de edição da organização é só de admins; as outras abas, de quem cuida do que
	// elas mostram: pessoas, clientes e projetos.
	admin := m.RequireAdminPage(p.NotFound)
	orgCan := func(key string) echo.MiddlewareFunc { return m.RequireOrgPermissionPage(p.NotFound, key) }
	g.GET("/orgs/:orgId/settings", p.OrgSettings, org, admin)
	g.GET("/orgs/:orgId/people", p.People, org, orgCan(permission.PeopleManage))
	g.GET("/orgs/:orgId/customers", p.Customers, org, orgCan(permission.CustomersManage))
	g.GET("/orgs/:orgId/projects", p.OrgProjects, org, orgCan(permission.ProjectsCreate))

	prj := m.RequireOrgPage(auth.KindProject, "projectId", p.NotFound)
	g.GET("/projects/:projectId", p.Project, prj)
	g.GET("/projects/:projectId/overview", p.MyOverview, prj)
	g.GET("/projects/:projectId/tasks", p.Tasks, prj)
	g.GET("/projects/:projectId/my-tasks", p.MyTasks, prj)
	// O Ponto foi para o Início do projeto; o endereço antigo continua levando para lá.
	g.GET("/projects/:projectId/time-tracking", p.ToHome, prj)
	g.GET("/projects/:projectId/collaborators", p.Collaborators, prj)
	// A Gestão é a área do projeto de quem cuida dele: cada aba pede a permissão do que
	// mostra, e quem não tem nenhuma recebe o 404.
	can := func(keys ...string) echo.MiddlewareFunc { return m.RequireProjectPermissionPage(p.NotFound, keys...) }
	g.GET("/projects/:projectId/management", p.Management, prj, can(permission.ProjectKeys...))
	g.GET("/projects/:projectId/management/overview", p.Overview, prj, can(permission.BillingView))
	g.GET("/projects/:projectId/management/teams", p.Teams, prj, can(permission.CollaboratorsManage, permission.TeamsManage, permission.RatesView, permission.RatesManage))
	g.GET("/projects/:projectId/management/integrations", p.Integrations, prj, can(permission.IntegrationsManage))
	// Conectar com o GitHub: a ida leva a pessoa a autorizar lá (a mesma permissão da aba), a volta
	// cai num endereço fixo, o que está cadastrado no app, e confere tudo de novo pelo cookie.
	g.GET("/projects/:projectId/management/integrations/github/connect", oauth.GitHubConnect, prj, can(permission.IntegrationsManage))
	g.GET(integration.CallbackPath, oauth.GitHubCallback)
	g.GET("/projects/:projectId/management/settings", p.ProjectSettings, prj, can(permission.ProjectEdit, permission.BillingView, permission.BillingManage))
	// Os caminhos de antes da Gestão continuam levando às mesmas abas.
	g.GET("/projects/:projectId/teams", p.ToManagement("teams"), prj)
	g.GET("/projects/:projectId/integrations", p.ToManagement("integrations"), prj)
	g.GET("/projects/:projectId/settings", p.ToManagement("settings"), prj)

	g.GET("/tasks/:taskId", p.TaskDetail, m.RequireOrgPage(auth.KindTask, "taskId", p.NotFound))
}
