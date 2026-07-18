package service

import (
	"errors"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"

	"github.com/google/uuid"
)

type ProjectService struct {
	store *store.ProjectStore
}

func NewProjectService(store *store.ProjectStore) *ProjectService {
	return &ProjectService{store: store}
}

func (s *ProjectService) Create(orgID, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string) (*model.Project, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if sprintDurationDays <= 0 {
		sprintDurationDays = 14
	}
	project := &model.Project{
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

func (s *ProjectService) ListByOrg(orgID string) ([]model.Project, error) {
	return s.store.ListByOrg(orgID)
}

func (s *ProjectService) Get(id string) (*model.Project, error) {
	return s.store.GetByID(id)
}

func (s *ProjectService) Update(id, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string) (*model.Project, error) {
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

func (s *ProjectService) Delete(id string) error {
	return s.store.Delete(id)
}
