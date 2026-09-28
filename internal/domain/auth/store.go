package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/invite"
	"working-time-tracker/ent/person"
	"working-time-tracker/ent/session"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// NewAccount é o que o signup e o aceite de convite gravam de uma vez.
type NewAccount struct {
	OrganizationID   *uuid.UUID // nil cria uma organização nova com OrganizationName
	OrganizationName string
	Name             string
	Email            string
	PasswordHash     string
	Role             string
	SessionHash      string
	SessionExpires   time.Time
	InviteID         *uuid.UUID // convite a marcar como aceito na mesma transação
	Now              time.Time
}

// CreateAccount cria (se preciso) a organização, a pessoa e a sessão numa transação.
func (s *Store) CreateAccount(a NewAccount) (*Identity, error) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func(err error) (*Identity, error) {
		tx.Rollback()
		return nil, err
	}

	if a.InviteID != nil {
		// Marca o convite como usado só se ele ainda estiver válido. Se outro aceite
		// chegou antes, nenhuma linha é afetada e este falha.
		n, err := tx.Invite.Update().
			Where(invite.IDEQ(*a.InviteID), invite.AcceptedAtIsNil(), invite.ExpiresAtGT(a.Now)).
			SetAcceptedAt(a.Now).
			Save(ctx)
		if err != nil {
			return rollback(err)
		}
		if n == 0 {
			return rollback(ErrInviteInvalid)
		}
	}

	var org *ent.Organization
	if a.OrganizationID == nil {
		org, err = tx.Organization.Create().SetName(a.OrganizationName).Save(ctx)
	} else {
		org, err = tx.Organization.Get(ctx, *a.OrganizationID)
	}
	if err != nil {
		return rollback(err)
	}

	p, err := tx.Person.Create().
		SetName(a.Name).
		SetEmail(a.Email).
		SetOrganizationID(org.ID).
		SetPasswordHash(a.PasswordHash).
		SetRole(person.Role(a.Role)).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return rollback(ErrEmailInUse)
		}
		return rollback(err)
	}

	if _, err := tx.Session.Create().
		SetPersonID(p.ID).
		SetTokenHash(a.SessionHash).
		SetExpiresAt(a.SessionExpires).
		Save(ctx); err != nil {
		return rollback(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return identityOf(p, org), nil
}

func (s *Store) EmailInUse(email string) (bool, error) {
	n, err := s.client.Person.Query().Where(person.EmailEQ(email)).Count(context.Background())
	return n > 0, err
}

// PersonForLogin devolve a pessoa com a organização carregada, ou database.ErrNotFound.
func (s *Store) PersonForLogin(email string) (*ent.Person, error) {
	p, err := s.client.Person.Query().
		Where(person.EmailEQ(email)).
		WithOrganization().
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	return p, err
}

func (s *Store) PasswordHash(personID uuid.UUID) (string, error) {
	p, err := s.client.Person.Get(context.Background(), personID)
	if err != nil {
		return "", err
	}
	if p.PasswordHash == nil {
		return "", nil
	}
	return *p.PasswordHash, nil
}

func (s *Store) SetPasswordHash(personID uuid.UUID, hash string) error {
	return s.client.Person.UpdateOneID(personID).SetPasswordHash(hash).Exec(context.Background())
}

func (s *Store) CreateSession(personID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	return s.client.Session.Create().
		SetPersonID(personID).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt).
		Exec(context.Background())
}

// SessionIdentity devolve a identidade dona da sessão, ou database.ErrNotFound
// quando a sessão não existe ou expirou.
func (s *Store) SessionIdentity(tokenHash string, now time.Time) (*Identity, error) {
	sess, err := s.client.Session.Query().
		Where(session.TokenHashEQ(tokenHash), session.ExpiresAtGT(now)).
		WithPerson(func(q *ent.PersonQuery) { q.WithOrganization() }).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := sess.Edges.Person
	return identityOf(p, p.Edges.Organization), nil
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.client.Session.Delete().Where(session.TokenHashEQ(tokenHash)).Exec(context.Background())
	return err
}

// DeleteOtherSessions encerra as outras sessões da pessoa, por exemplo depois
// de uma troca de senha.
func (s *Store) DeleteOtherSessions(personID uuid.UUID, keepHash string) error {
	_, err := s.client.Session.Delete().
		Where(session.PersonIDEQ(personID), session.TokenHashNEQ(keepHash)).
		Exec(context.Background())
	return err
}

func (s *Store) DeleteExpiredSessions(now time.Time) error {
	_, err := s.client.Session.Delete().Where(session.ExpiresAtLTE(now)).Exec(context.Background())
	return err
}

func (s *Store) CreateInvite(orgID, createdBy uuid.UUID, email *string, role, tokenHash string, expiresAt time.Time) (*Invite, error) {
	q := s.client.Invite.Create().
		SetOrganizationID(orgID).
		SetCreatedByID(createdBy).
		SetRole(invite.Role(role)).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt)
	if email != nil {
		q = q.SetEmail(*email)
	}
	inv, err := q.Save(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainInvite(inv), nil
}

// PendingInvites lista os convites ainda utilizáveis, do mais novo para o mais antigo.
func (s *Store) PendingInvites(orgID uuid.UUID, now time.Time) ([]Invite, error) {
	invs, err := s.client.Invite.Query().
		Where(invite.OrganizationIDEQ(orgID), invite.AcceptedAtIsNil(), invite.ExpiresAtGT(now)).
		WithCreatedBy().
		Order(ent.Desc(invite.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]Invite, len(invs))
	for i, inv := range invs {
		result[i] = *toDomainInvite(inv)
	}
	return result, nil
}

func (s *Store) DeleteInvite(id uuid.UUID) error {
	err := s.client.Invite.DeleteOneID(id).Exec(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	return err
}

// ValidInvite devolve o convite com a organização, ou ErrInviteInvalid quando o
// token não existe, já foi usado ou expirou.
func (s *Store) ValidInvite(tokenHash string, now time.Time) (*ent.Invite, error) {
	inv, err := s.client.Invite.Query().
		Where(invite.TokenHashEQ(tokenHash), invite.AcceptedAtIsNil(), invite.ExpiresAtGT(now)).
		WithOrganization().
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, ErrInviteInvalid
	}
	return inv, err
}

func identityOf(p *ent.Person, org *ent.Organization) *Identity {
	id := &Identity{
		PersonID:       p.ID,
		Name:           p.Name,
		Email:          p.Email,
		Role:           string(p.Role),
		OrganizationID: p.OrganizationID,
	}
	if org != nil {
		id.OrganizationName = org.Name
	}
	return id
}

func toDomainInvite(e *ent.Invite) *Invite {
	inv := &Invite{
		ID:             e.ID,
		OrganizationID: e.OrganizationID,
		Email:          e.Email,
		Role:           string(e.Role),
		ExpiresAt:      e.ExpiresAt,
		AcceptedAt:     e.AcceptedAt,
		CreatedAt:      e.CreatedAt,
	}
	if e.Edges.CreatedBy != nil {
		inv.CreatedByName = e.Edges.CreatedBy.Name
	}
	return inv
}
