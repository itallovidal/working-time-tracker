package project

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entcustomer "working-time-tracker/ent/customer"
	entorg "working-time-tracker/ent/organization"
	"working-time-tracker/ent/predicate"
	"working-time-tracker/ent/project"
	entreceivable "working-time-tracker/ent/receivable"
	enttask "working-time-tracker/ent/task"
	entsession "working-time-tracker/ent/worksession"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/projectaccess"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(p *Project, billRateCents *int) error {
	q := s.client.Project.Create().
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
		SetNillableBillRateCents(billRateCents)
	if p.Customer != nil {
		q = q.SetCustomerID(p.Customer.ID)
	}
	created, err := q.Save(context.Background())
	if err != nil {
		return err
	}
	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	return nil
}

// inOrg escolhe os projetos da organização. Com member, só os em que essa pessoa está (valor por hora ou
// time); nil escolhe todos, o que só os admins veem.
func (s *Store) inOrg(orgID uuid.UUID, member *uuid.UUID) *ent.ProjectQuery {
	q := s.client.Project.Query().Where(project.OrganizationIDEQ(orgID))
	if member != nil {
		q = q.Where(projectaccess.ProjectsOf(*member))
	}
	return q
}

func (s *Store) ListByOrg(orgID string, member *uuid.UUID) ([]Project, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	projects, err := newestFirst(withCounts(s.inOrg(uid, member).WithCustomer())).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainProjects(projects), nil
}

// CountByOrg conta os projetos da organização (com member, os dessa pessoa).
func (s *Store) CountByOrg(orgID string, member *uuid.UUID) (int, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return 0, err
	}
	return s.inOrg(uid, member).Count(context.Background())
}

// ListPageByOrg devolve uma página dos projetos da organização (com member, os dessa pessoa), do mais
// novo para o mais antigo. page começa em 1.
func (s *Store) ListPageByOrg(orgID string, member *uuid.UUID, page, perPage int) ([]Project, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	projects, err := newestFirst(withCounts(s.inOrg(uid, member).WithCustomer())).
		Offset((page - 1) * perPage).
		Limit(perPage).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainProjects(projects), nil
}

// newestFirst ordena do projeto mais novo para o mais antigo. O id desempata, para duas
// páginas seguidas nunca repetirem nem pularem um projeto criado no mesmo instante.
func newestFirst(q *ent.ProjectQuery) *ent.ProjectQuery {
	return q.Order(ent.Desc(project.FieldCreatedAt), ent.Asc(project.FieldID))
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
	b := &Billing{Customer: customerRef(p), BillRateCents: p.BillRateCents}
	if c := p.Edges.Customer; c != nil {
		b.CustomerBillingMode = &c.BillingMode
	}
	return b, nil
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

// HasBilledHistory diz se o projeto tem sessões e o cliente tem algum recebível, confirmado ou recebido. É quando tirar
// o projeto do cliente refaria a atribuição das horas: elas passariam a entrar nas propostas do cliente novo, e o
// recebível do antigo continuaria cobrando as mesmas.
func (s *Store) HasBilledHistory(projectID, customerID uuid.UUID) (bool, error) {
	ctx := context.Background()
	has, err := s.client.Receivable.Query().
		Where(entreceivable.CustomerIDEQ(customerID)).
		Exist(ctx)
	if err != nil || !has {
		return false, err
	}
	return s.client.WorkSession.Query().
		Where(entsession.ProjectIDEQ(projectID)).
		Exist(ctx)
}

// CustomerBillingModeOfProject devolve o modo de cobrança do cliente (hourly ou monthly) e se ele existe e é da
// organização do projeto.
func (s *Store) CustomerBillingModeOfProject(customerID, projectID uuid.UUID) (mode string, ok bool, err error) {
	return s.billingMode(
		entcustomer.IDEQ(customerID),
		entcustomer.HasOrganizationWith(entorg.HasProjectsWith(project.IDEQ(projectID))),
	)
}

// CustomerBillingMode devolve o modo de cobrança do cliente (hourly ou monthly) e se ele existe e é da organização.
func (s *Store) CustomerBillingMode(customerID, orgID uuid.UUID) (mode string, ok bool, err error) {
	return s.billingMode(entcustomer.IDEQ(customerID), entcustomer.OrganizationIDEQ(orgID))
}

func (s *Store) billingMode(where ...predicate.Customer) (string, bool, error) {
	mode, err := s.client.Customer.Query().
		Where(where...).
		Select(entcustomer.FieldBillingMode).
		String(context.Background())
	if ent.IsNotFound(err) {
		return "", false, nil
	}
	return mode, err == nil, err
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
		return database.ErrNotFound
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
