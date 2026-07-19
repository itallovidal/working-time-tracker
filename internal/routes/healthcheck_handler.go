package routes

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func RegisterHealthcheckHandler(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
}

func RegisterHelloWorldHandler(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "hello world!"})
}
