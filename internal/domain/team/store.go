package team

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/team"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(t *Team) error {
	created, err := s.client.Team.Create().
		SetName(t.Name).
		SetProjectID(t.ProjectID).
		Save(context.Background())
	if err != nil {
		return err
	}
	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) ListByProject(projectID string) ([]Team, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	teams, err := s.client.Team.Query().
		Where(team.ProjectIDEQ(uid)).
		Order(ent.Desc(team.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainTeams(teams), nil
}

func (s *Store) GetByID(id string) (*Team, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	t, err := s.client.Team.Get(context.Background(), uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainTeam(t), nil
}

func (s *Store) Update(t *Team) error {
	_, err := s.client.Team.UpdateOneID(t.ID).
		SetName(t.Name).
		Save(context.Background())
	return err
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Team.DeleteOneID(uid).Exec(context.Background())
}

func toDomainTeam(e *ent.Team) *Team {
	if e == nil {
		return nil
	}
	return &Team{
		ID:        e.ID,
		Name:      e.Name,
		ProjectID: e.ProjectID,
		CreatedAt: e.CreatedAt,
	}
}

func toDomainTeams(es []*ent.Team) []Team {
	result := make([]Team, len(es))
	for i, e := range es {
		result[i] = *toDomainTeam(e)
	}
	return result
}
