package work_session

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(session *WorkSession) error {
	return s.db.Create(session).Error
}

func (s *Store) GetActiveByPerson(personID string) (*WorkSession, error) {
	var session WorkSession
	err := s.db.Where("person_id = ? AND end_at IS NULL", personID).
		Preload("Task").
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) Update(session *WorkSession) error {
	return s.db.Save(session).Error
}

func (s *Store) ListByProject(projectID string, taskID, personID *string) ([]WorkSession, error) {
	query := s.db.Joins("JOIN tasks ON tasks.id = work_sessions.task_id").
		Where("tasks.project_id = ?", projectID)

	if taskID != nil && *taskID != "" {
		query = query.Where("work_sessions.task_id = ?", *taskID)
	}
	if personID != nil && *personID != "" {
		query = query.Where("work_sessions.person_id = ?", *personID)
	}

	var sessions []WorkSession
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

func (s *Store) TotalDurationByTask(taskID string) (float64, error) {
	var result DurationResult
	err := s.db.Model(&WorkSession{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM COALESCE(end_at, NOW()) - start_at)), 0) as total_seconds").
		Where("task_id = ?", taskID).
		Scan(&result).Error
	return result.TotalSeconds, err
}

func (s *Store) TotalDurationByPerson(personID, projectID string) (float64, error) {
	var result DurationResult
	err := s.db.Model(&WorkSession{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM COALESCE(end_at, NOW()) - start_at)), 0) as total_seconds").
		Joins("JOIN tasks ON tasks.id = work_sessions.task_id").
		Where("work_sessions.person_id = ? AND tasks.project_id = ?", personID, projectID).
		Scan(&result).Error
	return result.TotalSeconds, err
}

func (s *Store) TotalDurationByTaskAndPerson(taskID, personID string) (float64, error) {
	var result DurationResult
	err := s.db.Model(&WorkSession{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM COALESCE(end_at, NOW()) - start_at)), 0) as total_seconds").
		Where("task_id = ? AND person_id = ?", taskID, personID).
		Scan(&result).Error
	return result.TotalSeconds, err
}
