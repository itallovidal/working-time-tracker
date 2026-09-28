package organization

import (
	"errors"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(name string) (*Organization, error) {
	if name == "" {
		return nil, errors.New("informe o nome")
	}
	org := &Organization{Name: name}
	if err := s.store.Create(org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *Service) List() ([]Organization, error) {
	return s.store.List()
}

func (s *Service) Get(id string) (*Organization, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id, name string) (*Organization, error) {
	if name == "" {
		return nil, errors.New("informe o nome")
	}
	org, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	org.Name = name
	if err := s.store.Update(org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *Service) Delete(id string) error {
	hasProjects, err := s.store.HasActiveProjects(id)
	if err != nil {
		return err
	}
	if hasProjects {
		return errors.New("exclua todos os projetos antes de excluir a organização")
	}
	return s.store.Delete(id)
}
