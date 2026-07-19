package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type WorkSessionStore struct {
	db *gorm.DB
}

func NewWorkSessionStore(db *gorm.DB) *WorkSessionStore {
	return &WorkSessionStore{db: db}
}

func (s *WorkSessionStore) Create(session *model.WorkSession) error {
	return s.db.Create(session).Error
}

func (s *WorkSessionStore) GetActiveByPerson(personID string) (*model.WorkSession, error) {
	var session model.WorkSession
	err := s.db.Where("person_id = ? AND end_at IS NULL", personID).
		Preload("Task").
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *WorkSessionStore) Update(session *model.WorkSession) error {
	return s.db.Save(session).Error
}

func (s *WorkSessionStore) ListByProject(projectID string, taskID, personID *string) ([]model.WorkSession, error) {
	query := s.db.Joins("JOIN tasks ON tasks.id = work_sessions.task_id").
		Where("tasks.project_id = ?", projectID)

	if taskID != nil && *taskID != "" {
		query = query.Where("work_sessions.task_id = ?", *taskID)
	}
	if personID != nil && *personID != "" {
		query = query.Where("work_sessions.person_id = ?", *personID)
	}

	var sessions []model.WorkSession
	err := query.
		Preload("Task").
		Preload("Person").
		Order("work_sessions.start_at DESC").
		Find(&sessions).Error
	return sessions, err
}

type DurationResult struct {
	TotalSeconds float64
}

func (s *WorkSessionStore) TotalDurationByTask(taskID string) (float64, error) {
	var result DurationResult
	err := s.db.Model(&model.WorkSession{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM COALESCE(end_at, NOW()) - start_at)), 0) as total_seconds").
		Where("task_id = ?", taskID).
		Scan(&result).Error
	return result.TotalSeconds, err
}

func (s *WorkSessionStore) TotalDurationByPerson(personID, projectID string) (float64, error) {
	var result DurationResult
	err := s.db.Model(&model.WorkSession{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM COALESCE(end_at, NOW()) - start_at)), 0) as total_seconds").
		Joins("JOIN tasks ON tasks.id = work_sessions.task_id").
		Where("work_sessions.person_id = ? AND tasks.project_id = ?", personID, projectID).
		Scan(&result).Error
	return result.TotalSeconds, err
}

func (s *WorkSessionStore) TotalDurationByTaskAndPerson(taskID, personID string) (float64, error) {
	var result DurationResult
	err := s.db.Model(&model.WorkSession{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM COALESCE(end_at, NOW()) - start_at)), 0) as total_seconds").
		Where("task_id = ? AND person_id = ?", taskID, personID).
		Scan(&result).Error
	return result.TotalSeconds, err
}
