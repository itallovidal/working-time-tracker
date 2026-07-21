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
	created, err := s.client.Person.Create().
		SetName(p.Name).
		SetEmail(p.Email).
		SetOrganizationID(p.OrganizationID).
		Save(context.Background())
	if err != nil {
		return err
	}
	p.ID = created.ID
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
		Order(ent.Desc(person.FieldCreatedAt)).
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

func (s *Store) ExistsByEmailInOrg(email, orgID string) (bool, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return false, err
	}
	count, err := s.client.Person.Query().
		Where(person.EmailEQ(email), person.OrganizationIDEQ(uid)).
		Count(context.Background())
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
