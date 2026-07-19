package service

import (
	"errors"
	"time"
	"working-time-tracker/internal/integration"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"

	"github.com/google/uuid"
)

type TaskService struct {
	taskStore        *store.TaskStore
	membershipStore  *store.TeamMembershipStore
	integrationSvc   *IntegrationService
}

func NewTaskService(taskStore *store.TaskStore, membershipStore *store.TeamMembershipStore, integrationSvc *IntegrationService) *TaskService {
	return &TaskService{taskStore: taskStore, membershipStore: membershipStore, integrationSvc: integrationSvc}
}

func (s *TaskService) Create(projectID, name, description, assigneeID string, deadline *time.Time) (*model.Task, error) {
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

	task := &model.Task{
		ProjectID:  uuid.MustParse(projectID),
		Name:       name,
		Description: description,
		AssigneeID: uuid.MustParse(assigneeID),
		Deadline:   dl,
	}
	if err := s.taskStore.Create(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(task.ID.String())
}

func (s *TaskService) ListByProject(projectID string) ([]model.Task, error) {
	return s.taskStore.ListByProject(projectID)
}

func (s *TaskService) Get(id string) (*model.Task, error) {
	return s.taskStore.GetByID(id)
}

func (s *TaskService) Update(id, name, description string, assigneeID *string, deadline *time.Time) (*model.Task, error) {
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

func (s *TaskService) Delete(id string) error {
	return s.taskStore.Delete(id)
}

func (s *TaskService) LinkExternalItem(taskID, integrationID, externalItemID, externalItemURL string) (*model.Task, error) {
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

func (s *TaskService) UnlinkExternalItem(taskID string) (*model.Task, error) {
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

func (s *TaskService) GetExternalDetails(taskID string) (*integration.ExternalDetailsResult, error) {
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
