package main

import (
	"log"

	"working-time-tracker/internal/config"
	"working-time-tracker/internal/handlers"
	tmpl "working-time-tracker/internal/template"
	"working-time-tracker/internal/routes"
	"working-time-tracker/internal/service"
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

	orgStore := store.NewOrganizationStore(db)
	personStore := store.NewPersonStore(db)
	projectStore := store.NewProjectStore(db)
	teamStore := store.NewTeamStore(db)
	teamMembershipStore := store.NewTeamMembershipStore(db)

	orgSvc := service.NewOrganizationService(orgStore)
	personSvc := service.NewPersonService(personStore)
	projectSvc := service.NewProjectService(projectStore)
	teamSvc := service.NewTeamService(teamStore)
	teamMembershipSvc := service.NewTeamMembershipService(teamMembershipStore)

	orgHandler := handlers.NewOrganizationHandler(orgSvc)
	personHandler := handlers.NewPersonHandler(personSvc)
	projectHandler := handlers.NewProjectHandler(projectSvc)
	teamHandler := handlers.NewTeamHandler(teamSvc, teamMembershipSvc)

	e := echo.New()

	e.Renderer = tmpl.NewRendererFromFS(web.FS, "templates/*.gohtml")

	e.StaticFS("/static", echo.MustSubFS(web.FS, "static"))

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	routes.HealthcheckRoutesRegister(e)
	routes.RegisterRoutes(e, orgHandler, personHandler, projectHandler, teamHandler)

	if err := e.Start(":" + cfg.APIPort); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
