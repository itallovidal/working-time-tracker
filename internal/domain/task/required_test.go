package task_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/validate"
)

type reqEnv struct {
	svc      *task.Service
	org      uuid.UUID
	project  string
	ana, bia *person.Person // ana tem conta no GitHub; bia não
}

func newReqEnv(t *testing.T) reqEnv {
	t.Helper()
	cleanup(t)
	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	if err != nil {
		t.Fatal(err)
	}
	prj, err := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Projeto", "", 0, project.Routine{})
	if err != nil {
		t.Fatal(err)
	}
	people := person.NewService(person.NewStore(testClient))
	mk := func(name, email string) *person.Person {
		p, err := people.Create(org.ID.String(), name, email)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(prj.ID.String(), p.ID.String(), 1000); err != nil {
			t.Fatal(err)
		}
		return p
	}
	e := reqEnv{org: org.ID, project: prj.ID.String(), ana: mk("Ana", "ana@test.com"), bia: mk("Bia", "bia@test.com")}
	accounts := personaccount.NewStore(testClient)
	if _, err := personaccount.NewService(accounts).Link(personaccount.Account{
		PersonID: e.ana.ID, OrganizationID: org.ID, Platform: personaccount.GitHub, Username: "ana-dev",
		Source: personaccount.SourceOAuth,
	}); err != nil {
		t.Fatal(err)
	}
	// "Não tenho conta" não é conta.
	if _, err := personaccount.NewService(accounts).Link(personaccount.Account{
		PersonID: e.bia.ID, OrganizationID: org.ID, Platform: personaccount.GitHub, Source: personaccount.SourceSelf,
	}); err != nil {
		t.Fatal(err)
	}
	store := task.NewStore(testClient)
	store.SetAccounts(accounts)
	e.svc = task.NewService(store, team.NewMembershipStore(testClient), nil)
	return e
}

func github() *[]string { return &[]string{"github"} }

func wantRequired(t *testing.T, err error, what string) {
	t.Helper()
	if !errors.Is(err, task.ErrAccountRequired) {
		t.Fatalf("%s = %v, want ErrAccountRequired", what, err)
	}
	var ae *apperr.Error
	if errors.As(err, &ae); ae.Params["platforms"] != "github" || ae.Params["names"] != "GitHub" {
		t.Errorf("%s params = %v", what, ae.Params)
	}
}

// Só quem tem a conta pega a tarefa com a trava.
func TestRequired_Claim(t *testing.T) {
	e := newReqEnv(t)
	tk, err := e.svc.CreateAs("", e.project, "Dev", "", "", nil, task.Attrs{RequiredAccounts: github()})
	if err != nil {
		t.Fatal(err)
	}
	if len(tk.RequiredAccounts) != 1 || tk.RequiredAccounts[0] != "github" {
		t.Fatalf("required = %v", tk.RequiredAccounts)
	}
	_, err = e.svc.Claim(e.bia.ID.String(), tk.ID.String())
	wantRequired(t, err, "bia claiming")
	if got, err := e.svc.Claim(e.ana.ID.String(), tk.ID.String()); err != nil || got.AssigneeID == nil || *got.AssigneeID != e.ana.ID {
		t.Errorf("ana claiming = %v, %v", got, err)
	}
}

// Escolher o responsável na criação e na edição passa pela trava; quem já é o responsável fica.
func TestRequired_AssigneeOnCreateAndEdit(t *testing.T) {
	e := newReqEnv(t)
	_, err := e.svc.CreateAs("", e.project, "Dev", "", e.bia.ID.String(), nil, task.Attrs{RequiredAccounts: github()})
	wantRequired(t, err, "create with bia")

	tk, err := e.svc.CreateAs("", e.project, "Livre", "", e.bia.ID.String(), nil, task.Attrs{})
	if err != nil {
		t.Fatal(err)
	}
	// Ligar a trava numa tarefa da Bia não a tira dela.
	if tk, err = e.svc.PatchAttrsAs("", tk.ID.String(), task.Attrs{RequiredAccounts: github()}, nil, validate.Optional[time.Time]{}); err != nil {
		t.Fatalf("turning the requirement on: %v", err)
	}
	if tk.AssigneeID == nil || *tk.AssigneeID != e.bia.ID {
		t.Errorf("assignee = %v, want Bia kept", tk.AssigneeID)
	}
	ana, bia := e.ana.ID.String(), e.bia.ID.String()
	if _, err := e.svc.PatchAttrsAs("", tk.ID.String(), task.Attrs{}, &ana, validate.Optional[time.Time]{}); err != nil {
		t.Fatalf("ana as assignee: %v", err)
	}
	_, err = e.svc.UpdateAs("", tk.ID.String(), "Livre", "", &bia, nil, task.Attrs{})
	wantRequired(t, err, "back to bia")
	var ae *apperr.Error
	if errors.As(err, &ae); ae.Params["field"] != "assignee_id" {
		t.Errorf("field = %v, want assignee_id", ae.Params["field"])
	}
	// Tirar a trava e escolher a Bia no mesmo pedido vale.
	if _, err := e.svc.UpdateAs("", tk.ID.String(), "Livre", "", &bia, nil, task.Attrs{RequiredAccounts: &[]string{}}); err != nil {
		t.Errorf("requirement off and bia: %v", err)
	}
}

// A trava só aceita as plataformas com conta, e as guarda sem repetir.
func TestRequired_Validation(t *testing.T) {
	e := newReqEnv(t)
	if _, err := e.svc.CreateAs("", e.project, "X", "", "", nil, task.Attrs{RequiredAccounts: &[]string{"trello"}}); !errors.Is(err, task.ErrInvalidRequiredAccount) {
		t.Errorf("trello = %v, want ErrInvalidRequiredAccount", err)
	}
	tk, err := e.svc.CreateAs("", e.project, "X", "", "", nil, task.Attrs{RequiredAccounts: &[]string{"gitlab", "github", "gitlab"}})
	if err != nil || len(tk.RequiredAccounts) != 2 || tk.RequiredAccounts[0] != "github" || tk.RequiredAccounts[1] != "gitlab" {
		t.Errorf("required = %v, %v, want [github gitlab]", tk.RequiredAccounts, err)
	}
	free, err := e.svc.CreateAs("", e.project, "Y", "", "", nil, task.Attrs{})
	if err != nil || free.RequiredAccounts == nil || len(free.RequiredAccounts) != 0 {
		t.Errorf("without a requirement = %#v, want []", free.RequiredAccounts)
	}
}

// O store confere direto, como o ponto usa.
func TestRequired_StoreCheck(t *testing.T) {
	e := newReqEnv(t)
	store := task.NewStore(testClient)
	tk := &task.Task{RequiredAccounts: []string{"github", "gitlab"}}
	if err := store.CheckRequired(tk, uuid.New()); err != nil {
		t.Errorf("a store without accounts must not check: %v", err)
	}
	store.SetAccounts(personaccount.NewStore(testClient))
	err := store.CheckRequired(tk, e.ana.ID)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Params["platforms"] != "gitlab" || ae.Params["names"] != "GitLab" {
		t.Errorf("ana without gitlab = %v", err)
	}
}

// No modo solo a trava não vale: a pessoa é uma só, e uma tarefa que ficou com ela não a trava.
func TestRequired_SoloSkipsTheCheck(t *testing.T) {
	e := newReqEnv(t)
	tk, err := e.svc.CreateAs("", e.project, "Dev", "", "", nil, task.Attrs{RequiredAccounts: github()})
	if err != nil {
		t.Fatal(err)
	}
	testClient.Organization.UpdateOneID(e.org).SetSolo(true).ExecX(context.Background())
	if got, err := e.svc.Claim(e.bia.ID.String(), tk.ID.String()); err != nil || got.AssigneeID == nil || *got.AssigneeID != e.bia.ID {
		t.Errorf("claiming in a solo organization = %v, %v, want it taken", got, err)
	}
}
