// Package issuesync sincroniza as issues abertas de um repositório do GitHub com as tarefas de um
// projeto, nos dois sentidos. O vínculo de cada issue guarda o snapshot do último acordo entre os dois
// lados, e é contra ele que se vê, campo a campo, quem mudou o quê desde a última rodada.
package issuesync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Este arquivo é a regra de conflito, sem tocar em banco nem em rede. A rodada (service.go) lê os três
// lados — o snapshot B do último acordo, a issue R no GitHub e a tarefa L aqui —, pergunta a Merge o
// que fazer e executa.
//
// Para cada campo: se o GitHub mudou desde B, o GitHub vence e a tarefa passa a ter o valor dele; se só
// a tarefa mudou, a mudança vai para a issue; se nenhum mudou, nada acontece. As etiquetas não
// conflitam: cada lado soma e tira o que fez, e o resultado vai para os dois.

// Remote é a issue no GitHub, no que a sincronização usa dela.
type Remote struct {
	Title    string
	Body     string
	Closed   bool
	Labels   []string
	Assignee []string // logins
	Deadline time.Time
}

// Local é a tarefa.
type Local struct {
	Title  string
	Body   string
	Closed bool // status "closed"; os outros três contam como aberta
	Labels []string
	People []uuid.UUID // os responsáveis
	// Deadline é o prazo da tarefa; zero (ou antes de 1971) é sem prazo.
	Deadline time.Time
}

// Snapshot é o último acordo entre os dois lados.
type Snapshot struct {
	Title  string
	Body   string
	Closed bool
	Labels []string
	// Logins são os responsáveis que a issue tinha no acordo; Mapped são os que deles são responsáveis da tarefa,
	// cada um com a pessoa. Os outros logins não se ligam a ninguém do projeto.
	Logins []string
	Mapped []Mapping
	// Deadline é o prazo que os dois lados tinham no acordo; zero é sem prazo.
	Deadline time.Time
}

// Resolver liga um login do GitHub a uma pessoa daqui, e o contrário. Os dois podem falar com a rede.
type Resolver interface {
	// PersonFor devolve a pessoa do projeto cujo e-mail é o público do login, ou nil.
	PersonFor(ctx context.Context, login string) (*uuid.UUID, error)
	// LoginFor devolve o login do GitHub da pessoa, ou vazio se não se acha um.
	LoginFor(ctx context.Context, person uuid.UUID) (string, error)
}

// LocalChange é o que a tarefa passa a ter por vir do GitHub; os campos nulos ficam como estão.
type LocalChange struct {
	Title  *string
	Body   *string
	Status *string // "closed", ou "backlog" na issue reaberta
	Labels *[]string
	// AddPeople e RemovePeople são quem entra e quem sai dos responsáveis da tarefa.
	AddPeople    []uuid.UUID
	RemovePeople []uuid.UUID
	// Deadline é o prazo novo da tarefa; apontar para o tempo zero a deixa sem prazo.
	Deadline *time.Time
}

// Empty diz se não há nada a gravar na tarefa.
func (c LocalChange) Empty() bool {
	return c.Title == nil && c.Body == nil && c.Status == nil && c.Labels == nil && len(c.AddPeople) == 0 &&
		len(c.RemovePeople) == 0 && c.Deadline == nil
}

// Push é o que a issue passa a ter por vir daqui; os campos nulos não vão no PATCH.
type Push struct {
	Title       *string
	Body        *string
	State       *string // "open" ou "closed"
	StateReason *string
	Labels      *[]string
	Assignees   *[]string
	// Deadline é a data de entrega nova do item; apontar para o tempo zero a tira.
	Deadline *time.Time
}

// Empty diz se não há nada a mandar.
func (p Push) Empty() bool {
	return p.Title == nil && p.Body == nil && p.State == nil && p.Labels == nil && p.Assignees == nil && p.Deadline == nil
}

// Signature identifica o conteúdo do que se manda. Uma mudança que o GitHub descarta sem avisar
// reaparece igual a cada rodada; com a assinatura da última tentativa guardada, a rodada vê que é a
// mesma e não insiste.
func (p Push) Signature() string {
	if p.Empty() {
		return ""
	}
	raw, _ := json.Marshal(struct {
		T, B, S *string
		L, A    *[]string
		D       *time.Time `json:",omitempty"` // fora da assinatura quando não há prazo, como antes dele existir
	}{p.Title, p.Body, p.State, sortedPtr(p.Labels), sortedPtr(p.Assignees), p.Deadline})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func sortedPtr(list *[]string) *[]string {
	if list == nil {
		return nil
	}
	out := make([]string, len(*list))
	for i, v := range *list {
		out[i] = strings.ToLower(v)
	}
	sort.Strings(out)
	return &out
}

// Mapping é a ligação entre um responsável da tarefa e um login da issue, a gravar no snapshot.
type Mapping struct {
	Login  string
	Person uuid.UUID
}

// Plan é o resultado de Merge.
type Plan struct {
	Local LocalChange
	Push  Push
	// Mapped são os pares login/pessoa do acordo novo.
	Mapped []Mapping
	// Problem é o código do que impediu parte do trabalho sem ser um erro (o login que não se acha).
	Problem string
}

// norm é o texto como se compara: finais de linha do Windows viram \n e as pontas ficam sem espaço. O
// editor do GitHub grava \r\n, a nossa caixa de texto grava \n, e nenhuma das duas é uma mudança.
func norm(s string) string {
	return strings.TrimSpace(lf(s))
}

// lf troca os finais de linha por \n, sem mexer no resto: é o texto que se grava na tarefa.
func lf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func key(label string) string { return strings.ToLower(strings.TrimSpace(label)) }

// keyed devolve o conjunto de nomes por chave (sem diferenciar maiúsculas), guardando o primeiro nome visto.
func keyed(names []string) map[string]string {
	out := make(map[string]string, len(names))
	for _, n := range names {
		if k := key(n); k != "" {
			if _, seen := out[k]; !seen {
				out[k] = strings.TrimSpace(n)
			}
		}
	}
	return out
}

func sameKeys(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func samePerson(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// MergeOption liga ou desliga o que um tipo de integração sabe sincronizar além do título, do corpo, do
// estado e das etiquetas.
type MergeOption func(*mergeOpts)

type mergeOpts struct {
	deadline bool
	noAssign bool
}

// WithDeadline faz o prazo da tarefa e a data de entrega do item se espelharem.
func WithDeadline() MergeOption { return func(o *mergeOpts) { o.deadline = true } }

// WithoutAssignee deixa o responsável de fora: o tipo não o liga a ninguém, e a fusão nem o procura.
func WithoutAssignee() MergeOption { return func(o *mergeOpts) { o.noAssign = true } }

// normDeadline é o prazo como se compara: em UTC, em segundos, e o tempo zero (ou qualquer data antes de
// 1971, como a tarefa guarda o "sem prazo") vira o zero. A plataforma devolve milissegundos e o que
// guardamos tem outra precisão; sem isto o mesmo instante pareceria mudar a cada rodada.
func normDeadline(t time.Time) time.Time {
	if t.IsZero() || t.Year() < 1971 {
		return time.Time{}
	}
	return t.UTC().Truncate(time.Second)
}

// Merge decide o que fazer com uma issue e a tarefa ligada a ela. Não grava nada.
func Merge(ctx context.Context, b Snapshot, r Remote, l Local, res Resolver, opts ...MergeOption) (Plan, error) {
	var plan Plan
	var o mergeOpts
	for _, opt := range opts {
		opt(&o)
	}

	// Título e corpo: o GitHub vence; senão, a tarefa segue para a issue.
	if t := normTitle(r.Title); t != normTitle(b.Title) {
		if t != normTitle(l.Title) {
			v := lf(r.Title)
			plan.Local.Title = &v
		}
	} else if t := normTitle(l.Title); t != normTitle(b.Title) {
		plan.Push.Title = &t
	}
	if t := norm(r.Body); t != norm(b.Body) {
		if t != norm(l.Body) {
			v := lf(r.Body)
			plan.Local.Body = &v
		}
	} else if t := norm(l.Body); t != norm(b.Body) {
		plan.Push.Body = &t
	}

	// Estado: fechar nos dois sentidos. A issue reaberta reabre a tarefa fechada em backlog; uma tarefa
	// aberta continua no status que tem (em progresso, aguardando fechamento).
	switch {
	case r.Closed != b.Closed:
		if r.Closed && !l.Closed {
			s := "closed"
			plan.Local.Status = &s
		}
		if !r.Closed && l.Closed {
			s := "backlog"
			plan.Local.Status = &s
		}
	case l.Closed != b.Closed:
		state, reason := "open", ""
		if l.Closed {
			state, reason = "closed", "completed"
		}
		plan.Push.State = &state
		if reason != "" {
			plan.Push.StateReason = &reason
		}
	}

	mergeLabels(&plan, b, r, l)

	if o.deadline {
		mergeDeadline(&plan, b, r, l)
	}
	if !o.noAssign {
		if err := mergeAssignee(ctx, &plan, b, r, l, res); err != nil {
			return Plan{}, err
		}
	}
	return plan, nil
}

// mergeDeadline: o prazo segue a regra do título. Se o item mudou a data desde o acordo, ele vence e a
// tarefa a adota (sem data a deixa sem prazo); senão, a data da tarefa, se mudou, vai para o item.
func mergeDeadline(plan *Plan, b Snapshot, r Remote, l Local) {
	rd, ld, bd := normDeadline(r.Deadline), normDeadline(l.Deadline), normDeadline(b.Deadline)
	switch {
	case !rd.Equal(bd):
		if !rd.Equal(ld) {
			plan.Local.Deadline = &rd
		}
	case !ld.Equal(bd):
		plan.Push.Deadline = &ld
	}
}

// mergeLabels: o resultado é (R ∪ (L∖B)) ∖ (B∖L): parte do que o GitHub tem, soma o que a tarefa
// ganhou e tira o que a tarefa perdeu. O que o GitHub ganhou ou perdeu entra pelo R.
func mergeLabels(plan *Plan, b Snapshot, r Remote, l Local) {
	bset, rset, lset := keyed(b.Labels), keyed(r.Labels), keyed(l.Labels)
	final := map[string]string{}
	var names []string
	add := func(k, name string) {
		if _, ok := final[k]; !ok {
			final[k] = name
			names = append(names, name)
		}
	}
	for _, n := range r.Labels {
		k := key(n)
		if _, kept := lset[k]; k != "" && (!inSet(bset, k) || kept) {
			add(k, rset[k])
		}
	}
	for _, n := range l.Labels {
		if k := key(n); k != "" && !inSet(bset, k) {
			add(k, lset[k])
		}
	}
	// Lista vazia e não nula: é ela que esvazia as etiquetas, no banco e no PATCH.
	if !sameKeys(final, lset) {
		out := append(make([]string, 0, len(names)), names...)
		plan.Local.Labels = &out
	}
	if !sameKeys(final, rset) {
		out := append(make([]string, 0, len(names)), names...)
		plan.Push.Labels = &out
	}
}

func inSet(set map[string]string, k string) bool {
	_, ok := set[k]
	return ok
}

// mergeAssignee liga os responsáveis da tarefa aos da issue, pessoa a pessoa, pelo login de cada uma (Decisão 31).
// Os logins da issue que não se ligam a ninguém do projeto ficam como estão nas duas direções. Cada par do último
// acordo (b.Mapped) se compara com os dois lados:
//   - a issue tirou o login: ela vence, e a pessoa sai da tarefa;
//   - a tarefa tirou a pessoa: o login sai da issue;
//   - um login da issue sem par que agora se liga a alguém (a issue o ganhou, ou a pessoa publicou o e-mail depois)
//     põe a pessoa na tarefa, salvo a que a tarefa acabou de tirar;
//   - uma pessoa da tarefa sem par põe o login dela na issue; sem login, o aviso no_login fica só para ela.
//
// O mesmo elemento mudado dos dois lados fica como a issue o tem.
func mergeAssignee(ctx context.Context, plan *Plan, b Snapshot, r Remote, l Local, res Resolver) error {
	rlogins, blogins := keyed(r.Assignee), keyed(b.Logins)
	local := map[uuid.UUID]bool{}
	for _, p := range l.People {
		local[p] = true
	}
	basePeople := map[uuid.UUID]bool{}
	baseLogin := map[string]bool{}
	for _, m := range b.Mapped {
		basePeople[m.Person] = true
		baseLogin[key(m.Login)] = true
	}

	var mapped []Mapping
	mappedPerson := map[uuid.UUID]bool{}
	keep := func(m Mapping) {
		mapped = append(mapped, m)
		mappedPerson[m.Person] = true
	}
	removeLocal := map[uuid.UUID]bool{}
	dropLogin := map[string]bool{}

	// Os pares do último acordo.
	for _, m := range b.Mapped {
		k := key(m.Login)
		_, onIssue := rlogins[k]
		switch {
		case !onIssue:
			if local[m.Person] {
				removeLocal[m.Person] = true
			}
		case !local[m.Person]:
			dropLogin[k] = true
		default:
			keep(Mapping{Login: rlogins[k], Person: m.Person})
		}
	}

	// Os logins da issue sem par.
	for _, name := range r.Assignee {
		k := key(name)
		if k == "" || baseLogin[k] {
			continue
		}
		p, err := res.PersonFor(ctx, name)
		if err != nil {
			return err
		}
		if p == nil || mappedPerson[*p] {
			continue
		}
		// A pessoa que a tarefa tirou desde o acordo, com outro login, não volta por este.
		if _, wasOnIssue := blogins[k]; wasOnIssue && basePeople[*p] && !local[*p] {
			continue
		}
		keep(Mapping{Login: strings.TrimSpace(name), Person: *p})
		switch {
		case removeLocal[*p]:
			delete(removeLocal, *p) // a issue trocou o login da pessoa: ela fica
		case !local[*p]:
			plan.Local.AddPeople = append(plan.Local.AddPeople, *p)
		}
	}
	for _, p := range l.People {
		if removeLocal[p] {
			plan.Local.RemovePeople = append(plan.Local.RemovePeople, p)
		}
	}

	// As pessoas da tarefa sem par: o login delas vai para a issue.
	var addLogins []string
	for _, p := range l.People {
		if mappedPerson[p] || removeLocal[p] {
			continue
		}
		login, err := res.LoginFor(ctx, p)
		if err != nil {
			return err
		}
		k := key(login)
		if k == "" {
			// Sem o login não há o que mandar; ela fica sem par, e a próxima rodada tenta de novo.
			plan.Problem = ErrNoLogin.Code
			continue
		}
		delete(dropLogin, k)
		if name, onIssue := rlogins[k]; onIssue {
			keep(Mapping{Login: name, Person: p})
			continue
		}
		keep(Mapping{Login: strings.TrimSpace(login), Person: p})
		addLogins = append(addLogins, strings.TrimSpace(login))
	}
	plan.Mapped = mapped

	remaining := make([]string, 0, len(r.Assignee)+len(addLogins))
	for _, name := range r.Assignee {
		if k := key(name); k != "" && !dropLogin[k] {
			remaining = append(remaining, name)
		}
	}
	remaining = append(remaining, addLogins...)
	if !sameKeys(keyed(remaining), rlogins) {
		plan.Push.Assignees = &remaining
	}
	return nil
}

// mappedPeople são as pessoas dos pares, para comparar com os responsáveis da tarefa.
func mappedPeople(m []Mapping) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool, len(m))
	for _, x := range m {
		out[x.Person] = true
	}
	return out
}

// samePeople diz se os responsáveis da tarefa são exatamente as pessoas dos pares.
func samePeople(people []uuid.UUID, m []Mapping) bool {
	set := mappedPeople(m)
	seen := map[uuid.UUID]bool{}
	for _, p := range people {
		if !set[p] {
			return false
		}
		seen[p] = true
	}
	return len(seen) == len(set)
}

// sameMapping diz se dois acordos ligam os mesmos logins às mesmas pessoas.
func sameMapping(a, b []Mapping) bool {
	if len(a) != len(b) {
		return false
	}
	byLogin := make(map[string]uuid.UUID, len(a))
	for _, m := range a {
		byLogin[key(m.Login)] = m.Person
	}
	for _, m := range b {
		if p, ok := byLogin[key(m.Login)]; !ok || p != m.Person {
			return false
		}
	}
	return true
}
