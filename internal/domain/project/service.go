package project

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

func (s *Service) Create(orgID, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string) (*Project, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if sprintDurationDays <= 0 {
		sprintDurationDays = 14
	}
	project := &Project{
		OrganizationID:     uuid.MustParse(orgID),
		Name:               name,
		Description:        description,
		SprintDurationDays: sprintDurationDays,
		DailyTime:          dailyTime,
		WeeklySyncDay:      weeklySyncDay,
	}
	if err := s.store.Create(project); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) ListByOrg(orgID string) ([]Project, error) {
	return s.store.ListByOrg(orgID)
}

func (s *Service) Get(id string) (*Project, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string) (*Project, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	project, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	project.Name = name
	project.Description = description
	if sprintDurationDays > 0 {
		project.SprintDurationDays = sprintDurationDays
	}
	project.DailyTime = dailyTime
	project.WeeklySyncDay = weeklySyncDay
	if err := s.store.Update(project); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}
