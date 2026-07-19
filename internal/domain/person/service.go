package person

import (
	"errors"

	"github.com/google/uuid"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(orgID, name, email string) (*Person, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if email == "" {
		return nil, errors.New("email is required")
	}
	exists, err := s.store.ExistsByEmailInOrg(email, orgID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("email already exists in this organization")
	}
	person := &Person{
		Name:           name,
		Email:          email,
		OrganizationID: uuid.MustParse(orgID),
	}
	if err := s.store.Create(person); err != nil {
		return nil, err
	}
	return person, nil
}

func (s *Service) ListByOrg(orgID string) ([]Person, error) {
	return s.store.ListByOrg(orgID)
}

func (s *Service) Get(id string) (*Person, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id, name, email string) (*Person, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if email == "" {
		return nil, errors.New("email is required")
	}
	person, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	person.Name = name
	person.Email = email
	if err := s.store.Update(person); err != nil {
		return nil, err
	}
	return person, nil
}
