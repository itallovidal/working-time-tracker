// Package mcp é o servidor MCP (Model Context Protocol) do sistema, em /mcp: o que o Claude Code usa para criar e
// mudar tarefas, pegar uma tarefa, sincronizá-la, bater o ponto e ver as horas em nome de uma pessoa.
//
// Como o internal/page, não é um domínio: lê os services dos outros. A pessoa entra em Authorization: Bearer com um
// token pessoal (gerado no perfil, ver auth/api_token.go) ou com o token de acesso de um app conectado pelo OAuth (o
// claude.ai, ver internal/oauth), e cada ferramenta age só sobre ela: o ponto é sempre o
// dela, e um projeto ou uma tarefa passam pela mesma checagem do RequireOrg (auth.Resolver.Reach). Nenhuma saída tem
// dinheiro (valor pago, cobrado, receita ou margem).
//
// O transporte é o Streamable HTTP sem estado (stateless, com respostas em JSON): nenhuma sessão fica na memória do
// processo, então a troca de processo de um deploy não derruba quem está conectado.
package mcp

import (
	"log/slog"
	"net/http"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/internal/i18n"
)

// Path é onde o servidor MCP atende.
const Path = "/mcp"

// maxBody é o maior corpo aceito, o mesmo da API.
const maxBody = 1 << 20

// Version é a versão do servidor que o initialize informa.
const Version = "1.0.0"

// Deps são os services que as ferramentas usam.
type Deps struct {
	Auth     *auth.Service
	Resolver *auth.Resolver
	Projects *project.Service
	Tasks    *task.Service
	Sessions *work_session.Service
	Overview *overview.Service
	Syncer   *issuesync.Syncer
	I18n     *i18n.Catalog
	// PublicURL é o endereço do sistema, sem barra no fim. Com ele, cada tarefa volta com o link da página dela.
	PublicURL string
	// Resource é o endereço canônico deste /mcp quando o OAuth está ligado: os tokens de acesso do OAuth só valem para
	// ele. Vazio, o OAuth está desligado e só o token pessoal entra.
	Resource string
	// ResourceMetadataURL é o endereço dos metadados do recurso (RFC 9728), que o 401 aponta para o Claude achar onde
	// autorizar. Vazio com o OAuth desligado.
	ResourceMetadataURL string
	Logger              *slog.Logger
}

type server struct {
	Deps
}

// instructions é o que o initialize diz ao Claude sobre como o sistema funciona.
const instructions = `Working Time Tracker: time tracking per task. You act as the person who owns the token.

- Tasks live in projects. Use list_projects, then list_tasks or my_tasks to find ids.
- Task status: backlog, in_progress, awaiting_closure, closed. Priority: urgent, high, medium, low, none.
- A person has at most one open work session (the clock). clock_in opens it on one task, and the task becomes
  in_progress and, if it had no assignee, the person's.
- To work on another task of the same project, use add_task_to_session (switch=true stops the other tasks).
  A task of another project needs clock_out first, then clock_in.
- A session keeps at least one running task: to stop everything, use clock_out.
- Dates: deadline is YYYY-MM-DD, in the organization's time zone. Times come back in RFC 3339.
- Errors come back as text with a stable code in parentheses.`

// NewHandler monta o servidor MCP e devolve o handler HTTP do /mcp, já com a checagem do token, a proteção contra
// pedidos de outra origem (só um navegador manda Origin) e o limite do corpo.
func NewHandler(d Deps) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	s := &server{Deps: d}
	srv := mcp.NewServer(&mcp.Implementation{Name: "working-time-tracker", Title: "Working Time Tracker", Version: Version},
		&mcp.ServerOptions{Instructions: instructions})
	s.addTools(srv)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: maxBody,
	})
	bearer := mcpauth.RequireBearerToken(s.verify, &mcpauth.RequireBearerTokenOptions{
		Scopes: []string{scope}, ResourceMetadataURL: d.ResourceMetadataURL,
	})
	return http.NewCrossOriginProtection().Handler(bearer(handler))
}

// addTools registra as ferramentas. Leitura primeiro, depois as que mudam alguma coisa.
func (s *server) addTools(srv *mcp.Server) {
	s.addAccountTools(srv)
	s.addTaskTools(srv)
	s.addSessionTools(srv)
}

// readOnly e write são as anotações das ferramentas: as de leitura não mudam nada; as de escrita mudam, mas nenhuma
// apaga nada que não se refaça (tirar uma tarefa da sessão só tira o trecho dela).
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true}
}

func write(title string, idempotent bool) *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &no, IdempotentHint: idempotent, OpenWorldHint: &no}
}
