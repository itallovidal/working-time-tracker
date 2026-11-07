package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	"working-time-tracker/ent/apitoken"
	"working-time-tracker/ent/person"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/validate"
)

// Os tokens pessoais do Claude. A pessoa gera um no perfil e o entrega ao Claude Code, que o manda em
// Authorization: Bearer para o servidor MCP (/mcp) e age em nome dela. O token só vale no /mcp: a API continua só com o
// cookie, então um token não cria outro token nem mexe na conta.

const (
	// APITokenPrefix começa todo token, para quem o vê (e os scanners de segredo) saber o que é.
	APITokenPrefix = "wtt_"
	// APITokenMax é quantos tokens válidos uma pessoa pode ter de uma vez.
	APITokenMax = 10
	// APITokenDefaultDays é a validade de quem não escolhe outra.
	APITokenDefaultDays = 90
	// apiTokenShown é quantos caracteres do token, depois do prefixo, a lista mostra para reconhecê-lo.
	apiTokenShown = 6
	// apiTokenTouchEvery é o intervalo mínimo entre duas gravações do último uso: o Claude chama o /mcp muitas vezes
	// seguidas, e uma escrita por chamada não diria nada a mais.
	apiTokenTouchEvery = time.Minute
)

// APITokenDays são as validades que se pode escolher, em dias. Não há "nunca": todo token vence.
var APITokenDays = []int{30, 90, 365}

// APIToken é um token como a lista do perfil o mostra. O token em si nunca volta, só o começo dele (Prefix).
type APIToken struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// APITokenInput é o pedido de um token novo. Days zero é a validade padrão.
type APITokenInput struct {
	Name string `json:"name"`
	Days int    `json:"days"`
}

// APITokenCreated é a resposta da criação: a única vez em que o token aparece em claro.
type APITokenCreated struct {
	*APIToken
	Token string `json:"token"`
}

// --- store ---

// CreateAPIToken grava o token da pessoa se ela ainda cabe no limite. Os vencidos dela saem antes e não contam. A linha
// da pessoa fica travada na transação, para duas criações ao mesmo tempo não passarem juntas do limite.
func (s *Store) CreateAPIToken(personID uuid.UUID, name, hash, prefix string, expiresAt, now time.Time) (*APIToken, error) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func(err error) (*APIToken, error) {
		_ = tx.Rollback() // o erro que volta é o que motivou desfazer
		return nil, err
	}
	if _, err := tx.Person.Query().Where(person.IDEQ(personID)).ForUpdate().Only(ctx); err != nil {
		if ent.IsNotFound(err) {
			return rollback(database.ErrNotFound)
		}
		return rollback(err)
	}
	if _, err := tx.APIToken.Delete().
		Where(apitoken.PersonIDEQ(personID), apitoken.ExpiresAtLTE(now)).
		Exec(ctx); err != nil {
		return rollback(err)
	}
	n, err := tx.APIToken.Query().Where(apitoken.PersonIDEQ(personID)).Count(ctx)
	if err != nil {
		return rollback(err)
	}
	if n >= APITokenMax {
		return rollback(ErrTokenLimit.With("max", APITokenMax))
	}
	row, err := tx.APIToken.Create().
		SetPersonID(personID).
		SetName(name).
		SetTokenHash(hash).
		SetPrefix(prefix).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return toDomainAPIToken(row), nil
}

// ListAPITokens devolve os tokens da pessoa, do mais novo para o mais antigo, os vencidos também (a tela os marca).
func (s *Store) ListAPITokens(personID uuid.UUID) ([]APIToken, error) {
	rows, err := s.client.APIToken.Query().
		Where(apitoken.PersonIDEQ(personID)).
		Order(ent.Desc(apitoken.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]APIToken, len(rows))
	for i, r := range rows {
		out[i] = *toDomainAPIToken(r)
	}
	return out, nil
}

// DeleteAPIToken apaga um token da pessoa. O de outra pessoa é como um que não existe: database.ErrNotFound.
func (s *Store) DeleteAPIToken(personID, id uuid.UUID) error {
	n, err := s.client.APIToken.Delete().
		Where(apitoken.IDEQ(id), apitoken.PersonIDEQ(personID)).
		Exec(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return database.ErrNotFound
	}
	return nil
}

// APITokenIdentity devolve a identidade dona do token, lida agora (um cargo que mudou vale na hora), e o token, ou
// database.ErrNotFound quando o token não existe ou venceu.
func (s *Store) APITokenIdentity(tokenHash string, now time.Time) (*Identity, *APIToken, error) {
	row, err := s.client.APIToken.Query().
		Where(apitoken.TokenHashEQ(tokenHash), apitoken.ExpiresAtGT(now)).
		WithPerson(func(q *ent.PersonQuery) { q.WithOrganization() }).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, nil, database.ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	p := row.Edges.Person
	return identityOf(p, p.Edges.Organization), toDomainAPIToken(row), nil
}

// TouchAPIToken grava o último uso do token quando o anterior é mais velho que before (ou não existe). A condição
// fica no UPDATE, então chamadas seguidas gravam uma vez só.
func (s *Store) TouchAPIToken(id uuid.UUID, now, before time.Time) error {
	return s.client.APIToken.Update().
		Where(apitoken.IDEQ(id), apitoken.Or(apitoken.LastUsedAtIsNil(), apitoken.LastUsedAtLT(before))).
		SetLastUsedAt(now).
		Exec(context.Background())
}

func toDomainAPIToken(r *ent.APIToken) *APIToken {
	return &APIToken{
		ID:         r.ID,
		Name:       r.Name,
		Prefix:     r.Prefix,
		ExpiresAt:  r.ExpiresAt,
		LastUsedAt: r.LastUsedAt,
		CreatedAt:  r.CreatedAt,
	}
}

// --- service ---

// CreateAPIToken gera um token para quem está logado. O token em claro só existe no retorno desta chamada.
func (s *Service) CreateAPIToken(actor *Identity, in APITokenInput) (*APIToken, string, error) {
	name, err := requiredText(ErrTokenNameRequired, "name", in.Name, validate.MaxName)
	if err != nil {
		return nil, "", err
	}
	days := in.Days
	if days == 0 {
		days = APITokenDefaultDays
	}
	if !slices.Contains(APITokenDays, days) {
		return nil, "", ErrTokenDaysInvalid.With("field", "days")
	}
	raw, _, err := NewToken()
	if err != nil {
		return nil, "", err
	}
	token := APITokenPrefix + raw
	now := s.now()
	created, err := s.store.CreateAPIToken(actor.PersonID, name, HashToken(token),
		token[:len(APITokenPrefix)+apiTokenShown], now.AddDate(0, 0, days), now)
	if err != nil {
		return nil, "", err
	}
	return created, token, nil
}

// ListAPITokens devolve os tokens de quem está logado.
func (s *Service) ListAPITokens(actor *Identity) ([]APIToken, error) {
	return s.store.ListAPITokens(actor.PersonID)
}

// RevokeAPIToken apaga um token de quem está logado; o Claude que o usava deixa de entrar na hora.
func (s *Service) RevokeAPIToken(actor *Identity, id string) error {
	tokenID, err := uuid.Parse(id)
	if err != nil {
		return ErrTokenNotFound
	}
	if err := s.store.DeleteAPIToken(actor.PersonID, tokenID); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrTokenNotFound
		}
		return err
	}
	return nil
}

// AuthenticateAPIToken resolve o token do Authorization: Bearer do /mcp para a pessoa dona dele, com a hora em que ele
// vence. Um token que não existe, venceu ou foi revogado é ErrUnauthenticated.
func (s *Service) AuthenticateAPIToken(token string) (*Identity, time.Time, error) {
	if !strings.HasPrefix(token, APITokenPrefix) {
		return nil, time.Time{}, ErrUnauthenticated
	}
	now := s.now()
	id, tok, err := s.store.APITokenIdentity(HashToken(token), now)
	if errors.Is(err, database.ErrNotFound) {
		return nil, time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	// O último uso é só informação: uma falha ao gravá-lo não impede a entrada.
	_ = s.store.TouchAPIToken(tok.ID, now, now.Add(-apiTokenTouchEvery))
	return id, tok.ExpiresAt, nil
}

// --- handler ---

// ListAPITokens devolve os tokens do Claude de quem está logado.
func (h *Handler) ListAPITokens(c *echo.Context) error {
	tokens, err := h.svc.ListAPITokens(CurrentPerson(c))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, tokens)
}

// CreateAPIToken gera um token do Claude para quem está logado e o devolve em claro, uma vez só.
func (h *Handler) CreateAPIToken(c *echo.Context) error {
	var body APITokenInput
	if err := c.Bind(&body); err != nil {
		return badBody(c, err)
	}
	tok, token, err := h.svc.CreateAPIToken(CurrentPerson(c), body)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusCreated, APITokenCreated{APIToken: tok, Token: token})
}

// RevokeAPIToken apaga um token do Claude de quem está logado.
func (h *Handler) RevokeAPIToken(c *echo.Context) error {
	if err := h.svc.RevokeAPIToken(CurrentPerson(c), c.Param("tokenId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
