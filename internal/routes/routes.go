package routes

import (
	"working-time-tracker/internal/handlers"

	"github.com/labstack/echo/v5"
)

func RegisterRoutes(
	e *echo.Echo,
	orgHandler *handlers.OrganizationHandler,
	personHandler *handlers.PersonHandler,
	projectHandler *handlers.ProjectHandler,
	teamHandler *handlers.TeamHandler,
	taskHandler *handlers.TaskHandler,
) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/", orgHandler.Create)
	orgs.GET("/", orgHandler.List)
	orgs.GET("/:orgId", orgHandler.Get)
	orgs.PATCH("/:orgId", orgHandler.Update)
	orgs.DELETE("/:orgId", orgHandler.Delete)

	orgs.POST("/:orgId/persons", personHandler.Create)
	orgs.GET("/:orgId/persons", personHandler.ListByOrg)

	orgs.POST("/:orgId/projects", projectHandler.Create)
	orgs.GET("/:orgId/projects", projectHandler.ListByOrg)

	projects := e.Group("/api/projects")
	projects.GET("/:projectId", projectHandler.Get)
	projects.PATCH("/:projectId", projectHandler.Update)
	projects.DELETE("/:projectId", projectHandler.Delete)

	projects.POST("/:projectId/teams", teamHandler.Create)
	projects.GET("/:projectId/teams", teamHandler.ListByProject)

	projects.POST("/:projectId/tasks", taskHandler.Create)
	projects.GET("/:projectId/tasks", taskHandler.ListByProject)

	tasks := e.Group("/api/tasks")
	tasks.GET("/:taskId", taskHandler.Get)
	tasks.PATCH("/:taskId", taskHandler.Update)
	tasks.DELETE("/:taskId", taskHandler.Delete)
	tasks.POST("/:taskId/link-external-item", taskHandler.LinkExternalItem)
	tasks.DELETE("/:taskId/link-external-item", taskHandler.UnlinkExternalItem)
	tasks.GET("/:taskId/external-details", taskHandler.GetExternalDetails)

	teams := e.Group("/api/teams")
	teams.GET("/:teamId", teamHandler.Get)
	teams.PATCH("/:teamId", teamHandler.Update)
	teams.DELETE("/:teamId", teamHandler.Delete)
	teams.POST("/:teamId/members", teamHandler.AddMember)
	teams.DELETE("/:teamId/members", teamHandler.RemoveMember)
	teams.GET("/:teamId/members", teamHandler.ListMembers)

	persons := e.Group("/api/persons")
	persons.GET("/:personId", personHandler.Get)
	persons.PATCH("/:personId", personHandler.Update)
}
