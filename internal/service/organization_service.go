package service

import (
	"errors"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"
)

type OrganizationService struct {
	store *store.OrganizationStore
}

func NewOrganizationService(store *store.OrganizationStore) *OrganizationService {
	return &OrganizationService{store: store}
}

func (s *OrganizationService) Create(name string) (*model.Organization, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	org := &model.Organization{Name: name}
	if err := s.store.Create(org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrganizationService) List() ([]model.Organization, error) {
	return s.store.List()
}

func (s *OrganizationService) Get(id string) (*model.Organization, error) {
	return s.store.GetByID(id)
}

func (s *OrganizationService) Update(id, name string) (*model.Organization, error) {
	if name == "" {
		return nil, errors.New("name is required")
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

func (s *OrganizationService) Delete(id string) error {
	hasProjects, err := s.store.HasActiveProjects(id)
	if err != nil {
		return err
	}
	if hasProjects {
		return errors.New("cannot delete organization with active projects")
	}
	return s.store.Delete(id)
}
