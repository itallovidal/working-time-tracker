package task

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/issuesync"
	entlabel "working-time-tracker/ent/label"
	entorganization "working-time-tracker/ent/organization"
	entperson "working-time-tracker/ent/person"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/ent/task"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/internal/domain/projectaccess"
)

type Store struct {
	client *ent.Client
	changeHook
	// accounts são as contas das pessoas no GitHub e no GitLab, para a trava da tarefa. Nulo, a trava não confere.
	accounts *personaccount.Store
	// work para o ponto de quem sai da tarefa (ou é tirado dela). Nulo, ninguém é parado.
	work WorkStopper
}

// WorkStopper para o trabalho de uma pessoa numa tarefa, quando ela deixa de ser responsável: o trecho dela na tarefa
// termina agora e, se era a única tarefa rodando na sessão, a sessão fecha. Quem não está com o ponto aberto na
// tarefa passa sem nada mudar. É o work_session.Service, ligado no servidor.
type WorkStopper interface {
	StopTaskFor(personID, taskID uuid.UUID) error
}

// SetWorkStopper liga quem para o ponto de quem sai da tarefa.
func (s *Store) SetWorkStopper(w WorkStopper) { s.work = w }

// stopWork para o ponto de cada pessoa que sai da tarefa. Vem antes de a lista mudar: se parar falha, ela continua
// responsável e com o ponto como estava.
func (s *Store) stopWork(taskID uuid.UUID, leaving []uuid.UUID) error {
	if s.work == nil {
		return nil
	}
	for _, p := range leaving {
		if err := s.work.StopTaskFor(p, taskID); err != nil {
			return err
		}
	}
	return nil
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(t *Task) error {
	q := s.client.Task.Create().
		SetProjectID(t.ProjectID).
		SetName(t.Name).
		SetDescription(t.Description).
		SetPriority(priorityOf(t.Priority)).
		SetStatus(statusOf(t.Status)).
		AddLabelIDs(labelIDs(t.Labels)...).
		AddAssigneeIDs(t.AssigneeIDs()...).
		SetDeadline(t.Deadline).
		SetRequiredAccounts(nonNil(t.RequiredAccounts))
	// A tarefa importada nasce com a data de criação do item, e não com a de agora.
	if !t.CreatedAt.IsZero() {
		q = q.SetCreatedAt(t.CreatedAt)
	}
	created, err := q.Save(context.Background())
	if err != nil {
		return err
	}
	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	return nil
}

// noDeadline é o limite abaixo do qual um prazo conta como "sem prazo": uma
// tarefa antiga sem prazo, depois de editada, guarda o tempo zero do Go.
var noDeadline = time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)

// filtered monta a consulta das tarefas do projeto que passam pelos filtros.
func (s *Store) filtered(projectID uuid.UUID, f ListFilter) *ent.TaskQuery {
	return applyFilter(s.client.Task.Query().Where(task.ProjectIDEQ(projectID)), f)
}

// applyFilter aplica os filtros a uma consulta de tarefas, seja qual for o escopo dela.
func applyFilter(q *ent.TaskQuery, f ListFilter) *ent.TaskQuery {
	if f.Query != "" {
		q = q.Where(task.NameContainsFold(f.Query))
	}
	if f.Unassigned {
		q = q.Where(task.Not(task.HasAssignees()))
	} else if f.Assigned {
		q = q.Where(task.HasAssignees())
	} else if f.OthersOf != nil {
		q = q.Where(task.HasAssignees(), task.Not(task.HasAssigneesWith(entperson.IDEQ(*f.OthersOf))))
	} else if f.AssigneeID != nil {
		q = q.Where(task.HasAssigneesWith(entperson.IDEQ(*f.AssigneeID)))
	}
	if f.DeadlineTo != nil {
		q = q.Where(task.DeadlineLTE(*f.DeadlineTo), task.DeadlineGTE(noDeadline))
	}
	if len(f.Priorities) > 0 {
		ps := make([]task.Priority, len(f.Priorities))
		for i, p := range f.Priorities {
			ps[i] = priorityOf(p)
		}
		q = q.Where(task.PriorityIn(ps...))
	}
	if len(f.Statuses) > 0 {
		ss := make([]task.Status, len(f.Statuses))
		for i, st := range f.Statuses {
			ss[i] = statusOf(st)
		}
		q = q.Where(task.StatusIn(ss...))
	}
	if len(f.LabelIDs) > 0 {
		q = q.Where(task.HasLabelsWith(entlabel.IDIn(f.LabelIDs...)))
	}
	for _, w := range f.Words {
		q = q.Where(task.Or(task.NameContainsFold(w), task.DescriptionContainsFold(w)))
	}
	if f.ItemID != "" {
		q = q.Where(task.HasIssueSyncsWith(issuesync.ItemIDEQ(f.ItemID)))
	}
	if f.InProjectsOf != nil {
		q = q.Where(task.HasProjectWith(projectaccess.ProjectsOf(*f.InProjectsOf)))
	}
	return q
}

// priorityOf converte a prioridade do domínio para a do Ent; vazio é sem prioridade.
func priorityOf(p string) task.Priority {
	if p == "" {
		return task.PriorityNone
	}
	return task.Priority(p)
}

// statusOf converte o status do domínio para o do Ent; vazio é backlog.
func statusOf(s string) task.Status {
	if s == "" {
		return task.StatusBacklog
	}
	return task.Status(s)
}

func labelIDs(ls []Label) []uuid.UUID {
	ids := make([]uuid.UUID, len(ls))
	for i, l := range ls {
		ids[i] = l.ID
	}
	return ids
}

// withLabels carrega as etiquetas da tarefa em ordem alfabética.
func withLabels(q *ent.LabelQuery) {
	q.Order(ent.Asc(entlabel.FieldName))
}

// withAssignees carrega os responsáveis da tarefa por nome (o id desempata dois nomes iguais).
func withAssignees(q *ent.PersonQuery) {
	q.Order(ent.Asc(entperson.FieldName, entperson.FieldID))
}

// withLinks carrega os vínculos da tarefa com a integração de cada um, do mais antigo para o mais novo.
func withLinks(q *ent.IssueSyncQuery) {
	q.WithIntegration().Order(ent.Asc(issuesync.FieldCreatedAt, issuesync.FieldID))
}

// ListByProject lista as tarefas do projeto, da mais nova para a mais antiga.
// Com f.Page maior que zero, devolve só aquela página.
func (s *Store) ListByProject(projectID string, f ListFilter) ([]Task, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	// O id desempata tarefas criadas no mesmo instante, para a ordem não mudar
	// de uma página para a outra.
	// Os vínculos vêm junto, com a integração, para a lista saber de que plataformas são os itens.
	q := s.filtered(uid, f).
		WithAssignees(withAssignees).
		WithIssueSyncs(withLinks).
		WithLabels(withLabels).
		Order(ent.Desc(task.FieldCreatedAt, task.FieldID))
	if f.Page > 0 {
		q = q.Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage)
	}
	tasks, err := q.All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainTasks(tasks), nil
}

// CountByProject conta as tarefas do projeto que passam pelos filtros.
func (s *Store) CountByProject(projectID string, f ListFilter) (int, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return 0, err
	}
	return s.filtered(uid, f).Count(context.Background())
}

// assigned monta a consulta das tarefas de que uma pessoa é responsável (sozinha ou com outras) em todos os
// projetos da organização que passam pelos filtros. O responsável vem do argumento, não do filtro.
func (s *Store) assigned(orgID, personID uuid.UUID, f ListFilter) *ent.TaskQuery {
	q := s.client.Task.Query().Where(
		task.HasAssigneesWith(entperson.IDEQ(personID)),
		task.HasProjectWith(entproject.OrganizationIDEQ(orgID)),
	)
	return applyFilter(q, f)
}

// ListAssigned lista as tarefas da pessoa em todos os projetos da organização, da mais nova para a
// mais antiga, cada uma com o nome do projeto. Com f.Page maior que zero, devolve só aquela página.
func (s *Store) ListAssigned(orgID, personID string, f ListFilter) ([]Assigned, error) {
	oid, pid, err := parseOrgPerson(orgID, personID)
	if err != nil {
		return nil, err
	}
	q := s.assigned(oid, pid, f).
		WithProject().
		WithAssignees(withAssignees).
		Order(ent.Desc(task.FieldCreatedAt, task.FieldID))
	if f.Page > 0 {
		q = q.Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage)
	}
	tasks, err := q.All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Assigned, len(tasks))
	for i, e := range tasks {
		out[i] = Assigned{Task: *toDomainTask(e)}
		if e.Edges.Project != nil {
			out[i].ProjectName = e.Edges.Project.Name
		}
	}
	return out, nil
}

// inOrganization monta a consulta das tarefas de todos os projetos da organização (ou só do projeto, quando vem um)
// que passam pelos filtros. Quem não é admin limita com f.InProjectsOf.
func (s *Store) inOrganization(orgID uuid.UUID, projectID *uuid.UUID, f ListFilter) *ent.TaskQuery {
	q := s.client.Task.Query().Where(task.HasProjectWith(entproject.OrganizationIDEQ(orgID)))
	if projectID != nil {
		q = q.Where(task.ProjectIDEQ(*projectID))
	}
	return applyFilter(q, f)
}

// ListInOrganization lista as tarefas da organização que passam pelos filtros, da mais nova para a mais antiga, cada
// uma com o projeto, o responsável, as etiquetas e os vínculos. Com f.Page maior que zero, devolve só aquela página.
func (s *Store) ListInOrganization(orgID uuid.UUID, projectID *uuid.UUID, f ListFilter) ([]Assigned, error) {
	q := s.inOrganization(orgID, projectID, f).
		WithProject().
		WithAssignees(withAssignees).
		WithIssueSyncs(withLinks).
		WithLabels(withLabels).
		Order(ent.Desc(task.FieldCreatedAt, task.FieldID))
	if f.Page > 0 {
		q = q.Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage)
	}
	tasks, err := q.All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Assigned, len(tasks))
	for i, e := range tasks {
		out[i] = Assigned{Task: *toDomainTask(e)}
		if e.Edges.Project != nil {
			out[i].ProjectName = e.Edges.Project.Name
		}
	}
	return out, nil
}

// CountInOrganization conta as tarefas da organização que passam pelos filtros.
func (s *Store) CountInOrganization(orgID uuid.UUID, projectID *uuid.UUID, f ListFilter) (int, error) {
	return s.inOrganization(orgID, projectID, f).Count(context.Background())
}

// CountAssigned conta as tarefas da pessoa na organização que passam pelos filtros.
func (s *Store) CountAssigned(orgID, personID string, f ListFilter) (int, error) {
	oid, pid, err := parseOrgPerson(orgID, personID)
	if err != nil {
		return 0, err
	}
	return s.assigned(oid, pid, f).Count(context.Background())
}

// AssignedByStatus conta as tarefas da pessoa na organização que passam pelos filtros (o que importa é
// f.InProjectsOf) por status, numa consulta só. Um status sem tarefas não aparece no mapa.
func (s *Store) AssignedByStatus(orgID, personID string, f ListFilter) (map[string]int, error) {
	oid, pid, err := parseOrgPerson(orgID, personID)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	if err := s.assigned(oid, pid, f).
		GroupBy(task.FieldStatus).
		Aggregate(ent.Count()).
		Scan(context.Background(), &rows); err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.Status] = r.Count
	}
	return counts, nil
}

func parseOrgPerson(orgID, personID string) (org, person uuid.UUID, err error) {
	if org, err = uuid.Parse(orgID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if person, err = uuid.Parse(personID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return org, person, nil
}

// AssigneesByProject lista, sem repetir e por nome, quem é responsável por
// alguma tarefa do projeto, esteja ou não nos times.
func (s *Store) AssigneesByProject(projectID string) ([]Person, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	persons, err := s.client.Person.Query().
		Where(entperson.HasAssignedTasksWith(task.ProjectIDEQ(uid))).
		Order(ent.Asc(entperson.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]Person, len(persons))
	for i, p := range persons {
		result[i] = Person{ID: p.ID, Name: p.Name, Email: p.Email}
	}
	return result, nil
}

func (s *Store) GetByID(id string) (*Task, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	t, err := s.client.Task.Query().
		Where(task.IDEQ(uid)).
		WithAssignees(withAssignees).
		WithIssueSyncs(withLinks).
		WithLabels(withLabels).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainTask(t), nil
}

// Update grava a tarefa inteira, com a lista de responsáveis que ela tem. Quem sai da lista perde o ponto na tarefa
// (stopWork) antes.
func (s *Store) Update(t *Task) error {
	leaving, err := s.leaving(t.ID, t.AssigneeIDs())
	if err != nil {
		return err
	}
	if err := s.stopWork(t.ID, leaving); err != nil {
		return err
	}
	q := s.client.Task.UpdateOneID(t.ID).
		SetName(t.Name).
		SetDescription(t.Description).
		SetPriority(priorityOf(t.Priority)).
		SetStatus(statusOf(t.Status)).
		ClearLabels().
		AddLabelIDs(labelIDs(t.Labels)...).
		SetDeadline(t.Deadline).
		SetRequiredAccounts(nonNil(t.RequiredAccounts)).
		ClearAssignees().
		AddAssigneeIDs(t.AssigneeIDs()...)
	if _, err := q.Save(context.Background()); err != nil {
		return err
	}
	s.changed(t.ID)
	return nil
}

// leaving são as pessoas que deixam de ser responsáveis quando a tarefa passa a ter a lista next.
func (s *Store) leaving(taskID uuid.UUID, next []uuid.UUID) ([]uuid.UUID, error) {
	current, err := s.client.Task.Query().Where(task.IDEQ(taskID)).QueryAssignees().IDs(context.Background())
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for _, p := range current {
		if !slices.Contains(next, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// ClaimIfUnassigned passa a tarefa para a pessoa só se ela ainda não tem
// responsável: de dois pontos batidos juntos na mesma tarefa livre, só o primeiro a leva.
func (s *Store) ClaimIfUnassigned(taskID, personID uuid.UUID) error {
	_, err := s.TryClaim(taskID, personID)
	return err
}

// TryClaim é o ClaimIfUnassigned que diz se a pessoa levou a tarefa: false quando ela já
// tinha responsável (de dois pedidos juntos, só um recebe true). A linha da tarefa fica travada na transação
// enquanto se confere que ela está livre, e é isso que decide a disputa no banco.
func (s *Store) TryClaim(taskID, personID uuid.UUID) (bool, error) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	claimed, err := func() (bool, error) {
		if _, err := tx.Task.Query().Where(task.IDEQ(taskID)).ForUpdate().Only(ctx); err != nil {
			if ent.IsNotFound(err) {
				return false, database.ErrNotFound
			}
			return false, err
		}
		taken, err := tx.Task.Query().Where(task.IDEQ(taskID), task.HasAssignees()).Exist(ctx)
		if err != nil || taken {
			return false, err
		}
		return true, tx.Task.UpdateOneID(taskID).AddAssigneeIDs(personID).Exec(ctx)
	}()
	if err != nil || !claimed {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	s.changed(taskID)
	return true, nil
}

// Unclaim desfaz o TryClaim de quem acabou não abrindo o ponto: tira a pessoa dos responsáveis.
func (s *Store) Unclaim(taskID, personID uuid.UUID) error {
	err := s.client.Task.UpdateOneID(taskID).RemoveAssigneeIDs(personID).Exec(context.Background())
	if err == nil {
		s.changed(taskID)
	}
	return err
}

// attrsPatch é o que Store.UpdateAttrs grava: cada campo nil fica como está. assignees é a lista inteira de
// responsáveis; vazia deixa a tarefa livre.
type attrsPatch struct {
	priority, status *string
	labels           *[]Label
	assignees        *[]uuid.UUID
	deadline         *time.Time
	required         *[]string
}

// UpdateAttrs grava só os campos do patch, sem reescrever o resto da tarefa: uma edição dos detalhes não
// desfaz o nome, a descrição ou um responsável que chegaram no meio.
func (s *Store) UpdateAttrs(id uuid.UUID, p attrsPatch) error {
	if p.assignees != nil {
		leaving, err := s.leaving(id, *p.assignees)
		if err != nil {
			return err
		}
		if err := s.stopWork(id, leaving); err != nil {
			return err
		}
	}
	q := s.client.Task.UpdateOneID(id)
	if p.priority != nil {
		q = q.SetPriority(priorityOf(*p.priority))
	}
	if p.status != nil {
		q = q.SetStatus(statusOf(*p.status))
	}
	if p.labels != nil {
		q = q.ClearLabels().AddLabelIDs(labelIDs(*p.labels)...)
	}
	if p.assignees != nil {
		q = q.ClearAssignees().AddAssigneeIDs(*p.assignees...)
	}
	if p.deadline != nil {
		q = q.SetDeadline(*p.deadline)
	}
	if p.required != nil {
		q = q.SetRequiredAccounts(nonNil(*p.required))
	}
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	if err == nil {
		s.changed(id)
	}
	return err
}

// StartProgress põe a tarefa em progresso, seja qual for o status atual: bater o ponto
// nela é começar (ou retomar) o trabalho. Quem para ou pausa não a tira de lá.
func (s *Store) StartProgress(taskID uuid.UUID) error {
	n, err := s.client.Task.Update().
		Where(task.IDEQ(taskID), task.StatusNEQ(task.StatusInProgress)).
		SetStatus(task.StatusInProgress).
		Save(context.Background())
	if n > 0 {
		s.changed(taskID)
	}
	return err
}

// IntegrationProjectID devolve o projeto dono da integração, ou database.ErrNotFound.
func (s *Store) IntegrationProjectID(integrationID uuid.UUID) (uuid.UUID, error) {
	it, err := s.client.Integration.Get(context.Background(), integrationID)
	if ent.IsNotFound(err) {
		return uuid.Nil, database.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return it.ProjectID, nil
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Task.DeleteOneID(uid).Exec(context.Background())
}

// nonNil faz a lista vazia sair [] (no banco e no JSON), e não nula.
func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// SetAccounts liga as contas das pessoas no GitHub e no GitLab, que a trava da tarefa confere (CheckRequired).
func (s *Store) SetAccounts(accounts *personaccount.Store) { s.accounts = accounts }

// CheckRequired confere que a pessoa tem conta ligada em cada plataforma que a tarefa exige: é a trava de quem pode
// ser um dos responsáveis. Sem trava (ou sem as contas ligadas ao store), passa; no modo solo também, porque a pessoa é uma
// só e não há de quem restringir.
func (s *Store) CheckRequired(t *Task, personID uuid.UUID) error {
	if len(t.RequiredAccounts) == 0 || s.accounts == nil {
		return nil
	}
	if solo, err := s.client.Project.Query().
		Where(entproject.IDEQ(t.ProjectID), entproject.HasOrganizationWith(entorganization.Solo(true))).
		Exist(context.Background()); err != nil {
		return err
	} else if solo {
		return nil
	}
	linked, err := s.accounts.LinkedPlatforms([]uuid.UUID{personID})
	if err != nil {
		return err
	}
	var missing, names []string
	for _, p := range t.RequiredAccounts {
		if !slices.Contains(linked[personID], p) {
			missing = append(missing, p)
			names = append(names, personaccount.Name(p))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return ErrAccountRequired.With("platforms", strings.Join(missing, ","), "names", strings.Join(names, ", "))
}

func toDomainTask(e *ent.Task) *Task {
	if e == nil {
		return nil
	}
	t := &Task{
		ID:          e.ID,
		ProjectID:   e.ProjectID,
		Name:        e.Name,
		Description: e.Description,
		Priority:    string(e.Priority),
		Status:      string(e.Status),
		Labels:      []Label{},
		Assignees:   []Person{},
		Deadline:    e.Deadline,
		Links:       []Link{},
		CreatedAt:   e.CreatedAt,

		RequiredAccounts: nonNil(e.RequiredAccounts),
	}
	for _, p := range e.Edges.Assignees {
		t.Assignees = append(t.Assignees, Person{ID: p.ID, Name: p.Name, Email: p.Email})
	}
	for _, l := range e.Edges.Labels {
		t.Labels = append(t.Labels, Label{ID: l.ID, Name: l.Name})
	}
	for _, row := range e.Edges.IssueSyncs {
		link := Link{IntegrationID: row.IntegrationID, ItemID: row.ItemID, URL: row.URL, LastError: row.LastError}
		if in := row.Edges.Integration; in != nil {
			link.Integration = &Integration{ID: in.ID, Type: in.Type, Name: in.DisplayName}
		}
		t.Links = append(t.Links, link)
	}
	return t
}

func toDomainTasks(es []*ent.Task) []Task {
	result := make([]Task, len(es))
	for i, e := range es {
		result[i] = *toDomainTask(e)
	}
	return result
}
