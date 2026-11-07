package mcp

import (
	"context"
	"errors"
	"net/http"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
)

// scope é o escopo que todo token do Claude tem e que o /mcp exige. Ele também faz o 401 levar o desafio
// WWW-Authenticate: Bearer scope="mcp". Quando o OAuth chegar, o token de lá terá o mesmo escopo.
const scope = "mcp"

// identityKey é onde a identidade da pessoa fica no TokenInfo da requisição.
const identityKey = "wtt.identity"

// errInternal é o que quem chama vê de uma falha inesperada ao conferir o token; o detalhe fica no log.
var errInternal = errors.New("internal error")

// verify é o TokenVerifier do /mcp: resolve o token para a pessoa dona dele, lida agora do banco (um cargo que mudou
// vale na hora). É a peça que o OAuth vai trocar ou completar.
func (s *server) verify(_ context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	id, expires, err := s.Auth.AuthenticateAPIToken(token)
	if errors.Is(err, auth.ErrUnauthenticated) {
		return nil, mcpauth.ErrInvalidToken
	}
	if err != nil {
		s.Logger.Error("mcp: token check failed", "error", err)
		return nil, errInternal
	}
	return &mcpauth.TokenInfo{
		Scopes:     []string{scope},
		Expiration: expires,
		UserID:     id.PersonID.String(),
		Extra:      map[string]any{identityKey: id},
	}, nil
}

// me é a pessoa dona do token da chamada. Sem ela (o que o middleware do token não deixa acontecer) a ferramenta
// recusa como recusaria um token inválido.
func me(req *mcp.CallToolRequest) (*auth.Identity, error) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return nil, auth.ErrUnauthenticated
	}
	id, ok := req.Extra.TokenInfo.Extra[identityKey].(*auth.Identity)
	if !ok || id == nil {
		return nil, auth.ErrUnauthenticated
	}
	return id, nil
}
