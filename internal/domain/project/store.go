package project

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/project"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(p *Project) error {
	created, err := s.client.Project.Create().
		SetOrganizationID(p.OrganizationID).
		SetName(p.Name).
		SetDescription(p.Description).
		SetNillableGithubRepoURL(p.GithubRepoURL).
		SetNillableGitlabRepoURL(p.GitlabRepoURL).
		SetSprintDurationDays(p.SprintDurationDays).
		SetNillableDailyTime(p.DailyTime).
		SetNillableWeeklySyncDay(p.WeeklySyncDay).
		Save(context.Background())
	if err != nil {
		return err
	}
	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) ListByOrg(orgID string) ([]Project, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	projects, err := s.client.Project.Query().
		Where(project.OrganizationIDEQ(uid)).
		Order(ent.Desc(project.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainProjects(projects), nil
}

func (s *Store) GetByID(id string) (*Project, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	p, err := s.client.Project.Get(context.Background(), uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainProject(p), nil
}

func (s *Store) Update(p *Project) error {
	_, err := s.client.Project.UpdateOneID(p.ID).
		SetName(p.Name).
		SetDescription(p.Description).
		SetNillableGithubRepoURL(p.GithubRepoURL).
		SetNillableGitlabRepoURL(p.GitlabRepoURL).
		SetSprintDurationDays(p.SprintDurationDays).
		SetNillableDailyTime(p.DailyTime).
		SetNillableWeeklySyncDay(p.WeeklySyncDay).
		Save(context.Background())
	return err
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Project.DeleteOneID(uid).Exec(context.Background())
}

func toDomainProject(e *ent.Project) *Project {
	if e == nil {
		return nil
	}
	return &Project{
		ID:                 e.ID,
		OrganizationID:     e.OrganizationID,
		Name:               e.Name,
		Description:        e.Description,
		GithubRepoURL:      e.GithubRepoURL,
		GitlabRepoURL:      e.GitlabRepoURL,
		SprintDurationDays: e.SprintDurationDays,
		DailyTime:          e.DailyTime,
		WeeklySyncDay:      e.WeeklySyncDay,
		CreatedAt:          e.CreatedAt,
	}
}

func toDomainProjects(es []*ent.Project) []Project {
	result := make([]Project, len(es))
	for i, e := range es {
		result[i] = *toDomainProject(e)
	}
	return result
}
