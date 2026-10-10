package issuesync_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/validate"
	"working-time-tracker/testutil"
)

// Um título maior que o nome de uma tarefa (255 caracteres) é cortado na importação, sem partir um caractere,
// e a rodada seguinte não mexe nele: nem a tarefa muda, nem o título inteiro do GitHub é trocado pelo cortado.
func TestSync_LongTitleIsClippedOnImport(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	long := strings.Repeat("ç", validate.MaxTaskName+40)
	one := e.add(testutil.GitHubIssue{Title: long})
	exact := e.add(testutil.GitHubIssue{Title: strings.Repeat("b", validate.MaxTaskName)})

	sum := e.sync(issuesync.Full)
	if sum.Created != 2 || sum.Errors != 0 {
		t.Fatalf("summary = %+v, want both issues imported", sum)
	}
	tk := e.taskFor(one)
	if utf8.RuneCountInString(tk.Name) != validate.MaxTaskName || !utf8.ValidString(tk.Name) || tk.Name != strings.Repeat("ç", validate.MaxTaskName) {
		t.Errorf("name = %d runes (valid %v), want %d", utf8.RuneCountInString(tk.Name), utf8.ValidString(tk.Name), validate.MaxTaskName)
	}
	if got := e.taskFor(exact).Name; utf8.RuneCountInString(got) != validate.MaxTaskName {
		t.Errorf("a title at the limit = %d runes, want it whole", utf8.RuneCountInString(got))
	}

	// Mais duas rodadas: nada novo vai ao GitHub.
	sum = e.sync(issuesync.Full)
	if sum.Pushed != 0 || sum.Updated != 0 || e.fake.Writes() != 0 {
		t.Errorf("a second round = %+v, %d writes; want nothing pushed", sum, e.fake.Writes())
	}
	sum = e.sync(issuesync.Full)
	if sum.Pushed != 0 || sum.Updated != 0 || e.fake.Writes() != 0 {
		t.Errorf("a third round = %+v, %d writes; want nothing pushed", sum, e.fake.Writes())
	}

	// O GitHub muda o título para outro longo: a tarefa o acompanha, cortado.
	e.fake.EditIssue(repo, one, func(i *testutil.GitHubIssue) { i.Title = strings.Repeat("x", 400) })
	e.sync(issuesync.Full)
	if got := e.taskFor(one).Name; got != strings.Repeat("x", validate.MaxTaskName) {
		t.Errorf("name after the GitHub change = %d runes", utf8.RuneCountInString(got))
	}
}
