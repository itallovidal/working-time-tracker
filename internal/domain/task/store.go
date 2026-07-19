package task

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(task *Task) error {
	return s.db.Create(task).Error
}

func (s *Store) ListByProject(projectID string) ([]Task, error) {
	var tasks []Task
	err := s.db.Where("project_id = ?", projectID).
		Preload("Assignee").
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

func (s *Store) GetByID(id string) (*Task, error) {
	var task Task
	err := s.db.Preload("Assignee").
		Preload("ExternalIntegration").
		First(&task, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Store) Update(task *Task) error {
	return s.db.Save(task).Error
}

func (s *Store) Delete(id string) error {
	return s.db.Delete(&Task{}, "id = ?", id).Error
}
