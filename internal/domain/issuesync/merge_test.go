package issuesync

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fakeResolver liga logins a pessoas por tabelas, e conta as perguntas.
type fakeResolver struct {
	people map[string]uuid.UUID // login → pessoa
	logins map[uuid.UUID]string // pessoa → login
	err    error
	asked  int
}

func (f *fakeResolver) PersonFor(_ context.Context, login string) (*uuid.UUID, error) {
	f.asked++
	if f.err != nil {
		return nil, f.err
	}
	if p, ok := f.people[strings.ToLower(login)]; ok {
		return &p, nil
	}
	return nil, nil
}

func (f *fakeResolver) LoginFor(_ context.Context, person uuid.UUID) (string, error) {
	f.asked++
	return f.logins[person], f.err
}

var (
	ana = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	bia = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	cai = uuid.MustParse("00000000-0000-0000-0000-00000000000c") // existe aqui mas não tem login achável
)

func newResolver() *fakeResolver {
	return &fakeResolver{
		people: map[string]uuid.UUID{"ana-dev": ana, "bia-dev": bia},
		logins: map[uuid.UUID]string{ana: "ana-dev", bia: "bia-dev"},
	}
}

func str(s string) *string       { return &s }
func list(s ...string) *[]string { return &s }
func pid(u uuid.UUID) *uuid.UUID { return &u }
func sorted(p *[]string) []string {
	if p == nil {
		return nil
	}
	out := slices.Clone(*p)
	slices.Sort(out)
	return out
}

// agreed é um snapshot em que os dois lados concordam, com tudo preenchido.
func agreed() (Snapshot, Remote, Local) {
	return Snapshot{Title: "Corrigir login", Body: "Passo 1\nPasso 2", Labels: []string{"bug"}, Logins: []string{"ana-dev"}, MappedLogin: "ana-dev", MappedPerson: pid(ana)},
		Remote{Title: "Corrigir login", Body: "Passo 1\nPasso 2", Labels: []string{"bug"}, Assignee: []string{"ana-dev"}},
		Local{Title: "Corrigir login", Body: "Passo 1\nPasso 2", Labels: []string{"bug"}, Person: pid(ana)}
}

func merge(t *testing.T, b Snapshot, r Remote, l Local, res Resolver) Plan {
	t.Helper()
	plan, err := Merge(context.Background(), b, r, l, res)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	return plan
}

func TestMerge_NothingChanged(t *testing.T) {
	b, r, l := agreed()
	res := newResolver()
	plan := merge(t, b, r, l, res)
	if !plan.Local.Empty() || !plan.Push.Empty() || plan.Problem != "" {
		t.Errorf("plan = %+v, want nothing to do", plan)
	}
	if plan.Mapped.Login != "ana-dev" || !samePerson(plan.Mapped.Person, pid(ana)) {
		t.Errorf("mapping = %+v, want it kept", plan.Mapped)
	}
	if res.asked != 0 {
		t.Errorf("a round with no difference asked the resolver %d times", res.asked)
	}
}

func TestMerge_TitleAndBody(t *testing.T) {
	cases := []struct {
		name      string
		edit      func(*Remote, *Local)
		wantLocal func(LocalChange) bool
		wantPush  func(Push) bool
	}{
		{"GitHub changed the title", func(r *Remote, l *Local) { r.Title = "Novo título" },
			func(c LocalChange) bool { return c.Title != nil && *c.Title == "Novo título" && c.Body == nil },
			func(p Push) bool { return p.Empty() }},
		{"the task changed the title", func(r *Remote, l *Local) { l.Title = "Meu título" },
			func(c LocalChange) bool { return c.Empty() },
			func(p Push) bool { return p.Title != nil && *p.Title == "Meu título" && p.Body == nil }},
		{"both changed the title: GitHub wins", func(r *Remote, l *Local) { r.Title = "Do GitHub"; l.Title = "Daqui" },
			func(c LocalChange) bool { return c.Title != nil && *c.Title == "Do GitHub" },
			func(p Push) bool { return p.Empty() }},
		{"both changed the title to the same", func(r *Remote, l *Local) { r.Title = "Igual"; l.Title = "Igual" },
			func(c LocalChange) bool { return c.Empty() },
			func(p Push) bool { return p.Empty() }},
		{"title and body changed on each side", func(r *Remote, l *Local) { r.Title = "Do GitHub"; l.Body = "corpo daqui" },
			func(c LocalChange) bool { return c.Title != nil && c.Body == nil },
			func(p Push) bool { return p.Title == nil && p.Body != nil && *p.Body == "corpo daqui" }},
		{"GitHub edited the body with CRLF only", func(r *Remote, l *Local) { r.Body = "Passo 1\r\nPasso 2\r\n" },
			func(c LocalChange) bool { return c.Empty() },
			func(p Push) bool { return p.Empty() }},
		{"the task body differs only by whitespace", func(r *Remote, l *Local) { l.Body = "  Passo 1\nPasso 2\n\n" },
			func(c LocalChange) bool { return c.Empty() },
			func(p Push) bool { return p.Empty() }},
		{"GitHub body with CRLF changed: stored with LF", func(r *Remote, l *Local) { r.Body = "Linha 1\r\nLinha 2 nova" },
			func(c LocalChange) bool { return c.Body != nil && *c.Body == "Linha 1\nLinha 2 nova" },
			func(p Push) bool { return p.Empty() }},
		{"an old CRLF snapshot does not look like a change", func(r *Remote, l *Local) {},
			func(c LocalChange) bool { return c.Empty() },
			func(p Push) bool { return p.Empty() }},
		{"GitHub emptied the body", func(r *Remote, l *Local) { r.Body = "" },
			func(c LocalChange) bool { return c.Body != nil && *c.Body == "" },
			func(p Push) bool { return p.Empty() }},
		{"the task emptied the body", func(r *Remote, l *Local) { l.Body = "" },
			func(c LocalChange) bool { return c.Empty() },
			func(p Push) bool { return p.Body != nil && *p.Body == "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, r, l := agreed()
			if tc.name == "an old CRLF snapshot does not look like a change" {
				b.Body = "Passo 1\r\nPasso 2"
			}
			tc.edit(&r, &l)
			plan := merge(t, b, r, l, newResolver())
			if !tc.wantLocal(plan.Local) {
				t.Errorf("local change = %+v", plan.Local)
			}
			if !tc.wantPush(plan.Push) {
				t.Errorf("push = %+v", plan.Push)
			}
		})
	}
}

func TestMerge_State(t *testing.T) {
	cases := []struct {
		name       string
		bClosed    bool
		rClosed    bool
		lClosed    bool
		wantStatus string
		wantPush   string
		wantReason string
	}{
		{"nothing changed", false, false, false, "", "", ""},
		{"issue closed, task open", false, true, false, "closed", "", ""},
		{"issue closed, task already closed", false, true, true, "", "", ""},
		{"task closed", false, false, true, "", "closed", "completed"},
		{"issue reopened, task closed", true, false, true, "backlog", "", ""},
		{"issue reopened, task already open", true, false, false, "", "", ""},
		{"task reopened", true, true, false, "", "open", ""},
		{"issue closed while the task was closed meanwhile", false, true, true, "", "", ""},
		{"issue closed and task reopened: GitHub wins", true, false, false, "", "", ""},
		{"issue reopened while the task was reopened too", true, false, false, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, r, l := agreed()
			b.Closed, r.Closed, l.Closed = tc.bClosed, tc.rClosed, tc.lClosed
			plan := merge(t, b, r, l, newResolver())
			got := func(p *string) string {
				if p == nil {
					return ""
				}
				return *p
			}
			if got(plan.Local.Status) != tc.wantStatus {
				t.Errorf("status = %q, want %q", got(plan.Local.Status), tc.wantStatus)
			}
			if got(plan.Push.State) != tc.wantPush || got(plan.Push.StateReason) != tc.wantReason {
				t.Errorf("push state = %q/%q, want %q/%q", got(plan.Push.State), got(plan.Push.StateReason), tc.wantPush, tc.wantReason)
			}
		})
	}
}

func TestMerge_Labels(t *testing.T) {
	cases := []struct {
		name      string
		b, r, l   []string
		wantLocal []string // nil: não muda
		wantPush  []string
	}{
		{"no change", []string{"bug"}, []string{"bug"}, []string{"bug"}, nil, nil},
		{"GitHub added one", []string{"bug"}, []string{"bug", "ux"}, []string{"bug"}, []string{"bug", "ux"}, nil},
		{"the task added one", []string{"bug"}, []string{"bug"}, []string{"bug", "api"}, nil, []string{"api", "bug"}},
		{"each added a different one: both get both", []string{"bug"}, []string{"bug", "ux"}, []string{"bug", "api"}, []string{"api", "bug", "ux"}, []string{"api", "bug", "ux"}},
		{"GitHub removed one", []string{"bug", "ux"}, []string{"bug"}, []string{"bug", "ux"}, []string{"bug"}, nil},
		{"the task removed one", []string{"bug", "ux"}, []string{"bug", "ux"}, []string{"bug"}, nil, []string{"bug"}},
		{"both removed the same one", []string{"bug", "ux"}, []string{"bug"}, []string{"bug"}, nil, nil},
		{"GitHub removed, the task added another", []string{"bug", "ux"}, []string{"bug"}, []string{"bug", "ux", "api"}, []string{"api", "bug"}, []string{"api", "bug"}},
		{"names differ only by case", []string{"Bug"}, []string{"bug"}, []string{"BUG"}, nil, nil},
		{"the task removed the label GitHub renamed to another case", []string{"Bug"}, []string{"bug"}, []string{}, nil, []string{}},
		{"both added the same one with another case", []string{}, []string{"UX"}, []string{"ux"}, nil, nil},
		{"the task cleared them all", []string{"bug", "ux"}, []string{"bug", "ux"}, nil, nil, []string{}},
		{"blank names are not labels", []string{}, []string{" "}, []string{}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, r, l := agreed()
			b.Labels, r.Labels, l.Labels = tc.b, tc.r, tc.l
			plan := merge(t, b, r, l, newResolver())
			if !reflect.DeepEqual(sortedCase(plan.Local.Labels), tc.wantLocal) {
				t.Errorf("local labels = %v, want %v", plan.Local.Labels, tc.wantLocal)
			}
			if !reflect.DeepEqual(sortedCase(plan.Push.Labels), tc.wantPush) {
				t.Errorf("pushed labels = %v, want %v", plan.Push.Labels, tc.wantPush)
			}
		})
	}
}

func sortedCase(p *[]string) []string {
	if p == nil {
		return nil
	}
	out := slices.Clone(*p)
	slices.Sort(out)
	return out
}

// Os nomes que voltam têm a caixa de quem os tem: a do GitHub para os que ele tem.
func TestMerge_LabelCase(t *testing.T) {
	b, r, l := agreed()
	b.Labels, r.Labels, l.Labels = []string{}, []string{"Bug"}, []string{"api"}
	plan := merge(t, b, r, l, newResolver())
	if got := sortedCase(plan.Local.Labels); !reflect.DeepEqual(got, []string{"Bug", "api"}) {
		t.Errorf("local = %v", got)
	}
}

func TestMerge_Assignee(t *testing.T) {
	unmappedSnapshot := func() (Snapshot, Remote, Local) {
		b, r, l := agreed()
		b.Logins, b.MappedLogin, b.MappedPerson = []string{}, "", nil
		r.Assignee, l.Person = nil, nil
		return b, r, l
	}
	t.Run("GitHub removed the linked assignee: the task is cleared", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = nil
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.SetAssignee || plan.Local.Person != nil || plan.Mapped.Login != "" || plan.Mapped.Person != nil || !plan.Push.Empty() {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("GitHub replaced the linked assignee by another we know", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"bia-dev"}
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.SetAssignee || !samePerson(plan.Local.Person, pid(bia)) || plan.Mapped.Login != "bia-dev" || !plan.Push.Empty() {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("GitHub replaced it by someone we cannot link: the task is cleared", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"estranho"}
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.SetAssignee || plan.Local.Person != nil || plan.Mapped.Login != "" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("GitHub added an unrelated assignee: the task keeps its own", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"ana-dev", "estranho"}
		plan := merge(t, b, r, l, newResolver())
		if plan.Local.SetAssignee || !plan.Push.Empty() || plan.Mapped.Login != "ana-dev" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("GitHub added an unrelated assignee while the task changed its own: the change goes", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"ana-dev", "estranho"}
		l.Person = pid(bia)
		plan := merge(t, b, r, l, newResolver())
		if plan.Local.SetAssignee || plan.Push.Assignees == nil || !reflect.DeepEqual(sorted(plan.Push.Assignees), []string{"bia-dev", "estranho"}) {
			t.Errorf("plan = %+v; the unrelated assignee must stay and the linked one be replaced", plan)
		}
		if plan.Mapped.Login != "bia-dev" || !samePerson(plan.Mapped.Person, pid(bia)) {
			t.Errorf("mapping = %+v", plan.Mapped)
		}
	})
	t.Run("GitHub assigned someone we know to an issue with no linked assignee", func(t *testing.T) {
		b, r, l := unmappedSnapshot()
		r.Assignee = []string{"estranho", "bia-dev"}
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.SetAssignee || !samePerson(plan.Local.Person, pid(bia)) || plan.Mapped.Login != "bia-dev" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("GitHub assigned someone we do not know: the task is untouched", func(t *testing.T) {
		b, r, l := unmappedSnapshot()
		r.Assignee = []string{"estranho"}
		l.Person = nil
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.Empty() || !plan.Push.Empty() || plan.Mapped.Login != "" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("an unlinked assignee that became known is linked on a later round, without the issue changing", func(t *testing.T) {
		b, r, l := unmappedSnapshot()
		r.Assignee, b.Logins = []string{"bia-dev"}, []string{"bia-dev"}
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.SetAssignee || !samePerson(plan.Local.Person, pid(bia)) || plan.Mapped.Login != "bia-dev" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("a pending choice of the task is not overridden by an assignee that was already there", func(t *testing.T) {
		b, r, l := unmappedSnapshot()
		r.Assignee, b.Logins = []string{"bia-dev"}, []string{"bia-dev"}
		l.Person = pid(ana)
		plan := merge(t, b, r, l, newResolver())
		if plan.Local.SetAssignee || plan.Push.Assignees == nil || !reflect.DeepEqual(sorted(plan.Push.Assignees), []string{"ana-dev", "bia-dev"}) {
			t.Errorf("plan = %+v; the task chose Ana, who joins the assignee that was already there", plan)
		}
	})
	t.Run("the task cleared its assignee: only the linked login leaves the issue", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"ana-dev", "estranho"}
		b.Logins = []string{"ana-dev", "estranho"}
		l.Person = nil
		plan := merge(t, b, r, l, newResolver())
		if plan.Push.Assignees == nil || !reflect.DeepEqual(sorted(plan.Push.Assignees), []string{"estranho"}) {
			t.Errorf("push assignees = %v, want the unlinked one kept", plan.Push.Assignees)
		}
		if plan.Mapped.Login != "" || plan.Mapped.Person != nil {
			t.Errorf("mapping = %+v", plan.Mapped)
		}
	})
	t.Run("the task got its first assignee", func(t *testing.T) {
		b, r, l := unmappedSnapshot()
		l.Person = pid(ana)
		plan := merge(t, b, r, l, newResolver())
		if plan.Push.Assignees == nil || !reflect.DeepEqual(*plan.Push.Assignees, []string{"ana-dev"}) || plan.Mapped.Login != "ana-dev" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("the task chose someone with no GitHub login we can find", func(t *testing.T) {
		b, r, l := unmappedSnapshot()
		l.Person = pid(cai)
		plan := merge(t, b, r, l, newResolver())
		if plan.Problem != ErrNoLogin.Code || !plan.Push.Empty() || plan.Mapped.Person != nil || plan.Mapped.Login != "" {
			t.Errorf("plan = %+v; the snapshot mapping must stay so the next round tries again", plan)
		}
	})
	t.Run("both changed the assignee: GitHub wins", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"bia-dev"}
		l.Person = pid(cai)
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.SetAssignee || !samePerson(plan.Local.Person, pid(bia)) || !plan.Push.Empty() {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("the login the task picked is already on the issue: only the old one leaves", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"ana-dev", "bia-dev"}
		b.Logins = []string{"ana-dev", "bia-dev"}
		l.Person = pid(bia)
		plan := merge(t, b, r, l, newResolver())
		if plan.Push.Assignees == nil || !reflect.DeepEqual(*plan.Push.Assignees, []string{"bia-dev"}) || plan.Mapped.Login != "bia-dev" {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("the login the task picked is on the issue and nothing else is: no push", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"bia-dev"}
		b.Logins = []string{"bia-dev"}
		b.MappedLogin, b.MappedPerson = "", nil
		l.Person = pid(bia)
		plan := merge(t, b, r, l, newResolver())
		if !plan.Push.Empty() || plan.Mapped.Login != "bia-dev" {
			t.Errorf("plan = %+v; the issue already has it, only the mapping moves", plan)
		}
	})
	t.Run("logins are compared without case", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"ANA-dev"}
		plan := merge(t, b, r, l, newResolver())
		if !plan.Local.Empty() || !plan.Push.Empty() {
			t.Errorf("plan = %+v", plan)
		}
	})
	t.Run("a resolver failure stops the merge", func(t *testing.T) {
		b, r, l := agreed()
		r.Assignee = []string{"bia-dev"}
		res := newResolver()
		res.err = errors.New("rede")
		if _, err := Merge(context.Background(), b, r, l, res); err == nil {
			t.Error("want the resolver error")
		}
	})
}

func TestPush_Signature(t *testing.T) {
	if (Push{}).Signature() != "" {
		t.Error("an empty push has no signature")
	}
	a := Push{Title: str("x"), Labels: list("B", "a")}
	b := Push{Title: str("x"), Labels: list("a", "b")}
	if a.Signature() == "" || a.Signature() != b.Signature() {
		t.Errorf("the order and case of labels must not change the signature: %q vs %q", a.Signature(), b.Signature())
	}
	if a.Signature() == (Push{Title: str("y"), Labels: list("a", "b")}).Signature() {
		t.Error("another title must change the signature")
	}
	if (Push{Assignees: list()}).Signature() == (Push{Labels: list()}).Signature() {
		t.Error("clearing the assignees is not clearing the labels")
	}
}
