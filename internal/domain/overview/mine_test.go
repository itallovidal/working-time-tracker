package overview_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	enttask "working-time-tracker/ent/task"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/testutil"
)

const saoPaulo = "America/Sao_Paulo" // UTC-3 o ano todo, sem horário de verão

// at lê um instante de um cenário (RFC 3339).
func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("time %q: %v", s, err)
	}
	return v
}

// mineTask cria uma tarefa da pessoa direto no banco, com o status, a prioridade, o prazo (nil: sem
// prazo) e o instante de criação que o cenário pede.
func (f *fixture) mineTask(projectID string, assignee uuid.UUID, name string, status enttask.Status, priority enttask.Priority, deadline *time.Time, created time.Time) uuid.UUID {
	f.t.Helper()
	q := testClient.Task.Create().
		SetProjectID(uuid.MustParse(projectID)).SetName(name).SetAssigneeID(assignee).
		SetStatus(status).SetPriority(priority).SetCreatedAt(created)
	if deadline != nil {
		q = q.SetDeadline(*deadline)
	}
	return q.SaveX(context.Background()).ID
}

func (f *fixture) mine(personID uuid.UUID, tz string) *overview.Mine {
	f.t.Helper()
	m, err := f.svc.Mine(f.orgID, personID.String(), tz, false)
	if err != nil {
		f.t.Fatalf("mine: %v", err)
	}
	return m
}

// O cenário: hoje é quarta-feira 7/10/2026, meio-dia em São Paulo (15:00 UTC), e a semana começou na
// segunda 5/10 à meia-noite de lá (03:00 UTC). A Ana tem, em dois projetos: uma sessão que começa no
// domingo à noite e entra na segunda (só o pedaço de segunda conta), uma de 1h30 na terça, uma de 1h
// hoje e uma aberta desde as 11:00 de lá (1h até agora); uma da semana passada, que fica de fora. O
// Bruno tem uma sessão na quarta, que não é dela.
func TestMine_Hours(t *testing.T) {
	f := setup(t)
	f.now = at(t, "2026-10-07T15:00:00Z")
	ana, bruno := f.person("Ana"), f.person("Bruno")
	x, y := f.project("Projeto X"), f.project("Projeto Y")
	tx, ty := f.task(x, ana, nil), f.task(y, ana, nil)
	tb := f.task(x, bruno, nil)

	end := func(s string) *time.Time { v := at(t, s); return &v }
	testutil.Session(t, testClient, tx, ana, at(t, "2026-10-05T01:00:00Z"), end("2026-10-05T05:00:00Z"), nil, nil) // dom 22h–seg 02h: 2h de segunda
	testutil.Session(t, testClient, ty, ana, at(t, "2026-10-06T10:00:00Z"), end("2026-10-06T11:30:00Z"), nil, nil) // terça, em outro projeto
	testutil.Session(t, testClient, tx, ana, at(t, "2026-10-07T12:00:00Z"), end("2026-10-07T13:00:00Z"), nil, nil) // hoje, 1h
	testutil.Session(t, testClient, tx, ana, at(t, "2026-10-02T12:00:00Z"), end("2026-10-02T20:00:00Z"), nil, nil) // semana passada
	testutil.Session(t, testClient, tb, bruno, at(t, "2026-10-07T12:00:00Z"), end("2026-10-07T14:00:00Z"), nil, nil)

	// Sem ponto aberto: só o que já foi fechado.
	m := f.mine(ana, saoPaulo)
	if m.Timezone != saoPaulo || !m.WeekStart.Equal(at(t, "2026-10-05T03:00:00Z")) {
		t.Errorf("zone = %s, week start = %s, want %s from Monday 00:00 there (03:00 UTC)", m.Timezone, m.WeekStart, saoPaulo)
	}
	if m.WorkingNow || len(m.WorkingOn) != 0 {
		t.Errorf("working now = %v on %v, want not working", m.WorkingNow, m.WorkingOn)
	}
	want := []float64{2 * 3600, 90 * 60, 3600, 0, 0, 0, 0}
	got := make([]float64, len(m.Hours.Days))
	for i, d := range m.Hours.Days {
		got[i] = d.Seconds
	}
	if !slices.Equal(got, want) {
		t.Errorf("days = %v, want %v", got, want)
	}
	if first, last := m.Hours.Days[0].Date, m.Hours.Days[6].Date; first != "2026-10-05" || last != "2026-10-11" {
		t.Errorf("days run %s to %s, want 2026-10-05 (Monday) to 2026-10-11 (Sunday)", first, last)
	}
	if m.Hours.WeekSeconds != 2*3600+90*60+3600 || m.Hours.TodaySeconds != 3600 {
		t.Errorf("week = %.0fs, today = %.0fs, want 16200 and 3600", m.Hours.WeekSeconds, m.Hours.TodaySeconds)
	}

	// As horas da semana por projeto: o X teve a segunda e a quarta (3h), o Y, a terça (1h30), do maior para o menor.
	projects := func(m *overview.Mine) string {
		out := ""
		for _, p := range m.Hours.Projects {
			out += fmt.Sprintf("%s=%.0f ", p.Project.Name, p.Seconds)
		}
		return out
	}
	if got := projects(m); got != "Projeto X=10800 Projeto Y=5400 " {
		t.Errorf("hours by project = %q, want the X with 10800s then the Y with 5400s", got)
	}

	// Com o ponto aberto desde as 11:00 de lá (14:00 UTC): mais 1h hoje, até agora, e a tarefa aparece.
	open := f.mineTask(x, ana, "Em curso", enttask.StatusInProgress, enttask.PriorityNone, nil, f.now)
	testutil.Session(t, testClient, open, ana, at(t, "2026-10-07T14:00:00Z"), nil, nil, nil)
	m = f.mine(ana, saoPaulo)
	if m.Hours.TodaySeconds != 2*3600 || m.Hours.Days[2].Seconds != 2*3600 || m.Hours.WeekSeconds != 16200+3600 {
		t.Errorf("with the clock open: today = %.0fs, Wednesday = %.0fs, week = %.0fs, want 7200, 7200, 19800", m.Hours.TodaySeconds, m.Hours.Days[2].Seconds, m.Hours.WeekSeconds)
	}
	if got := projects(m); got != "Projeto X=14400 Projeto Y=5400 " {
		t.Errorf("hours by project with the clock open = %q, want the X with 14400s then the Y with 5400s", got)
	}
	if !m.WorkingNow || len(m.WorkingOn) != 1 || m.WorkingOn[0].Task.ID != open || m.WorkingOn[0].Project.Name != "Projeto X" {
		t.Errorf("working on = %+v (now = %v), want the open task of Projeto X", m.WorkingOn, m.WorkingNow)
	}

	// O Bruno vê só o dele, e quem não bateu ponto, zero.
	if b := f.mine(bruno, saoPaulo); b.Hours.TodaySeconds != 2*3600 || b.Hours.WeekSeconds != 2*3600 || b.WorkingNow {
		t.Errorf("Bruno: today = %.0fs, week = %.0fs, working = %v, want 7200, 7200 and not working", b.Hours.TodaySeconds, b.Hours.WeekSeconds, b.WorkingNow)
	}
	if b := f.mine(bruno, saoPaulo); len(b.Hours.Projects) != 1 || b.Hours.Projects[0].Project.Name != "Projeto X" {
		t.Errorf("Bruno's hours by project = %+v, want only the Projeto X", b.Hours.Projects)
	}
	if c := f.mine(f.person("Carla"), saoPaulo); c.Hours.WeekSeconds != 0 || len(c.Hours.Days) != 7 || c.Hours.Projects == nil || len(c.Hours.Projects) != 0 {
		t.Errorf("Carla: week = %.0fs with %d days and projects %v, want 0, still seven days and an empty list (not null)", c.Hours.WeekSeconds, len(c.Hours.Days), c.Hours.Projects)
	}
}

// O fuso decide onde o dia e a semana começam: a mesma sessão da segunda às 01:00–02:00 UTC é do
// domingo em São Paulo e da segunda em UTC. Sem fuso, ou com um que não existe, vale UTC, e a resposta
// diz qual foi usado.
func TestMine_Timezone(t *testing.T) {
	f := setup(t)
	f.now = at(t, "2026-10-07T15:00:00Z")
	ana := f.person("Ana")
	tx := f.task(f.project("Projeto X"), ana, nil)
	end := at(t, "2026-10-05T02:00:00Z")
	testutil.Session(t, testClient, tx, ana, at(t, "2026-10-05T01:00:00Z"), &end, nil, nil)

	if m := f.mine(ana, saoPaulo); m.Hours.WeekSeconds != 0 {
		t.Errorf("in %s the session is last Sunday night: week = %.0fs, want 0", saoPaulo, m.Hours.WeekSeconds)
	}
	for _, tz := range []string{"", "Mars/Olympus", "Local"} {
		m := f.mine(ana, tz)
		if m.Timezone != "UTC" || m.Hours.WeekSeconds != 3600 || m.Hours.Days[0].Seconds != 3600 {
			t.Errorf("tz %q: zone = %s, week = %.0fs, Monday = %.0fs, want UTC, 3600 and 3600", tz, m.Timezone, m.Hours.WeekSeconds, m.Hours.Days[0].Seconds)
		}
	}
	// No domingo o dia ainda é o último da semana que começou na segunda anterior.
	f.now = at(t, "2026-10-11T23:30:00Z")
	if m := f.mine(ana, "UTC"); !m.WeekStart.Equal(at(t, "2026-10-05T00:00:00Z")) {
		t.Errorf("on Sunday the week starts %s, want Monday 2026-10-05", m.WeekStart)
	}
}

// A jornada semanal combinada vem junto, e nula quando ninguém informou.
func TestMine_WeeklyHours(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	if m := f.mine(ana, saoPaulo); m.WeeklyHours != nil {
		t.Errorf("weekly hours = %d, want none", *m.WeeklyHours)
	}
	hours := 30
	if _, err := person.NewService(person.NewStore(testClient)).SetWeeklyHours(ana.String(), &hours); err != nil {
		t.Fatalf("set weekly hours: %v", err)
	}
	if m := f.mine(ana, saoPaulo); m.WeeklyHours == nil || *m.WeeklyHours != 30 {
		t.Errorf("weekly hours = %v, want 30", m.WeeklyHours)
	}
}

// Hoje é quarta 7/10 ao meio-dia em São Paulo; a semana acaba no domingo 11/10 às 23:59 de lá. A Ana
// tem seis tarefas abertas (duas em backlog, uma em andamento, uma aguardando) e duas fechadas, em dois
// projetos; o Bruno tem uma, que não conta para ela.
func TestMine_Tasks(t *testing.T) {
	f := setup(t)
	f.now = at(t, "2026-10-07T15:00:00Z")
	ana, bruno := f.person("Ana"), f.person("Bruno")
	x, y := f.project("Projeto X"), f.project("Projeto Y")
	long := f.now.Add(-30 * day)
	d := func(s string) *time.Time { v := at(t, s); return &v }

	f.mineTask(x, ana, "Atrasada", enttask.StatusBacklog, enttask.PriorityNone, d("2026-10-06T15:00:00Z"), long)             // ontem
	f.mineTask(x, ana, "Atrasada em curso", enttask.StatusInProgress, enttask.PriorityNone, d("2026-10-07T14:59:00Z"), long) // um minuto atrás
	f.mineTask(y, ana, "Hoje", enttask.StatusBacklog, enttask.PriorityNone, d("2026-10-07T15:00:01Z"), long)                 // um segundo adiante
	f.mineTask(y, ana, "Domingo", enttask.StatusAwaitingClosure, enttask.PriorityNone, d("2026-10-12T02:59:00Z"), long)      // domingo 23:59 em SP
	f.mineTask(y, ana, "Segunda que vem", enttask.StatusBacklog, enttask.PriorityNone, d("2026-10-12T03:00:00Z"), long)      // segunda 00:00 em SP
	f.mineTask(y, ana, "Sem prazo", enttask.StatusBacklog, enttask.PriorityHigh, nil, long)                                  // não conta em nenhuma
	f.mineTask(x, ana, "Fechada", enttask.StatusClosed, enttask.PriorityNone, d("2026-09-01T12:00:00Z"), long)               // prazo passado, mas fechada
	f.mineTask(y, ana, "Fechada sem prazo", enttask.StatusClosed, enttask.PriorityNone, nil, long)
	f.mineTask(x, bruno, "Do Bruno", enttask.StatusBacklog, enttask.PriorityNone, d("2026-09-01T12:00:00Z"), long)

	m := f.mine(ana, saoPaulo)
	want := overview.MineTasks{Total: 8, Open: 6, Backlog: 4, InProgress: 1, AwaitingClosure: 1, Closed: 2, Overdue: 2, DueThisWeek: 2}
	// Backlog: Atrasada, Hoje, Segunda que vem, Sem prazo.
	if m.Tasks != want {
		t.Errorf("tasks = %+v, want %+v", m.Tasks, want)
	}
	if b := f.mine(bruno, saoPaulo).Tasks; b.Total != 1 || b.Overdue != 1 || b.DueThisWeek != 0 {
		t.Errorf("Bruno's tasks = %+v, want his one overdue task", b)
	}
	if empty := f.mine(f.person("Carla"), saoPaulo).Tasks; empty != (overview.MineTasks{}) {
		t.Errorf("a person with no tasks has %+v, want all zeros", empty)
	}
}

// As tarefas abertas saem com o prazo mais perto primeiro (as atrasadas à frente), depois as sem prazo
// pela prioridade e as mais novas; cada uma com o projeto dela. As fechadas ficam de fora, o prazo
// de uma tarefa sem prazo vem nulo, e uma página além da última volta a última.
func TestMyTasks_OpenOrderAndPages(t *testing.T) {
	f := setup(t)
	f.now = at(t, "2026-10-07T15:00:00Z")
	ana, bruno := f.person("Ana"), f.person("Bruno")
	x, y := f.project("Projeto X"), f.project("Projeto Y")
	base := f.now.Add(-10 * day)
	d := func(s string) *time.Time { v := at(t, s); return &v }

	f.mineTask(x, ana, "E sem prazo, baixa", enttask.StatusBacklog, enttask.PriorityLow, nil, base)
	f.mineTask(y, ana, "C no prazo", enttask.StatusInProgress, enttask.PriorityLow, d("2026-10-20T12:00:00Z"), base)
	f.mineTask(x, ana, "A atrasada", enttask.StatusBacklog, enttask.PriorityLow, d("2026-10-01T12:00:00Z"), base)
	f.mineTask(y, ana, "B atrasada urgente", enttask.StatusAwaitingClosure, enttask.PriorityUrgent, d("2026-10-03T12:00:00Z"), base)
	f.mineTask(y, ana, "D sem prazo, urgente", enttask.StatusBacklog, enttask.PriorityUrgent, nil, base)
	f.mineTask(x, ana, "F sem prazo, baixa, mais nova", enttask.StatusBacklog, enttask.PriorityLow, nil, base.Add(day))
	f.mineTask(x, ana, "Fechada", enttask.StatusClosed, enttask.PriorityUrgent, d("2026-09-01T12:00:00Z"), base)
	f.mineTask(x, bruno, "Do Bruno", enttask.StatusBacklog, enttask.PriorityUrgent, d("2026-09-01T12:00:00Z"), base)

	page := func(state string, n, per int) *overview.MyTasksPage {
		t.Helper()
		p, err := f.svc.MyTasks(f.orgID, ana.String(), state, false, n, per)
		if err != nil {
			t.Fatalf("my tasks %s page %d: %v", state, n, err)
		}
		return p
	}
	names := func(p *overview.MyTasksPage) []string {
		out := []string{}
		for _, it := range p.Items {
			out = append(out, it.Name)
		}
		return out
	}

	all := page(overview.StateOpen, 1, 20)
	wantOrder := []string{"A atrasada", "B atrasada urgente", "C no prazo", "D sem prazo, urgente", "F sem prazo, baixa, mais nova", "E sem prazo, baixa"}
	if got := names(all); !slices.Equal(got, wantOrder) || all.Total != 6 {
		t.Errorf("open tasks = %v (total %d), want %v (total 6)", got, all.Total, wantOrder)
	}
	first := all.Items[0]
	if first.Project.Name != "Projeto X" || first.Project.ID.String() != x || first.Status != "backlog" || first.Priority != "low" || first.Deadline == nil || !first.Deadline.Equal(at(t, "2026-10-01T12:00:00Z")) {
		t.Errorf("first task = %+v, want A atrasada of Projeto X with its deadline", first)
	}
	if last := all.Items[5]; last.Deadline != nil {
		t.Errorf("task without deadline has deadline %v, want nil", last.Deadline)
	}

	// Duas por página: três páginas, sem repetir nem pular tarefa, e além da última volta a última.
	var seen []string
	for n := 1; n <= 3; n++ {
		p := page(overview.StateOpen, n, 2)
		if p.Page != n || p.PerPage != 2 || p.Total != 6 || len(p.Items) != 2 {
			t.Errorf("page %d = page %d of %d per page, total %d, %d items, want page %d, 2 per page, total 6, 2 items", n, p.Page, p.PerPage, p.Total, len(p.Items), n)
		}
		seen = append(seen, names(p)...)
	}
	if !slices.Equal(seen, wantOrder) {
		t.Errorf("the three pages hold %v, want %v in order", seen, wantOrder)
	}
	if p := page(overview.StateOpen, 9, 2); p.Page != 3 || len(p.Items) != 2 {
		t.Errorf("page 9 = page %d with %d items, want the last page (3) with 2", p.Page, len(p.Items))
	}
	// Sem page nem per_page: a primeira, dez por página; per_page acima do teto vale o teto.
	if p := page(overview.StateOpen, 0, 0); p.Page != 1 || p.PerPage != 10 {
		t.Errorf("default page = %d of %d per page, want 1 and 10", p.Page, p.PerPage)
	}
	if p := page(overview.StateOpen, 1, 5000); p.PerPage != 100 {
		t.Errorf("per page above the ceiling = %d, want 100", p.PerPage)
	}
}

// As fechadas: da mais nova para a mais antiga, em páginas, sem as abertas; sem nenhuma, a lista vem
// vazia e não nula. Um id malformado é "não encontrado".
func TestMyTasks_Closed(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	x := f.project("Projeto X")
	base := time.Now().Add(-10 * day).Truncate(time.Second)

	empty, err := f.svc.MyTasks(f.orgID, ana.String(), overview.StateClosed, false, 1, 5)
	if err != nil || empty.Items == nil || len(empty.Items) != 0 || empty.Total != 0 || empty.Page != 1 {
		t.Fatalf("closed tasks of a person with none = %+v (%v), want an empty list, total 0, page 1", empty, err)
	}

	for i := 1; i <= 7; i++ {
		f.mineTask(x, ana, fmt.Sprintf("Fechada %d", i), enttask.StatusClosed, enttask.PriorityNone, nil, base.Add(time.Duration(i)*time.Hour))
	}
	f.mineTask(x, ana, "Aberta", enttask.StatusInProgress, enttask.PriorityNone, nil, base)

	p, err := f.svc.MyTasks(f.orgID, ana.String(), overview.StateClosed, false, 1, 5)
	if err != nil || p.Total != 7 || len(p.Items) != 5 || p.Items[0].Name != "Fechada 7" || p.Items[4].Name != "Fechada 3" {
		t.Fatalf("first page of closed = %+v (%v), want 5 of 7, newest first (Fechada 7 to Fechada 3)", p, err)
	}
	p, err = f.svc.MyTasks(f.orgID, ana.String(), overview.StateClosed, false, 2, 5)
	if err != nil || len(p.Items) != 2 || p.Items[0].Name != "Fechada 2" || p.Items[1].Name != "Fechada 1" {
		t.Fatalf("second page of closed = %+v (%v), want Fechada 2 and Fechada 1", p, err)
	}
	if p, _ = f.svc.MyTasks(f.orgID, ana.String(), overview.StateClosed, false, 9, 5); p.Page != 2 {
		t.Errorf("closed page 9 = page %d, want the last (2)", p.Page)
	}

	for _, bad := range [][2]string{{"not-a-uuid", ana.String()}, {f.orgID, "not-a-uuid"}} {
		if _, err := f.svc.MyTasks(bad[0], bad[1], overview.StateOpen, false, 1, 5); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("MyTasks(%q, %q) error = %v, want not found", bad[0], bad[1], err)
		}
		if _, err := f.svc.Mine(bad[0], bad[1], "", false); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Mine(%q, %q) error = %v, want not found", bad[0], bad[1], err)
		}
	}
}
