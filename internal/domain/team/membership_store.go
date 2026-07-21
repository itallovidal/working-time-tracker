package team

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/team"
	enttm "working-time-tracker/ent/teammembership"
)

type MembershipStore struct {
	client *ent.Client
}

func NewMembershipStore(client *ent.Client) *MembershipStore {
	return &MembershipStore{client: client}
}

func (s *MembershipStore) Add(m *TeamMembership) error {
	_, err := s.client.TeamMembership.Create().
		SetPersonID(m.PersonID).
		SetTeamID(m.TeamID).
		Save(context.Background())
	return err
}

func (s *MembershipStore) Remove(teamID, personID string) error {
	tuid, err := uuid.Parse(teamID)
	if err != nil {
		return err
	}
	puid, err := uuid.Parse(personID)
	if err != nil {
		return err
	}
	_, err = s.client.TeamMembership.Delete().
		Where(enttm.TeamIDEQ(tuid), enttm.PersonIDEQ(puid)).
		Exec(context.Background())
	return err
}

func (s *MembershipStore) ListByTeam(teamID string) ([]TeamMembership, error) {
	tuid, err := uuid.Parse(teamID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.client.TeamMembership.Query().
		Where(enttm.TeamIDEQ(tuid)).
		WithPerson().
		Order(ent.Desc(enttm.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainMemberships(memberships), nil
}

func (s *MembershipStore) Exists(teamID, personID string) (bool, error) {
	tuid, err := uuid.Parse(teamID)
	if err != nil {
		return false, err
	}
	puid, err := uuid.Parse(personID)
	if err != nil {
		return false, err
	}
	count, err := s.client.TeamMembership.Query().
		Where(enttm.TeamIDEQ(tuid), enttm.PersonIDEQ(puid)).
		Count(context.Background())
	return count > 0, err
}

func (s *MembershipStore) IsPersonInProject(personID, projectID string) (bool, error) {
	puid, err := uuid.Parse(personID)
	if err != nil {
		return false, err
	}
	prjid, err := uuid.Parse(projectID)
	if err != nil {
		return false, err
	}
	count, err := s.client.TeamMembership.Query().
		Where(enttm.PersonIDEQ(puid)).
		QueryTeam().
		Where(team.ProjectIDEQ(prjid)).
		Count(context.Background())
	return count > 0, err
}

func toDomainMembership(e *ent.TeamMembership) TeamMembership {
	m := TeamMembership{
		PersonID:  e.PersonID,
		TeamID:    e.TeamID,
		CreatedAt: e.CreatedAt,
	}
	if e.Edges.Person != nil {
		m.Person = &Person{
			ID:    e.Edges.Person.ID,
			Name:  e.Edges.Person.Name,
			Email: e.Edges.Person.Email,
		}
	}
	return m
}

func toDomainMemberships(es []*ent.TeamMembership) []TeamMembership {
	result := make([]TeamMembership, len(es))
	for i, e := range es {
		result[i] = toDomainMembership(e)
	}
	return result
}
