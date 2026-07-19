package integration

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(integration *Integration) error {
	return s.db.Create(integration).Error
}

func (s *Store) ListByProject(projectID string) ([]Integration, error) {
	var integrations []Integration
	err := s.db.Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&integrations).Error
	return integrations, err
}

func (s *Store) GetByID(id string) (*Integration, error) {
	var integration Integration
	err := s.db.First(&integration, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

func (s *Store) Update(integration *Integration) error {
	return s.db.Save(integration).Error
}

func (s *Store) Delete(id string) error {
	return s.db.Delete(&Integration{}, "id = ?", id).Error
}
