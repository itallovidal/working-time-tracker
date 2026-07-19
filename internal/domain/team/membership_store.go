package team

import (
	"gorm.io/gorm"
)

type MembershipStore struct {
	db *gorm.DB
}

func NewMembershipStore(db *gorm.DB) *MembershipStore {
	return &MembershipStore{db: db}
}

func (s *MembershipStore) Add(membership *TeamMembership) error {
	return s.db.Create(membership).Error
}

func (s *MembershipStore) Remove(teamID, personID string) error {
	return s.db.Where("team_id = ? AND person_id = ?", teamID, personID).Delete(&TeamMembership{}).Error
}

func (s *MembershipStore) ListByTeam(teamID string) ([]TeamMembership, error) {
	var memberships []TeamMembership
	err := s.db.Where("team_id = ?", teamID).
		Preload("Person").
		Order("created_at DESC").
		Find(&memberships).Error
	return memberships, err
}

func (s *MembershipStore) Exists(teamID, personID string) (bool, error) {
	var count int64
	err := s.db.Model(&TeamMembership{}).
		Where("team_id = ? AND person_id = ?", teamID, personID).
		Count(&count).Error
	return count > 0, err
}

func (s *MembershipStore) IsPersonInProject(personID, projectID string) (bool, error) {
	var count int64
	err := s.db.Model(&TeamMembership{}).
		Joins("JOIN teams ON teams.id = team_memberships.team_id").
		Where("team_memberships.person_id = ? AND teams.project_id = ?", personID, projectID).
		Count(&count).Error
	return count > 0, err
}
