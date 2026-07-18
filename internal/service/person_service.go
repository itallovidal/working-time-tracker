package service

import (
	"errors"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"

	"github.com/google/uuid"
)

type PersonService struct {
	store *store.PersonStore
}

func NewPersonService(store *store.PersonStore) *PersonService {
	return &PersonService{store: store}
}

func (s *PersonService) Create(orgID, name, email string) (*model.Person, error) {
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
	person := &model.Person{
		Name:           name,
		Email:          email,
		OrganizationID: uuid.MustParse(orgID),
	}
	if err := s.store.Create(person); err != nil {
		return nil, err
	}
	return person, nil
}

func (s *PersonService) ListByOrg(orgID string) ([]model.Person, error) {
	return s.store.ListByOrg(orgID)
}

func (s *PersonService) Get(id string) (*model.Person, error) {
	return s.store.GetByID(id)
}

func (s *PersonService) Update(id, name, email string) (*model.Person, error) {
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
