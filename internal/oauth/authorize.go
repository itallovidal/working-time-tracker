package oauth

import (
	"context"
	"errors"
	"net/url"

	"working-time-tracker/internal/domain/auth"
)

// Request é um pedido de autorização que passou em tudo: é o que a tela de consentimento mostra e devolve no
// "Permitir". Resource é o do /mcp, na forma canônica, e Scope é sempre o único escopo.
type Request struct {
	ClientID      string `json:"client_id"`
	ClientName    string `json:"client_name"`
	RedirectURI   string `json:"redirect_uri"`
	State         string `json:"state"`
	CodeChallenge string `json:"code_challenge"`
	Resource      string `json:"resource"`
	Scope         string `json:"scope"`
}

// RedirectHost é o nome (com a porta) para onde a autorização volta, que a tela mostra para a pessoa saber aonde o
// código vai.
func (r *Request) RedirectHost() string {
	u, err := url.Parse(r.RedirectURI)
	if err != nil {
		return r.RedirectURI
	}
	return u.Host
}

// Loopback diz se a volta é para um programa no próprio computador (o Claude Code): a tela avisa.
func (r *Request) Loopback() bool {
	return loopbackRedirect(r.RedirectURI)
}

// As razões de um pedido que não pode voltar ao app: o app não é conhecido, ou o endereço de volta não é dele, ou falta
// um dos dois. A tela de erro mostra o texto de cada uma (oauth.error.<razão>).
const (
	ReasonClientUnknown   = "client_unknown"
	ReasonRedirectInvalid = "redirect_invalid"
	ReasonRequestInvalid  = "request_invalid"
)

// Failure é um pedido que não serve. Com Redirect, o erro volta ao app por ele (o OAuth manda assim); sem, o
// app ou o endereço de volta não são confiáveis, e a página mostra o erro pela Reason, sem redirecionar.
type Failure struct {
	Reason   string
	Redirect string
}

// ParseAuthorize confere um pedido de autorização (os parâmetros do /oauth/authorize, ou os que a tela de
// consentimento devolve). Primeiro o app e o endereço de volta, que decidem se o erro pode voltar a ele; depois o
// resto: response_type code, PKCE S256 e o recurso, que, se vier, tem de ser o /mcp. O escopo pedido não importa: a
// concessão é sempre do único escopo, e o Claude Code nem manda escopo.
func (s *Server) ParseAuthorize(ctx context.Context, q url.Values) (*Request, *Failure) {
	clientID, ok := single(q, "client_id")
	if !ok || clientID == "" {
		return nil, &Failure{Reason: ReasonRequestInvalid}
	}
	client, err := s.clients.Lookup(ctx, clientID)
	if errors.Is(err, errUnknownClient) {
		return nil, &Failure{Reason: ReasonClientUnknown}
	}
	if err != nil {
		return nil, &Failure{Reason: ReasonRequestInvalid}
	}
	redirect, ok := single(q, "redirect_uri")
	if !ok || redirect == "" || !MatchRedirect(client.RedirectURIs, redirect) {
		return nil, &Failure{Reason: ReasonRedirectInvalid}
	}

	state, ok := single(q, "state")
	if !ok {
		return nil, &Failure{Redirect: s.errorURL(redirect, "invalid_request", "state is repeated", "")}
	}
	back := func(code, description string) (*Request, *Failure) {
		return nil, &Failure{Redirect: s.errorURL(redirect, code, description, state)}
	}
	for _, name := range []string{"response_type", "code_challenge", "code_challenge_method", "scope"} {
		if _, ok := single(q, name); !ok {
			return back("invalid_request", name+" is repeated")
		}
	}
	if q.Get("response_type") != "code" {
		return back("unsupported_response_type", "response_type must be code")
	}
	challenge := q.Get("code_challenge")
	if challenge == "" {
		return back("invalid_request", "code_challenge is required (PKCE)")
	}
	if q.Get("code_challenge_method") != "S256" {
		return back("invalid_request", "code_challenge_method must be S256")
	}
	if !auth.ValidPKCEChallenge(challenge) {
		return back("invalid_request", "code_challenge is not an S256 challenge")
	}
	switch resources := q["resource"]; len(resources) {
	case 0:
	case 1:
		if got, err := CanonicalResource(resources[0]); err != nil || got != s.resource {
			return back("invalid_target", "resource must be "+s.resource)
		}
	default:
		return back("invalid_target", "only one resource is allowed")
	}
	return &Request{
		ClientID:      client.ID,
		ClientName:    client.Name,
		RedirectURI:   redirect,
		State:         state,
		CodeChallenge: challenge,
		Resource:      s.resource,
		Scope:         auth.OAuthScope,
	}, nil
}

// Values são os parâmetros do pedido como o /oauth/authorize os recebe, para validar de novo o que a tela devolve.
func (r *Request) Values() url.Values {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", r.ClientID)
	q.Set("redirect_uri", r.RedirectURI)
	q.Set("code_challenge", r.CodeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("resource", r.Resource)
	q.Set("scope", r.Scope)
	if r.State != "" {
		q.Set("state", r.State)
	}
	return q
}

// single devolve o parâmetro, e false quando ele vem repetido (o OAuth recusa).
func single(q url.Values, name string) (string, bool) {
	v := q[name]
	if len(v) > 1 {
		return "", false
	}
	if len(v) == 0 {
		return "", true
	}
	return v[0], true
}

// successURL é a volta da autorização com o código. A query que o endereço de volta já tinha continua nele.
func (s *Server) successURL(redirect, code, state string) string {
	return s.withParams(redirect, map[string]string{"code": code, "state": state})
}

// errorURL é a volta da autorização com o erro (RFC 6749, seção 4.1.2.1).
func (s *Server) errorURL(redirect, code, description, state string) string {
	return s.withParams(redirect, map[string]string{"error": code, "error_description": description, "state": state})
}

func (s *Server) withParams(redirect string, params map[string]string) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return redirect // não acontece: o endereço já passou pelo MatchRedirect
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	q.Set("iss", s.issuer)
	u.RawQuery = q.Encode()
	return u.String()
}
