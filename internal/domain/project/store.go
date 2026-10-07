package project

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entcustomer "working-time-tracker/ent/customer"
	entorg "working-time-tracker/ent/organization"
	"working-time-tracker/ent/project"
	enttask "working-time-tracker/ent/task"
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
		SetNillableWeeklySyncTime(p.WeeklySyncTime).
		SetNillableCustomerMeetingDay(p.CustomerMeetingDay).
		SetNillableCustomerMeetingTime(p.CustomerMeetingTime).
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
	projects, err := withCounts(s.client.Project.Query().
		Where(project.OrganizationIDEQ(uid)).
		WithCustomer()).
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
	p, err := withCounts(s.client.Project.Query().
		Where(project.IDEQ(uid)).
		WithCustomer()).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainProject(p), nil
}

// withCounts carrega o que MemberCount e TaskCount precisam: os times com as
// pessoas, os valores por hora e, das tarefas, só o bastante para contar.
func withCounts(q *ent.ProjectQuery) *ent.ProjectQuery {
	return q.
		WithTeams(func(t *ent.TeamQuery) { t.WithMemberships() }).
		WithAllocations().
		WithTasks(func(t *ent.TaskQuery) { t.Select(enttask.FieldID, enttask.FieldProjectID) })
}

// memberCount conta os colaboradores do projeto: quem está em algum time dele
// ou tem valor por hora nele. Cada pessoa conta uma vez.
func memberCount(e *ent.Project) int {
	people := map[uuid.UUID]bool{}
	for _, team := range e.Edges.Teams {
		for _, m := range team.Edges.Memberships {
			people[m.PersonID] = true
		}
	}
	for _, a := range e.Edges.Allocations {
		people[a.PersonID] = true
	}
	return len(people)
}

func (s *Store) Billing(projectID uuid.UUID) (*Billing, error) {
	p, err := s.client.Project.Query().
		Where(project.IDEQ(projectID)).
		WithCustomer().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return &Billing{Customer: customerRef(p), BillRateCents: p.BillRateCents}, nil
}

// SetBilling grava o cliente e o valor cobrado como vieram: nil apaga.
func (s *Store) SetBilling(projectID uuid.UUID, customerID *uuid.UUID, billRateCents *int) error {
	q := s.client.Project.UpdateOneID(projectID)
	if customerID != nil {
		q = q.SetCustomerID(*customerID)
	} else {
		// Sem cliente não há reunião com ele.
		q = q.ClearCustomer().ClearCustomerMeetingDay().ClearCustomerMeetingTime()
	}
	if billRateCents != nil {
		q = q.SetBillRateCents(*billRateCents)
	} else {
		q = q.ClearBillRateCents()
	}
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	return err
}

// CustomerInProjectOrganization diz se o cliente existe e é da organização do projeto.
func (s *Store) CustomerInProjectOrganization(customerID, projectID uuid.UUID) (bool, error) {
	return s.client.Customer.Query().
		Where(
			entcustomer.IDEQ(customerID),
			entcustomer.HasOrganizationWith(entorg.HasProjectsWith(project.IDEQ(projectID))),
		).
		Exist(context.Background())
}

// Update grava o projeto como está: um campo opcional nil é apagado no banco.
// Quem decide entre manter e apagar é o service.
func (s *Store) Update(p *Project) error {
	q := s.client.Project.UpdateOneID(p.ID).
		SetName(p.Name).
		SetDescription(p.Description).
		SetSprintDurationDays(p.SprintDurationDays)
	if p.GithubRepoURL != nil {
		q = q.SetGithubRepoURL(*p.GithubRepoURL)
	} else {
		q = q.ClearGithubRepoURL()
	}
	if p.GitlabRepoURL != nil {
		q = q.SetGitlabRepoURL(*p.GitlabRepoURL)
	} else {
		q = q.ClearGitlabRepoURL()
	}
	if p.DailyTime != nil {
		q = q.SetDailyTime(*p.DailyTime)
	} else {
		q = q.ClearDailyTime()
	}
	if p.WeeklySyncDay != nil {
		q = q.SetWeeklySyncDay(*p.WeeklySyncDay)
	} else {
		q = q.ClearWeeklySyncDay()
	}
	if p.WeeklySyncTime != nil {
		q = q.SetWeeklySyncTime(*p.WeeklySyncTime)
	} else {
		q = q.ClearWeeklySyncTime()
	}
	if p.CustomerMeetingDay != nil {
		q = q.SetCustomerMeetingDay(*p.CustomerMeetingDay)
	} else {
		q = q.ClearCustomerMeetingDay()
	}
	if p.CustomerMeetingTime != nil {
		q = q.SetCustomerMeetingTime(*p.CustomerMeetingTime)
	} else {
		q = q.ClearCustomerMeetingTime()
	}
	_, err := q.Save(context.Background())
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
		ID:                  e.ID,
		OrganizationID:      e.OrganizationID,
		Name:                e.Name,
		Description:         e.Description,
		GithubRepoURL:       e.GithubRepoURL,
		GitlabRepoURL:       e.GitlabRepoURL,
		SprintDurationDays:  e.SprintDurationDays,
		DailyTime:           e.DailyTime,
		WeeklySyncDay:       e.WeeklySyncDay,
		WeeklySyncTime:      e.WeeklySyncTime,
		CustomerMeetingDay:  e.CustomerMeetingDay,
		CustomerMeetingTime: e.CustomerMeetingTime,
		Customer:            customerRef(e),
		MemberCount:         memberCount(e),
		TaskCount:           len(e.Edges.Tasks),
		CreatedAt:           e.CreatedAt,
	}
}

func customerRef(e *ent.Project) *CustomerRef {
	if c := e.Edges.Customer; c != nil {
		return &CustomerRef{ID: c.ID, Name: c.Name}
	}
	return nil
}

func toDomainProjects(es []*ent.Project) []Project {
	result := make([]Project, len(es))
	for i, e := range es {
		result[i] = *toDomainProject(e)
	}
	return result
}
