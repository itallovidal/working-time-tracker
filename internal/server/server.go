package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/ent"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/payment"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/projectinvite"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/internal/i18n"
	"working-time-tracker/internal/page"
	"working-time-tracker/internal/routes"
	tmpl "working-time-tracker/internal/template"
	"working-time-tracker/web"
)

type Options struct {
	EncryptKey   string
	CookieSecure bool
	// AuthRateLimit é o limite de requisições por segundo, por IP, nas rotas
	// públicas de autenticação. Zero usa o padrão (10 por minuto, rajada de 10).
	AuthRateLimit float64
	// GitHubOAuth é o app OAuth do GitHub deste servidor. Nulo ou sem credenciais, o botão
	// Conectar com o GitHub avisa que não está configurado.
	GitHubOAuth *adapter.GitHubOAuth
	// TrelloAuth é a autorização do Trello deste servidor (a chave do app). Nulo ou sem chave, o botão
	// Conectar com o Trello avisa que não está configurado.
	TrelloAuth *adapter.TrelloAuth
	// Clerk liga o login pelo Clerk. Nulo, ele fica desligado: o login é só por e-mail e senha.
	Clerk *ClerkOptions
	// Sync são os ajustes da sincronização das issues. O valor zero serve: sem Interval a rotina de
	// fundo não olha o GitHub (só o botão e o gancho das tarefas).
	Sync issuesync.Config
}

// ClerkOptions são os dados do login pelo Clerk.
type ClerkOptions struct {
	Provider adapter.ClerkProvider
	// PublishableKey é a chave pública do app, que o navegador usa para carregar o clerk-js.
	PublishableKey string
	// PublicURL é o endereço do sistema, sem barra no fim: o convite por e-mail leva a pessoa a ele.
	PublicURL string
	// InviteDelivery é "email" (o Clerk manda o e-mail do convite) ou "terminal" (não manda e mostra o link no
	// log). Vazio vale "email".
	InviteDelivery string
	// InviteLog recebe o link do convite no modo terminal. Nulo usa o log do servidor.
	InviteLog func(msg string, args ...any)
}

// App é o servidor montado e a sincronização das issues, que anda ao lado dele: quem sobe o servidor
// também chama Sync.Run, e espera por ele antes de fechar o banco.
type App struct {
	Echo *echo.Echo
	Sync *issuesync.Syncer
}

// New monta o servidor HTTP completo (stores, services, handlers, middlewares e rotas).
// É usado pelos testes que precisam exercitar o router real; o cmd/main.go usa Build, que também
// devolve a sincronização das issues.
func New(client *ent.Client, opts Options) (*echo.Echo, error) {
	app, err := Build(client, opts)
	if err != nil {
		return nil, err
	}
	return app.Echo, nil
}

// Build monta o servidor HTTP completo e a sincronização das issues, e liga uma à outra: toda tarefa
// que muda por dentro do sistema avisa a sincronização (que só anota; quem fala com o GitHub é o
// Sync.Run ou, nos testes, o Sync.Flush).
func Build(client *ent.Client, opts Options) (*App, error) {
	// Stores
	orgStore := organization.NewStore(client)
	customerStore := customer.NewStore(client)
	personStore := person.NewStore(client)
	projectStore := project.NewStore(client)
	teamStore := team.NewStore(client)
	membershipStore := team.NewMembershipStore(client)
	allocationStore := allocation.NewStore(client)
	taskStore := task.NewStore(client)
	sessionStore := work_session.NewStore(client)
	integrationStore := integration.NewStore(client)
	authStore := auth.NewStore(client)

	// Services (integration before task; task before work_session due to cross-domain deps)
	orgSvc := organization.NewService(orgStore)
	customerSvc := customer.NewService(customerStore)
	personSvc := person.NewService(personStore)
	projectSvc := project.NewService(projectStore)
	teamSvc := team.NewService(teamStore)
	membershipSvc := team.NewMembershipService(membershipStore)
	allocationSvc := allocation.NewService(allocationStore)
	collaboratorSvc := collaborator.NewService(collaborator.NewStore(client))
	integrationSvc := integration.NewService(integrationStore, opts.EncryptKey)
	taskSvc := task.NewService(taskStore, membershipStore, integrationSvc)
	workSessionSvc := work_session.NewService(sessionStore, taskStore, allocationStore)
	overviewSvc := overview.NewService(overview.Deps{
		Projects:      projectSvc,
		Collaborators: collaboratorSvc,
		Teams:         teamSvc,
		Sessions:      workSessionSvc,
		Integrations:  integrationSvc,
		Tasks:         taskStore,
		People:        personSvc,
	})
	paymentSvc := payment.NewService(payment.Deps{Orgs: orgSvc, People: personSvc, Sessions: workSessionSvc, Projects: projectSvc})
	authSvc := auth.NewService(authStore)
	var clerkPage *page.Clerk
	if opts.Clerk != nil && opts.Clerk.Provider != nil {
		clerkPage = &page.Clerk{
			PublishableKey: opts.Clerk.PublishableKey,
			FrontendAPI:    adapter.ClerkFrontendAPI(opts.Clerk.PublishableKey),
		}
	}

	e := echo.New()
	// Quem aceita um convite que leva a um projeto entra nele com o valor, o grupo e o time escolhidos.
	authSvc.SetProjectApplier(projectinvite.NewApplier(allocationSvc, membershipSvc), e.Logger.Warn)
	if opts.Clerk != nil && opts.Clerk.Provider != nil {
		log := opts.Clerk.InviteLog
		if log == nil {
			log = e.Logger.Info
		}
		authSvc.SetClerk(opts.Clerk.Provider, auth.ClerkSettings{
			PublicURL: opts.Clerk.PublicURL, InviteDelivery: opts.Clerk.InviteDelivery, Log: log,
		})
	}
	if opts.Sync.Logger == nil {
		opts.Sync.Logger = e.Logger
	}
	syncer := issuesync.New(issuesync.Deps{
		Integrations: integrationSvc, Tasks: taskStore, People: personStore,
		Members: membershipStore, Rows: issuesync.NewStore(client),
	}, opts.Sync)
	taskStore.SetChangeHook(syncer.Notify)
	taskStore.SetCreateHook(syncer.NotifyCreated)
	taskSvc.SetRemover(syncer)

	personHandler := person.NewHandler(personSvc)
	handlers := routes.Handlers{
		Auth:          auth.NewHandler(authSvc, opts.CookieSecure),
		Organization:  organization.NewHandler(orgSvc),
		Customer:      customer.NewHandler(customerSvc),
		Person:        personHandler,
		Permission:    permission.NewHandler(),
		Project:       project.NewHandler(projectSvc, allocationSvc),
		Team:          team.NewHandler(teamSvc, membershipSvc),
		Allocation:    allocation.NewHandler(allocationSvc),
		Collaborator:  collaborator.NewHandler(collaboratorSvc),
		Overview:      overview.NewHandler(overviewSvc),
		Payment:       payment.NewHandler(paymentSvc),
		Task:          task.NewHandler(taskSvc),
		WorkSession:   work_session.NewHandler(workSessionSvc),
		Integration:   integration.NewHandler(integrationSvc),
		Sync:          issuesync.NewHandler(syncer),
		ProjectInvite: projectinvite.NewHandler(authSvc, teamSvc),
	}
	resolver := auth.NewResolver(client)
	authMW := auth.NewMiddleware(authSvc, resolver, opts.CookieSecure)
	// Quem não é admin nem cuida de pessoas só vê a si mesmo e quem divide projeto com ele.
	personHandler.SetScope(authMW.PeopleScope)
	// A regra de pagamento é só da própria pessoa, dos admins e de quem cuida de pessoas.
	personHandler.SetPaymentGuard(func(c *echo.Context, personID uuid.UUID) bool {
		me := auth.CurrentPerson(c)
		return me != nil && (me.PersonID == personID || me.IsAdmin() || me.Can(permission.PeopleManage))
	})
	oauthHandler := integration.NewOAuthHandler(integrationSvc, opts.GitHubOAuth, opts.TrelloAuth, resolver, opts.EncryptKey, opts.CookieSecure)
	catalog, err := i18n.Load()
	if err != nil {
		return nil, err
	}
	pages := page.NewHandler(page.Deps{
		Orgs: orgSvc, Projects: projectSvc, Tasks: taskSvc, People: personSvc,
		I18n: catalog, CookieSecure: opts.CookieSecure,
		OAuthConfigured: map[string]bool{"github": opts.GitHubOAuth.Configured(), "trello": opts.TrelloAuth.Configured()},
		Clerk:           clerkPage,
		InviteByEmail:   clerkPage != nil,
	})

	renderer, err := tmpl.New(web.FS)
	if err != nil {
		return nil, err
	}

	e.Renderer = renderer
	e.HTTPErrorHandler = errorHandler(pages)

	e.StaticFS("/static", echo.MustSubFS(web.FS, "static"))

	e.Pre(middleware.RemoveTrailingSlash())
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
	e.Use(authMW.LoadSession)
	e.Use(catalog.Middleware())

	routes.HealthcheckRoutesRegister(e)
	routes.RegisterRoutes(e, handlers, authMW, authRateLimiter(opts.AuthRateLimit))
	routes.RegisterPages(e, pages, authMW, oauthHandler)

	return &App{Echo: e, Sync: syncer}, nil
}

func authRateLimiter(perSecond float64) echo.MiddlewareFunc {
	burst := 10
	if perSecond <= 0 {
		perSecond = 10.0 / 60
	} else {
		burst = int(perSecond) + 1
	}
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:  perSecond,
			Burst: burst,
		}),
		DenyHandler: func(c *echo.Context, _ string, _ error) error {
			return apperr.Respond(c, http.StatusTooManyRequests, apperr.ErrTooMany)
		},
	})
}

// errorHandler mostra a página de 404 em HTML para rotas do navegador e mantém
// o JSON padrão do Echo para a API e os arquivos estáticos.
func errorHandler(pages *page.Handler) echo.HTTPErrorHandler {
	fallback := echo.DefaultHTTPErrorHandler(false)
	return func(c *echo.Context, err error) {
		var sc echo.HTTPStatusCoder
		path := c.Request().URL.Path
		if c.Request().Method == http.MethodGet && errors.As(err, &sc) && sc.StatusCode() == http.StatusNotFound &&
			!strings.HasPrefix(path, "/api") && !strings.HasPrefix(path, "/static") {
			if pages.NotFound(c) == nil {
				return
			}
		}
		fallback(c, err)
	}
}
