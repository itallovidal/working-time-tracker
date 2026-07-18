package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type OrganizationStore struct {
	db *gorm.DB
}

func NewOrganizationStore(db *gorm.DB) *OrganizationStore {
	return &OrganizationStore{db: db}
}

func (s *OrganizationStore) Create(org *model.Organization) error {
	return s.db.Create(org).Error
}

func (s *OrganizationStore) List() ([]model.Organization, error) {
	var orgs []model.Organization
	err := s.db.Order("created_at DESC").Find(&orgs).Error
	return orgs, err
}

func (s *OrganizationStore) GetByID(id string) (*model.Organization, error) {
	var org model.Organization
	err := s.db.First(&org, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (s *OrganizationStore) Update(org *model.Organization) error {
	return s.db.Save(org).Error
}

func (s *OrganizationStore) Delete(id string) error {
	return s.db.Delete(&model.Organization{}, "id = ?", id).Error
}

func (s *OrganizationStore) HasActiveProjects(orgID string) (bool, error) {
	var count int64
	err := s.db.Model(&model.Project{}).Where("organization_id = ?", orgID).Count(&count).Error
	return count > 0, err
}
