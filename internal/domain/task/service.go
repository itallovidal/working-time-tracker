package task

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/team"
)

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
	if name == "" {
		return nil, errors.New("informe o nome")
	}
	if assigneeID == "" {
		return nil, errors.New("escolha o responsável")
	}

	isMember, err := s.membershipStore.IsPersonInProject(assigneeID, projectID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errors.New("o responsável precisa estar em algum time deste projeto")
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
		AssigneeID:  uuid.MustParse(assigneeID),
		Deadline:    dl,
	}
	if err := s.taskStore.Create(task); err != nil {
		return nil, err
	}
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
	if name == "" {
		return nil, errors.New("informe o nome")
	}
	task, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	task.Name = name
	task.Description = description
	if assigneeID != nil {
		uid, err := uuid.Parse(*assigneeID)
		if err != nil {
			return nil, errors.New("responsável inválido")
		}
		if uid != task.AssigneeID {
			isMember, err := s.membershipStore.IsPersonInProject(*assigneeID, task.ProjectID.String())
			if err != nil {
				return nil, err
			}
			if !isMember {
				return nil, errors.New("o responsável precisa estar em algum time deste projeto")
			}
		}
		task.AssigneeID = uid
	}
	if deadline != nil {
		task.Deadline = *deadline
	}
	if err := s.taskStore.Update(task); err != nil {
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
		return nil, errors.New("integração não encontrada")
	}
	integrationProject, err := s.taskStore.IntegrationProjectID(eid)
	if errors.Is(err, database.ErrNotFound) {
		return nil, errors.New("integração não encontrada")
	}
	if err != nil {
		return nil, err
	}
	if integrationProject != task.ProjectID {
		return nil, errors.New("a integração é de outro projeto")
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
		return nil, errors.New("a tarefa não tem item externo vinculado")
	}
	if s.integrationSvc == nil {
		return nil, errors.New("integrações indisponíveis")
	}
	return s.integrationSvc.FetchItemDetails(task.ExternalIntegrationID.String(), *task.ExternalItemID)
}
