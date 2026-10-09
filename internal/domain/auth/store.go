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
	PasswordHash     *string // nil: a conta entra só pelo Clerk, sem senha
	ClerkUserID      *string
	Role             string
	IsOwner          bool // só o signup cria o dono, junto com a organização
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

	create := tx.Person.Create().
		SetName(a.Name).
		SetEmail(a.Email).
		SetOrganizationID(org.ID).
		SetNillablePasswordHash(a.PasswordHash).
		SetNillableClerkUserID(a.ClerkUserID).
		SetRole(person.Role(a.Role)).
		SetIsOwner(a.IsOwner)
	p, err := create.Save(ctx)
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

// MarkOnboarded registra que a pessoa viu as boas-vindas. Só grava na primeira vez, para a data ser a de quando ela
// terminou, e repetir a chamada (o modal pode avisar duas vezes) não muda nada.
func (s *Store) MarkOnboarded(personID uuid.UUID, at time.Time) error {
	return s.client.Person.Update().
		Where(person.IDEQ(personID), person.OnboardedAtIsNil()).
		SetOnboardedAt(at).
		Exec(context.Background())
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

func (s *Store) CreateInvite(orgID, createdBy uuid.UUID, email *string, role string, setup *ProjectSetup, clerkInvitationID *string, tokenHash string, expiresAt time.Time) (*Invite, error) {
	q := s.client.Invite.Create().
		SetOrganizationID(orgID).
		SetCreatedByID(createdBy).
		SetRole(invite.Role(role)).
		SetNillableClerkInvitationID(clerkInvitationID).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt)
	if email != nil {
		q = q.SetEmail(*email)
	}
	if setup != nil {
		q = q.SetProjectID(setup.ProjectID).SetNillablePayRateCents(setup.PayRateCents).SetNillableTeamID(setup.TeamID)
		if setup.Preset != "" {
			q = q.SetPreset(setup.Preset)
		}
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
		WithProject().
		WithTeam().
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

// PendingInvitesOfProject lista os convites ainda utilizáveis que levam a pessoa a esse projeto, do mais novo
// para o mais antigo.
func (s *Store) PendingInvitesOfProject(projectID uuid.UUID, now time.Time) ([]Invite, error) {
	invs, err := s.client.Invite.Query().
		Where(invite.ProjectIDEQ(projectID), invite.AcceptedAtIsNil(), invite.ExpiresAtGT(now)).
		WithCreatedBy().
		WithProject().
		WithTeam().
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

// DeleteInvite apaga o convite e devolve o id dele no Clerk (nulo quando não foi criado lá).
func (s *Store) DeleteInvite(id uuid.UUID) (*string, error) {
	ctx := context.Background()
	inv, err := s.client.Invite.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.client.Invite.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return inv.ClerkInvitationID, nil
}

// PendingInvitesOfEmail lista os convites ainda utilizáveis da organização para esse e-mail, menos o dado.
func (s *Store) PendingInvitesOfEmail(orgID uuid.UUID, email string, now time.Time, except uuid.UUID) ([]*ent.Invite, error) {
	return s.client.Invite.Query().
		Where(invite.OrganizationIDEQ(orgID), invite.EmailEQ(email), invite.IDNEQ(except),
			invite.AcceptedAtIsNil(), invite.ExpiresAtGT(now)).
		All(context.Background())
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
		HasPassword:    p.PasswordHash != nil,
		PersonID:       p.ID,
		Name:           p.Name,
		Email:          p.Email,
		Role:           string(p.Role),
		IsOwner:        p.IsOwner,
		Permissions:    append([]string{}, p.Permissions...),
		OrganizationID: p.OrganizationID,

		NeedsOnboarding: p.IsOwner && p.OnboardedAt == nil,
	}
	if org != nil {
		id.OrganizationName = org.Name
		id.OrganizationCurrency = org.Currency
	}
	return id
}

// projectSetupOf é o que o convite leva para um projeto, ou nulo quando é só para a organização.
func projectSetupOf(e *ent.Invite) *ProjectSetup {
	if e.ProjectID == nil {
		return nil
	}
	setup := &ProjectSetup{ProjectID: *e.ProjectID, PayRateCents: e.PayRateCents, TeamID: e.TeamID}
	if e.Preset != nil {
		setup.Preset = *e.Preset
	}
	return setup
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
	if setup := projectSetupOf(e); setup != nil {
		if e.Edges.Project != nil {
			setup.ProjectName = e.Edges.Project.Name
		}
		if e.Edges.Team != nil {
			setup.TeamName = e.Edges.Team.Name
		}
		inv.Project = setup
	}
	return inv
}

// PersonByClerkID devolve a pessoa ligada ao usuário do Clerk, com a organização, ou database.ErrNotFound.
func (s *Store) PersonByClerkID(clerkID string) (*ent.Person, error) {
	p, err := s.client.Person.Query().
		Where(person.ClerkUserIDEQ(clerkID)).
		WithOrganization().
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	return p, err
}

// LinkClerk liga a pessoa ao usuário do Clerk. Só liga uma pessoa que não está ligada a ninguém ou, quando
// replacing não é vazio, que está ligada a esse usuário (o antigo, que já não existe). Devolve false quando a
// pessoa já mudou nesse meio tempo (outra requisição ligou antes).
func (s *Store) LinkClerk(personID uuid.UUID, clerkID, replacing string) (bool, error) {
	q := s.client.Person.Update().Where(person.IDEQ(personID))
	if replacing == "" {
		q = q.Where(person.ClerkUserIDIsNil())
	} else {
		q = q.Where(person.ClerkUserIDEQ(replacing))
	}
	n, err := q.SetClerkUserID(clerkID).Save(context.Background())
	if ent.IsConstraintError(err) {
		return false, ErrAccountExists
	}
	return n == 1, err
}

// PendingInvitesForEmail lista os convites ainda utilizáveis feitos para esse e-mail, com a organização, do mais
// novo para o mais antigo.
func (s *Store) PendingInvitesForEmail(email string, now time.Time) ([]*ent.Invite, error) {
	return s.client.Invite.Query().
		Where(invite.EmailEQ(email), invite.AcceptedAtIsNil(), invite.ExpiresAtGT(now)).
		WithOrganization().
		Order(ent.Desc(invite.FieldCreatedAt)).
		All(context.Background())
}

// InviteByID devolve o convite ainda utilizável, com a organização, ou ErrInviteInvalid.
func (s *Store) InviteByID(id uuid.UUID, now time.Time) (*ent.Invite, error) {
	inv, err := s.client.Invite.Query().
		Where(invite.IDEQ(id), invite.AcceptedAtIsNil(), invite.ExpiresAtGT(now)).
		WithOrganization().
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, ErrInviteInvalid
	}
	return inv, err
}
