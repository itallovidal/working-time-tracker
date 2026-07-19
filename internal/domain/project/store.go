package project

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(project *Project) error {
	return s.db.Create(project).Error
}

func (s *Store) ListByOrg(orgID string) ([]Project, error) {
	var projects []Project
	err := s.db.Where("organization_id = ?", orgID).Order("created_at DESC").Find(&projects).Error
	return projects, err
}

func (s *Store) GetByID(id string) (*Project, error) {
	var project Project
	err := s.db.First(&project, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Store) Update(project *Project) error {
	return s.db.Save(project).Error
}

func (s *Store) Delete(id string) error {
	return s.db.Delete(&Project{}, "id = ?", id).Error
}
