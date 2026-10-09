package person

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/organization"
	"working-time-tracker/ent/person"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/projectaccess"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(p *Person) error {
	q := s.client.Person.Create().
		SetName(p.Name).
		SetEmail(p.Email).
		SetOrganizationID(p.OrganizationID)
	if p.Role != "" {
		q = q.SetRole(person.Role(p.Role))
	}
	created, err := q.Save(context.Background())
	if err != nil {
		return err
	}
	p.ID = created.ID
	p.Role = string(created.Role)
	p.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) ListByOrg(orgID string) ([]Person, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	persons, err := s.client.Person.Query().
		Where(person.OrganizationIDEQ(uid)).
		Order(ent.Asc(person.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainPersons(persons), nil
}

// ListByOrgScoped lista as pessoas da organização que quem pede pode ver: com viewer, só ele e quem está em
// algum projeto dele; nil lista todas.
func (s *Store) ListByOrgScoped(orgID string, viewer *uuid.UUID) ([]Person, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	q := s.client.Person.Query().Where(person.OrganizationIDEQ(uid))
	if viewer != nil {
		q = q.Where(projectaccess.ColleaguesOf(*viewer))
	}
	persons, err := q.Order(ent.Asc(person.FieldName)).All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainPersons(persons), nil
}

// Visible diz se a pessoa é uma das que viewer pode ver (ele mesmo ou quem divide projeto com ele).
func (s *Store) Visible(personID, viewer uuid.UUID) (bool, error) {
	return s.client.Person.Query().Where(person.IDEQ(personID), projectaccess.ColleaguesOf(viewer)).Exist(context.Background())
}

func (s *Store) GetByID(id string) (*Person, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	p, err := s.client.Person.Get(context.Background(), uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainPerson(p), nil
}

func (s *Store) Update(p *Person) error {
	_, err := s.client.Person.UpdateOneID(p.ID).
		SetName(p.Name).
		SetEmail(p.Email).
		Save(context.Background())
	return err
}

// SetRole muda o papel numa transação que trava a linha da organização. Assim,
// duas mudanças de papel na mesma org rodam uma de cada vez, e a contagem de
// admins já enxerga a mudança da outra. A trava fica na organização porque o
// Postgres não aceita FOR UPDATE junto com count().
func (s *Store) SetRole(id, role string) (*Person, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func(err error) (*Person, error) {
		tx.Rollback()
		return nil, err
	}

	p, err := tx.Person.Get(ctx, uid)
	if ent.IsNotFound(err) {
		return rollback(database.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if _, err := tx.Organization.Query().Where(organization.IDEQ(p.OrganizationID)).ForUpdate().Only(ctx); err != nil {
		return rollback(err)
	}
	// Relê depois da trava: o papel pode ter mudado enquanto esta transação esperava.
	if p, err = tx.Person.Get(ctx, uid); err != nil {
		return rollback(err)
	}

	if p.IsOwner && role == RoleMember {
		return rollback(ErrOwnerRole)
	}
	if p.Role == person.RoleAdmin && role == RoleMember {
		admins, err := tx.Person.Query().
			Where(person.OrganizationIDEQ(p.OrganizationID), person.RoleEQ(person.RoleAdmin)).
			Count(ctx)
		if err != nil {
			return rollback(err)
		}
		if admins <= 1 {
			return rollback(ErrLastAdmin)
		}
	}

	if p, err = tx.Person.UpdateOneID(uid).SetRole(person.Role(role)).Save(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return toDomainPerson(p), nil
}

// SetPermissions grava as permissões da organização da pessoa.
func (s *Store) SetPermissions(id string, permissions []string) (*Person, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, database.ErrNotFound
	}
	p, err := s.client.Person.UpdateOneID(uid).SetPermissions(permissions).Save(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomainPerson(p), nil
}

// SetWeeklyHours grava a jornada semanal da pessoa; nil apaga.
func (s *Store) SetWeeklyHours(id string, hours *int) (*Person, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, database.ErrNotFound
	}
	q := s.client.Person.UpdateOneID(uid)
	if hours != nil {
		q = q.SetWeeklyHours(*hours)
	} else {
		q = q.ClearWeeklyHours()
	}
	p, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomainPerson(p), nil
}

// EmailInUse diz se o email já pertence a outra pessoa. exceptID permite
// ignorar a própria pessoa num update.
func (s *Store) EmailInUse(email string, exceptID *uuid.UUID) (bool, error) {
	q := s.client.Person.Query().Where(person.EmailEQ(email))
	if exceptID != nil {
		q = q.Where(person.IDNEQ(*exceptID))
	}
	count, err := q.Count(context.Background())
	return count > 0, err
}

// FindByEmailInOrg acha a pessoa da organização com este e-mail, sem diferenciar maiúsculas, ou
// database.ErrNotFound. O e-mail é único no sistema inteiro: sem o filtro da organização, o e-mail
// público de um usuário do GitHub ligaria a tarefa a alguém de outra empresa.
func (s *Store) FindByEmailInOrg(orgID uuid.UUID, email string) (*Person, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, database.ErrNotFound
	}
	p, err := s.client.Person.Query().
		Where(person.OrganizationIDEQ(orgID), person.EmailEqualFold(email)).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainPerson(p), nil
}

func toDomainPerson(e *ent.Person) *Person {
	if e == nil {
		return nil
	}
	return &Person{
		ID:             e.ID,
		Name:           e.Name,
		Email:          e.Email,
		OrganizationID: e.OrganizationID,
		Role:           string(e.Role),
		IsOwner:        e.IsOwner,
		Permissions:    append([]string{}, e.Permissions...),
		WeeklyHours:    e.WeeklyHours,
		CreatedAt:      e.CreatedAt,
	}
}

func toDomainPersons(es []*ent.Person) []Person {
	result := make([]Person, len(es))
	for i, e := range es {
		result[i] = *toDomainPerson(e)
	}
	return result
}
