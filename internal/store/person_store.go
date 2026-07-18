package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type PersonStore struct {
	db *gorm.DB
}

func NewPersonStore(db *gorm.DB) *PersonStore {
	return &PersonStore{db: db}
}

func (s *PersonStore) Create(person *model.Person) error {
	return s.db.Create(person).Error
}

func (s *PersonStore) ListByOrg(orgID string) ([]model.Person, error) {
	var persons []model.Person
	err := s.db.Where("organization_id = ?", orgID).Order("created_at DESC").Find(&persons).Error
	return persons, err
}

func (s *PersonStore) GetByID(id string) (*model.Person, error) {
	var person model.Person
	err := s.db.First(&person, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &person, nil
}

func (s *PersonStore) Update(person *model.Person) error {
	return s.db.Save(person).Error
}

func (s *PersonStore) ExistsByEmailInOrg(email, orgID string) (bool, error) {
	var count int64
	err := s.db.Model(&model.Person{}).
		Where("email = ? AND organization_id = ?", email, orgID).
		Count(&count).Error
	return count > 0, err
}
