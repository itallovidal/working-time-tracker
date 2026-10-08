package task

import (
	"context"
	"strings"
	"sync"
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

func (s *Store) changed(id uuid.UUID) {
	s.hookMu.RLock()
	hook := s.hook
	s.hookMu.RUnlock()
	if hook != nil {
		hook(id)
	}
}

type changeHook struct {
	hookMu sync.RWMutex
	hook   func(uuid.UUID)
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
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	return err
}

// Imported é a tarefa que nasce de uma issue.
type Imported struct {
	ProjectID     uuid.UUID
	IntegrationID uuid.UUID
	ItemID        string // o número da issue
	URL           string
	Name          string
	Description   string
	Labels        []Label
	AssigneeID    *uuid.UUID
}

// CreateImported cria a tarefa de uma issue: em backlog, sem prazo (a issue não tem um) e já ligada a
// ela. Não passa pelas regras de CreateAs: o responsável já foi conferido por quem importa.
func (s *Store) CreateImported(in Imported) (*Task, error) {
	t := &Task{
		ProjectID:             in.ProjectID,
		Name:                  in.Name,
		Description:           in.Description,
		Priority:              "none",
		Status:                StatusBacklog,
		Labels:                in.Labels,
		AssigneeID:            in.AssigneeID,
		ExternalIntegrationID: &in.IntegrationID,
		ExternalItemID:        &in.ItemID,
		ExternalItemURL:       &in.URL,
	}
	if err := s.Create(t); err != nil {
		return nil, err
	}
	return s.GetByID(t.ID.String())
}

// ListLinked lista as tarefas ligadas a algum item da integração, da mais antiga para a mais nova.
func (s *Store) ListLinked(integrationID uuid.UUID) ([]Task, error) {
	rows, err := s.client.Task.Query().
		Where(task.ExternalIntegrationIDEQ(integrationID), task.ExternalItemIDNotNil()).
		WithAssignee().WithExternalIntegration().WithLabels(withLabels).
		Order(ent.Asc(task.FieldCreatedAt, task.FieldID)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainTasks(rows), nil
}

// FindLinked lista as tarefas ligadas a este item da integração (a mais antiga primeiro): pode haver
// mais de uma, porque o vínculo manual não impede dois.
func (s *Store) FindLinked(integrationID uuid.UUID, itemID string) ([]Task, error) {
	rows, err := s.client.Task.Query().
		Where(task.ExternalIntegrationIDEQ(integrationID), task.ExternalItemIDEQ(itemID)).
		WithAssignee().WithExternalIntegration().WithLabels(withLabels).
		Order(ent.Asc(task.FieldCreatedAt, task.FieldID)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainTasks(rows), nil
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
