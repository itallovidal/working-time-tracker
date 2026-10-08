package issuesync

import (
	"context"
	"testing"
	"time"
)

func mergeWith(t *testing.T, b Snapshot, r Remote, l Local, res Resolver, opts ...MergeOption) Plan {
	t.Helper()
	plan, err := Merge(context.Background(), b, r, l, res, opts...)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	return plan
}

func day(d int) time.Time { return time.Date(2026, 10, d, 14, 30, 0, 0, time.UTC) }

func TestMerge_Deadline(t *testing.T) {
	var none time.Time
	cases := []struct {
		name         string
		base, remote time.Time
		local        time.Time
		wantLocal    *time.Time // o prazo que a tarefa passa a ter (ponteiro para o tempo zero é "sem prazo")
		wantPush     *time.Time // a data de entrega que o item passa a ter
	}{
		{name: "all agree", base: day(10), remote: day(10), local: day(10)},
		{name: "nobody has one", base: none, remote: none, local: none},
		{name: "the item changed it, the task did not", base: day(10), remote: day(12), local: day(10), wantLocal: &[]time.Time{day(12)}[0]},
		{name: "the task changed it, the item did not", base: day(10), remote: day(10), local: day(15), wantPush: &[]time.Time{day(15)}[0]},
		{name: "both changed to the same date", base: day(10), remote: day(12), local: day(12)},
		{name: "both changed: the item wins", base: day(10), remote: day(12), local: day(15), wantLocal: &[]time.Time{day(12)}[0]},
		{name: "the item dropped the date", base: day(10), remote: none, local: day(10), wantLocal: &[]time.Time{none}[0]},
		{name: "the task dropped the date", base: day(10), remote: day(10), local: none, wantPush: &[]time.Time{none}[0]},
		{name: "the item got a date for the first time", base: none, remote: day(12), local: none, wantLocal: &[]time.Time{day(12)}[0]},
		{name: "the task got a date for the first time", base: none, remote: none, local: day(15), wantPush: &[]time.Time{day(15)}[0]},
		{
			name: "milliseconds and zones are not a change", base: day(10),
			remote: day(10).Add(123 * time.Millisecond).In(time.FixedZone("BRT", -3*3600)), local: day(10).Add(900 * time.Millisecond),
		},
		{name: "a date before 1971 is no date", base: none, remote: none, local: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "the task stored as no date (year 1) matches an item without one", base: day(10), remote: none, local: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, r, l := agreed()
			b.Deadline, r.Deadline, l.Deadline = c.base, c.remote, c.local
			plan := mergeWith(t, b, r, l, newResolver(), WithDeadline())
			if !sameDeadlinePtr(plan.Local.Deadline, c.wantLocal) {
				t.Errorf("local deadline = %v, want %v", plan.Local.Deadline, c.wantLocal)
			}
			if !sameDeadlinePtr(plan.Push.Deadline, c.wantPush) {
				t.Errorf("pushed deadline = %v, want %v", plan.Push.Deadline, c.wantPush)
			}
		})
	}
}

func sameDeadlinePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return normDeadline(*a).Equal(normDeadline(*b))
}

// Sem WithDeadline o prazo não é assunto da fusão (é o caso do GitHub): nada vai para lá nem volta.
func TestMerge_DeadlineIsIgnoredWithoutTheOption(t *testing.T) {
	b, r, l := agreed()
	b.Deadline, r.Deadline, l.Deadline = day(10), day(12), day(15)
	plan := mergeWith(t, b, r, l, newResolver())
	if plan.Local.Deadline != nil || plan.Push.Deadline != nil || !plan.Local.Empty() || !plan.Push.Empty() {
		t.Errorf("plan = %+v, want nothing", plan)
	}
}

// Um tipo que não liga o responsável (o Trello) não pergunta por ninguém e não acusa login que falta.
func TestMerge_WithoutAssigneeSkipsTheAssignee(t *testing.T) {
	res := newResolver()
	b, r, l := agreed()
	b.MappedLogin, b.MappedPerson, b.Logins = "", nil, nil
	r.Assignee = nil
	l.Person = pid(cai) // tem responsável aqui, mas ninguém acha o login dele
	plan := mergeWith(t, b, r, l, res, WithoutAssignee())
	if plan.Problem != "" || plan.Push.Assignees != nil || plan.Local.SetAssignee {
		t.Errorf("plan = %+v, want the assignee left alone", plan)
	}
	if res.asked != 0 {
		t.Errorf("the resolver was asked %d times, want none", res.asked)
	}
	// E com o responsável ligado (o GitHub), a mesma situação acusa o login que falta.
	if plan := mergeWith(t, b, r, l, newResolver()); plan.Problem == "" {
		t.Error("with the assignee on, a person with no login must be reported")
	}
}

func TestPush_SignatureCountsTheDeadline(t *testing.T) {
	a, b := day(10), day(11)
	plain := Push{Title: str("x")}
	if plain.Signature() != (Push{Title: str("x")}).Signature() {
		t.Error("the same push must have the same signature")
	}
	if (Push{Deadline: &a}).Signature() == (Push{Deadline: &b}).Signature() {
		t.Error("pushes of different dates must have different signatures")
	}
	if plain.Signature() == (Push{Title: str("x"), Deadline: &a}).Signature() {
		t.Error("a push with a date must differ from the same push without one")
	}
	if (Push{Deadline: &a}).Empty() {
		t.Error("a push with only a date is not empty")
	}
	if (LocalChange{Deadline: &a}).Empty() {
		t.Error("a local change with only a date is not empty")
	}
}
