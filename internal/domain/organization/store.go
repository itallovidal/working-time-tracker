package organization

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(org *Organization) error {
	return s.db.Create(org).Error
}

func (s *Store) List() ([]Organization, error) {
	var orgs []Organization
	err := s.db.Order("created_at DESC").Find(&orgs).Error
	return orgs, err
}

func (s *Store) GetByID(id string) (*Organization, error) {
	var org Organization
	err := s.db.First(&org, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (s *Store) Update(org *Organization) error {
	return s.db.Save(org).Error
}

func (s *Store) Delete(id string) error {
	return s.db.Delete(&Organization{}, "id = ?", id).Error
}

func (s *Store) HasActiveProjects(orgID string) (bool, error) {
	var count int64
	err := s.db.Table("projects").Where("organization_id = ?", orgID).Count(&count).Error
	return count > 0, err
}
