package team

import (
	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
)

// ids confere os dois IDs. Um time malformado vale como "não encontrado"; uma pessoa malformada é erro do campo
// person_id (400), em vez de falha interna.
func ids(teamID, personID string) (uuid.UUID, uuid.UUID, error) {
	tid, err := uuid.Parse(teamID)
	if err != nil {
		return uuid.Nil, uuid.Nil, database.ErrNotFound
	}
	pid, err := uuid.Parse(personID)
	if err != nil {
		return uuid.Nil, uuid.Nil, apperr.ErrFieldInvalid.With("field", "person_id")
	}
	return tid, pid, nil
}

type MembershipService struct {
	store *MembershipStore
}

func NewMembershipService(store *MembershipStore) *MembershipService {
	return &MembershipService{store: store}
}

func (s *MembershipService) Add(teamID, personID string) (*TeamMembership, error) {
	tid, pid, err := ids(teamID, personID)
	if err != nil {
		return nil, err
	}
	sameOrg, err := s.store.SameOrganization(teamID, personID)
	if err != nil {
		return nil, err
	}
	if !sameOrg {
		return nil, ErrPersonNotInOrg.With("field", "person_id")
	}
	hasRate, err := s.store.HasRate(teamID, personID)
	if err != nil {
		return nil, err
	}
	if !hasRate {
		return nil, ErrNoRate.With("field", "person_id")
	}
	exists, err := s.store.Exists(teamID, personID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadyMember.With("field", "person_id")
	}
	membership := &TeamMembership{TeamID: tid, PersonID: pid}
	if err := s.store.Add(membership); err != nil {
		// Duas chamadas ao mesmo tempo passam pelo Exists e a segunda bate na chave única: é o mesmo caso.
		if ent.IsConstraintError(err) {
			return nil, ErrAlreadyMember.With("field", "person_id")
		}
		return nil, err
	}
	return membership, nil
}

func (s *MembershipService) Remove(teamID, personID string) error {
	if _, _, err := ids(teamID, personID); err != nil {
		return err
	}
	return s.store.Remove(teamID, personID)
}

func (s *MembershipService) ListPersonsInProject(projectID string) ([]Person, error) {
	return s.store.ListPersonsInProject(projectID)
}

func (s *MembershipService) ListByTeam(teamID string) ([]TeamMembership, error) {
	return s.store.ListByTeam(teamID)
}
