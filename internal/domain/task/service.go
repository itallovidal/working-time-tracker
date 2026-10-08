package task

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/team"
)

// MaxDescriptionLen é o tamanho máximo da descrição de uma tarefa, em caracteres: o mesmo do corpo de uma
// issue do GitHub, para a sincronização não cortar o texto nem num sentido nem no outro. Ela é
// Markdown e vira HTML no navegador de todo mundo que abre a tarefa.
const MaxDescriptionLen = 65536

// Tamanho de página da lista de tarefas: o padrão e o teto que a API aceita.
const (
	DefaultPerPage = 10
	MaxPerPage     = 100
)

type Service struct {
	taskStore       *Store
	membershipStore *team.MembershipStore
	integrationSvc  *integration.Service
}

func NewService(taskStore *Store, membershipStore *team.MembershipStore, integrationSvc *integration.Service) *Service {
	return &Service{taskStore: taskStore, membershipStore: membershipStore, integrationSvc: integrationSvc}
}

func (s *Service) Create(projectID, name, description, assigneeID string, deadline *time.Time) (*Task, error) {
	return s.CreateAs("", projectID, name, description, assigneeID, deadline, Attrs{})
}

// CreateAs cria a tarefa em nome de quem está logado. Quem escolhe a si mesmo
// como responsável pode, mesmo fora dos times: é o "atribuir a mim". Qualquer
// outra pessoa precisa estar em algum time do projeto.
func (s *Service) CreateAs(selfID, projectID, name, description, assigneeID string, deadline *time.Time, attrs Attrs) (*Task, error) {
	if name == "" {
		return nil, ErrNameRequired
	}
	if utf8.RuneCountInString(description) > MaxDescriptionLen {
		return nil, ErrDescriptionTooLong.With("max", MaxDescriptionLen)
	}
	priority := "none"
	if attrs.Priority != nil && *attrs.Priority != "" {
		if !validPriority(*attrs.Priority) {
			return nil, ErrInvalidPriority
		}
		priority = *attrs.Priority
	}
	labels := []Label{}
	if attrs.LabelIDs != nil {
		var err error
		if labels, err = s.resolveLabels(projectID, *attrs.LabelIDs); err != nil {
			return nil, err
		}
	}
	// Sem responsável, a tarefa fica disponível: quem bater o ponto nela a pega.
	var assignee *uuid.UUID
	if assigneeID != "" {
		uid, err := uuid.Parse(assigneeID)
		if err != nil {
			return nil, ErrInvalidAssignee
		}
		if assigneeID != selfID {
			isMember, err := s.membershipStore.IsPersonInProject(assigneeID, projectID)
			if err != nil {
				return nil, err
			}
			if !isMember {
				return nil, ErrAssigneeNotInTeam
			}
		}
		assignee = &uid
	}

	var dl time.Time
	if deadline != nil {
		dl = *deadline
	} else {
		dl = time.Now().Add(7 * 24 * time.Hour)
	}

	task := &Task{
		ProjectID:   uuid.MustParse(projectID),
		Name:        name,
		Description: description,
		Priority:    priority,
		Status:      StatusBacklog,
		Labels:      labels,
		AssigneeID:  assignee,
		Deadline:    dl,
	}
	if err := s.taskStore.Create(task); err != nil {
		return nil, err
	}
	s.taskStore.created(task.ID)
	return s.taskStore.GetByID(task.ID.String())
}

// ListByProject devolve, de uma vez, todas as tarefas do projeto que passam
// pelos filtros.
func (s *Service) ListByProject(projectID string, f ListFilter) ([]Task, error) {
	f.Page, f.PerPage = 0, 0
	return s.taskStore.ListByProject(projectID, f)
}

// ListPage devolve uma página das tarefas do projeto. Quem pede uma página
// além do fim recebe a última, e a resposta diz qual foi.
func (s *Service) ListPage(projectID string, f ListFilter) (*Page, error) {
	if f.PerPage <= 0 {
		f.PerPage = DefaultPerPage
	}
	if f.PerPage > MaxPerPage {
		f.PerPage = MaxPerPage
	}
	total, err := s.taskStore.CountByProject(projectID, f)
	if err != nil {
		return nil, err
	}
	last := max(1, (total+f.PerPage-1)/f.PerPage)
	f.Page = min(max(f.Page, 1), last)

	items, err := s.taskStore.ListByProject(projectID, f)
	if err != nil {
		return nil, err
	}
	assignees, err := s.taskStore.AssigneesByProject(projectID)
	if err != nil {
		return nil, err
	}
	return &Page{Items: items, Total: total, Page: f.Page, PerPage: f.PerPage, Assignees: assignees}, nil
}

func (s *Service) Get(id string) (*Task, error) {
	return s.taskStore.GetByID(id)
}

func (s *Service) Update(id, name, description string, assigneeID *string, deadline *time.Time) (*Task, error) {
	return s.UpdateAs("", id, name, description, assigneeID, deadline, Attrs{})
}

// UpdateAs altera a tarefa em nome de quem está logado; a regra do responsável é a do CreateAs.
func (s *Service) UpdateAs(selfID, id, name, description string, assigneeID *string, deadline *time.Time, attrs Attrs) (*Task, error) {
	if name == "" {
		return nil, ErrNameRequired
	}
	if utf8.RuneCountInString(description) > MaxDescriptionLen {
		return nil, ErrDescriptionTooLong.With("max", MaxDescriptionLen)
	}
	if attrs.Priority != nil && !validPriority(*attrs.Priority) {
		return nil, ErrInvalidPriority
	}
	if attrs.Status != nil && !validStatus(*attrs.Status) {
		return nil, ErrInvalidStatus
	}
	task, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	task.Name = name
	task.Description = description
	if attrs.Priority != nil {
		task.Priority = *attrs.Priority
	}
	if attrs.Status != nil {
		task.Status = *attrs.Status
	}
	if attrs.LabelIDs != nil {
		if task.Labels, err = s.resolveLabels(task.ProjectID.String(), *attrs.LabelIDs); err != nil {
			return nil, err
		}
	}
	if assigneeID != nil && *assigneeID == "" {
		task.AssigneeID = nil // vazio desvincula: a tarefa volta a ficar disponível
	} else if assigneeID != nil {
		uid, err := uuid.Parse(*assigneeID)
		if err != nil {
			return nil, ErrInvalidAssignee
		}
		if (task.AssigneeID == nil || uid != *task.AssigneeID) && *assigneeID != selfID {
			isMember, err := s.membershipStore.IsPersonInProject(*assigneeID, task.ProjectID.String())
			if err != nil {
				return nil, err
			}
			if !isMember {
				return nil, ErrAssigneeNotInTeam
			}
		}
		task.AssigneeID = &uid
	}
	if deadline != nil {
		task.Deadline = *deadline
	}
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(id)
}

// Claim passa a tarefa para quem pediu, sem bater o ponto e sem mexer no status: é pegar a tarefa para
// fazer depois. Só vale para uma tarefa sem responsável (ErrAlreadyAssigned se já é de outra pessoa); quem
// pega a que já é sua não muda nada, e de dois pedidos juntos só um leva (o outro recebe o conflito).
func (s *Service) Claim(selfID, id string) (*Task, error) {
	person, err := uuid.Parse(selfID)
	if err != nil {
		return nil, ErrInvalidAssignee
	}
	t, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	if t.AssigneeID != nil {
		if *t.AssigneeID == person {
			return t, nil
		}
		return nil, ErrAlreadyAssigned
	}
	claimed, err := s.taskStore.TryClaim(t.ID, person)
	if err != nil {
		return nil, err
	}
	t, err = s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	if !claimed && (t.AssigneeID == nil || *t.AssigneeID != person) {
		return nil, ErrAlreadyAssigned // outra pessoa levou entre a leitura e o UPDATE
	}
	return t, nil
}

// UpdateAttrs é a atualização rápida: muda só a prioridade, o status e as etiquetas que vierem (as que
// faltam ficam como estão), sem o nome, a descrição, o responsável e o prazo da edição completa.
func (s *Service) UpdateAttrs(id string, attrs Attrs) (*Task, error) {
	if attrs.Priority != nil && !validPriority(*attrs.Priority) {
		return nil, ErrInvalidPriority
	}
	if attrs.Status != nil && !validStatus(*attrs.Status) {
		return nil, ErrInvalidStatus
	}
	t, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	var labels *[]Label
	if attrs.LabelIDs != nil {
		resolved, err := s.resolveLabels(t.ProjectID.String(), *attrs.LabelIDs)
		if err != nil {
			return nil, err
		}
		labels = &resolved
	}
	if err := s.taskStore.UpdateAttrs(t.ID, attrs.Priority, attrs.Status, labels); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(id)
}

func (s *Service) Delete(id string) error {
	return s.taskStore.Delete(id)
}

func (s *Service) LinkExternalItem(taskID, integrationID, externalItemID, externalItemURL string) (*Task, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(integrationID)
	if err != nil {
		return nil, ErrIntegrationNotFound
	}
	integrationProject, err := s.taskStore.IntegrationProjectID(eid)
	if errors.Is(err, database.ErrNotFound) {
		return nil, ErrIntegrationNotFound
	}
	if err != nil {
		return nil, err
	}
	if integrationProject != task.ProjectID {
		return nil, ErrIntegrationOtherProject
	}
	task.ExternalIntegrationID = &eid
	task.ExternalItemID = &externalItemID
	task.ExternalItemURL = &externalItemURL
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(taskID)
}

func (s *Service) UnlinkExternalItem(taskID string) (*Task, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	task.ExternalIntegrationID = nil
	task.ExternalItemID = nil
	task.ExternalItemURL = nil
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(taskID)
}

func (s *Service) GetExternalDetails(taskID string) (*adapter.ExternalDetailsResult, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	if task.ExternalIntegrationID == nil {
		return nil, ErrNoExternalItem
	}
	if s.integrationSvc == nil {
		return nil, ErrIntegrationsUnavailable
	}
	return s.integrationSvc.FetchItemDetails(task.ExternalIntegrationID.String(), *task.ExternalItemID)
}
