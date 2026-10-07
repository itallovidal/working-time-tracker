package team

import (
	"github.com/google/uuid"
)

type MembershipService struct {
	store *MembershipStore
}

func NewMembershipService(store *MembershipStore) *MembershipService {
	return &MembershipService{store: store}
}

func (s *MembershipService) Add(teamID, personID string) (*TeamMembership, error) {
	sameOrg, err := s.store.SameOrganization(teamID, personID)
	if err != nil {
		return nil, err
	}
	if !sameOrg {
		return nil, ErrPersonNotInOrg
	}
	hasRate, err := s.store.HasRate(teamID, personID)
	if err != nil {
		return nil, err
	}
	if !hasRate {
		return nil, ErrNoRate
	}
	exists, err := s.store.Exists(teamID, personID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadyMember
	}
	membership := &TeamMembership{
		TeamID:   uuid.MustParse(teamID),
		PersonID: uuid.MustParse(personID),
	}
	if err := s.store.Add(membership); err != nil {
		return nil, err
	}
	return membership, nil
}

func (s *MembershipService) Remove(teamID, personID string) error {
	return s.store.Remove(teamID, personID)
}

func (s *MembershipService) ListPersonsInProject(projectID string) ([]Person, error) {
	return s.store.ListPersonsInProject(projectID)
}

func (s *MembershipService) ListByTeam(teamID string) ([]TeamMembership, error) {
	return s.store.ListByTeam(teamID)
}
