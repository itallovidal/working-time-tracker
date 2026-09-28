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

	org := m.RequireOrgPage(auth.KindOrganization, "orgId", p.NotFound)
	g.GET("/orgs/:orgId", p.Org, org)
	g.GET("/orgs/:orgId/people", p.People, org)
	g.GET("/orgs/:orgId/settings", p.OrgSettings, org)

	prj := m.RequireOrgPage(auth.KindProject, "projectId", p.NotFound)
	g.GET("/projects/:projectId", p.Project, prj)
	g.GET("/projects/:projectId/teams", p.Teams, prj)
	g.GET("/projects/:projectId/settings", p.ProjectSettings, prj)
}
