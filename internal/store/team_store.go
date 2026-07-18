package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type TeamStore struct {
	db *gorm.DB
}

func NewTeamStore(db *gorm.DB) *TeamStore {
	return &TeamStore{db: db}
}

func (s *TeamStore) Create(team *model.Team) error {
	return s.db.Create(team).Error
}

func (s *TeamStore) ListByProject(projectID string) ([]model.Team, error) {
	var teams []model.Team
	err := s.db.Where("project_id = ?", projectID).Order("created_at DESC").Find(&teams).Error
	return teams, err
}

func (s *TeamStore) GetByID(id string) (*model.Team, error) {
	var team model.Team
	err := s.db.First(&team, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (s *TeamStore) Update(team *model.Team) error {
	return s.db.Save(team).Error
}

func (s *TeamStore) Delete(id string) error {
	return s.db.Delete(&model.Team{}, "id = ?", id).Error
}
