package person

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(person *Person) error {
	return s.db.Create(person).Error
}

func (s *Store) ListByOrg(orgID string) ([]Person, error) {
	var persons []Person
	err := s.db.Where("organization_id = ?", orgID).Order("created_at DESC").Find(&persons).Error
	return persons, err
}

func (s *Store) GetByID(id string) (*Person, error) {
	var person Person
	err := s.db.First(&person, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &person, nil
}

func (s *Store) Update(person *Person) error {
	return s.db.Save(person).Error
}

func (s *Store) ExistsByEmailInOrg(email, orgID string) (bool, error) {
	var count int64
	err := s.db.Model(&Person{}).
		Where("email = ? AND organization_id = ?", email, orgID).
		Count(&count).Error
	return count > 0, err
}
