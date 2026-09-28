package team

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

func (s *Service) Create(projectID, name string) (*Team, error) {
	if name == "" {
		return nil, errors.New("informe o nome")
	}
	team := &Team{
		ProjectID: uuid.MustParse(projectID),
		Name:      name,
	}
	if err := s.store.Create(team); err != nil {
		return nil, err
	}
	return team, nil
}

func (s *Service) ListByProject(projectID string) ([]Team, error) {
	return s.store.ListByProject(projectID)
}

func (s *Service) Get(id string) (*Team, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id, name string) (*Team, error) {
	if name == "" {
		return nil, errors.New("informe o nome")
	}
	team, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	team.Name = name
	if err := s.store.Update(team); err != nil {
		return nil, err
	}
	return team, nil
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}
