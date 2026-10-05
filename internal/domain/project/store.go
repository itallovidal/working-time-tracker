package project

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entcustomer "working-time-tracker/ent/customer"
	entorg "working-time-tracker/ent/organization"
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

// defaultSprintDays vale para organizações que não definiram uma sprint padrão.
const defaultSprintDays = 14

// DefaultSprintDays devolve a duração de sprint que a organização usa em
// projetos novos.
func (s *Store) DefaultSprintDays(orgID uuid.UUID) (int, error) {
	org, err := s.client.Organization.Get(context.Background(), orgID)
	if ent.IsNotFound(err) {
		// O Create falha em seguida pela chave estrangeira.
		return defaultSprintDays, nil
	}
	if err != nil {
		return 0, err
	}
	if org.DefaultSprintDays == nil {
		return defaultSprintDays, nil
	}
	return *org.DefaultSprintDays, nil
}

func (s *Store) ListByOrg(orgID string) ([]Project, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	projects, err := s.client.Project.Query().
		Where(project.OrganizationIDEQ(uid)).
		WithCustomer().
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
	p, err := s.client.Project.Query().
		Where(project.IDEQ(uid)).
		WithCustomer().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainProject(p), nil
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
		q = q.ClearCustomer()
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
		ID:                 e.ID,
		OrganizationID:     e.OrganizationID,
		Name:               e.Name,
		Description:        e.Description,
		GithubRepoURL:      e.GithubRepoURL,
		GitlabRepoURL:      e.GitlabRepoURL,
		SprintDurationDays: e.SprintDurationDays,
		DailyTime:          e.DailyTime,
		WeeklySyncDay:      e.WeeklySyncDay,
		Customer:           customerRef(e),
		CreatedAt:          e.CreatedAt,
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
