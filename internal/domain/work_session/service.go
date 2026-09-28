package work_session

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
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
		return nil, errors.New("tarefa não encontrada")
	}
	if task.ProjectID.String() != projectID {
		return nil, errors.New("a tarefa não é deste projeto")
	}

	personUID, err := uuid.Parse(personID)
	if err != nil {
		return nil, errors.New("pessoa não encontrada")
	}
	inOrg, err := s.sessionStore.PersonInProjectOrganization(personID, projectID)
	if err != nil {
		return nil, err
	}
	if !inOrg {
		return nil, errors.New("pessoa não encontrada nesta organização")
	}

	_, err = s.sessionStore.GetActiveByPerson(personID)
	if err == nil {
		return nil, errors.New("já existe um ponto aberto para esta pessoa; pare a sessão atual antes de iniciar outra")
	}
	if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	session := &WorkSession{
		TaskID:   task.ID,
		PersonID: personUID,
		StartAt:  time.Now(),
	}
	if err := s.sessionStore.Create(session); err != nil {
		// Duas requisições simultâneas passam pela checagem acima; o índice
		// one_active_session barra a segunda aqui.
		if ent.IsConstraintError(err) {
			return nil, errors.New("já existe um ponto aberto para esta pessoa; pare a sessão atual antes de iniciar outra")
		}
		return nil, err
	}
	return session, nil
}

// Active devolve a sessão aberta da pessoa, ou nil quando ela não está com o ponto aberto.
func (s *Service) Active(personID string) (*WorkSession, error) {
	active, err := s.sessionStore.GetActiveByPerson(personID)
	if errors.Is(err, database.ErrNotFound) {
		return nil, nil
	}
	return active, err
}

func (s *Service) ClockOut(personID string) (*WorkSession, error) {
	active, err := s.sessionStore.GetActiveByPerson(personID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, errors.New("não há ponto aberto para esta pessoa")
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
	return nil, errors.New("filtre por task_id ou person_id")
}
