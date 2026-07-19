package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type IntegrationStore struct {
	db *gorm.DB
}

func NewIntegrationStore(db *gorm.DB) *IntegrationStore {
	return &IntegrationStore{db: db}
}

func (s *IntegrationStore) Create(integration *model.Integration) error {
	return s.db.Create(integration).Error
}

func (s *IntegrationStore) ListByProject(projectID string) ([]model.Integration, error) {
	var integrations []model.Integration
	err := s.db.Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&integrations).Error
	return integrations, err
}

func (s *IntegrationStore) GetByID(id string) (*model.Integration, error) {
	var integration model.Integration
	err := s.db.First(&integration, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

func (s *IntegrationStore) Update(integration *model.Integration) error {
	return s.db.Save(integration).Error
}

func (s *IntegrationStore) Delete(id string) error {
	return s.db.Delete(&model.Integration{}, "id = ?", id).Error
}
