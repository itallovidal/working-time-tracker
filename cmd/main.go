package main

import (
	"log"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/internal/config"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/internal/routes"
	tmpl "working-time-tracker/internal/template"
	"working-time-tracker/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("migration: %v", err)
	}

	// Stores
	orgStore := organization.NewStore(db)
	personStore := person.NewStore(db)
	projectStore := project.NewStore(db)
	teamStore := team.NewStore(db)
	membershipStore := team.NewMembershipStore(db)
	taskStore := task.NewStore(db)
	sessionStore := work_session.NewStore(db)
	integrationStore := integration.NewStore(db)

	// Services (integration before task; task before work_session due to cross-domain deps)
	orgSvc := organization.NewService(orgStore)
	personSvc := person.NewService(personStore)
	projectSvc := project.NewService(projectStore)
	teamSvc := team.NewService(teamStore)
	membershipSvc := team.NewMembershipService(membershipStore)
	integrationSvc := integration.NewService(integrationStore, cfg.IntegrationEncryptKey)
	taskSvc := task.NewService(taskStore, membershipStore, integrationSvc)
	workSessionSvc := work_session.NewService(sessionStore, taskStore)

	// Handlers
	orgHandler := organization.NewHandler(orgSvc)
	personHandler := person.NewHandler(personSvc)
	projectHandler := project.NewHandler(projectSvc)
	teamHandler := team.NewHandler(teamSvc, membershipSvc)
	taskHandler := task.NewHandler(taskSvc)
	workSessionHandler := work_session.NewHandler(workSessionSvc)
	integrationHandler := integration.NewHandler(integrationSvc)

	e := echo.New()

	e.Renderer = tmpl.NewRendererFromFS(web.FS, "templates/*.gohtml")

	e.StaticFS("/static", echo.MustSubFS(web.FS, "static"))

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogProtocol: true,
		LogRemoteIP: true,
		LogHost:     true,
		LogMethod:   true,
		LogStatus:   true,
		LogLatency:  true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			c.Logger().Info("request",
				"method", v.Method,
				"host", v.Host,
				"remote_ip", v.RemoteIP,
				"status", v.Status,
				"latency", v.Latency,
			)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	routes.HealthcheckRoutesRegister(e)
	routes.RegisterRoutes(e, orgHandler, personHandler, projectHandler, teamHandler, taskHandler, workSessionHandler, integrationHandler)

	if err := e.Start(":" + cfg.APIPort); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
