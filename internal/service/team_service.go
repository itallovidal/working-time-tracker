package service

import (
	"errors"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"

	"github.com/google/uuid"
)

type TeamService struct {
	store *store.TeamStore
}

func NewTeamService(store *store.TeamStore) *TeamService {
	return &TeamService{store: store}
}

func (s *TeamService) Create(projectID, name string) (*model.Team, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	team := &model.Team{
		ProjectID: uuid.MustParse(projectID),
		Name:      name,
	}
	if err := s.store.Create(team); err != nil {
		return nil, err
	}
	return team, nil
}

func (s *TeamService) ListByProject(projectID string) ([]model.Team, error) {
	return s.store.ListByProject(projectID)
}

func (s *TeamService) Get(id string) (*model.Team, error) {
	return s.store.GetByID(id)
}

func (s *TeamService) Update(id, name string) (*model.Team, error) {
	if name == "" {
		return nil, errors.New("name is required")
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

func (s *TeamService) Delete(id string) error {
	return s.store.Delete(id)
}
