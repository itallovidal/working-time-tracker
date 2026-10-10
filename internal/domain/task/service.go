package task

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/validate"
)

// MaxDescriptionLen é o tamanho máximo da descrição de uma tarefa, em caracteres: o mesmo do corpo de uma
// issue do GitHub, para a sincronização não cortar o texto nem num sentido nem no outro. Ela é
// Markdown e vira HTML no navegador de todo mundo que abre a tarefa.
const MaxDescriptionLen = validate.MaxTaskDescription

// Os campos do corpo que os erros de validação apontam: a chave JSON, ou o rótulo do catálogo quando é o alias da
// tela (a descrição da tarefa é long_description em fields.*, e a tela o liga ao campo description).
const (
	fieldName        = "name"
	fieldDescription = "long_description"
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
	remover         Remover
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
	name, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	if err := checkDescription(description); err != nil {
		return nil, err
	}
	if deadline != nil && deadline.Before(noDeadline) {
		return nil, ErrDeadlineOutOfRange
	}
	priority := "none"
	if p, err := cleanPriority(attrs.Priority); err != nil {
		return nil, err
	} else if p != nil {
		priority = *p
	}
	labels := []Label{}
	if attrs.LabelIDs != nil {
		if labels, err = s.resolveLabels(projectID, *attrs.LabelIDs); err != nil {
			return nil, err
		}
	}
	// Sem responsável, a tarefa fica disponível: quem bater o ponto nela a pega.
	var assignee *uuid.UUID
	if assigneeID != "" {
		uid, err := uuid.Parse(assigneeID)
		if err != nil {
			return nil, ErrInvalidAssignee.With("field", "assignee_id")
		}
		if assigneeID != selfID {
			isMember, err := s.membershipStore.IsPersonInProject(assigneeID, projectID)
			if err != nil {
				return nil, err
			}
			if !isMember {
				return nil, ErrAssigneeNotInTeam.With("field", "assignee_id")
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
	s.taskStore.created(task.ID, attrs.SkipPublish)
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

// UpdateAs altera a tarefa em nome de quem está logado; a regra do responsável é a do CreateAs. A descrição e o
// prazo são sempre gravados (um prazo nil mantém o que a tarefa tem); quem precisa manter a descrição ou apagar
// o prazo usa PatchAs.
func (s *Service) UpdateAs(selfID, id, name, description string, assigneeID *string, deadline *time.Time, attrs Attrs) (*Task, error) {
	return s.PatchAs(selfID, id, name, &description, assigneeID, deadlineOpt(deadline), attrs)
}

// deadlineOpt é o prazo de quem não diz "apagar": nil é ausente, e o valor, um prazo novo.
func deadlineOpt(d *time.Time) validate.Optional[time.Time] {
	if d == nil {
		return validate.Optional[time.Time]{}
	}
	return validate.Optional[time.Time]{Set: true, Value: d}
}

// resolveDeadline traduz o prazo do pedido para o que se grava: nil mantém, o tempo zero apaga (a tarefa sem
// prazo guarda o tempo zero) e uma data antes de 1971 é recusada, em vez de virar "sem prazo" sem aviso.
func resolveDeadline(o validate.Optional[time.Time]) (*time.Time, error) {
	if !o.Set {
		return nil, nil
	}
	if o.Value == nil {
		return &time.Time{}, nil
	}
	if o.Value.Before(noDeadline) {
		return nil, ErrDeadlineOutOfRange
	}
	return o.Value, nil
}

// PatchAs é o UpdateAs da API: a descrição nil mantém a que a tarefa tem, e o prazo distingue ausente (mantém),
// null (apaga) e um valor.
func (s *Service) PatchAs(selfID, id, name string, description *string, assigneeID *string, deadline validate.Optional[time.Time], attrs Attrs) (*Task, error) {
	name, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	if description != nil {
		if err := checkDescription(*description); err != nil {
			return nil, err
		}
	}
	dl, err := resolveDeadline(deadline)
	if err != nil {
		return nil, err
	}
	if attrs.Priority, err = cleanPriority(attrs.Priority); err != nil {
		return nil, err
	}
	if attrs.Status != nil && !validStatus(*attrs.Status) {
		return nil, ErrInvalidStatus.With("field", "status")
	}
	task, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	task.Name = name
	if description != nil {
		task.Description = *description
	}
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
	assignee, unassign, err := s.checkAssignee(selfID, task, assigneeID)
	if err != nil {
		return nil, err
	}
	if assignee != nil {
		task.AssigneeID = assignee
	} else if unassign {
		task.AssigneeID = nil
	}
	if dl != nil {
		task.Deadline = *dl
	}
	if err := s.taskStore.Update(task); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(id)
}

// cleanName apara o nome da tarefa e confere o tamanho. O servidor é a regra: a tela só adianta o aviso.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNameRequired.With("field", fieldName)
	}
	return validate.Required(fieldName, name, validate.MaxTaskName)
}

// checkDescription confere o tamanho da descrição, em caracteres.
func checkDescription(description string) error {
	if utf8.RuneCountInString(description) > MaxDescriptionLen {
		return ErrDescriptionTooLong.With("max", MaxDescriptionLen, "field", fieldDescription)
	}
	return nil
}

// cleanPriority confere a prioridade. A vazia vale "none", na criação e na edição; nil é ausente.
func cleanPriority(p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		v = "none"
	}
	if !validPriority(v) {
		return nil, ErrInvalidPriority.With("field", "priority")
	}
	return &v, nil
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

// checkAssignee confere o responsável pedido para a tarefa t. Nil mantém o que ela tem, vazio a deixa sem
// responsável (volta a ficar disponível) e um id só vale para quem está em algum time do projeto, salvo quem
// pede para si mesmo ou quem já é o responsável. Devolve a pessoa a gravar, ou unassign para tirá-la; com
// os dois zerados, nada muda.
func (s *Service) checkAssignee(selfID string, t *Task, assigneeID *string) (assignee *uuid.UUID, unassign bool, err error) {
	if assigneeID == nil {
		return nil, false, nil
	}
	if *assigneeID == "" {
		return nil, true, nil
	}
	uid, err := uuid.Parse(*assigneeID)
	if err != nil {
		return nil, false, ErrInvalidAssignee.With("field", "assignee_id")
	}
	if (t.AssigneeID == nil || uid != *t.AssigneeID) && *assigneeID != selfID {
		isMember, err := s.membershipStore.IsPersonInProject(*assigneeID, t.ProjectID.String())
		if err != nil {
			return nil, false, err
		}
		if !isMember {
			return nil, false, ErrAssigneeNotInTeam.With("field", "assignee_id")
		}
	}
	return &uid, false, nil
}

// UpdateAttrs é UpdateAttrsAs sem responsável e sem prazo, os campos que só quem edita os detalhes manda.
func (s *Service) UpdateAttrs(id string, attrs Attrs) (*Task, error) {
	return s.UpdateAttrsAs("", id, attrs, nil, nil)
}

// UpdateAttrsAs é a edição dos detalhes da tarefa: muda só a prioridade, o status, as etiquetas, o
// responsável e o prazo que vierem (os que faltam ficam como estão), sem o nome e a descrição. A regra do
// responsável é a do UpdateAs.
func (s *Service) UpdateAttrsAs(selfID, id string, attrs Attrs, assigneeID *string, deadline *time.Time) (*Task, error) {
	return s.PatchAttrsAs(selfID, id, attrs, assigneeID, deadlineOpt(deadline))
}

// PatchAttrsAs é o UpdateAttrsAs da API: o prazo distingue ausente (mantém), null (apaga) e um valor.
func (s *Service) PatchAttrsAs(selfID, id string, attrs Attrs, assigneeID *string, deadline validate.Optional[time.Time]) (*Task, error) {
	var err error
	if attrs.Priority, err = cleanPriority(attrs.Priority); err != nil {
		return nil, err
	}
	if attrs.Status != nil && !validStatus(*attrs.Status) {
		return nil, ErrInvalidStatus.With("field", "status")
	}
	dl, err := resolveDeadline(deadline)
	if err != nil {
		return nil, err
	}
	t, err := s.taskStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	patch := attrsPatch{priority: attrs.Priority, status: attrs.Status, deadline: dl}
	if attrs.LabelIDs != nil {
		resolved, err := s.resolveLabels(t.ProjectID.String(), *attrs.LabelIDs)
		if err != nil {
			return nil, err
		}
		patch.labels = &resolved
	}
	if patch.assignee, patch.unassign, err = s.checkAssignee(selfID, t, assigneeID); err != nil {
		return nil, err
	}
	if err := s.taskStore.UpdateAttrs(t.ID, patch); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(id)
}

// LinkExternalItem liga a tarefa, à mão, a um item da integração (uma issue, um cartão). Uma tarefa tem no
// máximo um item por integração, e um item serve a uma tarefa só. O id vale como a pessoa o escreveu ou, nos
// tipos que sabem, como o link do cartão colado (a sincronização o lê pela chave).
func (s *Service) LinkExternalItem(taskID, integrationID, externalItemID, externalItemURL string) (*Task, error) {
	// Os três campos são obrigatórios; o erro diz qual falta. O servidor confere tudo antes de ler o banco.
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return nil, ErrLinkFieldsRequired.With("field", "integration_id")
	}
	raw := strings.TrimSpace(externalItemID)
	if raw == "" {
		return nil, ErrLinkFieldsRequired.With("field", "external_item_id")
	}
	if utf8.RuneCountInString(raw) > validate.MaxItemURL {
		return nil, apperr.ErrFieldTooLong.With("field", "external_item_id", "max", validate.MaxExternalItem)
	}
	link := strings.TrimSpace(externalItemURL)
	if link == "" {
		return nil, ErrLinkFieldsRequired.With("field", "external_item_url")
	}
	if utf8.RuneCountInString(link) > validate.MaxItemURL {
		return nil, apperr.ErrFieldTooLong.With("field", "external_item_url", "max", validate.MaxItemURL)
	}
	// O link vira o href do cartão da tarefa: só http e https passam.
	link, ok := validate.HTTPURL(link)
	if !ok {
		return nil, apperr.ErrFieldInvalid.With("field", "external_item_url")
	}
	if utf8.RuneCountInString(link) > validate.MaxItemURL {
		return nil, apperr.ErrFieldTooLong.With("field", "external_item_url", "max", validate.MaxItemURL)
	}

	t, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(integrationID)
	if err != nil {
		return nil, ErrIntegrationNotFound.With("field", "integration_id")
	}
	integrationProject, kind, err := s.taskStore.IntegrationInfo(eid)
	if errors.Is(err, database.ErrNotFound) {
		return nil, ErrIntegrationNotFound.With("field", "integration_id")
	}
	if err != nil {
		return nil, err
	}
	if integrationProject != t.ProjectID {
		return nil, ErrIntegrationOtherProject.With("field", "integration_id")
	}
	item, err := itemKey(kind, raw)
	if err != nil {
		return nil, err
	}
	if err := s.taskStore.LinkItem(t.ID, eid, item, link); err != nil {
		// O conflito diz o campo que o resolve: o item (de outra tarefa) ou a integração (a tarefa já tem um item
		// nela). O campo entra aqui, e não no erro de origem, porque publicar uma tarefa também devolve
		// already_linked, e lá ele não é de um campo do formulário de vincular.
		switch {
		case errors.Is(err, ErrItemTaken):
			return nil, ErrItemTaken.With("field", "external_item_id")
		case errors.Is(err, ErrAlreadyLinked):
			return nil, ErrAlreadyLinked.With("field", "integration_id")
		}
		return nil, err
	}
	return s.taskStore.GetByID(taskID)
}

// itemKey é a chave do item como a sincronização a lê, conforme o tipo da integração: o número da issue (sem
// zeros à esquerda, que a plataforma não escreve) nos tipos numéricos, o link curto do cartão no Trello. Um
// tipo que o programa não conhece fica com o que foi escrito.
func itemKey(kind, raw string) (string, error) {
	item := raw
	if impl, err := adapter.GetIntegration(kind); err == nil {
		key, ok := adapter.NewItemKeyer(impl).Key(raw)
		if !ok {
			return "", apperr.ErrFieldInvalid.With("field", "external_item_id")
		}
		item = key
		if impl.Descriptor().ItemNumeric {
			n, err := strconv.Atoi(key)
			if err != nil || n < 1 || strings.HasPrefix(key, "+") {
				return "", apperr.ErrFieldInvalid.With("field", "external_item_id")
			}
			item = strconv.Itoa(n)
		}
	}
	if utf8.RuneCountInString(item) > validate.MaxExternalItem {
		return "", apperr.ErrFieldTooLong.With("field", "external_item_id", "max", validate.MaxExternalItem)
	}
	return item, nil
}

// UnlinkExternalItem solta a tarefa do item da integração. Sem integração, solta o único item que ela
// tem; com vários, a integração é obrigatória.
func (s *Service) UnlinkExternalItem(taskID, integrationID string) (*Task, error) {
	t, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	link, err := linkOf(t, integrationID)
	if err != nil {
		return nil, err
	}
	if err := s.taskStore.UnlinkItem(t.ID, link.IntegrationID); err != nil {
		return nil, err
	}
	return s.taskStore.GetByID(taskID)
}

// GetExternalDetails lê na plataforma o item a que a tarefa está ligada naquela integração (sem integração,
// o único que ela tem).
func (s *Service) GetExternalDetails(taskID, integrationID string) (*adapter.ExternalDetailsResult, error) {
	t, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, err
	}
	link, err := linkOf(t, integrationID)
	if err != nil {
		return nil, err
	}
	if s.integrationSvc == nil {
		return nil, ErrIntegrationsUnavailable
	}
	return s.integrationSvc.FetchItemDetails(link.IntegrationID.String(), link.ItemID)
}

// linkOf acha o vínculo pedido. Sem integração, vale o único vínculo da tarefa; com mais de um é preciso
// dizer qual (a primeira é a mais antiga, mas quem desfaz ou lê um vínculo não deve adivinhar).
func linkOf(t *Task, integrationID string) (*Link, error) {
	if integrationID == "" && len(t.Links) > 1 {
		return nil, ErrLinkFieldsRequired.With("field", "integration_id")
	}
	link := t.LinkFor(integrationID)
	if link == nil {
		return nil, ErrNoExternalItem
	}
	return link, nil
}
