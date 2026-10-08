package task

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entlabel "working-time-tracker/ent/label"
	"working-time-tracker/ent/task"
	"working-time-tracker/internal/database"
)

// O que a sincronização das issues do GitHub precisa da tarefa. As escritas daqui (ApplyRemote,
// CreateImported) não disparam o gancho: o gancho existe para avisar a sincronização de que a tarefa
// mudou aqui, e o que ela mesma grava não pode voltar a ela como mudança.

// SetChangeHook registra a função chamada, com o id da tarefa, depois de toda escrita que muda uma
// tarefa por dentro do sistema: editar, mudar status/prioridade/etiquetas, pegar uma tarefa livre e
// bater o ponto nela. Quem registra recebe só o aviso e não pode bloquear: a escrita já terminou, mas
// quem a fez está esperando a resposta.
func (s *Store) SetChangeHook(hook func(taskID uuid.UUID)) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	s.hook = hook
}

// SetCreateHook registra a função chamada, com o id da tarefa e as integrações em que a pessoa pediu para não
// postá-la (Attrs.SkipPublish), depois que uma tarefa é criada por dentro do sistema (a tela, a API). É um aviso
// à parte do de mudança: a sincronização o usa para postar a tarefa nova na plataforma que as recebe sozinha (o
// Trello). A tarefa que a própria sincronização importa não o dispara, nem a do seed. Quem registra recebe só o
// aviso e não pode bloquear.
func (s *Store) SetCreateHook(hook func(taskID uuid.UUID, skip []uuid.UUID)) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	s.createHook = hook
}

// created avisa o gancho de criação, se há um.
func (s *Store) created(id uuid.UUID, skip []uuid.UUID) {
	s.hookMu.RLock()
	hook := s.createHook
	s.hookMu.RUnlock()
	if hook != nil {
		hook(id, skip)
	}
}

func (s *Store) changed(id uuid.UUID) {
	s.hookMu.RLock()
	hook := s.hook
	s.hookMu.RUnlock()
	if hook != nil {
		hook(id)
	}
}

type changeHook struct {
	hookMu     sync.RWMutex
	hook       func(uuid.UUID)
	createHook func(uuid.UUID, []uuid.UUID)
}

// RemotePatch é o que a sincronização grava numa tarefa: só os campos que vierem.
type RemotePatch struct {
	Name        *string
	Description *string
	Status      *string
	// SetAssignee diz que o responsável passa a ser AssigneeID; nulo tira o responsável.
	SetAssignee bool
	AssigneeID  *uuid.UUID
	Labels      *[]Label
	// Deadline é o prazo novo; apontar para o tempo zero deixa a tarefa sem prazo.
	Deadline *time.Time
}

// ApplyRemote grava o que veio da plataforma na tarefa, sem o gancho e sem as regras de quem edita
// (limite da descrição, responsável do time): o que vem da issue é aceito como está.
func (s *Store) ApplyRemote(id uuid.UUID, p RemotePatch) error {
	q := s.client.Task.UpdateOneID(id)
	if p.Name != nil {
		q = q.SetName(*p.Name)
	}
	if p.Description != nil {
		q = q.SetDescription(*p.Description)
	}
	if p.Status != nil {
		q = q.SetStatus(statusOf(*p.Status))
	}
	if p.SetAssignee {
		if p.AssigneeID != nil {
			q = q.SetAssigneeID(*p.AssigneeID)
		} else {
			q = q.ClearAssigneeID()
		}
	}
	if p.Labels != nil {
		q = q.ClearLabels().AddLabelIDs(labelIDs(*p.Labels)...)
	}
	if p.Deadline != nil {
		q = q.SetDeadline(*p.Deadline)
	}
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	return err
}

// Imported é a tarefa que nasce de um item da plataforma. O vínculo com o item é de quem importa: ele grava a
// linha junto com a tarefa, na mesma transação.
type Imported struct {
	ProjectID   uuid.UUID
	Name        string
	Description string
	Labels      []Label
	AssigneeID  *uuid.UUID
	// Deadline é o prazo da tarefa; zero é sem prazo (a issue não tem um). CreatedAt é quando o item
	// nasceu na plataforma; zero deixa a tarefa nascer agora.
	Deadline  time.Time
	CreatedAt time.Time
}

// CreateImported cria a tarefa de um item: em backlog, sem prazo (a menos que o item traga uma data de
// entrega). Não passa pelas regras de CreateAs: o responsável já foi conferido por quem importa.
func (s *Store) CreateImported(in Imported) (*Task, error) {
	t := &Task{
		ProjectID:   in.ProjectID,
		Name:        in.Name,
		Description: in.Description,
		Priority:    "none",
		Status:      StatusBacklog,
		Labels:      in.Labels,
		AssigneeID:  in.AssigneeID,
		Deadline:    in.Deadline,
		CreatedAt:   in.CreatedAt,
	}
	if err := s.Create(t); err != nil {
		return nil, err
	}
	return s.GetByID(t.ID.String())
}

// listByIDsChunk é quantos ids vão num IN: um repositório grande liga dezenas de milhares de tarefas.
const listByIDsChunk = 10000

// ListByIDs lista as tarefas com estes ids (as que não existem mais ficam de fora), com os vínculos.
func (s *Store) ListByIDs(ids []uuid.UUID) ([]Task, error) {
	var out []Task
	for start := 0; start < len(ids); start += listByIDsChunk {
		end := min(start+listByIDsChunk, len(ids))
		rows, err := s.client.Task.Query().
			Where(task.IDIn(ids[start:end]...)).
			WithAssignee().WithIssueSyncs(withLinks).WithLabels(withLabels).
			Order(ent.Asc(task.FieldCreatedAt, task.FieldID)).
			All(context.Background())
		if err != nil {
			return nil, err
		}
		out = append(out, toDomainTasks(rows)...)
	}
	return out, nil
}

// FindOrCreateLabels devolve as etiquetas do projeto com estes nomes, criando as que faltam. O nome
// vale sem diferenciar maiúsculas (a etiqueta que já existe é a que volta, com a caixa dela) e é
// cortado no tamanho máximo. Duas rodadas criando a mesma etiqueta ao mesmo tempo acabam na mesma.
func (s *Store) FindOrCreateLabels(projectID uuid.UUID, names []string) ([]Label, error) {
	var out []Label
	seen := map[uuid.UUID]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if utf8.RuneCountInString(name) > maxLabelLen {
			name = string([]rune(name)[:maxLabelLen])
		}
		if name == "" {
			continue
		}
		label, err := s.labelByName(projectID, name)
		if err != nil {
			return nil, err
		}
		if label == nil {
			if label, err = s.createLabel(projectID, name); err != nil {
				if !ent.IsConstraintError(err) {
					return nil, err
				}
				// Outra escrita criou a mesma etiqueta entre a busca e o INSERT.
				if label, err = s.labelByName(projectID, name); err != nil || label == nil {
					return nil, err
				}
			}
		}
		if !seen[label.ID] {
			seen[label.ID] = true
			out = append(out, *label)
		}
	}
	return out, nil
}

func (s *Store) labelByName(projectID uuid.UUID, name string) (*Label, error) {
	row, err := s.client.Label.Query().
		Where(entlabel.ProjectIDEQ(projectID), entlabel.NameEqualFold(name)).
		First(context.Background())
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Label{ID: row.ID, Name: row.Name}, nil
}

// taskIDsWithLabel lista as tarefas que têm a etiqueta, para avisar o gancho quando ela muda de nome
// ou some.
func (s *Store) taskIDsWithLabel(labelID uuid.UUID) ([]uuid.UUID, error) {
	return s.client.Task.Query().Where(task.HasLabelsWith(entlabel.IDEQ(labelID))).IDs(context.Background())
}
