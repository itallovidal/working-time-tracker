package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Os termos comerciais do cliente (modo solo) pela API e pelas páginas: a validação dos formulários, o modo travado, a
// recusa do mensal numa organização que não é solo, o valor cobrado do projeto por modo, e a organização sem nada
// disso, que segue igual.

// makeSolo liga o modo solo da organização direto no banco: a rota que o dono usa em Configurações é de outra frente.
func makeSolo(t *testing.T, orgID string, solo bool) {
	t.Helper()
	if err := testClient.Organization.UpdateOneID(uuid.MustParse(orgID)).SetSolo(solo).Exec(context.Background()); err != nil {
		t.Fatalf("set solo: %v", err)
	}
}

const monthlyBody = `"billing_mode":"monthly","monthly_fee_cents":630000,"contract_start":"2026-11-01"`

// Um cliente de antes dos termos (e um criado como sempre, só com o nome) é por hora, fecha no último dia e vence no
// fechamento, com os campos de sempre iguais; a organização com equipe não ganha nada novo para fazer.
func TestCustomerBilling_OrganizationWithoutTheTermsStaysTheSame(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")

	rec := do(e, "POST", "/api/orgs/"+owner.orgID+"/customers", `{"name":"Empresa A","contact_name":"Carla"}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	if got["name"] != "Empresa A" || got["contact_name"] != "Carla" || got["country"] != "BR" || got["project_count"] != float64(0) {
		t.Errorf("legacy fields = %v", got)
	}
	if got["billing_mode"] != "hourly" || got["closing_day"] != float64(31) || got["payment_days"] != float64(0) ||
		got["default_rate_cents"] != nil || got["monthly_fee_cents"] != nil || got["contract_start"] != nil || got["contract_end"] != nil ||
		got["late_fee_bp"] != float64(0) || got["late_interest_bp_month"] != float64(0) || got["grace_days"] != float64(0) {
		t.Errorf("terms of a plain customer = %v", got)
	}

	id := got["id"].(string)
	rec = do(e, "PATCH", "/api/customers/"+id, `{"name":"Empresa Alfa"}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["name"] != "Empresa Alfa" || body["billing_mode"] != "hourly" || body["closing_day"] != float64(31) {
		t.Errorf("PATCH name = %d %v", rec.Code, body)
	}
	list := decodeList(t, do(e, "GET", "/api/orgs/"+owner.orgID+"/customers", "", owner.session))
	if len(list) != 1 || list[0]["billing_mode"] != "hourly" {
		t.Errorf("list = %v", list)
	}

	// O projeto de um cliente por hora é o de sempre: valor obrigatório, com o piso.
	prj := createProject(t, e, owner, "Projeto")
	rec = do(e, "PUT", "/api/projects/"+prj+"/billing", `{"customer_id":"`+id+`"}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_required" {
		t.Errorf("billing without a rate = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "PUT", "/api/projects/"+prj+"/billing", `{"customer_id":"`+id+`","bill_rate_cents":15000}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["bill_rate_cents"] != float64(15000) || body["customer_billing_mode"] != "hourly" {
		t.Errorf("billing with a rate = %d %v", rec.Code, body)
	}

	// Fora do modo solo a tela de Clientes não tem os termos: nem o seletor, nem a coluna.
	page := do(e, "GET", "/orgs/"+owner.orgID+"/customers", "", owner.session).Body.String()
	for _, absent := range []string{`id="customer-billing-mode"`, `id="customer-monthly-fee"`, `id="customer-default-rate"`, `data-field="billing_mode"`} {
		if strings.Contains(page, absent) {
			t.Errorf("the customers page of an organization with a team has %s", absent)
		}
	}
	if !strings.Contains(page, `id="customer-name"`) {
		t.Error("the customers page lost the name field")
	}
}

// O corpo recusado devolve status, código e o campo (a chave do corpo) para a tela marcá-lo.
func TestCustomerBilling_FormValidation(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	makeSolo(t, owner.orgID, true)
	orgID := owner.orgID

	rec := do(e, "POST", "/api/orgs/"+orgID+"/customers", `{"name":"Hora"}`, owner.session)
	hourlyID := decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/orgs/"+orgID+"/customers", `{"name":"Mensal",`+monthlyBody+`}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create monthly = %d: %s", rec.Code, rec.Body.String())
	}
	monthlyID := decode(t, rec)["id"].(string)

	create := "/api/orgs/" + orgID + "/customers"
	cases := []struct {
		name, method, path, body string
		status                   int
		code, field              string
	}{
		{"unknown mode", "POST", create, `{"name":"X","billing_mode":"weekly"}`, 400, "customer.invalid_billing_mode", "billing_mode"},
		{"empty mode", "POST", create, `{"name":"X","billing_mode":""}`, 400, "customer.invalid_billing_mode", "billing_mode"},

		// Por hora.
		{"default rate under the floor", "POST", create, `{"name":"X","default_rate_cents":999}`, 400, "customer.invalid_default_rate", "default_rate_cents"},
		{"default rate zero", "POST", create, `{"name":"X","default_rate_cents":0}`, 400, "customer.invalid_default_rate", "default_rate_cents"},
		{"default rate over the cap", "POST", create, `{"name":"X","default_rate_cents":100000001}`, 400, "customer.invalid_default_rate", "default_rate_cents"},
		{"fee on an hourly customer", "POST", create, `{"name":"X","monthly_fee_cents":1000}`, 400, "customer.field_monthly_only", "monthly_fee_cents"},
		{"contract start on an hourly customer", "POST", create, `{"name":"X","contract_start":"2026-11-01"}`, 400, "customer.field_monthly_only", "contract_start"},
		{"late fee on an hourly customer", "PATCH", "/api/customers/" + hourlyID, `{"late_fee_bp":200}`, 400, "customer.field_monthly_only", "late_fee_bp"},
		{"grace on an hourly customer", "PATCH", "/api/customers/" + hourlyID, `{"grace_days":2}`, 400, "customer.field_monthly_only", "grace_days"},

		// Mensal.
		{"no fee", "POST", create, `{"name":"X","billing_mode":"monthly","contract_start":"2026-11-01"}`, 400, "request.field_required", "monthly_fee_cents"},
		{"fee zero", "POST", create, `{"name":"X","billing_mode":"monthly","monthly_fee_cents":0,"contract_start":"2026-11-01"}`, 400, "customer.invalid_monthly_fee", "monthly_fee_cents"},
		{"fee negative", "POST", create, `{"name":"X","billing_mode":"monthly","monthly_fee_cents":-5,"contract_start":"2026-11-01"}`, 400, "customer.invalid_monthly_fee", "monthly_fee_cents"},
		{"no start", "POST", create, `{"name":"X","billing_mode":"monthly","monthly_fee_cents":1000}`, 400, "request.field_required", "contract_start"},
		{"start not a date", "POST", create, `{"name":"X","billing_mode":"monthly","monthly_fee_cents":1000,"contract_start":"01/11/2026"}`, 400, "request.field_invalid", "contract_start"},
		{"end not after start", "POST", create, `{"name":"X","billing_mode":"monthly","monthly_fee_cents":1000,"contract_start":"2026-11-01","contract_end":"2026-11-01"}`, 400, "customer.invalid_contract_end", "contract_end"},
		{"end before start", "PATCH", "/api/customers/" + monthlyID, `{"contract_end":"2026-10-01"}`, 400, "customer.invalid_contract_end", "contract_end"},
		{"default rate on a monthly customer", "PATCH", "/api/customers/" + monthlyID, `{"default_rate_cents":15000}`, 400, "customer.field_hourly_only", "default_rate_cents"},
		{"late fee over 100%", "PATCH", "/api/customers/" + monthlyID, `{"late_fee_bp":10001}`, 400, "customer.invalid_late_fee", "late_fee_bp"},
		{"interest negative", "PATCH", "/api/customers/" + monthlyID, `{"late_interest_bp_month":-1}`, 400, "customer.invalid_late_interest", "late_interest_bp_month"},
		{"grace over the cap", "PATCH", "/api/customers/" + monthlyID, `{"grace_days":366}`, 400, "customer.invalid_grace_days", "grace_days"},

		// Nos dois modos.
		{"closing day zero", "POST", create, `{"name":"X","closing_day":0}`, 400, "customer.invalid_closing_day", "closing_day"},
		{"closing day 32", "PATCH", "/api/customers/" + monthlyID, `{"closing_day":32}`, 400, "customer.invalid_closing_day", "closing_day"},
		{"payment days negative", "POST", create, `{"name":"X","payment_days":-1}`, 400, "customer.invalid_payment_days", "payment_days"},
		{"payment days over the cap", "PATCH", "/api/customers/" + hourlyID, `{"payment_days":366}`, 400, "customer.invalid_payment_days", "payment_days"},

		// O tipo errado de um campo novo é corpo inválido, com o nome do campo.
		{"closing day as text", "POST", create, `{"name":"X","closing_day":"5"}`, 400, "request.invalid_body", "closing_day"},
		{"fee as text", "POST", create, `{"name":"X","billing_mode":"monthly","monthly_fee_cents":"muito"}`, 400, "request.invalid_body", "monthly_fee_cents"},
	}
	for _, tc := range cases {
		rec := do(e, tc.method, tc.path, tc.body, owner.session)
		code, params := projErrorOf(t, rec.Body.String())
		if rec.Code != tc.status || code != tc.code {
			t.Errorf("%s = %d %s, want %d %s", tc.name, rec.Code, rec.Body.String(), tc.status, tc.code)
			continue
		}
		if params["field"] != tc.field {
			t.Errorf("%s: field = %v, want %q (%s)", tc.name, params["field"], tc.field, rec.Body.String())
		}
	}

	// O que vale passa, nos limites, e volta como foi gravado.
	ok := []struct{ name, method, path, body string }{
		{"hourly with every term", "POST", create, `{"name":"A","default_rate_cents":1000,"closing_day":1,"payment_days":365}`},
		{"hourly with the defaults spelled out", "POST", create, `{"name":"B","billing_mode":"hourly","default_rate_cents":null,"monthly_fee_cents":null,"contract_start":"","contract_end":"","late_fee_bp":0,"late_interest_bp_month":0,"grace_days":0,"closing_day":31,"payment_days":0}`},
		{"monthly with every term", "POST", create, `{"name":"C",` + monthlyBody + `,"contract_end":"2027-10-31","closing_day":28,"payment_days":10,"late_fee_bp":10000,"late_interest_bp_month":100,"grace_days":365}`},
		{"monthly edit", "PATCH", "/api/customers/" + monthlyID, `{"monthly_fee_cents":700000,"contract_end":""}`},
	}
	for _, tc := range ok {
		rec := do(e, tc.method, tc.path, tc.body, owner.session)
		if rec.Code >= 300 {
			t.Errorf("%s = %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	edited := decode(t, do(e, "GET", "/api/customers/"+monthlyID, "", owner.session))
	if edited["monthly_fee_cents"] != float64(700000) || edited["contract_end"] != nil || edited["contract_start"] != "2026-11-01" || edited["billing_mode"] != "monthly" {
		t.Errorf("monthly customer after the edit = %v", edited)
	}
}

// O mensal só vale no modo solo; desligar o modo esconde, e o cliente mensal segue guardado.
func TestCustomerBilling_MonthlyNeedsSolo(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	create := "/api/orgs/" + owner.orgID + "/customers"

	rec := do(e, "POST", create, `{"name":"Mensal",`+monthlyBody+`}`, owner.session)
	if code, params := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "customer.monthly_needs_solo" || params["field"] != "billing_mode" {
		t.Errorf("monthly outside solo = %d %s, want 400 customer.monthly_needs_solo on billing_mode", rec.Code, rec.Body.String())
	}
	rec = do(e, "POST", create, `{"name":"Hora"}`, owner.session)
	hourlyID := decode(t, rec)["id"].(string)
	rec = do(e, "PATCH", "/api/customers/"+hourlyID, `{`+monthlyBody+`}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "customer.monthly_needs_solo" {
		t.Errorf("switch to monthly outside solo = %d %s", rec.Code, rec.Body.String())
	}

	makeSolo(t, owner.orgID, true)
	rec = do(e, "PATCH", "/api/customers/"+hourlyID, `{`+monthlyBody+`}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["billing_mode"] != "monthly" || body["monthly_fee_cents"] != float64(630000) {
		t.Errorf("switch to monthly in solo = %d %v", rec.Code, body)
	}

	// Com o modo desligado o cliente mensal fica guardado e se edita sem mexer nos termos.
	makeSolo(t, owner.orgID, false)
	rec = do(e, "PATCH", "/api/customers/"+hourlyID, `{"name":"Mensal 2","contact_name":"Carla"}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["name"] != "Mensal 2" || body["billing_mode"] != "monthly" || body["monthly_fee_cents"] != float64(630000) {
		t.Errorf("edit outside solo = %d %v; want the terms kept", rec.Code, body)
	}
}

// Quem tem projetos não troca de modo (409 customer.mode_locked), e quem tem recebíveis não é excluído.
func TestCustomerBilling_LockedModeAndReceivables(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	makeSolo(t, owner.orgID, true)

	prj := createProject(t, e, owner, "Projeto")
	customerID := setBilling(t, e, owner, "/api/projects/"+prj, 15000)

	rec := do(e, "PATCH", "/api/customers/"+customerID, `{`+monthlyBody+`}`, owner.session)
	if code, params := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusConflict || code != "customer.mode_locked" || params["field"] != "billing_mode" {
		t.Errorf("switch with projects = %d %s, want 409 customer.mode_locked on billing_mode", rec.Code, rec.Body.String())
	}
	if got := decode(t, do(e, "GET", "/api/customers/"+customerID, "", owner.session)); got["billing_mode"] != "hourly" || got["monthly_fee_cents"] != nil {
		t.Errorf("a locked switch changed the customer: %v", got)
	}
	// O mesmo modo e os outros termos seguem editáveis.
	rec = do(e, "PATCH", "/api/customers/"+customerID, `{"billing_mode":"hourly","default_rate_cents":18000,"payment_days":7}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["default_rate_cents"] != float64(18000) || body["payment_days"] != float64(7) {
		t.Errorf("edit of a locked customer = %d %v", rec.Code, body)
	}

	// Sem projetos, com um recebível: a exclusão é conflito.
	rec = do(e, "POST", "/api/orgs/"+owner.orgID+"/customers", `{"name":"Com recebível",`+monthlyBody+`}`, owner.session)
	withReceivable := decode(t, rec)["id"].(string)
	receivable := testClient.Receivable.Create().
		SetOrganizationID(uuid.MustParse(owner.orgID)).SetCustomerID(uuid.MustParse(withReceivable)).SetKind("monthly").
		SetPeriodStart("2026-11-01").SetPeriodEnd("2026-11-30").SetSeconds(0).
		SetCalculatedCents(630000).SetAmountCents(630000).SetCurrency("BRL").SetDueDate("2026-12-05").
		SaveX(context.Background())
	rec = do(e, "DELETE", "/api/customers/"+withReceivable, "", owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusConflict || code != "customer.has_receivables" {
		t.Errorf("delete with receivables = %d %s, want 409 customer.has_receivables", rec.Code, rec.Body.String())
	}
	testClient.Receivable.DeleteOneID(receivable.ID).ExecX(context.Background())
	if rec := do(e, "DELETE", "/api/customers/"+withReceivable, "", owner.session); rec.Code != http.StatusNoContent {
		t.Errorf("delete after undoing the receivable = %d %s", rec.Code, rec.Body.String())
	}
}

// O valor cobrado do projeto segue o modo do cliente: na criação, em PUT billing e ao trocar o cliente.
func TestCustomerBilling_ProjectRateFollowsTheMode(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	makeSolo(t, owner.orgID, true)
	orgID := owner.orgID

	monthly := decode(t, do(e, "POST", "/api/orgs/"+orgID+"/customers", `{"name":"Mensal",`+monthlyBody+`}`, owner.session))["id"].(string)
	hourly := decode(t, do(e, "POST", "/api/orgs/"+orgID+"/customers", `{"name":"Hora","default_rate_cents":15000}`, owner.session))["id"].(string)

	// Criação.
	rec := do(e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"P","customer_id":"`+monthly+`","bill_rate_cents":15000}`, owner.session)
	if code, params := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_not_allowed" || params["field"] != "bill_rate_cents" {
		t.Errorf("create with a rate on a monthly customer = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"P","customer_id":"`+monthly+`"}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create without a rate on a monthly customer = %d: %s", rec.Code, rec.Body.String())
	}
	monthlyProject := decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"P","customer_id":"`+hourly+`"}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_required" {
		t.Errorf("create without a rate on an hourly customer = %d %s", rec.Code, rec.Body.String())
	}

	// PUT billing: o cliente mensal não aceita valor, e o resultado diz o modo.
	billing := "/api/projects/" + monthlyProject + "/billing"
	if got := decode(t, do(e, "GET", billing, "", owner.session)); got["bill_rate_cents"] != nil || got["customer_billing_mode"] != "monthly" {
		t.Errorf("monthly project billing = %v", got)
	}
	rec = do(e, "PUT", billing, `{"customer_id":"`+monthly+`","bill_rate_cents":15000}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_not_allowed" {
		t.Errorf("PUT billing with a rate on a monthly customer = %d %s", rec.Code, rec.Body.String())
	}

	// Trocar o cliente do projeto revalida contra o modo do novo: do mensal para o por hora o valor passa a ser
	// obrigatório, e do por hora para o mensal o valor antigo não vai junto.
	rec = do(e, "PUT", billing, `{"customer_id":"`+hourly+`"}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_required" {
		t.Errorf("monthly to hourly without a rate = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "PUT", billing, `{"customer_id":"`+hourly+`","bill_rate_cents":15000}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["bill_rate_cents"] != float64(15000) || body["customer_billing_mode"] != "hourly" {
		t.Errorf("monthly to hourly with a rate = %d %v", rec.Code, body)
	}
	rec = do(e, "PUT", billing, `{"customer_id":"`+monthly+`","bill_rate_cents":15000}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_not_allowed" {
		t.Errorf("hourly to monthly carrying the rate = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "PUT", billing, `{"customer_id":"`+monthly+`","bill_rate_cents":null}`, owner.session)
	if body := decode(t, rec); rec.Code != http.StatusOK || body["bill_rate_cents"] != nil || body["customer_billing_mode"] != "monthly" {
		t.Errorf("hourly to monthly = %d %v", rec.Code, body)
	}

	// O projeto interno não mudou: um valor sem cliente continua recusado.
	rec = do(e, "PUT", billing, `{"bill_rate_cents":15000}`, owner.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.bill_rate_needs_customer" {
		t.Errorf("rate without a customer = %d %s", rec.Code, rec.Body.String())
	}
}

// O ponto no projeto de um cliente mensal: clock-in e clock-out pela API guardam o valor cobrado nulo, e as horas contam.
func TestCustomerBilling_ClockInOnAMonthlyCustomersProject(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	makeSolo(t, owner.orgID, true)

	customerID := decode(t, do(e, "POST", "/api/orgs/"+owner.orgID+"/customers", `{"name":"Mensal",`+monthlyBody+`}`, owner.session))["id"].(string)
	rec := do(e, "POST", "/api/orgs/"+owner.orgID+"/projects", `{"name":"Contrato","customer_id":"`+customerID+`"}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d: %s", rec.Code, rec.Body.String())
	}
	prj := decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Tarefa"}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	taskID := decode(t, rec)["id"].(string)

	rec = do(e, "POST", "/api/projects/"+prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("clock-in = %d: %s", rec.Code, rec.Body.String())
	}
	if opened := decode(t, rec); opened["bill_rate_cents"] != nil || opened["bill_amount_cents"] != nil || opened["owner_hours"] != true {
		t.Errorf("opened session = %v; want the owner's, with no billed rate or amount", opened)
	}
	rec = do(e, "POST", "/api/projects/"+prj+"/work-sessions/clock-out", `{}`, owner.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("clock-out = %d: %s", rec.Code, rec.Body.String())
	}
	if closed := decode(t, rec); closed["end_at"] == nil || closed["bill_rate_cents"] != nil || closed["bill_amount_cents"] != nil {
		t.Errorf("closed session = %v; want an end and no billed rate or amount", closed)
	}
}

// A tela de Clientes tem os termos só no modo solo, e o formulário de projeto sabe o modo do cliente.
func TestCustomerBilling_PagesShowTheTermsOnlyInSolo(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	makeSolo(t, owner.orgID, true)

	page := do(e, "GET", "/orgs/"+owner.orgID+"/customers", "", owner.session)
	if page.Code != http.StatusOK {
		t.Fatalf("customers page = %d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		`id="customer-billing-mode"`, `data-field="billing_mode"`, `data-field="default_rate_cents"`, `data-field="monthly_fee_cents"`,
		`data-field="contract_start"`, `data-field="contract_end"`, `data-field="closing_day"`, `data-field="payment_days"`,
		`data-field="late_fee_bp"`, `data-field="late_interest_bp_month"`, `data-field="grace_days"`, `<details class="form-fold"`,
		"Cobrança", "Mensal", "Por hora", "Multa, juros e tolerância",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the customers page of a solo organization lacks %s", want)
		}
	}
	// Os dois modos mostram o fechamento e o prazo; o bloco recolhido é só do mensal.
	if !strings.Contains(body, `x-show="draft.billing_mode === 'monthly'"`) {
		t.Error("the monthly fields are not tied to the monthly mode")
	}
	// O identity diz que a organização é solo, para o JavaScript.
	if me := decode(t, do(e, "GET", "/api/auth/me", "", owner.session)); me["organization_solo"] != true {
		t.Errorf("/api/auth/me organization_solo = %v, want true", me["organization_solo"])
	}

	projects := do(e, "GET", "/orgs/"+owner.orgID+"/projects", "", owner.session).Body.String()
	for _, want := range []string{`@change="customerChanged()"`, `x-show="hasRate()"`, `x-show="monthlyCustomer()"`} {
		if !strings.Contains(projects, want) {
			t.Errorf("the new project form lacks %s", want)
		}
	}
}
