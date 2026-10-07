package task_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

func setupDeps(t *testing.T) (*organization.Service, *person.Service, *project.Service, *team.Service, *team.MembershipService, *task.Service) {
	cleanup(t)

	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), nil)

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc
}

// createIntegration grava uma integração direto pelo Ent, sem validar credenciais
// na API externa. Serve para testes que só precisam de uma FK válida.
func createIntegration(t *testing.T, projectID string) string {
	t.Helper()
	it, err := testClient.Integration.Create().
		SetProjectID(uuid.MustParse(projectID)).
		SetType("github").
		SetDisplayName("GitHub").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	return it.ID.String()
}

// join põe a pessoa no time. Um time só aceita quem já está no projeto, então
// ela recebe antes um valor por hora nele.
func join(t *testing.T, memberSvc *team.MembershipService, tm *team.Team, personID string) {
	t.Helper()
	if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(tm.ProjectID.String(), personID, 1000); err != nil {
		t.Fatalf("set rate: %v", err)
	}
	if _, err := memberSvc.Add(tm.ID.String(), personID); err != nil {
		t.Fatalf("add member: %v", err)
	}
}

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create_DefaultDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	task1, err := taskSvc.Create(proj.ID.String(), "Task A", "desc", p.ID.String(), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if task1.Name != "Task A" {
		t.Errorf("name = %q, want %q", task1.Name, "Task A")
	}
	if task1.Deadline.IsZero() {
		t.Error("expected non-zero deadline")
	}
	expected := time.Now().Add(7 * 24 * time.Hour)
	if task1.Deadline.Before(expected.Add(-time.Hour)) || task1.Deadline.After(expected.Add(time.Hour)) {
		t.Errorf("deadline = %v, expected ~%v", task1.Deadline, expected)
	}
}

func TestService_Create_ExplicitDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	dl := time.Now().Add(30 * 24 * time.Hour)
	task1, err := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), &dl)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// O Postgres guarda microssegundos, então o prazo lido pode perder os nanossegundos.
	if task1.Deadline.Sub(dl).Abs() >= time.Microsecond {
		t.Errorf("deadline = %v, want %v", task1.Deadline, dl)
	}
}

func TestService_Create_AssigneeNotTeamMember(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	_, err := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for non-member assignee, got nil")
	}
}

// "Atribuir a mim" vale para quem está logado mesmo fora dos times; outra pessoa
// fora dos times continua recusada, na criação e na edição.
func TestService_AssignToSelfWithoutTeam(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	me, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	other, _ := personSvc.Create(org.ID.String(), "Bia", "bia@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	pid, meID, otherID := proj.ID.String(), me.ID.String(), other.ID.String()

	own, err := taskSvc.CreateAs(meID, pid, "Minha", "", meID, nil, task.Attrs{})
	if err != nil || own.Assignee == nil || own.Assignee.ID != me.ID {
		t.Fatalf("create assigned to the caller outside any team: %v, %+v", err, own)
	}
	if _, err := taskSvc.CreateAs(meID, pid, "Dela", "", otherID, nil, task.Attrs{}); err == nil {
		t.Error("assigning to someone else outside the teams must fail")
	}
	if _, err := taskSvc.Create(pid, "Sem sessão", "", meID, nil); err == nil {
		t.Error("without a caller, assigning to a person outside the teams must fail")
	}

	free, _ := taskSvc.Create(pid, "Livre", "", "", nil)
	if _, err := taskSvc.UpdateAs(otherID, free.ID.String(), "Livre", "", &meID, nil, task.Attrs{}); err == nil {
		t.Error("update: assigning to someone who is not the caller and is outside the teams must fail")
	}
	updated, err := taskSvc.UpdateAs(meID, free.ID.String(), "Livre", "", &meID, nil, task.Attrs{})
	if err != nil || updated.Assignee == nil || updated.Assignee.ID != me.ID {
		t.Errorf("update assigned to the caller: %v, %+v", err, updated)
	}
}

// Quem tem valor por hora no projeto, mesmo sem time, pode ser responsável.
func TestService_AssignToCollaboratorWithoutTeam(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	bruno, _ := personSvc.Create(org.ID.String(), "Bruno", "bruno@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	pid, bid := proj.ID.String(), bruno.ID.String()

	if _, err := taskSvc.Create(pid, "Antes", "", bid, nil); err == nil {
		t.Fatal("a person outside the project must not be assignable")
	}
	testClient.Allocation.Create().SetProjectID(proj.ID).SetPersonID(bruno.ID).SetPayRateCents(5000).SaveX(context.Background())
	task, err := taskSvc.Create(pid, "Depois", "", bid, nil)
	if err != nil || task.Assignee == nil || task.Assignee.ID != bruno.ID {
		t.Fatalf("assigning to a collaborator without a team: %v, %+v", err, task)
	}
}

// A descrição é Markdown e vira HTML para todo mundo: tem um teto, contado em caracteres.
func TestService_DescriptionLimit(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	pid := proj.ID.String()

	exact := strings.Repeat("é", task.MaxDescriptionLen) // 10.000 caracteres, 20.000 bytes
	tk, err := taskSvc.Create(pid, "Longa", exact, "", nil)
	if err != nil || tk.Description != exact {
		t.Fatalf("a description at the limit must be accepted: %v", err)
	}
	if _, err := taskSvc.Create(pid, "Longa demais", exact+"x", "", nil); err == nil {
		t.Error("a description over the limit must be refused on create")
	}
	if _, err := taskSvc.Update(tk.ID.String(), "Longa", exact+"x", nil, nil); err == nil {
		t.Error("a description over the limit must be refused on update")
	}
}

// A prioridade padrão é "sem prioridade", só as cinco são aceitas, e omitir mantém.
func TestService_Priority(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	pid := proj.ID.String()

	plain, err := taskSvc.Create(pid, "Sem nada", "", "", nil)
	if err != nil || plain.Priority != "none" || plain.Labels == nil || len(plain.Labels) != 0 {
		t.Fatalf("a new task has priority %q and labels %v (err %v), want none and an empty list", plain.Priority, plain.Labels, err)
	}
	urgent := "urgent"
	tk, err := taskSvc.CreateAs("", pid, "Fogo", "", "", nil, task.Attrs{Priority: &urgent})
	if err != nil || tk.Priority != "urgent" {
		t.Fatalf("create with priority: %v, %+v", err, tk)
	}
	bad := "critical"
	if _, err := taskSvc.CreateAs("", pid, "X", "", "", nil, task.Attrs{Priority: &bad}); err != task.ErrInvalidPriority {
		t.Errorf("invalid priority on create: err = %v", err)
	}
	if _, err := taskSvc.UpdateAs("", tk.ID.String(), "Fogo", "", nil, nil, task.Attrs{Priority: &bad}); err != task.ErrInvalidPriority {
		t.Errorf("invalid priority on update: err = %v", err)
	}
	kept, err := taskSvc.Update(tk.ID.String(), "Fogo 2", "", nil, nil)
	if err != nil || kept.Priority != "urgent" {
		t.Errorf("an update without priority must keep it, got %q (%v)", kept.Priority, err)
	}
	low := "low"
	moved, _ := taskSvc.UpdateAs("", tk.ID.String(), "Fogo 2", "", nil, nil, task.Attrs{Priority: &low})
	if moved.Priority != "low" {
		t.Errorf("priority after the update = %q, want low", moved.Priority)
	}
}

// As etiquetas são do projeto: criar, achar nome repetido sem diferenciar maiúsculas, dar
// a uma tarefa, recusar a de outro projeto, trocar, manter ao omitir e limpar.
func TestService_Labels(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	a, _ := projSvc.Create(org.ID.String(), "A", "", 0, nil, nil, nil)
	b, _ := projSvc.Create(org.ID.String(), "B", "", 0, nil, nil, nil)
	aid, bid := a.ID.String(), b.ID.String()

	bug, err := taskSvc.CreateLabel(aid, "  bug ")
	if err != nil || bug.Name != "bug" {
		t.Fatalf("create label: %v, %+v", err, bug)
	}
	if _, err := taskSvc.CreateLabel(aid, "BUG"); err != task.ErrLabelNameTaken {
		t.Errorf("same name with another case: err = %v, want taken", err)
	}
	if _, err := taskSvc.CreateLabel(aid, "   "); err != task.ErrLabelNameRequired {
		t.Errorf("empty name: err = %v", err)
	}
	if _, err := taskSvc.CreateLabel(aid, strings.Repeat("x", 31)); err == nil {
		t.Error("a 31-character name must be refused")
	}
	design, _ := taskSvc.CreateLabel(aid, "design")
	other, _ := taskSvc.CreateLabel(bid, "bug") // o mesmo nome em outro projeto vale
	if other == nil {
		t.Fatal("the same name in another project must be allowed")
	}
	if list, _ := taskSvc.ListLabels(aid); len(list) != 2 || list[0].Name != "bug" || list[1].Name != "design" {
		t.Errorf("labels of A = %+v, want bug and design in order", list)
	}

	ids := []string{bug.ID.String(), design.ID.String(), bug.ID.String()}
	tk, err := taskSvc.CreateAs("", aid, "Com etiquetas", "", "", nil, task.Attrs{LabelIDs: &ids})
	if err != nil || len(tk.Labels) != 2 || tk.Labels[0].Name != "bug" || tk.Labels[1].Name != "design" {
		t.Fatalf("create with labels (a repeated id counts once): %v, %+v", err, tk.Labels)
	}
	foreign := []string{other.ID.String()}
	if _, err := taskSvc.CreateAs("", aid, "X", "", "", nil, task.Attrs{LabelIDs: &foreign}); err != task.ErrLabelOtherProject {
		t.Errorf("a label of another project on create: err = %v", err)
	}
	if _, err := taskSvc.UpdateAs("", tk.ID.String(), "X", "", nil, nil, task.Attrs{LabelIDs: &foreign}); err != task.ErrLabelOtherProject {
		t.Errorf("a label of another project on update: err = %v", err)
	}

	kept, _ := taskSvc.Update(tk.ID.String(), "Renomeada", "", nil, nil)
	if len(kept.Labels) != 2 {
		t.Errorf("an update without label_ids must keep the labels, got %+v", kept.Labels)
	}
	one := []string{design.ID.String()}
	swapped, _ := taskSvc.UpdateAs("", tk.ID.String(), "Renomeada", "", nil, nil, task.Attrs{LabelIDs: &one})
	if len(swapped.Labels) != 1 || swapped.Labels[0].Name != "design" {
		t.Errorf("labels after the swap = %+v, want only design", swapped.Labels)
	}
	none := []string{}
	cleared, _ := taskSvc.UpdateAs("", tk.ID.String(), "Renomeada", "", nil, nil, task.Attrs{LabelIDs: &none})
	if len(cleared.Labels) != 0 || cleared.Labels == nil {
		t.Errorf("labels after clearing = %+v, want an empty list", cleared.Labels)
	}

	// Renomear confere o nome; excluir tira a etiqueta das tarefas e deixa as tarefas.
	if _, err := taskSvc.RenameLabel(aid, design.ID.String(), "BUG"); err != task.ErrLabelNameTaken {
		t.Errorf("renaming onto an existing name: err = %v", err)
	}
	if _, err := taskSvc.RenameLabel(bid, design.ID.String(), "x"); err == nil {
		t.Error("renaming a label through another project must fail")
	}
	renamed, err := taskSvc.RenameLabel(aid, design.ID.String(), "Design")
	if err != nil || renamed.Name != "Design" {
		t.Errorf("rename: %v, %+v", err, renamed)
	}
	both := []string{bug.ID.String(), design.ID.String()}
	taskSvc.UpdateAs("", tk.ID.String(), "Renomeada", "", nil, nil, task.Attrs{LabelIDs: &both})
	if err := taskSvc.DeleteLabel(bid, bug.ID.String()); err == nil {
		t.Error("deleting a label through another project must fail")
	}
	if err := taskSvc.DeleteLabel(aid, bug.ID.String()); err != nil {
		t.Fatalf("delete label: %v", err)
	}
	left, err := taskSvc.Get(tk.ID.String())
	if err != nil || len(left.Labels) != 1 || left.Labels[0].Name != "Design" {
		t.Errorf("the task after deleting a label = %+v (%v), want it kept with Design only", left, err)
	}
}

// O filtro de prioridade aceita várias, o de etiqueta vale para a tarefa que tem
// qualquer uma delas, e os dois juntos se somam; o total e a página seguem o filtro.
func TestService_ListFilters_PriorityAndLabels(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "P", "", 0, nil, nil, nil)
	pid := proj.ID.String()
	bug, _ := taskSvc.CreateLabel(pid, "bug")
	ux, _ := taskSvc.CreateLabel(pid, "ux")

	mk := func(name, priority string, labels ...uuid.UUID) {
		ids := []string{}
		for _, l := range labels {
			ids = append(ids, l.String())
		}
		if _, err := taskSvc.CreateAs("", pid, name, "", "", nil, task.Attrs{Priority: &priority, LabelIDs: &ids}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	mk("a", "urgent", bug.ID)
	mk("b", "high", ux.ID)
	mk("c", "high", bug.ID, ux.ID)
	mk("d", "low")
	mk("e", "none")

	names := func(f task.ListFilter) string {
		page, err := taskSvc.ListPage(pid, f)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		got := []string{}
		for _, it := range page.Items {
			got = append(got, it.Name)
		}
		sort.Strings(got)
		return strings.Join(got, "") + "/" + strconv.Itoa(page.Total)
	}
	cases := []struct {
		name string
		f    task.ListFilter
		want string
	}{
		{"one priority", task.ListFilter{Priorities: []string{"high"}}, "bc/2"},
		{"two priorities", task.ListFilter{Priorities: []string{"urgent", "low"}}, "ad/2"},
		{"one label", task.ListFilter{LabelIDs: []uuid.UUID{bug.ID}}, "ac/2"},
		{"any of two labels, a task counts once", task.ListFilter{LabelIDs: []uuid.UUID{bug.ID, ux.ID}}, "abc/3"},
		{"priority and label together", task.ListFilter{Priorities: []string{"high"}, LabelIDs: []uuid.UUID{bug.ID}}, "c/1"},
		{"no match", task.ListFilter{Priorities: []string{"medium"}}, "/0"},
		{"paged", task.ListFilter{LabelIDs: []uuid.UUID{bug.ID, ux.ID}, Page: 1, PerPage: 2}, ""},
	}
	for _, c := range cases[:6] {
		if got := names(c.f); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if page, _ := taskSvc.ListPage(pid, cases[6].f); page.Total != 3 || len(page.Items) != 2 {
		t.Errorf("paged: total %d with %d items, want 3 and 2", page.Total, len(page.Items))
	}
}

// Uma tarefa nasce sem responsável: fica disponível para quem bater o ponto nela.
func TestService_Create_WithoutAssignee(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	tk, err := taskSvc.Create(proj.ID.String(), "Task A", "", "", nil)
	if err != nil {
		t.Fatalf("create without assignee: %v", err)
	}
	if tk.AssigneeID != nil || tk.Assignee != nil {
		t.Errorf("assignee = %v / %v, want none", tk.AssigneeID, tk.Assignee)
	}
}

func TestService_Create_InvalidAssignee(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	if _, err := taskSvc.Create(proj.ID.String(), "Task A", "", "not-a-uuid", nil); !errors.Is(err, task.ErrInvalidAssignee) {
		t.Errorf("err = %v, want ErrInvalidAssignee", err)
	}
}

// Um assignee vazio no PATCH desvincula, e a tarefa entra no filtro "sem responsável".
func TestService_Update_UnassignAndFilter(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	assigned, _ := taskSvc.Create(proj.ID.String(), "Assigned", "", p.ID.String(), nil)
	free, _ := taskSvc.Create(proj.ID.String(), "Free", "", "", nil)

	empty := ""
	updated, err := taskSvc.Update(assigned.ID.String(), "Assigned", "", &empty, nil)
	if err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if updated.AssigneeID != nil || updated.Assignee != nil {
		t.Errorf("assignee after unassign = %v, want none", updated.AssigneeID)
	}

	none, err := taskSvc.ListByProject(proj.ID.String(), task.ListFilter{Unassigned: true})
	if err != nil || len(none) != 2 {
		t.Fatalf("unassigned = %d tasks (%v), want 2", len(none), err)
	}

	// Voltar a pôr um responsável continua exigindo que ele esteja num time.
	pid := p.ID.String()
	if _, err := taskSvc.Update(free.ID.String(), "Free", "", &pid, nil); err != nil {
		t.Fatalf("assign again: %v", err)
	}
	none, _ = taskSvc.ListByProject(proj.ID.String(), task.ListFilter{Unassigned: true})
	if len(none) != 1 {
		t.Errorf("unassigned after assigning = %d, want 1", len(none))
	}
}

func TestService_Create_MissingName(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	_, err := taskSvc.Create(proj.ID.String(), "", "", p.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Update(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	created, _ := taskSvc.Create(proj.ID.String(), "Old", "", p.ID.String(), nil)
	updated, err := taskSvc.Update(created.ID.String(), "New", "new desc", nil, nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("name = %q, want %q", updated.Name, "New")
	}
	if updated.Description != "new desc" {
		t.Errorf("description = %q, want %q", updated.Description, "new desc")
	}
}

func TestService_Delete(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	err := taskSvc.Delete(task1.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = taskSvc.Get(task1.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_LinkUnlinkExternalItem(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	linked, err := taskSvc.LinkExternalItem(task1.ID.String(), createIntegration(t, proj.ID.String()), "42", "https://example.com/42")
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}
	if linked.ExternalItemID == nil || *linked.ExternalItemID != "42" {
		t.Errorf("external_item_id = %v, want 42", linked.ExternalItemID)
	}

	unlinked, err := taskSvc.UnlinkExternalItem(task1.ID.String())
	if err != nil {
		t.Fatalf("unlink failed: %v", err)
	}
	if unlinked.ExternalItemID != nil {
		t.Errorf("expected nil external_item_id after unlink, got %v", *unlinked.ExternalItemID)
	}
}

func TestService_Delete_CascadesWorkSessions(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)
	ctx := context.Background()

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	testClient.WorkSession.Create().SetTaskID(task1.ID).SetPersonID(p.ID).
		SetStartAt(time.Now().Add(-time.Hour)).SetEndAt(time.Now()).SaveX(ctx)
	testClient.WorkSession.Create().SetTaskID(task1.ID).SetPersonID(p.ID).
		SetStartAt(time.Now()).SaveX(ctx)

	if err := taskSvc.Delete(task1.ID.String()); err != nil {
		t.Fatalf("delete task with work sessions failed: %v", err)
	}
	if n := testClient.WorkSession.Query().CountX(ctx); n != 0 {
		t.Errorf("work_sessions: %d rows left, want 0", n)
	}
}

func TestService_LinkExternalItem_IntegrationFromAnotherProject(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	otherProj, _ := projSvc.Create(org.ID.String(), "Other", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	foreign := createIntegration(t, otherProj.ID.String())
	if _, err := taskSvc.LinkExternalItem(task1.ID.String(), foreign, "42", "https://example.com/42"); err == nil {
		t.Fatal("expected error when linking an integration from another project")
	}
	if _, err := taskSvc.LinkExternalItem(task1.ID.String(), "not-a-uuid", "42", "https://example.com/42"); err == nil {
		t.Fatal("expected error for an invalid integration id")
	}
}

func TestService_Update_AssigneeMustBeProjectMember(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	outsider, _ := personSvc.Create(org.ID.String(), "Maria", "maria@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	outsiderID := outsider.ID.String()
	if _, err := taskSvc.Update(task1.ID.String(), "Task A", "", &outsiderID, nil); err == nil {
		t.Fatal("expected error when assigning someone outside the project's teams")
	}
	bad := "not-a-uuid"
	if _, err := taskSvc.Update(task1.ID.String(), "Task A", "", &bad, nil); err == nil {
		t.Fatal("expected error for an invalid assignee id")
	}
}

// O vínculo com a integração precisa sobreviver à leitura e à edição da tarefa.
// Antes, o store não copiava external_integration_id: /external-details sempre
// dizia que não havia vínculo, e editar a tarefa apagava a integração.
func TestService_LinkSurvivesReadAndUpdate(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	integrationID := createIntegration(t, proj.ID.String())

	if _, err := taskSvc.LinkExternalItem(task1.ID.String(), integrationID, "42", "https://example.com/42"); err != nil {
		t.Fatalf("link: %v", err)
	}
	got, _ := taskSvc.Get(task1.ID.String())
	if got.ExternalIntegrationID == nil || got.ExternalIntegrationID.String() != integrationID {
		t.Fatalf("external_integration_id after link = %v, want %s", got.ExternalIntegrationID, integrationID)
	}
	// A lista também traz a integração: é dela que sai a plataforma do rótulo ("GitHub #42").
	listed, err := taskSvc.ListByProject(proj.ID.String(), task.ListFilter{})
	if err != nil || len(listed) != 1 || listed[0].ExternalIntegration == nil || listed[0].ExternalIntegration.Type != "github" {
		t.Errorf("list after link = %+v (%v), want the task with its integration", listed, err)
	}

	updated, err := taskSvc.Update(task1.ID.String(), "Task A renomeada", "", nil, nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ExternalIntegrationID == nil || updated.ExternalItemID == nil || *updated.ExternalItemID != "42" {
		t.Errorf("update dropped the link: integration=%v item=%v", updated.ExternalIntegrationID, updated.ExternalItemID)
	}
}

// taskNames devolve os nomes em ordem alfabética, para comparar sem depender
// da ordem de criação.
func taskNames(tasks []task.Task) []string {
	names := make([]string, len(tasks))
	for i, tk := range tasks {
		names[i] = tk.Name
	}
	slices.Sort(names)
	return names
}

func TestService_ListByProject_Filters(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	john, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	maria, _ := personSvc.Create(org.ID.String(), "Maria", "maria@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	other, _ := projSvc.Create(org.ID.String(), "Other", "", 0, nil, nil, nil)
	for _, p := range []string{proj.ID.String(), other.ID.String()} {
		tm, _ := teamSvc.Create(p, "Team")
		join(t, memberSvc, tm, john.ID.String())
		join(t, memberSvc, tm, maria.ID.String())
	}

	soon := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	later := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	pid := proj.ID.String()
	create := func(project, name string, assignee uuid.UUID, deadline time.Time) {
		t.Helper()
		if _, err := taskSvc.Create(project, name, "", assignee.String(), &deadline); err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
	}
	create(pid, "Monthly Report", john.ID, soon)
	create(pid, "report of hours", maria.ID, later)
	create(pid, "100% coverage", maria.ID, soon)
	create(pid, "snake_case fields", john.ID, later)
	// Tarefa de outro projeto: não pode aparecer em nenhum filtro.
	create(other.ID.String(), "Report elsewhere", maria.ID, soon)

	cutoff := soon.Add(time.Hour)
	cases := []struct {
		name string
		f    task.ListFilter
		want []string
	}{
		{"no filter", task.ListFilter{}, []string{"100% coverage", "Monthly Report", "report of hours", "snake_case fields"}},
		{"name ignores case", task.ListFilter{Query: "REPORT"}, []string{"Monthly Report", "report of hours"}},
		{"percent is literal", task.ListFilter{Query: "%"}, []string{"100% coverage"}},
		{"underscore is literal", task.ListFilter{Query: "_"}, []string{"snake_case fields"}},
		{"assignee", task.ListFilter{AssigneeID: &maria.ID}, []string{"100% coverage", "report of hours"}},
		{"deadline", task.ListFilter{DeadlineTo: &cutoff}, []string{"100% coverage", "Monthly Report"}},
		{"deadline is inclusive", task.ListFilter{DeadlineTo: &soon}, []string{"100% coverage", "Monthly Report"}},
		{"combined", task.ListFilter{Query: "report", AssigneeID: &john.ID, DeadlineTo: &cutoff}, []string{"Monthly Report"}},
		{"nothing matches", task.ListFilter{Query: "report", AssigneeID: &maria.ID, DeadlineTo: &cutoff}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := taskSvc.ListByProject(pid, tc.f)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if names := taskNames(got); !slices.Equal(names, tc.want) {
				t.Errorf("got %v, want %v", names, tc.want)
			}
		})
	}
}

// Uma tarefa antiga sem prazo, depois de editada, guarda o tempo zero. Ela não
// pode entrar no filtro de prazo como se estivesse atrasada.
func TestService_ListByProject_DeadlineFilterSkipsTasksWithoutDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)
	ctx := context.Background()

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	join(t, memberSvc, tm, p.ID.String())

	overdue := time.Now().Add(-24 * time.Hour)
	taskSvc.Create(proj.ID.String(), "Overdue", "", p.ID.String(), &overdue)
	testClient.Task.Create().SetProjectID(proj.ID).SetName("Null deadline").SetAssigneeID(p.ID).SaveX(ctx)
	testClient.Task.Create().SetProjectID(proj.ID).SetName("Zero deadline").SetAssigneeID(p.ID).
		SetDeadline(time.Time{}).SaveX(ctx)

	now := time.Now()
	got, err := taskSvc.ListByProject(proj.ID.String(), task.ListFilter{DeadlineTo: &now})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if names := taskNames(got); !slices.Equal(names, []string{"Overdue"}) {
		t.Errorf("got %v, want only the overdue task", names)
	}
}

func TestService_ListPage(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	john, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	maria, _ := personSvc.Create(org.ID.String(), "Maria", "maria@test.com")
	idle, _ := personSvc.Create(org.ID.String(), "Zed", "zed@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	for _, p := range []string{john.ID.String(), maria.ID.String(), idle.ID.String()} {
		join(t, memberSvc, tm, p)
	}
	pid := proj.ID.String()

	empty, err := taskSvc.ListPage(pid, task.ListFilter{Page: 1})
	if err != nil {
		t.Fatalf("empty page: %v", err)
	}
	if empty.Total != 0 || empty.Page != 1 || empty.PerPage != task.DefaultPerPage || empty.Items == nil || len(empty.Items) != 0 {
		t.Errorf("empty project page = %+v, want page 1 with an empty, non-nil list", empty)
	}

	var newest *task.Task
	for i := range 25 {
		assignee := john
		if i%5 == 0 {
			assignee = maria
		}
		created, err := taskSvc.Create(pid, fmt.Sprintf("Task %02d", i), "", assignee.ID.String(), nil)
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
		newest = created
	}
	// Maria sai dos times e continua responsável pelas tarefas dela.
	if err := memberSvc.Remove(tm.ID.String(), maria.ID.String()); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	seen := map[uuid.UUID]bool{}
	for page, size := range map[int]int{1: 10, 2: 10, 3: 5} {
		got, err := taskSvc.ListPage(pid, task.ListFilter{Page: page})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if got.Total != 25 || got.Page != page || got.PerPage != 10 || len(got.Items) != size {
			t.Fatalf("page %d: total=%d page=%d per_page=%d items=%d, want 25/%d/10/%d",
				page, got.Total, got.Page, got.PerPage, len(got.Items), page, size)
		}
		if page == 1 && got.Items[0].ID != newest.ID {
			t.Errorf("page 1 starts with %q, want the newest task %q", got.Items[0].Name, newest.Name)
		}
		for _, tk := range got.Items {
			if seen[tk.ID] {
				t.Errorf("task %q shows up on more than one page", tk.Name)
			}
			seen[tk.ID] = true
		}
	}
	if len(seen) != 25 {
		t.Errorf("pages covered %d tasks, want 25", len(seen))
	}

	beyond, err := taskSvc.ListPage(pid, task.ListFilter{Page: 99})
	if err != nil {
		t.Fatalf("page 99: %v", err)
	}
	if beyond.Page != 3 || len(beyond.Items) != 5 {
		t.Errorf("page 99: page=%d items=%d, want the last page (3) with 5 items", beyond.Page, len(beyond.Items))
	}

	capped, err := taskSvc.ListPage(pid, task.ListFilter{Page: 1, PerPage: 1000})
	if err != nil {
		t.Fatalf("per_page 1000: %v", err)
	}
	if capped.PerPage != task.MaxPerPage || len(capped.Items) != 25 {
		t.Errorf("per_page 1000: per_page=%d items=%d, want %d and 25", capped.PerPage, len(capped.Items), task.MaxPerPage)
	}

	filtered, err := taskSvc.ListPage(pid, task.ListFilter{Page: 1, PerPage: 3, AssigneeID: &maria.ID})
	if err != nil {
		t.Fatalf("maria's tasks: %v", err)
	}
	if filtered.Total != 5 || len(filtered.Items) != 3 {
		t.Errorf("maria's tasks: total=%d items=%d, want 5 and 3", filtered.Total, len(filtered.Items))
	}

	// Responsáveis: sem repetir, por nome, com quem saiu dos times e sem quem
	// não tem tarefa.
	var names []string
	for _, a := range filtered.Assignees {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []string{"John", "Maria"}) {
		t.Errorf("assignees = %v, want [John Maria]", names)
	}
}
