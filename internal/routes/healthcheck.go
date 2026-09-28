package routes

import (
	"github.com/labstack/echo/v5"
)

func HealthcheckRoutesRegister(e *echo.Echo) error {
	group := e.Group("/healthcheck")

	group.GET("", RegisterHealthcheckHandler)
	group.GET("/hello", RegisterHelloWorldHandler)

	return nil
}
