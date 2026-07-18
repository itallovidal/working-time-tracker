package routes

import (
	"working-time-tracker/internal/handlers"

	"github.com/labstack/echo/v5"
)

func HealthcheckRoutesRegister(e *echo.Echo) error {
	group := e.Group("/healthcheck")

	group.GET("/", handlers.RegisterHealthcheckHandler)
	group.GET("/hello", handlers.RegisterHelloWorldHandler)

	return nil
}
