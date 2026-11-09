// Package oauth é o servidor de autorização OAuth 2.1 do servidor MCP: o que deixa o claude.ai, o Claude Desktop e o
// app do celular (que só conectam um servidor remoto por OAuth) e o Claude Code entrarem no /mcp em nome de uma pessoa.
//
// A pessoa cola o endereço do /mcp no Claude; o 401 do /mcp aponta para os metadados do recurso (RFC 9728), que
// apontam para os deste servidor (RFC 8414). O Claude manda a pessoa a /oauth/authorize, que passa pelo login daqui e
// mostra a tela de consentimento; o "Permitir" volta ao Claude com um código, que ele troca em /oauth/token pelos
// tokens (PKCE S256, com o recurso do RFC 8707).
//
// Só entram os apps da lista (ClaudeClients): cada um se identifica pela URL do documento de identidade dele (CIMD), que
// este servidor busca e guarda por um tempo, com uma cópia embutida para quando a busca falhar. Não há cadastro dinâmico
// (DCR) nem tabela de apps. Os códigos e os tokens ficam no auth (oauth_grant.go).
//
// Como o internal/mcp, não é um domínio: usa o auth. Não conhece as páginas: a tela de consentimento é do internal/page,
// que valida o pedido por aqui (ParseAuthorize).
package oauth

import (
	"log/slog"
	"net/http"
	"strings"

	"working-time-tracker/internal/domain/auth"
)

// Os caminhos do servidor de autorização. O de consentimento é da API: a tela o chama com o cookie da sessão.
const (
	AuthorizePath      = "/oauth/authorize"
	TokenPath          = "/oauth/token"
	RevokePath         = "/oauth/revoke"
	ConsentPath        = "/api/oauth/consent"
	ServerMetadataPath = "/.well-known/oauth-authorization-server"
	// WellKnownPrefix começa todos os documentos de descoberta.
	WellKnownPrefix = "/.well-known/"
)

// ResourceMetadataPath é onde ficam os metadados do recurso que está em resourcePath (o /mcp): o caminho dele depois de
// /.well-known/oauth-protected-resource, como o RFC 9728 pede.
func ResourceMetadataPath(resourcePath string) string {
	return "/.well-known/oauth-protected-resource/" + strings.TrimLeft(resourcePath, "/")
}

// Config monta o Server.
type Config struct {
	// Issuer é o PUBLIC_URL, sem barra no fim: o emissor dos metadados e do iss, que o Claude compara byte a byte.
	Issuer string
	// ResourcePath é o caminho do servidor MCP (/mcp). O recurso é Issuer mais ele.
	ResourcePath string
	Auth         *auth.Service
	// Clients são os apps que podem entrar. Vazio vale ClaudeClients.
	Clients []Client
	// HTTPClient busca os documentos de identidade dos apps. Nulo usa um com prazo curto e sem redirecionamentos.
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// Server é o servidor de autorização.
type Server struct {
	issuer   string
	resource string // canônico
	auth     *auth.Service
	clients  *Registry
	logger   *slog.Logger
}

// New monta o servidor de autorização.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	clients := cfg.Clients
	if len(clients) == 0 {
		clients = ClaudeClients()
	}
	resource, err := CanonicalResource(cfg.Issuer + cfg.ResourcePath)
	if err != nil {
		resource = cfg.Issuer + cfg.ResourcePath // o config.Load já recusou um PUBLIC_URL que não serve
	}
	return &Server{
		issuer:   cfg.Issuer,
		resource: resource,
		auth:     cfg.Auth,
		clients:  NewRegistry(clients, cfg.HTTPClient, logger),
		logger:   logger,
	}
}

// Issuer é o emissor (o PUBLIC_URL).
func (s *Server) Issuer() string { return s.issuer }

// Resource é o endereço canônico do /mcp, o recurso dos tokens.
func (s *Server) Resource() string { return s.resource }

// ResourceMetadataURL é o endereço dos metadados do recurso, que o 401 do /mcp aponta.
func (s *Server) ResourceMetadataURL(resourcePath string) string {
	return s.issuer + ResourceMetadataPath(resourcePath)
}
