package routes

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
)

type Handlers struct {
	Auth         *auth.Handler
	Organization *organization.Handler
	Person       *person.Handler
	Project      *project.Handler
	Team         *team.Handler
	Task         *task.Handler
	WorkSession  *work_session.Handler
	Integration  *integration.Handler
}

// RegisterRoutes monta a API JSON. Tudo fora de /api/auth exige login, e cada
// rota com ID confere se o recurso é da organização de quem chama.
// authLimiter limita as tentativas nas rotas públicas de autenticação.
func RegisterRoutes(e *echo.Echo, h Handlers, m *auth.Middleware, authLimiter echo.MiddlewareFunc) {
	api := e.Group("/api", m.JSONOnly)

	// Públicas
	api.POST("/auth/signup", h.Auth.Signup, authLimiter)
	api.POST("/auth/login", h.Auth.Login, authLimiter)
	api.GET("/auth/invites/:token", h.Auth.InviteInfo, authLimiter)
	api.POST("/auth/invites/:token/accept", h.Auth.AcceptInvite, authLimiter)

	// Logadas
	r := api.Group("", m.RequireAPI)
	admin := m.RequireAdmin
	org := m.RequireOrg(auth.KindOrganization, "orgId")
	prj := m.RequireOrg(auth.KindProject, "projectId")
	tm := m.RequireOrg(auth.KindTeam, "teamId")
	tsk := m.RequireOrg(auth.KindTask, "taskId")
	integ := m.RequireOrg(auth.KindIntegration, "integrationId")
	per := m.RequireOrg(auth.KindPerson, "personId")
	inv := m.RequireOrg(auth.KindInvite, "inviteId")

	r.POST("/auth/logout", h.Auth.Logout)
	r.GET("/auth/me", h.Auth.Me)
	r.POST("/auth/password", h.Auth.ChangePassword)

	r.GET("/orgs/:orgId", h.Organization.Get, org)
	r.PATCH("/orgs/:orgId", h.Organization.Update, org, admin)
	r.DELETE("/orgs/:orgId", h.Organization.Delete, org, admin)
	r.GET("/orgs/:orgId/persons", h.Person.ListByOrg, org)
	r.POST("/orgs/:orgId/projects", h.Project.Create, org, admin)
	r.GET("/orgs/:orgId/projects", h.Project.ListByOrg, org)
	r.POST("/orgs/:orgId/invites", h.Auth.CreateInvite, org, admin)
	r.GET("/orgs/:orgId/invites", h.Auth.ListInvites, org, admin)
	r.DELETE("/invites/:inviteId", h.Auth.RevokeInvite, inv, admin)

	r.GET("/persons/:personId", h.Person.Get, per)
	r.PATCH("/persons/:personId", h.Person.Update, per, m.RequireSelfOrAdmin("personId"))
	r.PATCH("/persons/:personId/role", h.Person.SetRole, per, admin)

	r.GET("/projects/:projectId", h.Project.Get, prj)
	r.PATCH("/projects/:projectId", h.Project.Update, prj, admin)
	r.DELETE("/projects/:projectId", h.Project.Delete, prj, admin)
	r.POST("/projects/:projectId/teams", h.Team.Create, prj, admin)
	r.GET("/projects/:projectId/teams", h.Team.ListByProject, prj)
	r.POST("/projects/:projectId/tasks", h.Task.Create, prj)
	r.GET("/projects/:projectId/tasks", h.Task.ListByProject, prj)
	r.POST("/projects/:projectId/work-sessions/clock-in", h.WorkSession.ClockIn, prj)
	r.POST("/projects/:projectId/work-sessions/clock-out", h.WorkSession.ClockOut, prj)
	r.GET("/projects/:projectId/work-sessions", h.WorkSession.List, prj)
	r.GET("/projects/:projectId/work-sessions/total", h.WorkSession.Total, prj)
	r.POST("/projects/:projectId/integrations", h.Integration.Create, prj, admin)
	r.GET("/projects/:projectId/integrations", h.Integration.ListByProject, prj)

	r.GET("/teams/:teamId", h.Team.Get, tm)
	r.PATCH("/teams/:teamId", h.Team.Update, tm, admin)
	r.DELETE("/teams/:teamId", h.Team.Delete, tm, admin)
	r.POST("/teams/:teamId/members", h.Team.AddMember, tm, admin)
	r.DELETE("/teams/:teamId/members", h.Team.RemoveMember, tm, admin)
	r.GET("/teams/:teamId/members", h.Team.ListMembers, tm)

	r.GET("/tasks/:taskId", h.Task.Get, tsk)
	r.PATCH("/tasks/:taskId", h.Task.Update, tsk)
	r.DELETE("/tasks/:taskId", h.Task.Delete, tsk)
	r.POST("/tasks/:taskId/link-external-item", h.Task.LinkExternalItem, tsk)
	r.DELETE("/tasks/:taskId/link-external-item", h.Task.UnlinkExternalItem, tsk)
	r.GET("/tasks/:taskId/external-details", h.Task.GetExternalDetails, tsk)

	r.GET("/integrations/:integrationId", h.Integration.Get, integ)
	r.PATCH("/integrations/:integrationId", h.Integration.Update, integ, admin)
	r.DELETE("/integrations/:integrationId", h.Integration.Delete, integ, admin)
}
