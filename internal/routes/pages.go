package routes

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/page"
)

// RegisterPages monta as páginas HTML. As públicas mandam quem já está logado
// para o início; as outras mandam quem não está para o login.
func RegisterPages(e *echo.Echo, p *page.Handler, m *auth.Middleware) {
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
	g.GET("/projects/:projectId/tasks", p.Tasks, prj)
	g.GET("/projects/:projectId/time-tracking", p.TimeTracking, prj)
	g.GET("/projects/:projectId/teams", p.Teams, prj)
	g.GET("/projects/:projectId/rates", p.Rates, prj, admin)
	g.GET("/projects/:projectId/integrations", p.Integrations, prj)
	g.GET("/projects/:projectId/settings", p.ProjectSettings, prj)

	g.GET("/tasks/:taskId", p.TaskDetail, m.RequireOrgPage(auth.KindTask, "taskId", p.NotFound))
}
