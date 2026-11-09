package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	"working-time-tracker/testutil"
	"working-time-tracker/web"
)

// setSolo liga ou desliga o modo solo direto no banco: quem decide quando ele pode ligar é a organização (uma pessoa só,
// sem convite), e aqui só importa o que o A receber faz com a bandeira.
func setSolo(t *testing.T, orgID string, solo bool) {
	t.Helper()
	id, err := uuid.Parse(orgID)
	if err != nil {
		t.Fatal(err)
	}
	if err := testClient.Organization.UpdateOneID(id).SetSolo(solo).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// monthlyCustomer cria direto no banco um cliente mensal de 5.000,00 que fecha no último dia do mês, com o contrato
// começando no mês passado, 10 dias de prazo, 2% de multa e 1% ao mês de juros.
func monthlyCustomer(t *testing.T, orgID string) *ent.Customer {
	t.Helper()
	oid, _ := uuid.Parse(orgID)
	start := time.Now().In(time.FixedZone("BRT", -3*3600)).AddDate(0, -1, 0).Format("2006-01") + "-01"
	return testClient.Customer.Create().SetOrganizationID(oid).SetName("Mensal").SetBillingMode("monthly").
		SetMonthlyFeeCents(500000).SetContractStart(start).SetClosingDay(31).SetPaymentDays(10).
		SetLateFeeBp(200).SetLateInterestBpMonth(100).SaveX(context.Background())
}

// hourlyCustomerWithHours cria um cliente por hora (fecha no último dia do mês) com uma hora trabalhada no mês passado,
// a 100,00, no projeto dele.
func hourlyCustomerWithHours(t *testing.T, orgID, personID string) *ent.Customer {
	t.Helper()
	ctx := context.Background()
	oid, _ := uuid.Parse(orgID)
	pid, _ := uuid.Parse(personID)
	c := testClient.Customer.Create().SetOrganizationID(oid).SetName("Por hora").SetBillingMode("hourly").
		SetClosingDay(31).SetPaymentDays(5).SaveX(ctx)
	prj := testClient.Project.Create().SetOrganizationID(oid).SetName("Site").SetCustomerID(c.ID).SaveX(ctx)
	task := testClient.Task.Create().SetProjectID(prj.ID).SetName("T").SaveX(ctx)
	// O meio do mês passado, para a sessão cair inteira num período fechado seja qual for o dia de hoje.
	mid := time.Now().UTC().AddDate(0, -1, 0)
	start := time.Date(mid.Year(), mid.Month(), 15, 15, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	pay, bill := 0, 10000
	testutil.Session(t, testClient, task.ID, pid, start, &end, &pay, &bill)
	return c
}

func receivablesPath(a account) string { return "/api/orgs/" + a.orgID + "/receivables" }

// errCode lê o código de uma resposta de erro.
func errCode(t *testing.T, e *echo.Echo, method, path, body, session string) (int, string) {
	t.Helper()
	rec := do(e, method, path, body, session)
	detail, _ := decode(t, rec)["error"].(map[string]any)
	code, _ := detail["code"].(string)
	return rec.Code, code
}

// adjustedCount conta as contas gravadas (ajustadas) da lista A receber.
func adjustedCount(list map[string]any) int {
	n := 0
	for _, d := range list["to_receive"].([]any) {
		if d.(map[string]any)["id"] != nil {
			n++
		}
	}
	return n
}

// Fora do modo solo as nove rotas e a página não existem, nem para o dono; ligar o modo as abre, e desligá-lo as esconde
// de novo sem apagar o que foi gravado.
func TestReceivables_OnlyInSoloMode(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	org := receivablesPath(owner)
	const missing = "00000000-0000-0000-0000-000000000000"
	routes := []struct{ method, path, body string }{
		{"GET", org, ""},
		{"POST", org, `{}`},
		{"POST", org + "/receive", `{}`},
		{"GET", org + "/late-charges", ""},
		{"PATCH", org + "/" + missing, `{}`},
		{"DELETE", org + "/" + missing, ""},
		{"GET", org + "/" + missing + "/late-charges", ""},
		{"POST", org + "/" + missing + "/receive", `{}`},
		{"DELETE", org + "/" + missing + "/receive", ""},
	}
	for _, r := range routes {
		if status, code := errCode(t, e, r.method, r.path, r.body, owner.session); status != http.StatusNotFound || code != "request.not_found" {
			t.Errorf("%s %s outside the solo mode = %d %s, want 404 request.not_found", r.method, r.path, status, code)
		}
	}
	if got := status(e, "GET", "/orgs/"+owner.orgID+"/receivables", "", owner.session); got != http.StatusNotFound {
		t.Errorf("the page outside the solo mode = %d, want 404", got)
	}

	setSolo(t, owner.orgID, true)
	if got := status(e, "GET", org, "", owner.session); got != http.StatusOK {
		t.Fatalf("the list in the solo mode = %d, want 200", got)
	}
	if got := status(e, "GET", "/orgs/"+owner.orgID+"/receivables", "", owner.session); got != http.StatusOK {
		t.Errorf("the page in the solo mode = %d, want 200", got)
	}
	// Com o recurso inexistente, o dono do modo solo passa da porta e esbarra no 404 do recebível.
	if status, code := errCode(t, e, "DELETE", org+"/"+missing, "", owner.session); status != http.StatusNotFound || code != "receivable.not_found" {
		t.Errorf("a receivable that does not exist = %d %s, want 404 receivable.not_found", status, code)
	}

	// Desligar o modo esconde as rotas; os recebíveis ficam guardados e voltam quando ele liga de novo.
	customer := monthlyCustomer(t, owner.orgID)
	rec := do(e, "GET", org, "", owner.session)
	first := decode(t, rec)["to_receive"].([]any)[0].(map[string]any)
	body := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","amount_cents":500000}`, customer.ID, first["period_start"], first["period_end"])
	if rec := do(e, "POST", org, body, owner.session); rec.Code != http.StatusCreated {
		t.Fatalf("confirm = %d: %s", rec.Code, rec.Body.String())
	}
	setSolo(t, owner.orgID, false)
	if got := status(e, "GET", org, "", owner.session); got != http.StatusNotFound {
		t.Errorf("the list after turning the mode off = %d, want 404", got)
	}
	setSolo(t, owner.orgID, true)
	if got := adjustedCount(decode(t, do(e, "GET", org, "", owner.session))); got != 1 {
		t.Errorf("after turning the mode off and on again A receber has %d adjusted items, want the 1 that was recorded", got)
	}
}

// Só o dono usa o A receber: o admin e o colaborador recebem 403 na API e 404 na página, mesmo no modo solo.
func TestReceivables_OwnerOnly(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	member := invite(t, e, owner, "bruno@test.com", "member")
	setSolo(t, owner.orgID, true)
	org := receivablesPath(owner)

	for _, p := range []struct {
		name    string
		session string
	}{{"admin", admin.session}, {"member", member.session}} {
		if got := status(e, "GET", org, "", p.session); got != http.StatusForbidden {
			t.Errorf("%s: the list = %d, want 403", p.name, got)
		}
		if got := status(e, "POST", org, `{}`, p.session); got != http.StatusForbidden {
			t.Errorf("%s: confirm = %d, want 403", p.name, got)
		}
		if got := status(e, "GET", "/orgs/"+owner.orgID+"/receivables", "", p.session); got != http.StatusNotFound {
			t.Errorf("%s: the page = %d, want 404", p.name, got)
		}
	}
	if got := status(e, "GET", org, "", ""); got != http.StatusUnauthorized {
		t.Errorf("no session: the list = %d, want 401", got)
	}
}

// O caminho do dono com ajuste: a conta calculada, o ajuste (a confirmação), a edição, a sugestão de multa e juros, o
// recebimento e cada passo desfeito.
func TestReceivables_OwnerFlow(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	org := receivablesPath(owner)
	customer := monthlyCustomer(t, owner.orgID)

	list := decode(t, do(e, "GET", org, "", owner.session))
	// Do contrato do mês passado até o corrente: o mês passado já fechou (A receber) e o corrente está em andamento (o
	// seguinte só quando começar).
	toReceive, progress := list["to_receive"].([]any), list["in_progress"].([]any)
	if len(toReceive) != 1 || len(progress) != 1 {
		t.Fatalf("to receive %d, in progress %d, want 1 and 1: %v", len(toReceive), len(progress), list)
	}
	first := toReceive[0].(map[string]any)
	if first["kind"] != "monthly" || first["calculated_cents"] != float64(500000) || first["amount_cents"] != float64(500000) ||
		first["id"] != nil || first["adjusted"] != false || first["customer"].(map[string]any)["name"] != "Mensal" {
		t.Fatalf("the closed month = %v", first)
	}
	if p := progress[0].(map[string]any); p["id"] != nil || p["amount_cents"] != float64(500000) {
		t.Errorf("the current month = %v", p)
	}
	totals := list["totals"].(map[string]any)
	if totals["to_receive_count"] != float64(1) || totals["to_receive_cents"] != float64(500000) ||
		totals["in_progress_count"] != float64(1) || totals["in_progress_cents"] != float64(500000) {
		t.Errorf("totals = %v", totals)
	}

	confirm := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","amount_cents":630000,"invoice_number":"NF-1","note":"combinado"}`,
		customer.ID, first["period_start"], first["period_end"])
	rec := do(e, "POST", org, confirm, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("confirm = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)
	id := created["id"].(string)
	if created["amount_cents"] != float64(630000) || created["calculated_cents"] != float64(500000) || created["due_date"] != first["due_date"] || created["invoice_number"] != "NF-1" {
		t.Errorf("confirmed = %v", created)
	}
	if status, code := errCode(t, e, "POST", org, confirm, owner.session); status != http.StatusConflict || code != "receivable.already_confirmed" {
		t.Errorf("confirming twice = %d %s, want 409 receivable.already_confirmed", status, code)
	}

	// Editar vale enquanto não foi recebido.
	rec = do(e, "PATCH", org+"/"+id, `{"note":"","invoice_number":"NF-2"}`, owner.session)
	if edited := decode(t, rec); rec.Code != http.StatusOK || edited["invoice_number"] != "NF-2" || edited["note"] != "" || edited["amount_cents"] != float64(630000) {
		t.Errorf("edit = %d %v", rec.Code, edited)
	}

	// A sugestão de 10 dias depois do vencimento: 2% de multa e 1% ao mês de juros sobre 6.300,00.
	due, _ := time.Parse(time.DateOnly, created["due_date"].(string))
	on := due.AddDate(0, 0, 10).Format(time.DateOnly)
	rec = do(e, "GET", org+"/"+id+"/late-charges?on="+on, "", owner.session)
	charges := decode(t, rec)
	if rec.Code != http.StatusOK || charges["late_fee_cents"] != float64(12600) || charges["interest_cents"] != float64(2100) || charges["total_cents"] != float64(644700) || charges["days_late"] != float64(10) {
		t.Errorf("late charges = %d %v, want fee 126,00, interest 21,00, total 6.447,00", rec.Code, charges)
	}
	if status, code := errCode(t, e, "GET", org+"/"+id+"/late-charges?on=ontem", "", owner.session); status != http.StatusBadRequest || code != "request.field_invalid" {
		t.Errorf("a bad date = %d %s, want 400 request.field_invalid", status, code)
	}

	// Receber: o valor é livre, e o recebido hoje tem de ser pelo menos igual à multa e aos juros.
	if status, code := errCode(t, e, "POST", org+"/"+id+"/receive", `{"received_cents":100,"late_fee_cents":600,"interest_cents":600}`, owner.session); status != http.StatusBadRequest || code != "receivable.charges_exceed_received" {
		t.Errorf("charges over the amount = %d %s", status, code)
	}
	rec = do(e, "POST", org+"/"+id+"/receive", `{"received_cents":644700,"late_fee_cents":12600,"interest_cents":2100}`, owner.session)
	received := decode(t, rec)
	if rec.Code != http.StatusOK || received["received_cents"] != float64(644700) || received["received_on"] == nil || received["late_fee_cents"] != float64(12600) {
		t.Errorf("receive = %d %v", rec.Code, received)
	}
	if status, code := errCode(t, e, "POST", org+"/"+id+"/receive", `{}`, owner.session); status != http.StatusConflict || code != "receivable.already_received" {
		t.Errorf("receiving twice = %d %s", status, code)
	}
	if status, code := errCode(t, e, "PATCH", org+"/"+id, `{"note":"x"}`, owner.session); status != http.StatusConflict || code != "receivable.already_received" {
		t.Errorf("editing a received receivable = %d %s", status, code)
	}
	if status, code := errCode(t, e, "DELETE", org+"/"+id, "", owner.session); status != http.StatusConflict || code != "receivable.already_received" {
		t.Errorf("undoing the confirmation of a received receivable = %d %s", status, code)
	}

	// Recebido, o mês sai de A receber; o corrente segue em andamento.
	list = decode(t, do(e, "GET", org, "", owner.session))
	if got := len(list["received"].([]any)); got != 1 || len(list["to_receive"].([]any)) != 0 || len(list["in_progress"].([]any)) != 1 {
		t.Errorf("after receiving: %d received, %d to receive, %d in progress, want 1, 0 and 1", got, len(list["to_receive"].([]any)), len(list["in_progress"].([]any)))
	}

	// Cada passo se desfaz: o recebimento (a conta volta ajustada para A receber) e o ajuste (volta a ser calculada).
	if got := status(e, "DELETE", org+"/"+id+"/receive", "", owner.session); got != http.StatusNoContent {
		t.Errorf("undo the receipt = %d, want 204", got)
	}
	if status, code := errCode(t, e, "DELETE", org+"/"+id+"/receive", "", owner.session); status != http.StatusConflict || code != "receivable.not_received" {
		t.Errorf("undoing a receipt that is not there = %d %s", status, code)
	}
	if got := adjustedCount(decode(t, do(e, "GET", org, "", owner.session))); got != 1 {
		t.Errorf("after undoing the receipt A receber has %d adjusted items, want 1", got)
	}
	if got := status(e, "DELETE", org+"/"+id, "", owner.session); got != http.StatusNoContent {
		t.Errorf("undo the adjustment = %d, want 204", got)
	}
	list = decode(t, do(e, "GET", org, "", owner.session))
	if len(list["to_receive"].([]any)) != 1 || adjustedCount(list) != 0 {
		t.Errorf("the period did not come back as calculated: %v", list["to_receive"])
	}
}

// Receber direto, sem ajustar: o mês corrente adiantado, o mês fechado atrasado com a sugestão de multa e juros do período
// calculado, e as recusas.
func TestReceivables_ReceiveCalculated(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	org := receivablesPath(owner)
	customer := monthlyCustomer(t, owner.orgID)
	list := decode(t, do(e, "GET", org, "", owner.session))
	current := list["in_progress"].([]any)[0].(map[string]any)
	closed := list["to_receive"].([]any)[0].(map[string]any)
	periodBody := func(p map[string]any, extra string) string {
		return fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s"%s}`, customer.ID, p["period_start"], p["period_end"], extra)
	}

	if status, code := errCode(t, e, "POST", org+"/receive", `{"cliente":"x"}`, owner.session); status != http.StatusBadRequest || code != "request.invalid_body" {
		t.Errorf("an unknown key = %d %s, want 400 request.invalid_body", status, code)
	}
	rec := do(e, "POST", org+"/receive", periodBody(current, ""), owner.session)
	if got := decode(t, rec); rec.Code != http.StatusCreated || got["received_cents"] != float64(500000) || got["received_on"] == nil {
		t.Errorf("receive the current month in advance = %d %v", rec.Code, got)
	}
	if status, code := errCode(t, e, "POST", org+"/receive", periodBody(current, ""), owner.session); status != http.StatusConflict || code != "receivable.already_confirmed" {
		t.Errorf("receiving twice = %d %s, want 409 receivable.already_confirmed", status, code)
	}

	// A sugestão de 10 dias depois do vencimento: 2% de multa e 1% ao mês de juros sobre 5.000,00.
	due, _ := time.Parse(time.DateOnly, closed["due_date"].(string))
	on := due.AddDate(0, 0, 10).Format(time.DateOnly)
	query := fmt.Sprintf("?customer_id=%s&period_start=%s&period_end=%s&on=%s", customer.ID, closed["period_start"], closed["period_end"], on)
	rec = do(e, "GET", org+"/late-charges"+query, "", owner.session)
	charges := decode(t, rec)
	if rec.Code != http.StatusOK || charges["late_fee_cents"] != float64(10000) || charges["interest_cents"] != float64(1667) || charges["total_cents"] != float64(511667) {
		t.Errorf("late charges of the calculated period = %d %v, want fee 100,00, interest 16,67, total 5.116,67", rec.Code, charges)
	}
	rec = do(e, "POST", org+"/receive", periodBody(closed, `,"received_cents":511667,"late_fee_cents":10000,"interest_cents":1667,"invoice_number":"NF-5"`), owner.session)
	if got := decode(t, rec); rec.Code != http.StatusCreated || got["received_cents"] != float64(511667) || got["invoice_number"] != "NF-5" || got["amount_cents"] != float64(500000) {
		t.Errorf("receive the closed month = %d %v", rec.Code, got)
	}
	list = decode(t, do(e, "GET", org, "", owner.session))
	if len(list["received"].([]any)) != 2 || len(list["to_receive"].([]any)) != 0 || len(list["in_progress"].([]any)) != 0 {
		t.Errorf("after receiving both: %v", list)
	}

	// No por hora o período aberto é recusado, e multa e juros também.
	hourly := hourlyCustomerWithHours(t, owner.orgID, owner.id)
	now := time.Now().In(time.FixedZone("BRT", -3*3600))
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	open := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s"}`, hourly.ID, first.Format(time.DateOnly), first.AddDate(0, 1, -1).Format(time.DateOnly))
	if status, code := errCode(t, e, "POST", org+"/receive", open, owner.session); status != http.StatusConflict || code != "receivable.period_open" {
		t.Errorf("the open hourly period = %d %s, want 409 receivable.period_open", status, code)
	}
	var hourlyDue map[string]any
	for _, d := range decode(t, do(e, "GET", org, "", owner.session))["to_receive"].([]any) {
		hourlyDue = d.(map[string]any)
	}
	if hourlyDue == nil {
		t.Fatal("the hourly customer has nothing to receive")
	}
	withFee := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","late_fee_cents":100}`, hourly.ID, hourlyDue["period_start"], hourlyDue["period_end"])
	if status, code := errCode(t, e, "POST", org+"/receive", withFee, owner.session); status != http.StatusBadRequest || code != "receivable.charges_not_allowed" {
		t.Errorf("a late fee on an hourly period = %d %s", status, code)
	}
}

// O período aberto do cliente por hora é recusado (409), e o período fechado, com as horas do mês passado, confirma.
func TestReceivables_HourlyPeriodRules(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	org := receivablesPath(owner)
	customer := hourlyCustomerWithHours(t, owner.orgID, owner.id)

	// O mês corrente, no fuso de São Paulo (o padrão da organização), é o período aberto.
	now := time.Now().In(time.FixedZone("BRT", -3*3600))
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	open := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","amount_cents":10000}`,
		customer.ID, first.Format(time.DateOnly), first.AddDate(0, 1, -1).Format(time.DateOnly))
	if status, code := errCode(t, e, "POST", org, open, owner.session); status != http.StatusConflict || code != "receivable.period_open" {
		t.Errorf("the open period = %d %s, want 409 receivable.period_open", status, code)
	}

	proposals := decode(t, do(e, "GET", org, "", owner.session))["to_receive"].([]any)
	if len(proposals) != 1 {
		t.Fatalf("to receive = %v, want the one closed month with hours", proposals)
	}
	p := proposals[0].(map[string]any)
	if p["kind"] != "hourly" || p["calculated_cents"] != float64(10000) || p["seconds"] != float64(3600) {
		t.Errorf("proposal = %v, want 1h and 100,00", p)
	}
	closed := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","amount_cents":10000}`, customer.ID, p["period_start"], p["period_end"])
	rec := do(e, "POST", org, closed, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("confirm the closed period = %d: %s", rec.Code, rec.Body.String())
	}
	id := decode(t, rec)["id"].(string)
	// No cliente por hora, multa e juros são recusados.
	if status, code := errCode(t, e, "POST", org+"/"+id+"/receive", `{"late_fee_cents":100}`, owner.session); status != http.StatusBadRequest || code != "receivable.charges_not_allowed" {
		t.Errorf("a late fee on an hourly receivable = %d %s", status, code)
	}
	if got := status(e, "POST", org+"/"+id+"/receive", `{}`, owner.session); got != http.StatusOK {
		t.Errorf("receive an hourly receivable = %d, want 200", got)
	}
}

// Um corpo com campo desconhecido é recusado, e os IDs de outra organização valem como inexistentes.
func TestReceivables_BodyAndOrganizations(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	org := receivablesPath(owner)
	customer := monthlyCustomer(t, owner.orgID)

	if status, code := errCode(t, e, "POST", org, `{"cliente":"x"}`, owner.session); status != http.StatusBadRequest || code != "request.invalid_body" {
		t.Errorf("an unknown key = %d %s, want 400 request.invalid_body", status, code)
	}
	proposals := decode(t, do(e, "GET", org, "", owner.session))["to_receive"].([]any)
	p := proposals[0].(map[string]any)
	body := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","amount_cents":500000}`, customer.ID, p["period_start"], p["period_end"])
	id := decode(t, do(e, "POST", org, body, owner.session))["id"].(string)

	other := signup(t, e, "Outra", "bia@test.com")
	setSolo(t, other.orgID, true)
	theirs := receivablesPath(other)
	// O cliente de outra organização não confirma, e o recebível dela não se vê, edita, recebe nem desfaz.
	if status, code := errCode(t, e, "POST", theirs, body, other.session); status != http.StatusNotFound || code != "customer.not_found" {
		t.Errorf("another organization's customer = %d %s, want 404 customer.not_found", status, code)
	}
	for _, r := range []struct{ method, path, body string }{
		{"PATCH", theirs + "/" + id, `{"note":"x"}`},
		{"DELETE", theirs + "/" + id, ""},
		{"GET", theirs + "/" + id + "/late-charges", ""},
		{"POST", theirs + "/" + id + "/receive", `{}`},
		{"DELETE", theirs + "/" + id + "/receive", ""},
	} {
		if status, code := errCode(t, e, r.method, r.path, r.body, other.session); status != http.StatusNotFound || code != "receivable.not_found" {
			t.Errorf("%s %s from another organization = %d %s, want 404 receivable.not_found", r.method, r.path, status, code)
		}
	}
	// Chamar a rota da organização dela com a sessão de fora é recusado antes de tudo.
	if status, _ := errCode(t, e, "GET", org, "", other.session); status != http.StatusNotFound {
		t.Errorf("another organization's list = %d, want 404", status)
	}
	theirList := decode(t, do(e, "GET", theirs, "", other.session))
	if len(theirList["to_receive"].([]any)) != 0 || len(theirList["in_progress"].([]any)) != 0 {
		t.Errorf("the other organization sees %v", theirList)
	}
	// O recebível dela segue intacto.
	if due := decode(t, do(e, "GET", org, "", owner.session))["to_receive"].([]any); len(due) != 1 || due[0].(map[string]any)["id"] != id || due[0].(map[string]any)["note"] != "" {
		t.Errorf("the receivable changed: %v", due)
	}
}

// A página A receber: o esqueleto que o Alpine preenche, no idioma da pessoa, para o dono do modo solo.
func TestReceivables_Page(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	rec := do(e, "GET", "/orgs/"+owner.orgID+"/receivables", "", owner.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("page = %d", rec.Code)
	}
	html := rec.Body.String()
	for _, want := range []string{
		`x-data="orgReceivables"`, "/static/pages/receivables.js", "<h1>A receber</h1>",
		`data-field="amount_cents"`, `data-field="received_on"`, `data-field="late_fee_cents"`, "receivable-edit", "receivable-receive", `data-field="invoice_number"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// A permissão chega ao JavaScript.
	if !strings.Contains(html, "receivables.manage") {
		t.Error("window.BOOT.can lacks receivables.manage")
	}
}

// A página A receber: quem entra pela URL cai sempre numa resposta que faz sentido. Sem sessão vai ao login, lembrando a
// página; o dono de uma organização com equipe, o admin e o colaborador recebem a página de 404 (nunca uma página em
// branco, um 403 ou um redirecionamento); o dono do modo solo abre a página.
func TestReceivables_PageAccess(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	member := invite(t, e, owner, "bruno@test.com", "member")
	page := "/orgs/" + owner.orgID + "/receivables"

	notFound := func(who string, session string) {
		t.Helper()
		rec := do(e, "GET", page, "", session)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Página não encontrada") || rec.Header().Get("Location") != "" {
			t.Errorf("%s: GET %s = %d (location %q), want the 404 page", who, page, rec.Code, rec.Header().Get("Location"))
		}
	}

	// Sem sessão: o login, com a página pedida em ?next=, tenha a organização o modo solo ou não.
	wantLogin := "/login?next=" + url.QueryEscape(page)
	for _, solo := range []bool{false, true} {
		setSolo(t, owner.orgID, solo)
		rec := do(e, "GET", page, "", "")
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != wantLogin {
			t.Errorf("no session (solo %v): GET %s = %d to %q, want 303 to %q", solo, page, rec.Code, rec.Header().Get("Location"), wantLogin)
		}
	}

	// Organização com equipe: o dono também recebe o 404, e a API também (a página de erro é só das páginas).
	setSolo(t, owner.orgID, false)
	notFound("owner of an organization with a team", owner.session)
	if got := status(e, "GET", receivablesPath(owner), "", owner.session); got != http.StatusNotFound {
		t.Errorf("the API outside the solo mode = %d, want 404", got)
	}

	// Modo solo: o dono abre; o admin e o colaborador não.
	setSolo(t, owner.orgID, true)
	if rec := do(e, "GET", page, "", owner.session); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `x-data="orgReceivables"`) {
		t.Errorf("owner of the solo organization: GET %s = %d, want the page", page, rec.Code)
	}
	notFound("admin", admin.session)
	notFound("member", member.session)

	// O título da página é o texto de titles.receivables (o item da barra, que a frente do modo solo põe, marca a página
	// pela seção "receivables").
	rec := do(e, "GET", page, "", owner.session)
	if !strings.Contains(rec.Body.String(), "<title>A receber · Working Time Tracker</title>") {
		t.Error("the page title is not A receber")
	}
}

// Dentro do modo solo estas páginas respondem 404 e não podem ser linkadas: as de equipe (Colaboradores, o perfil de uma
// pessoa, os Pagamentos da equipe e, no projeto, as abas Colaboradores e Times).
var teamOnlyPages = []*regexp.Regexp{
	regexp.MustCompile(`^/orgs/[^/]+/people(/.*)?$`),
	regexp.MustCompile(`^/orgs/[^/]+/payments$`),
	regexp.MustCompile(`^/projects/[^/]+/collaborators$`),
	regexp.MustCompile(`^/projects/[^/]+/management/teams$`),
	regexp.MustCompile(`^/projects/[^/]+/teams$`),
}

// internalLinks são os href que a página renderizada leva para dentro do sistema, sem a query e o fragmento.
func internalLinks(html string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`\bhref="(/[^"]*)"`).FindAllStringSubmatch(html, -1) {
		link := m[1]
		if i := strings.IndexAny(link, "?#"); i >= 0 {
			link = link[:i]
		}
		out = append(out, strings.TrimSuffix(link, "/"))
	}
	return out
}

func teamOnly(link string) bool {
	for _, re := range teamOnlyPages {
		if re.MatchString(link) {
			return true
		}
	}
	return false
}

// A página A receber só linka o que existe no modo solo: os links do conteúdo dela, e o JavaScript e o template que a
// montam, nunca apontam para uma página que responde 404 nele. A barra superior é da frente do modo solo: enquanto ela
// ainda leva Colaboradores e Pagamentos, a conferência dela espera (e passa a valer assim que o item A receber entra).
func TestReceivables_PageLinksStayInSoloMode(t *testing.T) {
	if !teamOnly("/orgs/x/people") || !teamOnly("/orgs/x/people/y") || !teamOnly("/orgs/x/payments") || teamOnly("/orgs/x/customers") || teamOnly("/projects/x") {
		t.Fatal("the list of team-only pages is wrong")
	}

	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	rec := do(e, "GET", "/orgs/"+owner.orgID+"/receivables", "", owner.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("page = %d", rec.Code)
	}
	html := rec.Body.String()

	topbarStart, topbarEnd := strings.Index(html, `<header class="topbar">`), strings.Index(html, "</header>")
	if topbarStart < 0 || topbarEnd < topbarStart {
		t.Fatal("the page has no topbar to tell it apart from the content")
	}
	topbar, content := html[topbarStart:topbarEnd], html[topbarEnd:]

	links := internalLinks(content)
	for _, link := range links {
		if teamOnly(link) {
			t.Errorf("the page links to %s, which is a 404 in the solo mode", link)
		}
	}
	// O extrator acha os links de uma página que os tem, sem a query, e ignora os que saem do sistema.
	if got := internalLinks(`<a href="/orgs/x/people?a=1#b">p</a><a href="https://example.com/orgs/x/people">e</a>`); len(got) != 1 || got[0] != "/orgs/x/people" {
		t.Errorf("internalLinks = %v, want the one internal link without its query", got)
	}

	// O JavaScript e o template não escrevem à mão um caminho dessas páginas.
	for _, file := range []string{"static/pages/receivables.js", "templates/pages/org_receivables.gohtml"} {
		src, err := web.FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"/people", "/payments", "/collaborators", "management/teams", "/teams"} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s mentions %q, a page that is a 404 in the solo mode", file, bad)
			}
		}
	}

	// A barra: só se confere quando o item A receber já está nela, o que é a prova de que a barra do modo solo chegou.
	if !strings.Contains(topbar, `/receivables"`) {
		t.Log("the topbar has no A receber item yet (it belongs to the solo-mode front): its links are checked after the merge")
		return
	}
	for _, link := range internalLinks(topbar) {
		if teamOnly(link) {
			t.Errorf("the topbar links to %s, which is a 404 in the solo mode", link)
		}
	}
}
