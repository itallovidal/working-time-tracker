package routes

import (
	"working-time-tracker/internal/handlers"

	"github.com/labstack/echo/v5"
)

func RegisterRoutes(
	e *echo.Echo,
	orgHandler *handlers.OrganizationHandler,
	personHandler *handlers.PersonHandler,
) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/", orgHandler.Create)
	orgs.GET("/", orgHandler.List)
	orgs.GET("/:orgId", orgHandler.Get)
	orgs.PATCH("/:orgId", orgHandler.Update)
	orgs.DELETE("/:orgId", orgHandler.Delete)

	orgs.POST("/:orgId/persons", personHandler.Create)
	orgs.GET("/:orgId/persons", personHandler.ListByOrg)

	persons := e.Group("/api/persons")
	persons.GET("/:personId", personHandler.Get)
	persons.PATCH("/:personId", personHandler.Update)
}
