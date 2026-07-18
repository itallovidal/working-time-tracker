package service

import (
	"errors"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"

	"github.com/google/uuid"
)

type TeamMembershipService struct {
	store *store.TeamMembershipStore
}

func NewTeamMembershipService(store *store.TeamMembershipStore) *TeamMembershipService {
	return &TeamMembershipService{store: store}
}

func (s *TeamMembershipService) Add(teamID, personID string) (*model.TeamMembership, error) {
	exists, err := s.store.Exists(teamID, personID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("person is already a member of this team")
	}
	membership := &model.TeamMembership{
		TeamID:   uuid.MustParse(teamID),
		PersonID: uuid.MustParse(personID),
	}
	if err := s.store.Add(membership); err != nil {
		return nil, err
	}
	return membership, nil
}

func (s *TeamMembershipService) Remove(teamID, personID string) error {
	return s.store.Remove(teamID, personID)
}

func (s *TeamMembershipService) ListByTeam(teamID string) ([]model.TeamMembership, error) {
	return s.store.ListByTeam(teamID)
}
