package team

import (
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(team *Team) error {
	return s.db.Create(team).Error
}

func (s *Store) ListByProject(projectID string) ([]Team, error) {
	var teams []Team
	err := s.db.Where("project_id = ?", projectID).Order("created_at DESC").Find(&teams).Error
	return teams, err
}

func (s *Store) GetByID(id string) (*Team, error) {
	var team Team
	err := s.db.First(&team, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (s *Store) Update(team *Team) error {
	return s.db.Save(team).Error
}

func (s *Store) Delete(id string) error {
	return s.db.Delete(&Team{}, "id = ?", id).Error
}
