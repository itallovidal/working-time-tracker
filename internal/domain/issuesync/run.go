package issuesync

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/task"
)

func toRemote(i adapter.Issue) Remote {
	return Remote{Title: i.Title, Body: i.Body, Closed: i.State == stateClosed, Labels: i.Labels, Assignee: i.Assignees, Deadline: i.Deadline}
}

func toLocal(t *task.Task) Local {
	l := Local{Title: t.Name, Body: t.Description, Closed: t.Status == "closed", Person: t.AssigneeID, Deadline: t.Deadline}
	for _, label := range t.Labels {
		l.Labels = append(l.Labels, label.Name)
	}
	return l
}

// snapshotFrom é o acordo depois da rodada: a issue como ficou, e o responsável ligado que a rodada decidiu.
func snapshotFrom(i adapter.Issue, m Mapping) Snapshot {
	return Snapshot{
		Title:        norm(i.Title),
		Body:         norm(i.Body),
		Closed:       i.State == stateClosed,
		Labels:       append([]string{}, i.Labels...),
		Logins:       append([]string{}, i.Assignees...),
		MappedLogin:  m.Login,
		MappedPerson: m.Person,
		Deadline:     normDeadline(i.Deadline),
	}
}

func stateOf(i adapter.Issue) string {
	if i.State == stateClosed {
		return stateClosed
	}
	return stateOpen
}

func toPatch(p Push) adapter.IssuePatch {
	return adapter.IssuePatch{Title: p.Title, Body: p.Body, State: p.State, StateReason: p.StateReason, Labels: p.Labels, Assignees: p.Assignees, Deadline: p.Deadline}
}

// mergeOptions diz à fusão o que o tipo da integração sabe sincronizar.
func (r *run) mergeOptions() []MergeOption {
	var opts []MergeOption
	if r.caps.Deadline {
		opts = append(opts, WithDeadline())
	}
	if !r.caps.Assignee {
		opts = append(opts, WithoutAssignee())
	}
	return opts
}

// matches diz se a issue ficou com tudo o que se mandou. Um campo que o GitHub descarta sem avisar
// (o token sem permissão, a etiqueta que não existe, o responsável que não pode ser) aparece aqui como diferença.
func matches(after adapter.Issue, p Push) bool {
	switch {
	case p.Title != nil && norm(after.Title) != *p.Title,
		p.Body != nil && norm(after.Body) != *p.Body,
		p.State != nil && after.State != *p.State,
		p.Labels != nil && !sameKeys(keyed(after.Labels), keyed(*p.Labels)),
		p.Assignees != nil && !sameKeys(keyed(after.Assignees), keyed(*p.Assignees)),
		p.Deadline != nil && !normDeadline(after.Deadline).Equal(normDeadline(*p.Deadline)):
		return false
	}
	return true
}

// linkedTo diz se a tarefa ainda está ligada a este item desta integração. Quem a desvincula, ou a liga a
// outro, tira a tarefa da sincronização.
func (r *run) linkedTo(t *task.Task, id string) bool {
	if t.ExternalIntegrationID == nil || *t.ExternalIntegrationID != r.integ.ID || t.ExternalItemID == nil {
		return false
	}
	key, ok := r.k.key(*t.ExternalItemID)
	return ok && key == id
}

func sameRow(a, b *Row) bool {
	same := func(x, y *uuid.UUID) bool { return samePerson(x, y) }
	return a.State == b.State && same(a.TaskID, b.TaskID) && norm(a.Title) == norm(b.Title) && norm(a.Body) == norm(b.Body) &&
		sameKeys(keyed(a.Labels), keyed(b.Labels)) && sameKeys(keyed(a.Logins), keyed(b.Logins)) &&
		a.MappedLogin == b.MappedLogin && same(a.MappedPerson, b.MappedPerson) &&
		normDeadline(a.Deadline).Equal(normDeadline(b.Deadline)) &&
		a.StuckSig == b.StuckSig && a.LastError == b.LastError
}

// importIssue cria a tarefa de uma issue aberta que ainda não tinha, já ligada a ela.
func (r *run) importIssue(issue adapter.Issue) error {
	labels, err := r.s.d.Tasks.FindOrCreateLabels(r.integ.ProjectID, issue.Labels)
	if err != nil {
		return err
	}
	res := resolver{r}
	var mapped Mapping
	if r.caps.Assignee {
		for _, login := range issue.Assignees {
			p, err := res.PersonFor(r.ctx, login)
			if err != nil {
				return err
			}
			if p != nil {
				mapped = Mapping{Login: login, Person: p}
				break
			}
		}
	}
	name := strings.TrimSpace(lf(issue.Title))
	if name == "" {
		if r.numeric {
			name = "Issue #" + issue.ID
		} else {
			name = r.label + " " + issue.ID
		}
	}
	imported := task.Imported{
		ProjectID: r.integ.ProjectID, IntegrationID: r.integ.ID, ItemID: issue.ID, URL: issue.URL,
		Name: name, Description: lf(issue.Body), Labels: labels, AssigneeID: mapped.Person, CreatedAt: issue.CreatedAt,
	}
	if r.caps.Deadline {
		imported.Deadline = normDeadline(issue.Deadline)
	}
	created, err := r.s.d.Tasks.CreateImported(imported)
	if err != nil {
		return err
	}
	row := &Row{
		IntegrationID: r.integ.ID, TaskID: &created.ID, ItemID: issue.ID, State: stateOf(issue),
		Snapshot: snapshotFrom(issue, mapped), SyncedAt: r.s.cfg.Now(),
	}
	if err := r.s.d.Rows.Create(row); err != nil {
		// Sem o vínculo a tarefa seria importada de novo a cada rodada.
		r.s.d.Tasks.Delete(created.ID.String())
		return err
	}
	r.rows[issue.ID] = row
	r.tasks[created.ID] = created
	r.bound[created.ID] = true
	r.sum.Created++
	return nil
}

// adopt passa a sincronizar uma tarefa que alguém ligou à mão à issue. Não há acordo anterior: o GitHub
// vence no título e no corpo, as etiquetas dos dois lados se somam, e o responsável da tarefa fica como está.
func (r *run) adopt(row *Row, t *task.Task, issue adapter.Issue) error {
	if row == nil {
		row = &Row{IntegrationID: r.integ.ID, ItemID: issue.ID}
	}
	b := Snapshot{Logins: issue.Assignees, MappedPerson: t.AssigneeID}
	if err := r.apply(row, t, issue, b); err != nil {
		return err
	}
	return nil
}

// reconcile sincroniza uma issue que já tem tarefa.
func (r *run) reconcile(row *Row, issue adapter.Issue) error {
	t := r.tasks[*row.TaskID]
	if t == nil {
		got, err := r.s.d.Tasks.GetByID(row.TaskID.String())
		if errors.Is(err, database.ErrNotFound) {
			return r.release(row)
		}
		if err != nil {
			return err
		}
		t = got
	}
	if !r.linkedTo(t, row.ItemID) {
		return r.release(row)
	}
	return r.apply(row, t, issue, row.snapshot())
}

// release solta a tarefa do vínculo: a issue vira uma issue descartada.
func (r *run) release(row *Row) error {
	if row.TaskID != nil {
		delete(r.bound, *row.TaskID)
	}
	row.TaskID = nil
	return r.s.d.Rows.Save(row)
}

// apply é a sincronização de uma issue com a tarefa: decide pelo Merge, grava na tarefa o que veio do
// GitHub, manda para o GitHub o que mudou aqui e guarda o novo acordo. Qualquer erro no meio deixa o
// acordo como estava: tudo o que se grava na tarefa é repetível, e a próxima rodada refaz.
func (r *run) apply(row *Row, t *task.Task, issue adapter.Issue, b Snapshot) error {
	res := resolver{r}
	local := toLocal(t)
	plan, err := Merge(r.ctx, b, toRemote(issue), local, res, r.mergeOptions()...)
	if err != nil {
		return err
	}

	push, stuck := plan.Push, false
	if !push.Empty() {
		switch {
		case r.readOnly:
			push = Push{}
		case row.StuckSig == push.Signature():
			// O GitHub já descartou exatamente isto, e nada mudou: insistir não adianta.
			push, stuck = Push{}, true
		case r.pushes >= r.s.cfg.MaxPushes:
			push = Push{}
			r.sum.Partial = true
		default:
			// A lista pode ter alguns instantes: relê a issue para não passar por cima de quem mexeu depois.
			fresh, err := r.src.GetIssue(r.ctx, r.conn, issue.ID)
			if errors.Is(err, adapter.ErrIssueGone) || (err == nil && fresh.PullRequest) {
				return r.gone(row)
			}
			if err != nil {
				return err
			}
			issue = *fresh
			if plan, err = Merge(r.ctx, b, toRemote(issue), local, res, r.mergeOptions()...); err != nil {
				return err
			}
			push = plan.Push
			if !push.Empty() && row.StuckSig == push.Signature() {
				push, stuck = Push{}, true
			}
		}
	}

	if !plan.Local.Empty() {
		patch, err := r.localPatch(t, plan.Local)
		if err != nil {
			return err
		}
		if err := r.s.d.Tasks.ApplyRemote(t.ID, patch); err != nil {
			return err
		}
		r.sum.Updated++
		if plan.Local.Status != nil && *plan.Local.Status == "closed" {
			r.sum.Closed++
		}
	}

	after, discarded, rejected := issue, false, false
	if !push.Empty() {
		if push.Labels != nil {
			r.ensureLabels(*push.Labels)
		}
		updated, err := r.src.UpdateIssue(r.ctx, r.conn, issue.ID, toPatch(push))
		switch {
		case errors.Is(err, adapter.ErrIssueGone):
			return r.gone(row)
		case errors.Is(err, adapter.ErrForbidden):
			// O token não escreve aqui: as outras issues desta rodada nem tentam.
			r.readOnly = true
			r.note(ErrReadOnly.Code)
		case isRejection(err):
			rejected = true
		case err != nil:
			return err
		default:
			after = *updated
			r.pushes++
			r.sum.Pushed++
			discarded = !matches(after, push)
		}
	}

	next := *row
	next.TaskID = &t.ID
	next.State = stateOf(after)
	next.Snapshot = snapshotFrom(after, plan.Mapped)
	switch {
	case rejected:
		next.StuckSig, next.LastError = push.Signature(), ErrPushRejected.Code
	case discarded:
		next.StuckSig, next.LastError = push.Signature(), ErrPushDiscarded.Code
	case stuck:
		if next.LastError == "" {
			next.LastError = ErrPushDiscarded.Code
		}
	default:
		next.StuckSig, next.LastError = "", plan.Problem
	}
	r.note(next.LastError)
	r.note(plan.Problem)

	if row.ID != uuid.Nil && sameRow(row, &next) {
		return nil
	}
	next.SyncedAt = r.s.cfg.Now()
	if row.ID == uuid.Nil {
		if err := r.s.d.Rows.Create(&next); err != nil {
			return err
		}
	} else if err := r.s.d.Rows.Save(&next); err != nil {
		return err
	}
	*row = next
	r.rows[row.ItemID] = row
	r.bound[t.ID] = true
	return nil
}

// gone anota que a issue sumiu. O vínculo novo (de uma tarefa que se ia adotar) nem chega a ser criado.
func (r *run) gone(row *Row) error {
	if row.ID != uuid.Nil {
		r.markGone(row)
	}
	return nil
}

// isRejection diz se o GitHub recusou a mudança por ela mesma (422, 400): repetir não muda nada.
func isRejection(err error) bool {
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != adapter.ErrProviderStatus.Code {
		return false
	}
	status, ok := e.Params["status"].(int)
	return ok && status >= 400 && status < 500
}

// localPatch monta o que se grava na tarefa a partir do que o Merge decidiu.
func (r *run) localPatch(t *task.Task, c LocalChange) (task.RemotePatch, error) {
	var p task.RemotePatch
	if c.Title != nil && strings.TrimSpace(*c.Title) != "" {
		name := strings.TrimSpace(*c.Title)
		p.Name = &name
	}
	p.Description = c.Body
	p.Status = c.Status
	p.SetAssignee, p.AssigneeID = c.SetAssignee, c.Person
	p.Deadline = c.Deadline
	if c.Labels != nil {
		labels, err := r.s.d.Tasks.FindOrCreateLabels(t.ProjectID, *c.Labels)
		if err != nil {
			return p, err
		}
		if labels == nil {
			labels = []task.Label{}
		}
		p.Labels = &labels
	}
	return p, nil
}

// ensureLabels cria no repositório as etiquetas que a issue vai receber e ele ainda não tem: o GitHub
// descarta, sem dizer nada, a que não existe. Se o token não pode criá-las, o aviso fica na integração e
// o PATCH mostra o descarte.
func (r *run) ensureLabels(names []string) {
	if r.labels == nil {
		have, err := r.src.ListLabels(r.ctx, r.conn)
		if err != nil {
			r.s.cfg.Logger.Warn("issue sync: listing the repository labels", "integration", r.integ.ID, "error", err)
			return
		}
		r.labels = make(map[string]bool, len(have))
		for _, n := range have {
			r.labels[key(n)] = true
		}
	}
	for _, name := range names {
		if k := key(name); k == "" || r.labels[k] {
			continue
		} else if err := r.src.CreateLabel(r.ctx, r.conn, name); err != nil {
			if errors.Is(err, adapter.ErrForbidden) {
				r.note(ErrLabelRefused.Code)
			} else {
				r.s.cfg.Logger.Warn("issue sync: creating a label", "integration", r.integ.ID, "label", name, "error", err)
			}
		} else {
			r.labels[k] = true
		}
	}
}

// resolver liga logins do GitHub a pessoas do projeto, pelo e-mail.
type resolver struct{ r *run }

// PersonFor acha a pessoa do projeto cujo e-mail é o público do login. O e-mail só existe se a pessoa o
// publica no perfil do GitHub, e só vale se é de alguém da organização que está neste projeto: o e-mail
// é único no sistema todo, e sem essa conferência uma issue ligaria a tarefa a alguém de outra empresa.
func (x resolver) PersonFor(ctx context.Context, login string) (*uuid.UUID, error) {
	r := x.r
	email, err := r.s.cache.get(r.integ.ID.String()+"/user/"+key(login), cacheFound, cacheMissing, func() (string, error) {
		return r.src.UserEmail(ctx, r.conn, login)
	})
	if err != nil || email == "" {
		return nil, err
	}
	p, err := r.s.d.People.FindByEmailInOrg(r.orgID, email)
	if errors.Is(err, database.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inProject, err := r.s.d.Members.IsPersonInProject(p.ID.String(), r.integ.ProjectID.String())
	if err != nil || !inProject {
		return nil, err
	}
	return &p.ID, nil
}

// LoginFor acha o usuário do GitHub que publica o e-mail da pessoa.
func (x resolver) LoginFor(ctx context.Context, personID uuid.UUID) (string, error) {
	r := x.r
	p, err := r.s.d.People.GetByID(personID.String())
	if errors.Is(err, database.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return r.s.cache.get(r.integ.ID.String()+"/email/"+key(p.Email), cacheFound, cacheMissing, func() (string, error) {
		return r.src.FindLoginByEmail(ctx, r.conn, p.Email)
	})
}
