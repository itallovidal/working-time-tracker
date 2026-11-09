package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	"working-time-tracker/ent/oauthaccesstoken"
	"working-time-tracker/ent/oauthgrant"
	"working-time-tracker/ent/person"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/database"
)

// O OAuth do servidor MCP, do lado das concessões: o que o claude.ai (e o Claude Desktop e o celular, que usam a
// conexão dele) e o Claude Code recebem quando a pessoa autoriza na tela de consentimento. O protocolo (as rotas, a
// identidade dos apps, a validação do pedido) mora em internal/oauth; aqui ficam os códigos e os tokens.
//
// Uma concessão (oauth_grants) é um app conectado. Ela nasce com o código de autorização, que vale dois minutos e uma vez
// só; o app o troca pelo token de renovação e pelo primeiro token de acesso. O token de acesso vale uma hora e só abre o
// /mcp; o de renovação gira a cada uso e cada uso empurra a validade da conexão para daqui a 30 dias. Quem passa 30 dias
// sem usar precisa autorizar de novo. Revogar no perfil ou trocar a senha corta na hora.

const (
	// OAuthAccessPrefix e OAuthRefreshPrefix começam os tokens, para quem os vê (e os scanners de segredo) saber o que
	// são. Nenhum dos dois começa com APITokenPrefix ("wtt_"), então o /mcp sabe de qual tipo é cada token.
	OAuthAccessPrefix  = "wtta_"
	OAuthRefreshPrefix = "wttr_"
	// OAuthCodeTTL é quanto o código de autorização vale: o app o troca logo depois de a pessoa autorizar.
	OAuthCodeTTL = 2 * time.Minute
	// OAuthAccessTTL é quanto o token de acesso vale. O app pega outro com o token de renovação.
	OAuthAccessTTL = time.Hour
	// OAuthRefreshTTL é quanto a conexão aguenta sem uso: cada renovação a empurra.
	OAuthRefreshTTL = 30 * 24 * time.Hour
	// OAuthReplayWindow é a folga para dois processos que renovam o mesmo token ao mesmo tempo (o Claude Code divide a
	// credencial entre as janelas abertas): dentro dela, o token anterior recebe o mesmo sucessor; depois, é reuso e
	// derruba a concessão.
	OAuthReplayWindow = time.Minute
	// OAuthGrantMax é quantos apps conectados uma pessoa pode ter. Passando dele, sai o que está há mais tempo sem uso.
	OAuthGrantMax = 10
	// OAuthScope é o único escopo: tudo o que o /mcp faz.
	OAuthScope = "mcp"
	// oauthTouchEvery é o intervalo mínimo entre duas gravações do último uso, como no token pessoal.
	oauthTouchEvery = time.Minute
)

var (
	// ErrOAuthGrantInvalid é o invalid_grant do OAuth: o código ou o token de renovação não existe, venceu, já foi
	// usado ou não é do app que o manda.
	ErrOAuthGrantInvalid = errors.New("oauth: invalid grant")
	// ErrOAuthTargetInvalid é o invalid_target: o recurso pedido não é o dos tokens.
	ErrOAuthTargetInvalid = errors.New("oauth: invalid target")
)

// OAuthGrantRequest é o que a tela de consentimento autoriza, já conferido pelo internal/oauth.
type OAuthGrantRequest struct {
	ClientID      string
	ClientName    string
	RedirectURI   string
	Scope         string
	Resource      string
	CodeChallenge string
}

// OAuthExchange é a troca do código pelos tokens. Resource vazio é o app que não mandou o recurso: vale o da
// concessão.
type OAuthExchange struct {
	Code         string
	ClientID     string
	RedirectURI  string
	CodeVerifier string
	Resource     string
}

// OAuthTokens é a resposta do /oauth/token.
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // segundos
	Scope        string
}

// OAuthConnection é um app conectado como a lista do perfil o mostra.
type OAuthConnection struct {
	ID         uuid.UUID  `json:"id"`
	ClientID   string     `json:"client_id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

// SetOAuthKey dá a chave que cifra o sucessor guardado para a janela de renovação (OAuthReplayWindow). É a mesma das
// integrações (INTEGRATION_ENCRYPTION_KEY).
func (s *Service) SetOAuthKey(key string) {
	s.oauthKey = key
}

// PKCEMatches confere o code_verifier do app contra o code_challenge da autorização (PKCE S256, RFC 7636): o verificador
// tem de 43 a 128 caracteres do conjunto sem reserva, e o sha256 dele, em base64url sem preenchimento, é o desafio.
func PKCEMatches(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 || strings.Trim(verifier, pkceChars) != "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

// pkceChars são os caracteres que o verificador e o desafio do PKCE podem ter.
const pkceChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// ValidPKCEChallenge diz se o code_challenge tem a forma de um S256: 43 caracteres do conjunto sem reserva.
func ValidPKCEChallenge(challenge string) bool {
	return len(challenge) == 43 && strings.Trim(challenge, pkceChars) == ""
}

// --- store ---

// CreateOAuthGrant grava a concessão nova da pessoa, com o código de autorização. Antes saem as concessões vencidas e
// as pendentes cujo código venceu, e, se ela já tem o máximo, a que está há mais tempo sem uso. A linha da pessoa fica
// travada na transação, para duas autorizações ao mesmo tempo não passarem juntas do limite.
func (s *Store) CreateOAuthGrant(personID uuid.UUID, req OAuthGrantRequest, codeHash string, codeExpires, now time.Time) error {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		_ = tx.Rollback() // o erro que volta é o que motivou desfazer
		return err
	}
	if _, err := tx.Person.Query().Where(person.IDEQ(personID)).ForUpdate().Only(ctx); err != nil {
		if ent.IsNotFound(err) {
			return rollback(database.ErrNotFound)
		}
		return rollback(err)
	}
	if _, err := tx.OAuthGrant.Delete().Where(oauthgrant.PersonIDEQ(personID), oauthgrant.Or(
		oauthgrant.RefreshExpiresAtLTE(now),
		oauthgrant.And(oauthgrant.RefreshHashIsNil(), oauthgrant.CodeExpiresAtLTE(now)),
	)).Exec(ctx); err != nil {
		return rollback(err)
	}
	rows, err := tx.OAuthGrant.Query().Where(oauthgrant.PersonIDEQ(personID)).All(ctx)
	if err != nil {
		return rollback(err)
	}
	if extra := len(rows) - OAuthGrantMax + 1; extra > 0 {
		// Sai quem está há mais tempo sem uso; quem nunca foi usado conta da criação.
		slices.SortFunc(rows, func(a, b *ent.OAuthGrant) int { return lastUse(a).Compare(lastUse(b)) })
		ids := make([]uuid.UUID, extra)
		for i := range ids {
			ids[i] = rows[i].ID
		}
		if _, err := tx.OAuthGrant.Delete().Where(oauthgrant.IDIn(ids...)).Exec(ctx); err != nil {
			return rollback(err)
		}
	}
	if err := tx.OAuthGrant.Create().
		SetPersonID(personID).
		SetClientID(req.ClientID).
		SetClientName(req.ClientName).
		SetRedirectURI(req.RedirectURI).
		SetScope(req.Scope).
		SetResource(req.Resource).
		SetCodeHash(codeHash).
		SetCodeChallenge(req.CodeChallenge).
		SetCodeExpiresAt(codeExpires).
		SetCreatedAt(now).
		Exec(ctx); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func lastUse(g *ent.OAuthGrant) time.Time {
	if g.LastUsedAt != nil {
		return *g.LastUsedAt
	}
	return g.CreatedAt
}

// ClaimOAuthCode gasta o código de autorização e devolve a concessão dele. O UPDATE condicional faz o código valer uma
// vez só, mesmo com duas trocas ao mesmo tempo. Um código que já foi usado derruba a concessão (e os tokens que ela
// deu); um que venceu leva a concessão pendente junto. Os dois são ErrOAuthGrantInvalid, como um que não existe.
func (s *Store) ClaimOAuthCode(codeHash string, now time.Time) (*ent.OAuthGrant, error) {
	ctx := context.Background()
	n, err := s.client.OAuthGrant.Update().
		Where(oauthgrant.CodeHashEQ(codeHash), oauthgrant.CodeUsedAtIsNil(), oauthgrant.CodeExpiresAtGT(now)).
		SetCodeUsedAt(now).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.client.OAuthGrant.Query().Where(oauthgrant.CodeHashEQ(codeHash)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrOAuthGrantInvalid
	}
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if err := s.DeleteOAuthGrantByID(row.ID); err != nil {
			return nil, err
		}
		return nil, ErrOAuthGrantInvalid
	}
	return row, nil
}

// IssueOAuthTokens grava o primeiro token de renovação da concessão e o primeiro de acesso, juntos.
func (s *Store) IssueOAuthTokens(grantID uuid.UUID, refreshHash, accessHash string, refreshExpires, accessExpires, now time.Time) error {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	n, err := tx.OAuthGrant.Update().
		Where(oauthgrant.IDEQ(grantID), oauthgrant.RefreshHashIsNil()).
		SetRefreshHash(refreshHash).
		SetRefreshExpiresAt(refreshExpires).
		SetLastUsedAt(now).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if n == 0 {
		_ = tx.Rollback()
		return ErrOAuthGrantInvalid
	}
	if err := tx.OAuthAccessToken.Create().
		SetGrantID(grantID).
		SetTokenHash(accessHash).
		SetExpiresAt(accessExpires).
		SetCreatedAt(now).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// OAuthRotation é o que a renovação precisa gravar: o sucessor do token de renovação (o hash e, cifrado, ele mesmo,
// para a janela de renovação) e o token de acesso novo.
type OAuthRotation struct {
	ClientID       string
	Resource       string // vazio: o app não mandou o recurso
	RefreshHash    string
	ReplayEnc      string
	AccessHash     string
	RefreshExpires time.Time
	AccessExpires  time.Time
}

// RotateOAuthRefresh troca o token de renovação (pelo hash dele) por r. Devolve o sucessor cifrado quando o token é o
// anterior e chegou dentro da janela (o mesmo sucessor de quem renovou primeiro, com um token de acesso novo), ou ""
// quando a troca é nova. Um token anterior fora da janela é reuso e derruba a concessão; um vencido também.
func (s *Store) RotateOAuthRefresh(hash string, r OAuthRotation, now time.Time) (replay string, err error) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return "", err
	}
	rollback := func(err error) (string, error) {
		_ = tx.Rollback() // o erro que volta é o que motivou desfazer
		return "", err
	}
	// drop apaga a concessão e confirma a transação: o erro que volta é o invalid_grant.
	drop := func(id uuid.UUID) (string, error) {
		if _, err := tx.OAuthGrant.Delete().Where(oauthgrant.IDEQ(id)).Exec(ctx); err != nil {
			return rollback(err)
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return "", ErrOAuthGrantInvalid
	}
	g, err := tx.OAuthGrant.Query().Where(oauthgrant.RefreshHashEQ(hash)).ForUpdate().Only(ctx)
	current := err == nil
	if ent.IsNotFound(err) {
		g, err = tx.OAuthGrant.Query().Where(oauthgrant.PrevRefreshHashEQ(hash)).ForUpdate().Only(ctx)
	}
	if ent.IsNotFound(err) {
		return rollback(ErrOAuthGrantInvalid)
	}
	if err != nil {
		return rollback(err)
	}
	if g.ClientID != r.ClientID {
		return rollback(ErrOAuthGrantInvalid)
	}
	if r.Resource != "" && r.Resource != g.Resource {
		return rollback(ErrOAuthTargetInvalid)
	}
	if g.RefreshExpiresAt == nil || !g.RefreshExpiresAt.After(now) {
		return drop(g.ID)
	}
	if !current {
		if g.RotatedAt == nil || g.RefreshReplayEnc == nil || now.Sub(*g.RotatedAt) > OAuthReplayWindow {
			return drop(g.ID)
		}
		replay = *g.RefreshReplayEnc
	} else {
		if err := tx.OAuthGrant.UpdateOneID(g.ID).
			SetPrevRefreshHash(hash).
			SetRefreshHash(r.RefreshHash).
			SetRefreshReplayEnc(r.ReplayEnc).
			SetRotatedAt(now).
			SetRefreshExpiresAt(r.RefreshExpires).
			SetLastUsedAt(now).
			Exec(ctx); err != nil {
			return rollback(err)
		}
	}
	// Os tokens de acesso vencidos da concessão saem aqui: uma conexão em uso renova pelo menos uma vez por hora.
	if _, err := tx.OAuthAccessToken.Delete().
		Where(oauthaccesstoken.GrantIDEQ(g.ID), oauthaccesstoken.ExpiresAtLTE(now)).
		Exec(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.OAuthAccessToken.Create().
		SetGrantID(g.ID).
		SetTokenHash(r.AccessHash).
		SetExpiresAt(r.AccessExpires).
		SetCreatedAt(now).
		Exec(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return replay, nil
}

// OAuthAccessIdentity devolve a identidade dona do token de acesso, lida agora (um cargo que mudou vale na hora), a
// concessão dele e quando ele vence, ou database.ErrNotFound quando o token não existe ou venceu.
func (s *Store) OAuthAccessIdentity(tokenHash string, now time.Time) (*Identity, *ent.OAuthGrant, time.Time, error) {
	row, err := s.client.OAuthAccessToken.Query().
		Where(oauthaccesstoken.TokenHashEQ(tokenHash), oauthaccesstoken.ExpiresAtGT(now)).
		WithGrant(func(q *ent.OAuthGrantQuery) {
			q.WithPerson(func(q *ent.PersonQuery) { q.WithOrganization() })
		}).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, nil, time.Time{}, database.ErrNotFound
	}
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	g := row.Edges.Grant
	p := g.Edges.Person
	return identityOf(p, p.Edges.Organization), g, row.ExpiresAt, nil
}

// TouchOAuthGrant grava o último uso da concessão quando o anterior é mais velho que before, como no token pessoal.
func (s *Store) TouchOAuthGrant(id uuid.UUID, now, before time.Time) error {
	return s.client.OAuthGrant.Update().
		Where(oauthgrant.IDEQ(id), oauthgrant.Or(oauthgrant.LastUsedAtIsNil(), oauthgrant.LastUsedAtLT(before))).
		SetLastUsedAt(now).
		Exec(context.Background())
}

// ListOAuthGrants devolve os apps conectados da pessoa, do mais novo para o mais antigo. As concessões pendentes (o
// código ainda não foi trocado) e as vencidas ficam de fora: não há conexão.
func (s *Store) ListOAuthGrants(personID uuid.UUID, now time.Time) ([]OAuthConnection, error) {
	rows, err := s.client.OAuthGrant.Query().
		Where(oauthgrant.PersonIDEQ(personID), oauthgrant.RefreshHashNotNil(), oauthgrant.RefreshExpiresAtGT(now)).
		Order(ent.Desc(oauthgrant.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]OAuthConnection, len(rows))
	for i, r := range rows {
		out[i] = OAuthConnection{
			ID: r.ID, ClientID: r.ClientID, Name: r.ClientName,
			CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt, ExpiresAt: *r.RefreshExpiresAt,
		}
	}
	return out, nil
}

// DeleteOAuthGrant apaga uma concessão da pessoa, com os tokens dela. A de outra pessoa é como uma que não existe:
// database.ErrNotFound.
func (s *Store) DeleteOAuthGrant(personID, id uuid.UUID) error {
	n, err := s.client.OAuthGrant.Delete().
		Where(oauthgrant.IDEQ(id), oauthgrant.PersonIDEQ(personID)).
		Exec(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return database.ErrNotFound
	}
	return nil
}

// DeleteOAuthGrantByID apaga a concessão, com os tokens dela.
func (s *Store) DeleteOAuthGrantByID(id uuid.UUID) error {
	_, err := s.client.OAuthGrant.Delete().Where(oauthgrant.IDEQ(id)).Exec(context.Background())
	return err
}

// RevokeOAuthToken apaga o token de acesso com esse hash, ou a concessão inteira quando o hash é de um token de
// renovação (o atual ou o anterior).
func (s *Store) RevokeOAuthToken(hash string) error {
	ctx := context.Background()
	if _, err := s.client.OAuthAccessToken.Delete().Where(oauthaccesstoken.TokenHashEQ(hash)).Exec(ctx); err != nil {
		return err
	}
	_, err := s.client.OAuthGrant.Delete().
		Where(oauthgrant.Or(oauthgrant.RefreshHashEQ(hash), oauthgrant.PrevRefreshHashEQ(hash))).
		Exec(ctx)
	return err
}

// --- service ---

// CreateOAuthGrant grava a autorização que a pessoa deu na tela de consentimento e devolve o código, em claro, que
// volta ao app pelo redirect_uri. A concessão é sempre de quem está logado.
func (s *Service) CreateOAuthGrant(actor *Identity, req OAuthGrantRequest) (string, error) {
	code, hash, err := NewToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	if err := s.store.CreateOAuthGrant(actor.PersonID, req, hash, now.Add(OAuthCodeTTL), now); err != nil {
		return "", err
	}
	return code, nil
}

// ExchangeOAuthCode troca o código pelos tokens. O código se gasta na primeira tentativa: uma troca que não confere
// (outro app, outro redirect_uri, o verificador do PKCE errado, outro recurso) derruba a concessão pendente.
func (s *Service) ExchangeOAuthCode(in OAuthExchange) (*OAuthTokens, error) {
	now := s.now()
	g, err := s.store.ClaimOAuthCode(HashToken(in.Code), now)
	if err != nil {
		return nil, err
	}
	refuse := func(cause error) (*OAuthTokens, error) {
		if err := s.store.DeleteOAuthGrantByID(g.ID); err != nil {
			return nil, err
		}
		return nil, cause
	}
	if g.ClientID != in.ClientID || g.RedirectURI != in.RedirectURI || !PKCEMatches(in.CodeVerifier, g.CodeChallenge) {
		return refuse(ErrOAuthGrantInvalid)
	}
	if in.Resource != "" && in.Resource != g.Resource {
		return refuse(ErrOAuthTargetInvalid)
	}
	refresh, refreshHash, err := newOAuthToken(OAuthRefreshPrefix)
	if err != nil {
		return nil, err
	}
	access, accessHash, err := newOAuthToken(OAuthAccessPrefix)
	if err != nil {
		return nil, err
	}
	if err := s.store.IssueOAuthTokens(g.ID, refreshHash, accessHash, now.Add(OAuthRefreshTTL), now.Add(OAuthAccessTTL), now); err != nil {
		return nil, err
	}
	return &OAuthTokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: int(OAuthAccessTTL.Seconds()), Scope: g.Scope}, nil
}

// RefreshOAuthToken troca o token de renovação por um par novo e empurra a validade da conexão. Resource vazio é o app
// que não mandou o recurso (o SDK do Go não manda na renovação).
func (s *Service) RefreshOAuthToken(token, clientID, resource string) (*OAuthTokens, error) {
	if !strings.HasPrefix(token, OAuthRefreshPrefix) {
		return nil, ErrOAuthGrantInvalid
	}
	refresh, refreshHash, err := newOAuthToken(OAuthRefreshPrefix)
	if err != nil {
		return nil, err
	}
	access, accessHash, err := newOAuthToken(OAuthAccessPrefix)
	if err != nil {
		return nil, err
	}
	enc, err := adapter.EncryptConfig(map[string]any{"token": refresh}, s.oauthKey)
	if err != nil {
		return nil, err
	}
	sealed, _ := enc["encrypted_data"].(string)
	now := s.now()
	replay, err := s.store.RotateOAuthRefresh(HashToken(token), OAuthRotation{
		ClientID: clientID, Resource: resource,
		RefreshHash: refreshHash, ReplayEnc: sealed, AccessHash: accessHash,
		RefreshExpires: now.Add(OAuthRefreshTTL), AccessExpires: now.Add(OAuthAccessTTL),
	}, now)
	if err != nil {
		return nil, err
	}
	if replay != "" {
		plain, err := adapter.DecryptConfig(map[string]any{"encrypted_data": replay}, s.oauthKey)
		if err != nil {
			return nil, err
		}
		if refresh, _ = plain["token"].(string); refresh == "" {
			return nil, ErrOAuthGrantInvalid
		}
	}
	return &OAuthTokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: int(OAuthAccessTTL.Seconds()), Scope: OAuthScope}, nil
}

// RevokeOAuthToken é o /oauth/revoke (RFC 7009): apaga o token de acesso, ou a conexão inteira quando o token é o de
// renovação. Um token que não existe não é erro.
func (s *Service) RevokeOAuthToken(token string) error {
	if !strings.HasPrefix(token, OAuthAccessPrefix) && !strings.HasPrefix(token, OAuthRefreshPrefix) {
		return nil
	}
	return s.store.RevokeOAuthToken(HashToken(token))
}

// AuthenticateOAuthToken resolve o token de acesso do Authorization: Bearer do /mcp para a pessoa dona dele, com a hora
// em que ele vence. O token só vale para o recurso em que foi emitido (resource, na forma canônica). Um token que não
// existe, venceu, foi revogado ou é de outro recurso é ErrUnauthenticated.
func (s *Service) AuthenticateOAuthToken(token, resource string) (*Identity, time.Time, error) {
	if !strings.HasPrefix(token, OAuthAccessPrefix) {
		return nil, time.Time{}, ErrUnauthenticated
	}
	now := s.now()
	id, g, expires, err := s.store.OAuthAccessIdentity(HashToken(token), now)
	if errors.Is(err, database.ErrNotFound) {
		return nil, time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	if g.Resource != resource {
		return nil, time.Time{}, ErrUnauthenticated
	}
	// O último uso é só informação: uma falha ao gravá-lo não impede a entrada.
	_ = s.store.TouchOAuthGrant(g.ID, now, now.Add(-oauthTouchEvery))
	return id, expires, nil
}

// ListOAuthConnections devolve os apps conectados de quem está logado.
func (s *Service) ListOAuthConnections(actor *Identity) ([]OAuthConnection, error) {
	return s.store.ListOAuthGrants(actor.PersonID, s.now())
}

// RevokeOAuthConnection desconecta um app de quem está logado; ele deixa de entrar na hora.
func (s *Service) RevokeOAuthConnection(actor *Identity, id string) error {
	grantID, err := uuid.Parse(id)
	if err != nil {
		return ErrConnectionNotFound
	}
	if err := s.store.DeleteOAuthGrant(actor.PersonID, grantID); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrConnectionNotFound
		}
		return err
	}
	return nil
}

// newOAuthToken gera um token com o prefixo e o hash dele.
func newOAuthToken(prefix string) (token, hash string, err error) {
	raw, _, err := NewToken()
	if err != nil {
		return "", "", err
	}
	token = prefix + raw
	return token, HashToken(token), nil
}

// --- handler ---

// ListOAuthConnections devolve os apps conectados (OAuth do MCP) de quem está logado.
func (h *Handler) ListOAuthConnections(c *echo.Context) error {
	list, err := h.svc.ListOAuthConnections(CurrentPerson(c))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

// RevokeOAuthConnection desconecta um app de quem está logado.
func (h *Handler) RevokeOAuthConnection(c *echo.Context) error {
	if err := h.svc.RevokeOAuthConnection(CurrentPerson(c), c.Param("connectionId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
