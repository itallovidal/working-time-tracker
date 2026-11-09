package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
)

// scope é o escopo que todo token do Claude tem e que o /mcp exige. Ele também faz o 401 levar o desafio
// WWW-Authenticate: Bearer scope="mcp". O token do OAuth tem o mesmo escopo.
const scope = auth.OAuthScope

// identityKey é onde a identidade da pessoa fica no TokenInfo da requisição.
const identityKey = "wtt.identity"

// errInternal é o que quem chama vê de uma falha inesperada ao conferir o token; o detalhe fica no log.
var errInternal = errors.New("internal error")

// verify é o TokenVerifier do /mcp: resolve o token para a pessoa dona dele, lida agora do banco (um cargo que mudou
// vale na hora). O prefixo diz o tipo: o token de acesso do OAuth (wtta_, só com o OAuth ligado e só para este
// recurso) ou o token pessoal (wtt_). Um token que não existe, venceu ou foi revogado é o 401 que leva o Claude a
// autorizar de novo.
func (s *server) verify(_ context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	var (
		id      *auth.Identity
		expires time.Time
		err     error
	)
	switch {
	case strings.HasPrefix(token, auth.OAuthAccessPrefix):
		if s.Resource == "" {
			return nil, mcpauth.ErrInvalidToken
		}
		id, expires, err = s.Auth.AuthenticateOAuthToken(token, s.Resource)
	default:
		id, expires, err = s.Auth.AuthenticateAPIToken(token)
	}
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
