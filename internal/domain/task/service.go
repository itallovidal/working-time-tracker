package task

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/team"
)

type Service struct {
	taskStore       *Store
	membershipStore *team.MembershipStore
	integrationSvc  *integration.Service
}

func NewService(taskStore *Store, membershipStore *team.MembershipStore, integrationSvc *integration.Service) *Service {
	return &Service{taskStore: taskStore, membershipStore: membershipStore, integrationSvc: integrationSvc}
}

func (s *Service) Create(projectID, name, description, assigneeID string, deadline *time.Time) (*Task, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if assigneeID == "" {
		return nil, errors.New("assignee is required")
	}

	isMember, err := s.membershipStore.IsPersonInProject(assigneeID, projectID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errors.New("assignee must be a team member of this project")
	}

	var dl time.Time
	if deadline != nil {
		dl = *deadline
	} else {
		dl = time.Now().Add(7 * 24 * time.Hour)
	}

	task := &Task{
		ProjectID:   uuid.MustParse(projectID),
		Name:        name,
		Description: description,
		AssigneeID:  uuid.MustParse(assigneeID),
		Deadline:    dl,
	}
	if err := s.taskStore.Create(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(task.ID.String())
}

func (s *Service) ListByProject(projectID string) ([]Task, error) {
	return s.taskStore.ListByProject(projectID)
}

func (s *Service) Get(id string) (*Task, error) {
	return s.taskStore.GetByID(id)
}

func (s *Service) Update(id, name, description string, assigneeID *string, deadline *time.Time) (*Task, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	task, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	task.Name = name
	task.Description = description
	if assigneeID != nil {
		task.AssigneeID = uuid.MustParse(*assigneeID)
	}
	if deadline != nil {
		task.Deadline = *deadline
	}
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(id)
}

func (s *Service) Delete(id string) error {
	return s.taskStore.Delete(id)
}

func (s *Service) LinkExternalItem(taskID, integrationID, externalItemID, externalItemURL string) (*Task, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	eid := uuid.MustParse(integrationID)
	task.ExternalIntegrationID = &eid
	task.ExternalItemID = &externalItemID
	task.ExternalItemURL = &externalItemURL
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(taskID)
}

func (s *Service) UnlinkExternalItem(taskID string) (*Task, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	task.ExternalIntegrationID = nil
	task.ExternalItemID = nil
	task.ExternalItemURL = nil
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(taskID)
}

func (s *Service) GetExternalDetails(taskID string) (*adapter.ExternalDetailsResult, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	if task.ExternalIntegrationID == nil {
		return nil, errors.New("task has no linked external item")
	}
	if s.integrationSvc == nil {
		return nil, errors.New("integrations not available")
	}
	return s.integrationSvc.FetchItemDetails(task.ExternalIntegrationID.String(), *task.ExternalItemID)
}
