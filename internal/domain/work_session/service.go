package work_session

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	taskdom "working-time-tracker/internal/domain/task"
)

type Service struct {
	sessionStore *Store
	taskStore    *taskdom.Store
}

func NewService(sessionStore *Store, taskStore *taskdom.Store) *Service {
	return &Service{sessionStore: sessionStore, taskStore: taskStore}
}

func (s *Service) ClockIn(projectID, taskID, personID string) (*WorkSession, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, errors.New("task not found")
	}
	if task.ProjectID.String() != projectID {
		return nil, errors.New("task does not belong to this project")
	}

	_, err = s.sessionStore.GetActiveByPerson(personID)
	if err == nil {
		return nil, errors.New("already clocked in")
	}
	if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	session := &WorkSession{
		TaskID:   uuid.MustParse(taskID),
		PersonID: uuid.MustParse(personID),
		StartAt:  time.Now(),
	}
	if err := s.sessionStore.Create(session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Service) ClockOut(personID string) (*WorkSession, error) {
	active, err := s.sessionStore.GetActiveByPerson(personID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, errors.New("no active session found")
		}
		return nil, err
	}

	now := time.Now()
	active.EndAt = &now
	if err := s.sessionStore.Update(active); err != nil {
		return nil, err
	}
	return active, nil
}

func (s *Service) ListByProject(projectID string, taskID, personID *string) ([]WorkSession, error) {
	return s.sessionStore.ListByProject(projectID, taskID, personID)
}

type TotalTimeResult struct {
	TotalSeconds float64 `json:"total_seconds"`
}

func (s *Service) TotalTime(projectID string, taskID, personID *string) (*TotalTimeResult, error) {
	if taskID != nil && *taskID != "" && personID != nil && *personID != "" {
		total, err := s.sessionStore.TotalDurationByTaskAndPerson(*taskID, *personID)
		if err != nil {
			return nil, err
		}
		return &TotalTimeResult{TotalSeconds: total}, nil
	}
	if taskID != nil && *taskID != "" {
		total, err := s.sessionStore.TotalDurationByTask(*taskID)
		if err != nil {
			return nil, err
		}
		return &TotalTimeResult{TotalSeconds: total}, nil
	}
	if personID != nil && *personID != "" {
		total, err := s.sessionStore.TotalDurationByPerson(*personID, projectID)
		if err != nil {
			return nil, err
		}
		return &TotalTimeResult{TotalSeconds: total}, nil
	}
	return nil, errors.New("task_id or person_id filter is required")
}
