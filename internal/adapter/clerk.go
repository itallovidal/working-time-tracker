package adapter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/invitation"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

// Os erros do Clerk que o domínio entende. O detalhe técnico (o código e o rastro da resposta do Clerk) vai
// junto, embrulhado; o token da pessoa nunca.
var (
	// ErrClerkTokenInvalid: o token não é do Clerk deste app, venceu, é de outra origem ou o usuário não existe
	// mais (ou está bloqueado).
	ErrClerkTokenInvalid = errors.New("clerk: the session token is not valid")
	// ErrClerkUnavailable: o Clerk não respondeu direito (rede, 5xx, limite de requisições).
	ErrClerkUnavailable = errors.New("clerk: the service is unavailable")
	// ErrClerkRejected: o Clerk recusou o pedido (um 4xx), por exemplo um e-mail que ele não aceita.
	ErrClerkRejected = errors.New("clerk: the request was rejected")
)

// clerkLeeway é a folga para o relógio do servidor andar diferente do do Clerk: a sessão dele vale 60 segundos.
const clerkLeeway = 10 * time.Second

// Quanto tempo uma chave pública do Clerk fica guardada, e de quanto em quanto tempo se aceita buscar de novo
// porque apareceu uma chave desconhecida (a troca de chaves é rara; um token com chave inventada não pode
// fazer o servidor bater no Clerk a cada tentativa).
const (
	clerkKeyTTL      = time.Hour
	clerkKeyMinRetry = 30 * time.Second
)

// ClerkIdentity é quem o Clerk diz que a pessoa é.
type ClerkIdentity struct {
	UserID    string
	SessionID string
	// Email é o e-mail primário, em minúsculas. EmailVerified diz se o Clerk o verificou: só um e-mail verificado
	// prova que a pessoa é dona da caixa.
	Email         string
	EmailVerified bool
	Name          string // "Nome Sobrenome", ou vazio
}

// ClerkInviteParams é o convite a criar no Clerk.
type ClerkInviteParams struct {
	Email string
	// RedirectURL é para onde o Clerk leva a pessoa depois de abrir o link do convite, com o __clerk_ticket.
	RedirectURL string
	// Notify pede ao Clerk que mande o e-mail. Falso cria o convite sem mandar nada.
	Notify        bool
	ExpiresInDays int
}

// ClerkInvitation é o convite criado. URL é o link do Clerk (que abre e leva à RedirectURL com o ticket); o
// Clerk o devolve mesmo com Notify falso.
type ClerkInvitation struct {
	ID  string
	URL string
}

// ClerkProvider é o que o sistema usa do Clerk. Os testes trocam por um falso, sem rede.
type ClerkProvider interface {
	// Verify confere o token de sessão do Clerk (assinatura, prazo, origem) e já busca o usuário.
	Verify(ctx context.Context, sessionToken string) (*ClerkIdentity, error)
	// UserExists diz se o usuário ainda existe no Clerk.
	UserExists(ctx context.Context, userID string) (bool, error)
	CreateInvitation(ctx context.Context, p ClerkInviteParams) (*ClerkInvitation, error)
	// RevokeInvitation cancela um convite pendente. Um convite que já não existe conta como cancelado.
	RevokeInvitation(ctx context.Context, id string) error
}

// ClerkConfig são os dados para falar com o Clerk.
type ClerkConfig struct {
	SecretKey string
	// APIURL é o servidor da Backend API, sem o /v1. Vazio é api.clerk.com; só se muda para um servidor fake.
	APIURL string
	// AuthorizedParties são as origens das páginas que pedem o token (o azp dele). Um token sem azp, ou de
	// outra origem, é recusado: sem isso, a sessão de qualquer outro app da mesma instância do Clerk abriria
	// uma sessão aqui.
	AuthorizedParties []string
	HTTPClient        *http.Client
}

// Clerk fala com o Clerk de verdade pelo SDK dele.
type Clerk struct {
	users   *user.Client
	invites *invitation.Client
	jwks    *jwks.Client
	parties []string

	mu        sync.Mutex
	keys      map[string]*clerk.JSONWebKey
	fetchedAt time.Time
}

var _ ClerkProvider = (*Clerk)(nil)

// NewClerk monta o cliente. Cada cliente leva a própria chave (não se usa o clerk.SetKey, que é global).
func NewClerk(cfg ClerkConfig) *Clerk {
	bc := &clerk.BackendConfig{Key: &cfg.SecretKey, HTTPClient: cfg.HTTPClient}
	if bc.HTTPClient == nil {
		bc.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.APIURL != "" {
		// O SDK espera o prefixo da versão na URL base (api.clerk.com/v1); quem configura só diz o servidor.
		base := strings.TrimRight(cfg.APIURL, "/") + "/v1"
		bc.URL = &base
	}
	cc := &clerk.ClientConfig{BackendConfig: *bc}
	return &Clerk{
		users:   user.NewClient(cc),
		invites: invitation.NewClient(cc),
		jwks:    jwks.NewClient(cc),
		parties: cfg.AuthorizedParties,
	}
}

// Configured diz se há com que falar com o Clerk.
func (c *Clerk) Configured() bool { return c != nil && c.users != nil }

// ClerkFrontendAPI é o endereço do Frontend API do Clerk, de onde vem o clerk-js. Está escondido na chave
// pública: depois de pk_test_ ou pk_live_ vem o endereço em base64, com um "$" no fim. Devolve "" se a chave
// não tiver esse formato.
func ClerkFrontendAPI(publishableKey string) string {
	_, encoded, ok := strings.Cut(strings.TrimPrefix(publishableKey, "pk_"), "_")
	if !ok {
		return ""
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil {
		return ""
	}
	host, ok := strings.CutSuffix(string(raw), "$")
	if !ok || host == "" || strings.ContainsAny(host, "/ \t\r\n?#") {
		return ""
	}
	return host
}

func (c *Clerk) authorizedParty(azp string) bool {
	return azp != "" && slices.Contains(c.parties, azp)
}

// key devolve a chave pública do Clerk que assinou o token, de um cache. Uma chave desconhecida faz buscar o
// conjunto de novo, no máximo a cada clerkKeyMinRetry.
func (c *Clerk) key(ctx context.Context, kid string) (*clerk.JSONWebKey, error) {
	if kid == "" {
		return nil, fmt.Errorf("%w: the token has no key id", ErrClerkTokenInvalid)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fresh := time.Since(c.fetchedAt) < clerkKeyTTL
	if k, ok := c.keys[kid]; ok && fresh {
		return k, nil
	}
	if c.keys != nil && time.Since(c.fetchedAt) < clerkKeyMinRetry {
		return nil, fmt.Errorf("%w: unknown key id", ErrClerkTokenInvalid)
	}
	set, err := c.jwks.Get(ctx, &jwks.GetParams{})
	if err != nil {
		return nil, clerkError("fetching the signing keys", err)
	}
	c.keys = make(map[string]*clerk.JSONWebKey, len(set.Keys))
	for _, k := range set.Keys {
		if k != nil {
			c.keys[k.KeyID] = k
		}
	}
	c.fetchedAt = time.Now()
	k, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: unknown key id", ErrClerkTokenInvalid)
	}
	return k, nil
}

func (c *Clerk) Verify(ctx context.Context, sessionToken string) (*ClerkIdentity, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil, fmt.Errorf("%w: empty token", ErrClerkTokenInvalid)
	}
	unverified, err := jwt.Decode(ctx, &jwt.DecodeParams{Token: sessionToken})
	if err != nil {
		return nil, fmt.Errorf("%w: not a JWT", ErrClerkTokenInvalid)
	}
	jwk, err := c.key(ctx, unverified.KeyID)
	if err != nil {
		return nil, err
	}
	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{
		Token:                  sessionToken,
		JWK:                    jwk,
		Leeway:                 clerkLeeway,
		AuthorizedPartyHandler: c.authorizedParty,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClerkTokenInvalid, err)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: no subject", ErrClerkTokenInvalid)
	}

	u, err := c.users.Get(ctx, claims.Subject)
	if err != nil {
		if apiStatus(err) == http.StatusNotFound {
			return nil, fmt.Errorf("%w: the user no longer exists", ErrClerkTokenInvalid)
		}
		return nil, clerkError("fetching the user", err)
	}
	if u.Banned || u.Locked {
		return nil, fmt.Errorf("%w: the user is blocked", ErrClerkTokenInvalid)
	}
	return identityOfClerkUser(u, claims.SessionID), nil
}

func identityOfClerkUser(u *clerk.User, sessionID string) *ClerkIdentity {
	id := &ClerkIdentity{UserID: u.ID, SessionID: sessionID}
	if u.PrimaryEmailAddressID != nil {
		for _, e := range u.EmailAddresses {
			if e != nil && e.ID == *u.PrimaryEmailAddressID {
				id.Email = strings.ToLower(strings.TrimSpace(e.EmailAddress))
				id.EmailVerified = e.Verification != nil && e.Verification.Status == "verified"
				break
			}
		}
	}
	var first, last string
	if u.FirstName != nil {
		first = *u.FirstName
	}
	if u.LastName != nil {
		last = *u.LastName
	}
	id.Name = strings.TrimSpace(first + " " + last)
	return id
}

func (c *Clerk) UserExists(ctx context.Context, userID string) (bool, error) {
	if _, err := c.users.Get(ctx, userID); err != nil {
		if apiStatus(err) == http.StatusNotFound {
			return false, nil
		}
		return false, clerkError("fetching the user", err)
	}
	return true, nil
}

func (c *Clerk) CreateInvitation(ctx context.Context, p ClerkInviteParams) (*ClerkInvitation, error) {
	params := &invitation.CreateParams{
		EmailAddress: p.Email,
		RedirectURL:  &p.RedirectURL,
		Notify:       clerk.Bool(p.Notify),
		// Quem já é usuário do Clerk (entrou antes, sem organização aqui) também pode ser convidado.
		IgnoreExisting: clerk.Bool(true),
	}
	if p.ExpiresInDays > 0 {
		days := int64(p.ExpiresInDays)
		params.ExpiresInDays = &days
	}
	inv, err := c.invites.Create(ctx, params)
	if err != nil {
		return nil, clerkError("creating the invitation", err)
	}
	return &ClerkInvitation{ID: inv.ID, URL: inv.URL}, nil
}

func (c *Clerk) RevokeInvitation(ctx context.Context, id string) error {
	if _, err := c.invites.Revoke(ctx, id); err != nil {
		if apiStatus(err) == http.StatusNotFound {
			return nil
		}
		return clerkError("revoking the invitation", err)
	}
	return nil
}

// apiStatus é o status HTTP de uma resposta de erro do Clerk, ou 0 quando o erro não veio de uma resposta.
func apiStatus(err error) int {
	var resp *clerk.APIErrorResponse
	if errors.As(err, &resp) {
		return resp.HTTPStatusCode
	}
	return 0
}

// clerkError traduz a falha de uma chamada ao Clerk: um 4xx (menos o 429) é um pedido recusado; o resto
// (rede, 5xx, limite) é indisponibilidade. Do erro do SDK só o status e o rastro seguem adiante.
func clerkError(what string, err error) error {
	var resp *clerk.APIErrorResponse
	if errors.As(err, &resp) {
		kind := ErrClerkUnavailable
		if resp.HTTPStatusCode >= 400 && resp.HTTPStatusCode < 500 && resp.HTTPStatusCode != http.StatusTooManyRequests {
			kind = ErrClerkRejected
		}
		return fmt.Errorf("%w: %s: status %d, trace %s", kind, what, resp.HTTPStatusCode, resp.TraceID)
	}
	return fmt.Errorf("%w: %s: %v", ErrClerkUnavailable, what, err)
}
