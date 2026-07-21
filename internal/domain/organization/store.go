package organization

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entorg "working-time-tracker/ent/organization"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(org *Organization) error {
	created, err := s.client.Organization.Create().
		SetName(org.Name).
		Save(context.Background())
	if err != nil {
		return err
	}
	org.ID = created.ID
	org.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) List() ([]Organization, error) {
	orgs, err := s.client.Organization.Query().
		Order(ent.Desc(entorg.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainOrgs(orgs), nil
}

func (s *Store) GetByID(id string) (*Organization, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	org, err := s.client.Organization.Get(context.Background(), uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainOrg(org), nil
}

func (s *Store) Update(org *Organization) error {
	_, err := s.client.Organization.UpdateOneID(org.ID).
		SetName(org.Name).
		Save(context.Background())
	return err
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Organization.DeleteOneID(uid).Exec(context.Background())
}

func (s *Store) HasActiveProjects(orgID string) (bool, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return false, err
	}
	count, err := s.client.Project.Query().
		Where(entproject.OrganizationIDEQ(uid)).
		Count(context.Background())
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func toDomainOrg(e *ent.Organization) *Organization {
	if e == nil {
		return nil
	}
	return &Organization{
		ID:        e.ID,
		Name:      e.Name,
		CreatedAt: e.CreatedAt,
	}
}

func toDomainOrgs(es []*ent.Organization) []Organization {
	result := make([]Organization, len(es))
	for i, e := range es {
		result[i] = *toDomainOrg(e)
	}
	return result
}
