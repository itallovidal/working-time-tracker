package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type ProjectStore struct {
	db *gorm.DB
}

func NewProjectStore(db *gorm.DB) *ProjectStore {
	return &ProjectStore{db: db}
}

func (s *ProjectStore) Create(project *model.Project) error {
	return s.db.Create(project).Error
}

func (s *ProjectStore) ListByOrg(orgID string) ([]model.Project, error) {
	var projects []model.Project
	err := s.db.Where("organization_id = ?", orgID).Order("created_at DESC").Find(&projects).Error
	return projects, err
}

func (s *ProjectStore) GetByID(id string) (*model.Project, error) {
	var project model.Project
	err := s.db.First(&project, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *ProjectStore) Update(project *model.Project) error {
	return s.db.Save(project).Error
}

func (s *ProjectStore) Delete(id string) error {
	return s.db.Delete(&model.Project{}, "id = ?", id).Error
}
