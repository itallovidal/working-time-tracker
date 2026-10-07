package overview_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/testutil"
)

// periodLine resume uma janela para a mensagem de erro.
func periodLine(p overview.Period) string {
	return fmt.Sprintf("%.0fs (mine %.0fs) in %d sessions, pay %s, bill %s, margin %s",
		p.Seconds, p.MySeconds, p.SessionCount, show(p.Money.PayAmountCents), show(p.Money.BillAmountCents), show(p.Money.MarginCents))
}

// O cenário: a organização tem a Ana, o Bruno, a Carla e o Diego e dois projetos. Há sessões
// fora e dentro das janelas de 7 e 30 dias, uma que atravessa a borda dos 30 dias (metade de
// dentro), uma do dono (valor pago zero), uma aberta sem valor por hora, e uma de outra
// organização, que não entra em conta nenhuma.
func TestOrganization_Periods(t *testing.T) {
	f := setup(t)
	ana, bruno, carla, diego := f.person("Ana"), f.person("Bruno"), f.person("Carla"), f.person("Diego")
	x, y := f.project("Projeto X"), f.project("Projeto Y")
	tx, ty := f.task(x, ana, nil), f.task(y, ana, nil)

	pay, bill := cents(2000), cents(10000)
	f.session(tx, ana, 40*day, 2*time.Hour, pay, bill)                                    // 40,00 e 200,00, só no total
	f.session(ty, ana, 10*day, 90*time.Minute, pay, bill)                                 // 30,00 e 150,00, nos 30 dias
	f.session(tx, bruno, 3*day, 100*time.Second, pay, bill)                               // 0,56 e 2,78 (arredondados)
	f.session(ty, bruno, 2*day, 100*time.Second, pay, bill)                               // outra igual: a soma é 1,12, e não 1,11
	f.session(tx, ana, 30*day+time.Hour, 2*time.Hour, pay, bill)                          // atravessa a borda: 1h dentro dos 30 dias
	f.session(tx, ana, day, time.Hour, cents(0), bill)                                    // do dono: pago zero, cobrado 100,00
	testutil.Session(t, testClient, ty, diego, f.now.Add(-30*time.Minute), nil, nil, nil) // aberta e sem valor

	// Outra organização: a sessão dela fica fora de todas as contas.
	otherOrg, err := organization.NewService(organization.NewStore(testClient)).Create("Outra")
	if err != nil {
		t.Fatalf("other org: %v", err)
	}
	zed, err := person.NewService(person.NewStore(testClient)).Create(otherOrg.ID.String(), "Zed", "zed@outra.com")
	if err != nil {
		t.Fatalf("other person: %v", err)
	}
	z, err := f.projects.Create(otherOrg.ID.String(), "Projeto Z", "", 0, project.Routine{})
	if err != nil {
		t.Fatalf("other project: %v", err)
	}
	tz := f.task(z.ID.String(), zed.ID, nil)
	f.session(tz, zed.ID, day, time.Hour, pay, bill)

	got, err := f.svc.Organization(f.orgID, ana.String())
	if err != nil {
		t.Fatalf("organization overview: %v", err)
	}

	if got.People.Total != 4 || got.People.WorkingNow != 1 || got.Projects.Total != 2 {
		t.Errorf("people = %+v, projects = %+v; want 4 people, 1 working now, 2 projects", got.People, got.Projects)
	}
	if !got.GeneratedAt.Equal(f.now) {
		t.Errorf("generated at %s, want the injected clock %s", got.GeneratedAt, f.now)
	}
	for name, c := range map[string]struct {
		got  overview.Period
		want string
	}{
		"last 7 days":  {got.Periods.Last7Days, "5600s (mine 3600s) in 4 sessions, pay 112, bill 10556, margin 10444"},
		"last 30 days": {got.Periods.Last30Days, "14600s (mine 12600s) in 6 sessions, pay 5112, bill 35556, margin 30444"},
		"all time":     {got.Periods.AllTime, "25400s (mine 23400s) in 7 sessions, pay 11112, bill 65556, margin 54444"},
	} {
		if line := periodLine(c.got); line != c.want {
			t.Errorf("%s = %s\nwant      %s", name, line, c.want)
		}
	}

	// Todas as pessoas, da que mais trabalhou para a que menos; a Carla, sem ponto, vem com zeros.
	rows := make([]string, len(got.ByPerson))
	var sum7, sum30, sumAll float64
	for i, p := range got.ByPerson {
		rows[i] = fmt.Sprintf("%s: %.0f/%.0f/%.0f now=%v", p.Person.Name, p.Last7DaysSeconds, p.Last30DaysSeconds, p.TotalSeconds, p.WorkingNow)
		sum7 += p.Last7DaysSeconds
		sum30 += p.Last30DaysSeconds
		sumAll += p.TotalSeconds
	}
	want := []string{"Ana: 3600/12600/23400 now=false", "Diego: 1800/1800/1800 now=true", "Bruno: 200/200/200 now=false", "Carla: 0/0/0 now=false"}
	if !slices.Equal(rows, want) {
		t.Errorf("by person = %q\nwant        %q", rows, want)
	}
	if sum7 != got.Periods.Last7Days.Seconds || sum30 != got.Periods.Last30Days.Seconds || sumAll != got.Periods.AllTime.Seconds {
		t.Errorf("the rows add up to %.0f, %.0f and %.0f; the periods say %.0f, %.0f and %.0f",
			sum7, sum30, sumAll, got.Periods.Last7Days.Seconds, got.Periods.Last30Days.Seconds, got.Periods.AllTime.Seconds)
	}

	// "Meu tempo" é de quem pede: o Bruno vê o dele, e o resto da conta não muda.
	asBruno, err := f.svc.Organization(f.orgID, bruno.String())
	if err != nil {
		t.Fatalf("overview as Bruno: %v", err)
	}
	if p := asBruno.Periods; p.Last7Days.MySeconds != 200 || p.Last30Days.MySeconds != 200 || p.AllTime.MySeconds != 200 || p.AllTime.Seconds != 25400 {
		t.Errorf("as Bruno: mine = %.0f, %.0f, %.0f of %.0f; want 200 in each window of 25400", p.Last7Days.MySeconds, p.Last30Days.MySeconds, p.AllTime.MySeconds, p.AllTime.Seconds)
	}
	// Quem não bateu ponto nunca tem zero horas.
	asCarla, _ := f.svc.Organization(f.orgID, carla.String())
	if asCarla.Periods.AllTime.MySeconds != 0 {
		t.Errorf("as Carla: mine = %.0f, want 0", asCarla.Periods.AllTime.MySeconds)
	}
}

// Sem nenhuma sessão não há receita, custo nem margem para mostrar: os valores voltam nil, e
// não zero, como na visão geral do projeto. A pessoa sem ponto aparece mesmo assim.
func TestOrganization_NoSessions(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")

	got, err := f.svc.Organization(f.orgID, ana.String())
	if err != nil {
		t.Fatalf("organization overview: %v", err)
	}
	for name, p := range map[string]overview.Period{"7": got.Periods.Last7Days, "30": got.Periods.Last30Days, "all": got.Periods.AllTime} {
		if p.Seconds != 0 || p.SessionCount != 0 || p.Money.PayAmountCents != nil || p.Money.BillAmountCents != nil || p.Money.MarginCents != nil {
			t.Errorf("window %s = %s, want empty with nil money", name, periodLine(p))
		}
	}
	if got.People.Total != 1 || got.Projects.Total != 0 || len(got.ByPerson) != 1 {
		t.Errorf("people %+v, projects %+v, rows %d; want 1 person, no project, 1 row", got.People, got.Projects, len(got.ByPerson))
	}
}

// Uma sessão sem valor de receita mas com custo: a margem não existe sem receita.
func TestOrganization_NoRevenueNoMargin(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	x := f.project("Projeto X")
	f.session(f.task(x, ana, nil), ana, day, time.Hour, cents(2000), nil)

	got, err := f.svc.Organization(f.orgID, ana.String())
	if err != nil {
		t.Fatalf("organization overview: %v", err)
	}
	if m := got.Periods.AllTime.Money; show(m.PayAmountCents) != "2000" || m.BillAmountCents != nil || m.MarginCents != nil {
		t.Errorf("money = pay %s, bill %s, margin %s; want 2000, nil and nil", show(m.PayAmountCents), show(m.BillAmountCents), show(m.MarginCents))
	}
}

func TestOrganization_MalformedIDs(t *testing.T) {
	f := setup(t)
	ana := f.person("Ana")
	for name, ids := range map[string][2]string{
		"organization": {"not-a-uuid", ana.String()},
		"viewer":       {f.orgID, "not-a-uuid"},
	} {
		if _, err := f.svc.Organization(ids[0], ids[1]); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("malformed %s id = %v, want ErrNotFound", name, err)
		}
	}
}
