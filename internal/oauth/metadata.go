package oauth

import (
	"net/http"

	"github.com/labstack/echo/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"working-time-tracker/internal/domain/auth"
)

// serverMetadata são os metadados do servidor de autorização (RFC 8414). A struct é daqui, e não a
// oauthex.AuthServerMeta, porque aquela sempre escreve "jwks_uri", mesmo vazio.
//
// O Claude só usa a identidade publicada dele (CIMD) quando client_id_metadata_document_supported é true e "none" está
// nos métodos do token; sem cadastro dinâmico, não há registration_endpoint. O PKCE só aceita S256, e o iss volta na
// resposta da autorização (RFC 9207).
type serverMetadata struct {
	Issuer                                     string   `json:"issuer"`
	AuthorizationEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                              string   `json:"token_endpoint"`
	RevocationEndpoint                         string   `json:"revocation_endpoint"`
	ResponseTypesSupported                     []string `json:"response_types_supported"`
	ResponseModesSupported                     []string `json:"response_modes_supported"`
	GrantTypesSupported                        []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported              []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported          []string `json:"token_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported     []string `json:"revocation_endpoint_auth_methods_supported"`
	ScopesSupported                            []string `json:"scopes_supported"`
	ClientIDMetadataDocumentSupported          bool     `json:"client_id_metadata_document_supported"`
	AuthorizationResponseIssParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
}

// ServerMetadata responde os metadados do servidor de autorização. Eles são públicos: qualquer origem pode lê-los.
func (s *Server) ServerMetadata(c *echo.Context) error {
	allowAnyOrigin(c, "GET, OPTIONS")
	return c.JSON(http.StatusOK, serverMetadata{
		Issuer:                                     s.issuer,
		AuthorizationEndpoint:                      s.issuer + AuthorizePath,
		TokenEndpoint:                              s.issuer + TokenPath,
		RevocationEndpoint:                         s.issuer + RevokePath,
		ResponseTypesSupported:                     []string{"code"},
		ResponseModesSupported:                     []string{"query"},
		GrantTypesSupported:                        []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:              []string{"S256"},
		TokenEndpointAuthMethodsSupported:          []string{"none"},
		RevocationEndpointAuthMethodsSupported:     []string{"none"},
		ScopesSupported:                            []string{auth.OAuthScope},
		ClientIDMetadataDocumentSupported:          true,
		AuthorizationResponseIssParameterSupported: true,
	})
}

// ResourceMetadataHandler responde os metadados do recurso (RFC 9728) pelo handler do SDK, que já cuida do CORS: o
// /mcp, quem o autoriza (este servidor) e o escopo.
func (s *Server) ResourceMetadataHandler() http.Handler {
	return mcpauth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               s.resource,
		AuthorizationServers:   []string{s.issuer},
		ScopesSupported:        []string{auth.OAuthScope},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Working Time Tracker",
	})
}

// Preflight responde o OPTIONS do CORS das rotas que um navegador de outra origem pode chamar (os metadados, o token e a
// revogação). Nenhuma delas usa cookie: o CORS não abre nada que um servidor não abriria.
func Preflight(c *echo.Context) error {
	allowAnyOrigin(c, "GET, POST, OPTIONS")
	h := c.Response().Header()
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
	h.Set("Access-Control-Max-Age", "86400")
	return c.NoContent(http.StatusNoContent)
}

// WellKnownNotFound é o 404 dos documentos de descoberta que não existem aqui, em JSON. Sem ele, um /.well-known/ que
// não existe cairia no grupo das páginas, que manda quem não está logado para o login, e o cliente do OAuth leria um
// redirecionamento onde esperava um 404.
func WellKnownNotFound(c *echo.Context) error {
	allowAnyOrigin(c, "GET, OPTIONS")
	return c.JSON(http.StatusNotFound, map[string]string{"error": "not_found"})
}

func allowAnyOrigin(c *echo.Context, methods string) {
	h := c.Response().Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", methods)
}
