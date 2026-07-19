package store

import (
	"working-time-tracker/internal/model"

	"gorm.io/gorm"
)

type TeamMembershipStore struct {
	db *gorm.DB
}

func NewTeamMembershipStore(db *gorm.DB) *TeamMembershipStore {
	return &TeamMembershipStore{db: db}
}

func (s *TeamMembershipStore) Add(membership *model.TeamMembership) error {
	return s.db.Create(membership).Error
}

func (s *TeamMembershipStore) Remove(teamID, personID string) error {
	return s.db.Where("team_id = ? AND person_id = ?", teamID, personID).Delete(&model.TeamMembership{}).Error
}

func (s *TeamMembershipStore) ListByTeam(teamID string) ([]model.TeamMembership, error) {
	var memberships []model.TeamMembership
	err := s.db.Where("team_id = ?", teamID).
		Preload("Person").
		Order("created_at DESC").
		Find(&memberships).Error
	return memberships, err
}

func (s *TeamMembershipStore) Exists(teamID, personID string) (bool, error) {
	var count int64
	err := s.db.Model(&model.TeamMembership{}).
		Where("team_id = ? AND person_id = ?", teamID, personID).
		Count(&count).Error
	return count > 0, err
}

func (s *TeamMembershipStore) IsPersonInProject(personID, projectID string) (bool, error) {
	var count int64
	err := s.db.Model(&model.TeamMembership{}).
		Joins("JOIN teams ON teams.id = team_memberships.team_id").
		Where("team_memberships.person_id = ? AND teams.project_id = ?", personID, projectID).
		Count(&count).Error
	return count > 0, err
}
