package main

import (
	"log"

	"working-time-tracker/internal/config"
	tmpl "working-time-tracker/internal/template"
	"working-time-tracker/internal/routes"
	"working-time-tracker/internal/store"
	"working-time-tracker/web"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	if err := store.AutoMigrate(db); err != nil {
		log.Fatalf("migration: %v", err)
	}

	e := echo.New()

	e.Renderer = tmpl.NewRendererFromFS(web.FS, "templates/*.gohtml")

	e.StaticFS("/static", echo.MustSubFS(web.FS, "static"))

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	routes.HealthcheckRoutesRegister(e)

	if err := e.Start(":" + cfg.APIPort); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
