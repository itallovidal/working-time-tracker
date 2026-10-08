package issuesync

import (
	"context"
	"errors"

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
	if t.ExternalIntegrationID != nil || t.ExternalItemID != nil {
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
	if t.AssigneeID != nil {
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

	issue, err := r.gh.CreateIssue(ctx, r.conn, adapter.NewIssue{Title: t.Name, Body: t.Description, Labels: labels, Assignees: assignees})
	if err != nil {
		return nil, err
	}
	// O GitHub devolveu 201 mas o responsável pedido não ficou na issue (o usuário não pode ser designado).
	if len(assignees) > 0 && len(issue.Assignees) == 0 {
		r.note(ErrPushDiscarded.Code)
	}

	// Liga a tarefa à issue e deixa o vínculo de sincronização pronto. Se isto falhasse depois de a issue
	// existir, a próxima rodada completa a importaria como uma tarefa nova: o erro volta para quem chamou.
	number := issue.Number
	itemID, url := itoa(number), issue.URL
	t.ExternalIntegrationID, t.ExternalItemID, t.ExternalItemURL = &integrationID, &itemID, &url
	if err := s.d.Tasks.Update(t); err != nil {
		return nil, err
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	if err := r.handle(*issue); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	fresh, err := s.d.Tasks.GetByID(taskID.String())
	if err != nil {
		return nil, err
	}
	return &Published{Task: fresh, Problem: r.problem}, nil
}
