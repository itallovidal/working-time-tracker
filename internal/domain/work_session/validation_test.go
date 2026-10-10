package work_session_test

import (
	"errors"
	"testing"
	"time"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/work_session"
)

// codeAndField devolve o código do erro e o parâmetro field.
func codeAndField(err error) (code, field string) {
	var e *apperr.Error
	if !errors.As(err, &e) {
		return "", ""
	}
	field, _ = e.Params["field"].(string)
	return e.Code, field
}

// O intervalo recusado diz qual campo errou: o começo (from_at) ou o fim (until_at); a sobreposição diz from_at.
func TestSessionTasks_IntervalErrorsSayTheField(t *testing.T) {
	f := newSessionFixture(t)
	a, b := f.task("A"), f.task("B")
	session := f.past(a, 10*time.Hour, 4*time.Hour)
	sid := session.ID.String()

	before := session.StartAt.Add(-time.Hour)
	inside := session.StartAt.Add(time.Hour)
	later := session.StartAt.Add(2 * time.Hour)
	afterEnd := session.StartAt.Add(5 * time.Hour)

	cases := []struct {
		name        string
		from, until *time.Time
		wantCode    string
		wantField   string
	}{
		{"starts before the session", &before, nil, "work_session.invalid_interval", "from_at"},
		{"ends before it starts", &later, &inside, "work_session.invalid_interval", "until_at"},
		{"ends where it starts", &inside, &inside, "work_session.invalid_interval", "until_at"},
		{"ends after the session", &inside, &afterEnd, "work_session.invalid_interval", "until_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.ws.AddTask(f.proj, sid, b, tc.from, tc.until)
			if code, field := codeAndField(err); code != tc.wantCode || field != tc.wantField {
				t.Errorf("err = %v (field %q), want %s on %s", err, field, tc.wantCode, tc.wantField)
			}
		})
	}

	// A mesma tarefa duas vezes no mesmo período.
	_, err := f.ws.AddTask(f.proj, sid, a, nil, nil)
	if code, field := codeAndField(err); code != "work_session.task_overlap" || field != "from_at" {
		t.Errorf("the same task again = %v (field %q), want task_overlap on from_at", err, field)
	}
	// Uma tarefa que não existe diz task_id.
	_, err = f.ws.AddTask(f.proj, sid, "00000000-0000-0000-0000-000000000000", nil, nil)
	if code, field := codeAndField(err); code != "work_session.task_not_found" || field != "task_id" {
		t.Errorf("unknown task = %v (field %q), want task_not_found on task_id", err, field)
	}
}

// Parar o ponto pela rota de outro projeto é recusado: a rota diz qual projeto se está parando.
func TestClockOut_ChecksTheProjectOfTheOpenSession(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc, wsSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	ana, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	one, _ := projSvc.Create(org.ID.String(), "Um", "", 0, project.Routine{})
	other, _ := projSvc.Create(org.ID.String(), "Dois", "", 0, project.Routine{})
	setRate(t, one.ID.String(), ana.ID.String(), 2000)
	setRate(t, other.ID.String(), ana.ID.String(), 2000)
	tk, err := taskSvc.Create(one.ID.String(), "T", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wsSvc.ClockIn(one.ID.String(), tk.ID.String(), ana.ID.String()); err != nil {
		t.Fatal(err)
	}

	if _, err := wsSvc.ClockOut(other.ID.String(), ana.ID.String()); !errors.Is(err, work_session.ErrOpenInOtherProject) {
		t.Errorf("clock-out through the other project = %v, want work_session.open_in_other_project", err)
	}
	if _, err := wsSvc.ClockOut(one.ID.String(), ana.ID.String()); err != nil {
		t.Errorf("clock-out through its own project = %v, want it stopped", err)
	}
}

// A lista recusa um id malformado no filtro, como o total.
func TestListByProject_RejectsMalformedFilters(t *testing.T) {
	f := newSessionFixture(t)
	bad := "not-a-uuid"
	good := f.ana.String()

	_, err := f.ws.ListByProject(f.proj, &bad, nil)
	if code, field := codeAndField(err); code != "work_session.invalid_task_filter" || field != "task_id" {
		t.Errorf("malformed task = %v (field %q)", err, field)
	}
	_, err = f.ws.ListByProject(f.proj, nil, &bad)
	if code, field := codeAndField(err); code != "work_session.invalid_person_filter" || field != "person_id" {
		t.Errorf("malformed person = %v (field %q)", err, field)
	}
	if _, err := f.ws.ListByProject(f.proj, nil, &good); err != nil {
		t.Errorf("valid person = %v", err)
	}
	if _, err := f.ws.ListByProject(f.proj, nil, nil); err != nil {
		t.Errorf("no filter = %v", err)
	}
}
