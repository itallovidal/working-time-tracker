package issuesync

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/task"
)

// Published é o que sobrou de postar uma tarefa como issue: a tarefa já ligada a ela e, quando o GitHub
// deixou algo de fora (o responsável sem usuário que se ache pelo e-mail, uma etiqueta recusada), o
// código do aviso.
type Published struct {
	Task    *task.Task `json:"task"`
	Problem string     `json:"problem,omitempty"`
}

// Publish posta a tarefa como uma issue nova do repositório da integração e a liga a ela: o nome é o
// título, a descrição é o corpo, as etiquetas são criadas no repositório se faltarem, e o responsável
// só vai se o e-mail público de um usuário do GitHub for o dele. Prazo e prioridade não existem numa
// issue e ficam só aqui. Dali em diante a tarefa e a issue são sincronizadas como as outras.
//
// A integração precisa estar ativa, com a sincronização ligada (é ela que mantém as duas iguais) e com um
// token que escreve no repositório. Espera a vez da integração: uma rodada em andamento termina antes.
func (s *Syncer) Publish(ctx context.Context, taskID, integrationID uuid.UUID) (*Published, error) {
	t, err := s.d.Tasks.GetByID(taskID.String())
	if err != nil {
		return nil, err
	}
	if t.LinkFor(integrationID.String()) != nil {
		return nil, ErrAlreadyLinked
	}
	integ, err := s.d.Integrations.Get(integrationID.String())
	if err != nil {
		return nil, err
	}
	if integ.ProjectID != t.ProjectID {
		return nil, task.ErrIntegrationOtherProject
	}

	lock := s.lock(integrationID)
	lock.Lock()
	defer lock.Unlock()
	s.cache.forget(integrationID.String() + "/")

	// A conferência de cima foi antes da espera pela vez da integração, que pode ter sido longa (uma rodada
	// em andamento): outra postagem ou um vínculo à mão pode ter ligado a tarefa neste meio-tempo.
	if t, err = s.d.Tasks.GetByID(taskID.String()); err != nil {
		return nil, err
	}
	if t.LinkFor(integrationID.String()) != nil {
		return nil, ErrAlreadyLinked
	}

	r, err := s.newRun(ctx, integrationID, false)
	if err != nil {
		return nil, err
	}
	// Sem permissão de escrita o GitHub abre a issue mas joga fora as etiquetas e o responsável sem avisar:
	// a tarefa ficaria ligada a uma issue diferente dela. Melhor recusar.
	if r.readOnly {
		return nil, ErrPublishReadOnly
	}

	labels := make([]string, 0, len(t.Labels))
	for _, l := range t.Labels {
		labels = append(labels, l.Name)
	}
	r.ensureLabels(labels)
	var assignees []string
	if t.AssigneeID != nil && r.caps.Assignee {
		login, err := resolver{r}.LoginFor(ctx, *t.AssigneeID)
		if err != nil && !stop(err) {
			login = "" // a busca do usuário falhou: a issue sai sem responsável, e o aviso diz
		} else if err != nil {
			return nil, err
		}
		if login == "" {
			r.note(ErrNoLogin.Code)
		} else {
			assignees = []string{login}
		}
	}

	in := adapter.NewIssue{Title: t.Name, Body: t.Description, Labels: labels, Assignees: assignees}
	if r.caps.Deadline {
		in.Deadline = normDeadline(t.Deadline)
	}
	issue, err := r.src.CreateIssue(ctx, r.conn, in)
	if err != nil {
		return nil, err
	}
	// O GitHub devolveu 201 mas o responsável pedido não ficou na issue (o usuário não pode ser designado).
	if len(assignees) > 0 && len(issue.Assignees) == 0 {
		r.note(ErrPushDiscarded.Code)
	}

	// Liga a tarefa à issue: o vínculo nasce pending, e o handle abaixo o adota (grava o acordo e empurra o
	// que a plataforma deixou de fora). Se gravar o vínculo falhasse depois de a issue existir, a próxima
	// rodada completa a importaria como uma tarefa nova: tenta algumas vezes e, se não der, o erro volta para
	// quem chamou, com o endereço da issue no log.
	row := &Row{IntegrationID: integrationID, TaskID: &t.ID, ItemID: issue.ID, State: statePending, URL: issue.URL, SyncedAt: s.cfg.Now()}
	if err := s.createLink(ctx, row); err != nil {
		s.cfg.Logger.Error("issue sync: posted an item but could not link it", "task", t.ID, "integration", integrationID, "item", issue.URL, "error", err)
		return nil, err
	}
	// Relê a tarefa para ela já ter o vínculo novo (a sincronização trata diferente a tarefa em mais de um item).
	if t, err = s.d.Tasks.GetByID(taskID.String()); err != nil {
		return nil, err
	}
	r.rows = map[string]*Row{row.ItemID: row}
	r.tasks = map[uuid.UUID]*task.Task{t.ID: t}
	if err := r.handle(*issue); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	fresh, err := s.d.Tasks.GetByID(taskID.String())
	if err != nil {
		return nil, err
	}
	return &Published{Task: fresh, Problem: r.problem}, nil
}

// createLink grava o vínculo de um item recém-postado, tentando de novo se o banco falhar: o item já existe na
// plataforma, e sem o vínculo ele voltaria como tarefa nova.
func (s *Syncer) createLink(ctx context.Context, row *Row) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = s.d.Rows.Create(row); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	return err
}
