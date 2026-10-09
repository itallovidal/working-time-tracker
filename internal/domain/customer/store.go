package customer

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entcustomer "working-time-tracker/ent/customer"
	entorganization "working-time-tracker/ent/organization"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(c *Customer) error {
	created, err := s.client.Customer.Create().
		SetOrganizationID(c.OrganizationID).
		SetName(c.Name).
		SetCountry(c.Country).
		SetDocument(c.Document).
		SetContactName(c.ContactName).
		SetContactEmail(c.ContactEmail).
		SetContactPhone(c.ContactPhone).
		Save(context.Background())
	if err != nil {
		return err
	}
	c.ID = created.ID
	c.CreatedAt = created.CreatedAt
	return nil
}

// ListByOrg lista os clientes em ordem alfabética, com a contagem de projetos.
func (s *Store) ListByOrg(orgID string) ([]Customer, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.client.Customer.Query().
		Where(entcustomer.OrganizationIDEQ(uid)).
		WithProjects().
		Order(ent.Asc(entcustomer.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]Customer, len(customers))
	for i, c := range customers {
		result[i] = *toDomain(c)
	}
	return result, nil
}

func (s *Store) GetByID(id string) (*Customer, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, database.ErrNotFound
	}
	c, err := s.client.Customer.Query().
		Where(entcustomer.IDEQ(uid)).
		WithProjects().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomain(c), nil
}

func (s *Store) Update(c *Customer) error {
	_, err := s.client.Customer.UpdateOneID(c.ID).
		SetName(c.Name).
		SetCountry(c.Country).
		SetDocument(c.Document).
		SetContactName(c.ContactName).
		SetContactEmail(c.ContactEmail).
		SetContactPhone(c.ContactPhone).
		Save(context.Background())
	return err
}

// OrgCountry devolve o país da organização, que é o do cliente novo quando ninguém escolhe outro. Organização que não
// existe é ErrInvalidOrganization.
func (s *Store) OrgCountry(orgID uuid.UUID) (string, error) {
	code, err := s.client.Organization.Query().
		Where(entorganization.IDEQ(orgID)).
		Select(entorganization.FieldCountry).
		String(context.Background())
	if ent.IsNotFound(err) {
		return "", ErrInvalidOrganization
	}
	return code, err
}

func (s *Store) Delete(id uuid.UUID) error {
	return s.client.Customer.DeleteOneID(id).Exec(context.Background())
}

func toDomain(e *ent.Customer) *Customer {
	return &Customer{
		ID:             e.ID,
		OrganizationID: e.OrganizationID,
		Name:           e.Name,
		Country:        e.Country,
		Document:       e.Document,
		ContactName:    e.ContactName,
		ContactEmail:   e.ContactEmail,
		ContactPhone:   e.ContactPhone,
		ProjectCount:   len(e.Edges.Projects),
		CreatedAt:      e.CreatedAt,
	}
}
