package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type TaskStore struct {
	db *gorm.DB
}

func NewTaskStore(db *gorm.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) Create(task *model.Task) error {
	return s.db.Create(task).Error
}

func (s *TaskStore) ListByProject(projectID string) ([]model.Task, error) {
	var tasks []model.Task
	err := s.db.Where("project_id = ?", projectID).
		Preload("Assignee").
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

func (s *TaskStore) GetByID(id string) (*model.Task, error) {
	var task model.Task
	err := s.db.Preload("Assignee").
		Preload("ExternalIntegration").
		First(&task, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *TaskStore) Update(task *model.Task) error {
	return s.db.Save(task).Error
}

func (s *TaskStore) Delete(id string) error {
	return s.db.Delete(&model.Task{}, "id = ?", id).Error
}
