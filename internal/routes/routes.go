package routes

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
)

type Handlers struct {
	Auth         *auth.Handler
	Organization *organization.Handler
	Customer     *customer.Handler
	Person       *person.Handler
	Permission   *permission.Handler
	Project      *project.Handler
	Team         *team.Handler
	Allocation   *allocation.Handler
	Collaborator *collaborator.Handler
	Overview     *overview.Handler
	Task         *task.Handler
	WorkSession  *work_session.Handler
	Integration  *integration.Handler
	// Sync é o botão Sincronizar agora das issues de uma integração.
	Sync *issuesync.Handler
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
	owner := m.RequireOwner
	// Quem não é admin faz o que foi liberado a ele: na organização inteira (orgCan) ou
	// no projeto da rota (can), onde vale o que a alocação dele no projeto dá.
	orgCan := m.RequireOrgPermission
	can := m.RequireProjectPermission
	org := m.RequireOrg(auth.KindOrganization, "orgId")
	prj := m.RequireOrg(auth.KindProject, "projectId")
	tm := m.RequireOrg(auth.KindTeam, "teamId")
	tsk := m.RequireOrg(auth.KindTask, "taskId")
	integ := m.RequireOrg(auth.KindIntegration, "integrationId")
	per := m.RequireOrg(auth.KindPerson, "personId")
	inv := m.RequireOrg(auth.KindInvite, "inviteId")
	cust := m.RequireOrg(auth.KindCustomer, "customerId")

	r.POST("/auth/logout", h.Auth.Logout)
	r.GET("/auth/me", h.Auth.Me)
	r.POST("/auth/password", h.Auth.ChangePassword)

	r.GET("/orgs/:orgId", h.Organization.Get, org)
	r.PATCH("/orgs/:orgId", h.Organization.Update, org, admin)
	r.DELETE("/orgs/:orgId", h.Organization.Delete, org, owner)
	r.GET("/orgs/:orgId/persons", h.Person.ListByOrg, org)
	r.POST("/orgs/:orgId/projects", h.Project.Create, org, orgCan(permission.ProjectsCreate))
	r.GET("/orgs/:orgId/projects", h.Project.ListByOrg, org)
	// A visão geral da organização soma o dinheiro de todos os projetos e o tempo de cada pessoa: só dos admins.
	r.GET("/orgs/:orgId/overview", h.Overview.Organization, org, admin)
	r.POST("/orgs/:orgId/invites", h.Auth.CreateInvite, org, orgCan(permission.PeopleManage))
	r.GET("/orgs/:orgId/invites", h.Auth.ListInvites, org, orgCan(permission.PeopleManage))
	r.DELETE("/invites/:inviteId", h.Auth.RevokeInvite, inv, orgCan(permission.PeopleManage))

	// Os clientes são de quem cuida deles: admins e quem recebeu essa permissão.
	customers := orgCan(permission.CustomersManage)
	r.POST("/orgs/:orgId/customers", h.Customer.Create, org, customers)
	r.GET("/orgs/:orgId/customers", h.Customer.ListByOrg, org, customers)
	r.GET("/customers/:customerId", h.Customer.Get, cust, customers)
	r.PATCH("/customers/:customerId", h.Customer.Update, cust, customers)
	r.DELETE("/customers/:customerId", h.Customer.Delete, cust, customers)

	r.GET("/persons/:personId", h.Person.Get, per)
	r.PATCH("/persons/:personId", h.Person.Update, per, m.RequireSelfOrAdmin("personId"))
	// Os papéis e as permissões da organização são só do dono.
	r.PATCH("/persons/:personId/role", h.Person.SetRole, per, owner)
	r.PATCH("/persons/:personId/permissions", h.Person.SetPermissions, per, owner)
	r.PATCH("/persons/:personId/weekly-hours", h.Person.SetWeeklyHours, per, orgCan(permission.PeopleManage))
	// O handler só entrega os valores à própria pessoa ou a um admin.
	r.GET("/persons/:personId/allocations", h.Allocation.ListByPerson, per)

	r.GET("/projects/:projectId", h.Project.Get, prj)
	r.PATCH("/projects/:projectId", h.Project.Update, prj, can(permission.ProjectEdit))
	r.DELETE("/projects/:projectId", h.Project.Delete, prj, can(permission.ProjectEdit))
	// A visão geral soma o que o projeto custou e rendeu: de quem vê o faturamento.
	r.GET("/projects/:projectId/overview", h.Overview.Get, prj, can(permission.BillingView))
	r.POST("/projects/:projectId/teams", h.Team.Create, prj, can(permission.TeamsManage))
	r.GET("/projects/:projectId/teams", h.Team.ListByProject, prj)
	r.GET("/projects/:projectId/members", h.Team.ListProjectMembers, prj)
	r.GET("/projects/:projectId/billing", h.Project.GetBilling, prj, can(permission.BillingView))
	r.PUT("/projects/:projectId/billing", h.Project.SetBilling, prj, can(permission.BillingManage))
	// Um membro recebe só o próprio valor; o handler filtra.
	r.GET("/projects/:projectId/allocations", h.Allocation.ListByProject, prj)
	// Pôr alguém no projeto, trocar o valor ou o grupo dela: o handler confere qual permissão
	// cada mudança pede.
	r.PUT("/projects/:projectId/allocations/:personId", h.Allocation.Set, prj, per, can(permission.CollaboratorsManage, permission.RatesManage))
	// Quem está no projeto: a pessoa entra com o valor por hora (o PUT acima) e
	// sai pelo DELETE daqui, que apaga o valor e a tira dos times. O handler só
	// entrega o valor dos colegas a admins.
	r.GET("/projects/:projectId/collaborators", h.Collaborator.ListByProject, prj)
	r.DELETE("/projects/:projectId/collaborators/:personId", h.Collaborator.Remove, prj, per, can(permission.CollaboratorsManage))
	r.POST("/projects/:projectId/tasks", h.Task.Create, prj)
	r.GET("/projects/:projectId/tasks", h.Task.ListByProject, prj)
	// As etiquetas são do projeto: todos leem e escolhem, só quem cuida delas cria, renomeia e exclui.
	r.GET("/projects/:projectId/labels", h.Task.ListLabels, prj)
	r.POST("/projects/:projectId/labels", h.Task.CreateLabel, prj, can(permission.LabelsManage))
	r.PATCH("/projects/:projectId/labels/:labelId", h.Task.RenameLabel, prj, can(permission.LabelsManage))
	r.DELETE("/projects/:projectId/labels/:labelId", h.Task.DeleteLabel, prj, can(permission.LabelsManage))
	r.POST("/projects/:projectId/work-sessions/clock-in", h.WorkSession.ClockIn, prj)
	r.POST("/projects/:projectId/work-sessions/clock-out", h.WorkSession.ClockOut, prj)
	r.GET("/projects/:projectId/work-sessions", h.WorkSession.List, prj)
	r.GET("/projects/:projectId/work-sessions/total", h.WorkSession.Total, prj)
	r.POST("/projects/:projectId/work-sessions/:sessionId/tasks", h.WorkSession.AddTask, prj)
	r.PATCH("/projects/:projectId/work-sessions/:sessionId/tasks/:linkId", h.WorkSession.UpdateTask, prj)
	r.DELETE("/projects/:projectId/work-sessions/:sessionId/tasks/:linkId", h.WorkSession.RemoveTask, prj)
	r.GET("/work-sessions/active", h.WorkSession.Active)
	r.POST("/projects/:projectId/integrations", h.Integration.Create, prj, can(permission.IntegrationsManage))
	r.GET("/projects/:projectId/integrations", h.Integration.ListByProject, prj)

	r.GET("/teams/:teamId", h.Team.Get, tm)
	r.PATCH("/teams/:teamId", h.Team.Update, tm, can(permission.TeamsManage))
	r.DELETE("/teams/:teamId", h.Team.Delete, tm, can(permission.TeamsManage))
	r.POST("/teams/:teamId/members", h.Team.AddMember, tm, can(permission.TeamsManage))
	r.DELETE("/teams/:teamId/members", h.Team.RemoveMember, tm, can(permission.TeamsManage))
	r.GET("/teams/:teamId/members", h.Team.ListMembers, tm)

	r.GET("/tasks/:taskId", h.Task.Get, tsk)
	r.PATCH("/tasks/:taskId", h.Task.Update, tsk)
	r.POST("/tasks/:taskId/claim", h.Task.Claim, tsk)
	r.PATCH("/tasks/:taskId/attributes", h.Task.UpdateAttrs, tsk)
	r.DELETE("/tasks/:taskId", h.Task.Delete, tsk)
	r.POST("/tasks/:taskId/link-external-item", h.Task.LinkExternalItem, tsk)
	r.DELETE("/tasks/:taskId/link-external-item", h.Task.UnlinkExternalItem, tsk)
	r.GET("/tasks/:taskId/external-details", h.Task.GetExternalDetails, tsk)
	// Relê a issue da tarefa no GitHub e põe as duas em acordo, na hora (o GitHub não avisa quando muda).
	r.POST("/tasks/:taskId/sync", h.Sync.SyncTask, tsk)

	r.GET("/integrations/:integrationId", h.Integration.Get, integ)
	// O que a conexão enxerga (os repositórios do GitHub), para a pessoa escolher em vez de digitar.
	r.GET("/integrations/:integrationId/repositories", h.Integration.Repositories, integ, can(permission.IntegrationsManage))
	// Uma rodada completa de sincronização das issues com as tarefas, na hora.
	r.POST("/integrations/:integrationId/sync", h.Sync.Sync, integ, can(permission.IntegrationsManage))
	r.PATCH("/integrations/:integrationId", h.Integration.Update, integ, can(permission.IntegrationsManage))
	r.DELETE("/integrations/:integrationId", h.Integration.Delete, integ, can(permission.IntegrationsManage))

	// O catálogo do que se pode liberar, para as telas de permissões.
	r.GET("/permissions", h.Permission.List)
}
