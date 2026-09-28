package person

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/person"
	"working-time-tracker/internal/database"
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

func (s *Store) SetRole(id uuid.UUID, role string) error {
	_, err := s.client.Person.UpdateOneID(id).
		SetRole(person.Role(role)).
		Save(context.Background())
	return err
}

func (s *Store) CountAdmins(orgID uuid.UUID) (int, error) {
	return s.client.Person.Query().
		Where(person.OrganizationIDEQ(orgID), person.RoleEQ(person.RoleAdmin)).
		Count(context.Background())
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
