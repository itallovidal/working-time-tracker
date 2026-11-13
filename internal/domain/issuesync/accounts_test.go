package issuesync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/testutil"
)

func (e *env) linkAccount(person uuid.UUID, login, source string) {
	e.t.Helper()
	_, err := personaccount.NewService(personaccount.NewStore(testClient)).Link(personaccount.Account{
		PersonID: person, OrganizationID: uuid.MustParse(e.org), Platform: personaccount.GitHub,
		Username: login, Source: source,
	})
	must(e.t, err)
}

func (e *env) account(person uuid.UUID) *personaccount.Account {
	e.t.Helper()
	a, err := personaccount.NewStore(testClient).Get(person, personaccount.GitHub)
	if err != nil {
		return nil
	}
	return a
}

// A conta ligada liga o responsável sem e-mail público, nos dois sentidos.
func TestSync_AssigneeByLinkedAccount(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	e.linkAccount(e.dan.ID, "ghost", personaccount.SourceOAuth)
	n := e.add(testutil.GitHubIssue{Title: "Do fantasma", Assignees: []string{"ghost"}})
	e.sync(issuesync.Full)
	if got := e.taskFor(n); got.AssigneeID == nil || *got.AssigneeID != e.dan.ID {
		t.Fatalf("assignee = %v, want Dan by the linked account", got.AssigneeID)
	}
	if e.integration().SyncUnmatched != 0 {
		t.Errorf("unmatched = %d", e.integration().SyncUnmatched)
	}

	// Daqui para lá: a tarefa passa para Dan numa issue sem responsável, e o login vai da conta, sem busca por e-mail.
	m := e.add(testutil.GitHubIssue{Title: "Sem ninguém"})
	e.sync(issuesync.Full)
	tk := e.taskFor(m)
	dan := e.dan.ID.String()
	if _, err := e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", &dan, nil, task.Attrs{}); err != nil {
		t.Fatal(err)
	}
	e.fake.Reset()
	e.sync(issuesync.Full)
	if issue, _ := e.fake.Issue(repo, m); !eq(issue.Assignees, []string{"ghost"}) {
		t.Errorf("assignees = %v, want ghost", issue.Assignees)
	}
	if searches := e.fake.Count("GET", "/search/users"); searches != 0 {
		t.Errorf("%d searches by e-mail; the linked account must be enough", searches)
	}
}

// A conta de alguém fora do projeto não liga a tarefa a ele.
func TestSync_LinkedAccountOutsideTheProject(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	e.linkAccount(e.cai.ID, "ghost", personaccount.SourceOAuth)
	n := e.add(testutil.GitHubIssue{Title: "De fora", Assignees: []string{"ghost"}})
	e.sync(issuesync.Full)
	if got := e.taskFor(n); got.AssigneeID != nil {
		t.Errorf("assignee = %v, want nobody: Cai is not in the project", got.AssigneeID)
	}
}

// Quem casa pelo e-mail público fica com a conta guardada.
func TestSync_EmailMatchIsRemembered(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Da Ana", Assignees: []string{"ana-dev"}})
	e.sync(issuesync.Full)
	if got := e.taskFor(n); got.AssigneeID == nil || *got.AssigneeID != e.ana.ID {
		t.Fatalf("assignee = %v", got.AssigneeID)
	}
	a := e.account(e.ana.ID)
	if a == nil || a.Username != "ana-dev" || a.Source != personaccount.SourceEmail || a.Email != "ana@test.com" {
		t.Errorf("account = %+v, want ana-dev from the e-mail", a)
	}
}

// Quem disse que não tem conta não é procurado pelo e-mail: fica o aviso, sem busca.
func TestSync_NoAccountIsNotSearched(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Sem conta"})
	e.sync(issuesync.Full)
	_, err := personaccount.NewService(personaccount.NewStore(testClient)).Link(personaccount.Account{
		PersonID: e.bia.ID, OrganizationID: uuid.MustParse(e.org), Platform: personaccount.GitHub,
		Source: personaccount.SourceSelf,
	})
	must(t, err)
	tk := e.taskFor(n)
	bia := e.bia.ID.String()
	if _, err := e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", &bia, nil, task.Attrs{}); err != nil {
		t.Fatal(err)
	}
	e.fake.Reset()
	e.sync(issuesync.Full)
	if searches := e.fake.Count("GET", "/search/users"); searches != 0 {
		t.Errorf("%d searches for someone without an account", searches)
	}
	if e.row(n).LastError != "issue_sync.no_login" {
		t.Errorf("row error = %q, want issue_sync.no_login", e.row(n).LastError)
	}
	if issue, _ := e.fake.Issue(repo, n); len(issue.Assignees) != 0 {
		t.Errorf("assignees = %v, want none", issue.Assignees)
	}
}

// O admin escolhe a pessoa de um login sem correspondência: a rodada seguinte, que já vem junto, passa a tarefa.
func TestLinkLogin_ByTheAdmin(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Do fantasma", Assignees: []string{"ghost"}})
	e.sync(issuesync.Full)
	if it := e.integration(); it.SyncUnmatched != 1 || len(it.SyncUnmatchedLogins) != 1 || it.SyncUnmatchedLogins[0] != "ghost" {
		t.Fatalf("unmatched = %d %v, want [ghost]", it.SyncUnmatched, it.SyncUnmatchedLogins)
	}

	ctx := context.Background()
	if _, err := e.syncer.LinkLogin(ctx, e.it.ID, "ghost", e.cai.ID); !errors.Is(err, issuesync.ErrLinkNotInProject) {
		t.Errorf("someone outside the project = %v, want ErrLinkNotInProject", err)
	}
	it, err := e.syncer.LinkLogin(ctx, e.it.ID, "ghost", e.dan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.SyncUnmatched != 0 {
		t.Errorf("unmatched after linking = %d %v", it.SyncUnmatched, it.SyncUnmatchedLogins)
	}
	if got := e.taskFor(n); got.AssigneeID == nil || *got.AssigneeID != e.dan.ID {
		t.Errorf("assignee = %v, want Dan right after linking", got.AssigneeID)
	}

	// Um login que alguém autorizou na plataforma não vai para outra pessoa.
	e.linkAccount(e.ana.ID, "ana-dev", personaccount.SourceOAuth)
	if _, err := e.syncer.LinkLogin(ctx, e.it.ID, "ana-dev", e.bia.ID); !errors.Is(err, personaccount.ErrLoginTaken) {
		t.Errorf("a verified login = %v, want ErrLoginTaken", err)
	}
}

// A tarefa importada de uma issue do GitHub nasce exigindo a conta do GitHub.
func TestSync_ImportedTaskRequiresTheAccount(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "De dev"})
	e.sync(issuesync.Full)
	if got := e.taskFor(n).RequiredAccounts; len(got) != 1 || got[0] != "github" {
		t.Errorf("required = %v, want [github]", got)
	}
}

// No modo solo a pessoa é uma só: a tarefa importada não nasce com a trava.
func TestSync_SoloImportHasNoRequirement(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	testClient.Organization.UpdateOneID(uuid.MustParse(e.org)).SetSolo(true).ExecX(context.Background())
	n := e.add(testutil.GitHubIssue{Title: "Do solo"})
	e.sync(issuesync.Full)
	if got := e.taskFor(n).RequiredAccounts; len(got) != 0 {
		t.Errorf("required = %v, want none in a solo organization", got)
	}
}
