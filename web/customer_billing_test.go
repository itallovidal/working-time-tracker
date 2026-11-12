package web

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/validate"
)

// Os termos comerciais do cliente (modo solo) são escritos nos dois lados, a tela e a API, e o corpo da API é estrito
// (uma chave que ela não conhece é recusada). Estes testes travam o que o navegador não avisa: a tela mandar só as
// chaves que o servidor conhece, cada campo ter o erro embaixo dele, e os limites que o JavaScript lê existirem.

func staticFile(t *testing.T, name string) string {
	t.Helper()
	b, err := FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// inputKeys são as chaves JSON que a API do cliente aceita.
func inputKeys() map[string]bool {
	keys := map[string]bool{}
	typ := reflect.TypeOf(customer.Input{})
	for i := 0; i < typ.NumField(); i++ {
		keys[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	return keys
}

func TestCustomerBilling_ScreenSendsOnlyKnownKeys(t *testing.T) {
	js := staticFile(t, "static/pages/org.js")
	start := strings.Index(js, "const billingBody = (d) => {")
	if start < 0 {
		t.Fatal("org.js has no billingBody")
	}
	end := strings.Index(js[start:], "\n  };")
	if end < 0 {
		t.Fatal("billingBody has no end")
	}
	body := js[start : start+end]
	known := inputKeys()
	sent := regexp.MustCompile(`(?m)^\s+(\w+):`).FindAllStringSubmatch(body, -1)
	if len(sent) != 13 {
		t.Errorf("billingBody sends %d keys, want the 13 terms: %v", len(sent), sent)
	}
	for _, m := range sent {
		if !known[m[1]] {
			t.Errorf("billingBody sends %q, which customer.Input does not accept (the API refuses unknown keys)", m[1])
		}
	}

	// Cada termo tem um campo na tela, com o erro embaixo dele: o modo e o valor por hora no formulário de cliente (o
	// parcial customer_fields, do modal e das boas-vindas), e o resto no parcial customer_contract, que ele inclui.
	all := templates(t)
	tpl := all["templates/partials/customer_form.gohtml"] + all["templates/partials/customer_contract.gohtml"]
	for _, m := range sent {
		for _, want := range []string{`data-field="` + m[1] + `"`, `{{template "field_error" "` + m[1] + `"}}`} {
			if !strings.Contains(tpl, want) {
				t.Errorf("customer_form.gohtml + customer_contract.gohtml lack %s", want)
			}
		}
	}
}

// Toda tela que cria cliente manda o valor por hora, que é obrigatório no cliente por hora (Decisão 21). A etapa do
// primeiro cliente das boas-vindas não mandava, e o cliente não salvava: este teste lê cada POST de cliente do org.js e
// confere que a função dele monta hourly_rate_cents (direto, pelo billingBody ou pelo checkCustomer do formulário de
// cliente, cujo customerRequest monta os dois).
func TestCustomerCreation_EveryScreenSendsTheHourlyRate(t *testing.T) {
	js := staticFile(t, "static/pages/org.js")
	post := "api('POST', '/api/orgs/' + orgId + '/customers'"
	method := regexp.MustCompile(`(?m)^    \w+\([^)]*\) \{$`)
	found := 0
	for at := strings.Index(js, post); at >= 0; at = next(js, post, at) {
		found++
		starts := method.FindAllStringIndex(js[:at], -1)
		if len(starts) == 0 {
			t.Fatalf("the customer POST at %d is not inside a method", at)
		}
		fn := js[starts[len(starts)-1][0]:at]
		if !strings.Contains(fn, "hourly_rate_cents") && !strings.Contains(fn, "billingBody(") && !strings.Contains(fn, "this.checkCustomer(") {
			t.Errorf("%q creates a customer without hourly_rate_cents", strings.TrimSpace(strings.SplitN(fn, "\n", 2)[0]))
		}
	}
	if found < 2 {
		t.Errorf("found %d customer POSTs in org.js, want the customers modal and the welcome step", found)
	}
	request := jsBlock(t, js, "const customerRequest = (d) => {")
	for _, want := range []string{"billingBody(d)", "body.hourly_rate_cents = "} {
		if !strings.Contains(request, want) {
			t.Errorf("customerRequest lacks %s", want)
		}
	}
	if check := jsBlock(t, js, "    checkCustomer(d) {"); !strings.Contains(check, "customerRequest(d)") {
		t.Error("checkCustomer does not build the body with customerRequest")
	}
}

// jsBlock é o trecho do org.js que começa em start e vai até o fecho na mesma indentação.
func jsBlock(t *testing.T, js, start string) string {
	t.Helper()
	at := strings.Index(js, start)
	if at < 0 {
		t.Fatalf("org.js has no %q", start)
	}
	indent := start[:len(start)-len(strings.TrimLeft(start, " "))]
	end := strings.Index(js[at:], "\n"+indent+"}")
	if end < 0 {
		t.Fatalf("%q has no end", start)
	}
	return js[at : at+end]
}

// O formulário de cliente tem duas etapas, os dados e a cobrança, e um erro leva à etapa do campo (customerStepOf): toda
// chave que a tela manda para a API tem a etapa dela, e o campo está no bloco dessa etapa do parcial customer_fields
// (ou no customer_contract, que fica na etapa da cobrança).
func TestCustomerForm_EveryFieldHasItsStep(t *testing.T) {
	js := staticFile(t, "static/pages/org.js")
	keys := regexp.MustCompile(`(?m)^\s+(\w+):`)
	var sent []string
	for _, start := range []string{"const customerBody = (c) => ({", "const billingBody = (d) => {"} {
		for _, m := range keys.FindAllStringSubmatch(jsBlock(t, js, "  "+start), -1) {
			sent = append(sent, m[1])
		}
	}
	steps := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+): ([12])`).FindAllStringSubmatch(jsBlock(t, js, "  const customerStepOf = {"), -1) {
		steps[m[1]] = m[2]
	}
	all := templates(t)
	form := all["templates/partials/customer_form.gohtml"]
	data := strings.Index(form, `data-customer-step="1"`)
	billing := strings.Index(form, `data-customer-step="2"`)
	if data < 0 || billing < data {
		t.Fatal("customer_form.gohtml lacks the two step blocks, data before billing")
	}
	for _, key := range sent {
		step, ok := steps[key]
		if !ok {
			t.Errorf("customerStepOf has no step for %q: a server error on it would stay on a hidden field", key)
			continue
		}
		at := strings.Index(form, `data-field="`+key+`"`)
		switch {
		case at < 0 && step == "1" && strings.Contains(all["templates/partials/customer_document.gohtml"], `data-field="`+key+`"`):
			// No customer_document, que o parcial inclui no bloco dos dados.
		case at < 0 && step == "2" && strings.Contains(all["templates/partials/customer_contract.gohtml"], `data-field="`+key+`"`):
			// No customer_contract, que o parcial inclui no bloco da cobrança.
		case at < 0:
			t.Errorf("customer_form.gohtml has no field %q", key)
		case step == "1" && (at < data || at > billing):
			t.Errorf("%q is in step 1 of customerStepOf, but not in the data block", key)
		case step == "2" && at < billing:
			t.Errorf("%q is in step 2 of customerStepOf, but not in the billing block", key)
		}
	}
	if include := strings.Index(form, `{{template "customer_document"`); include < data || include > billing {
		t.Error("customer_document is not in the data block")
	}
	if include := strings.Index(form, `{{template "customer_contract"`); include < billing {
		t.Error("customer_contract is not in the billing block")
	}
}

// next é a próxima ocorrência de sub em s depois de at, ou -1.
func next(s, sub string, at int) int {
	i := strings.Index(s[at+1:], sub)
	if i < 0 {
		return -1
	}
	return at + 1 + i
}

// O modo, o contrato e os prazos só existem no modo solo: ficam dentro do {{if .Me.OrganizationSolo}}. O valor por hora
// fica fora, porque todo cliente por hora tem um, na equipe também (Decisão 21).
func TestCustomerBilling_FieldsAreInsideTheSoloGate(t *testing.T) {
	form := templates(t)["templates/partials/customer_form.gohtml"]
	step := strings.Index(form, `data-customer-step="2"`)
	if step < 0 {
		t.Fatal("customer_form.gohtml has no billing step")
	}
	gate := strings.Index(form[step:], "{{if .Me.OrganizationSolo}}")
	if gate < 0 {
		t.Fatal("the billing mode is not behind .Me.OrganizationSolo")
	}
	gate += step
	mode := strings.Index(form, `id="{{$id}}-billing-mode"`)
	if mode < gate {
		t.Error("the billing mode is outside the solo gate")
	}
	rate := strings.Index(form, `id="{{$id}}-hourly-rate"`)
	if rate < 0 {
		t.Error("customer_form.gohtml has no hourly rate field")
	}
	// O valor por hora vem depois do fim do bloco do modo, fora do modo solo.
	if end := strings.Index(form[mode:], "{{end}}"); end < 0 || rate < mode+end {
		t.Error("the hourly rate is inside the solo gate, but every hourly customer has a rate")
	}
	// O contrato vem do parcial, num bloco do modo solo depois do valor por hora.
	include := strings.Index(form, `{{template "customer_contract" (dict "Data" . "Model" $m "Id" $id)}}`)
	if include < 0 {
		t.Error("customer_form.gohtml does not include customer_contract")
	} else if gated := strings.LastIndex(form[:include], "{{if .Me.OrganizationSolo}}"); gated < rate {
		t.Error("customer_contract is included outside a solo gate")
	}
	// As duas telas de cliente usam o parcial: nenhuma escreve os campos à mão.
	tpl := templates(t)["templates/pages/org_projects.gohtml"]
	for path, want := range map[string]string{
		"templates/pages/org_projects.gohtml":  `{{template "customer_fields" (dict "Data" . "Model" "customerDraft" "Id" "customer")}}`,
		"templates/partials/onboarding.gohtml": `{{template "customer_fields" (dict "Data" . "Model" "customer" "Id" "ob-customer" "Step" "2")}}`,
	} {
		src := templates(t)[path]
		if !strings.Contains(src, want) {
			t.Errorf("%s does not include customer_fields", path)
		}
		if strings.Contains(src, `data-field="hourly_rate_cents"`) || strings.Contains(src, `data-field="contact_email"`) {
			t.Errorf("%s writes customer fields by hand instead of the customer_fields partial", path)
		}
	}
	contract := templates(t)["templates/partials/customer_contract.gohtml"]
	for _, id := range []string{"monthly-fee", "estimated-hours", "contract-start", "contract-end", "billing-cycle", "cycle-anchor", "closing-day", "payment-days", "late-fee", "late-interest", "grace-days"} {
		if !strings.Contains(contract, `id="{{$id}}-`+id+`"`) {
			t.Errorf("customer_contract.gohtml has no field {{$id}}-%s", id)
		}
	}
	// A cobrança (o valor e, no solo, o modo) fica só nos detalhes do cliente: o cabeçalho do grupo não a mostra.
	for _, gone := range []string{`class="catalog-billing"`, `billingAmount(`, `billingMode(g.customer)`} {
		if strings.Contains(tpl, gone) {
			t.Errorf("the customer header still shows the billing (%s)", gone)
		}
	}
}

// O cabeçalho do cliente, em Clientes e Projetos, abre os detalhes dele, e Editar e Excluir moram neles, não na lista.
func TestCustomers_RowOpensTheDetails(t *testing.T) {
	tpl := templates(t)["templates/pages/org_projects.gohtml"]
	details := strings.Index(tpl, `x-show="$store.modal.name === 'customer-details'"`)
	if details < 0 {
		t.Fatal("org_projects.gohtml has no customer-details modal")
	}
	for _, want := range []string{`'row-link'`, `openDetails(g.customer)`, `class="row-open"`} {
		if at := strings.Index(tpl, want); at < 0 || at > details {
			t.Errorf("the customer header lacks %s", want)
		}
	}
	for _, want := range []string{`@click="openForm(viewing())"`, `@click="remove()"`, `x-text="errors.remove"`, `customerFacts(viewing())`} {
		if at := strings.Index(tpl, want); at < details {
			t.Errorf("%s is not in the customer-details modal", want)
		}
	}
	if strings.Contains(tpl, `openForm(c)`) || strings.Contains(tpl, `openForm(g.customer)`) {
		t.Error("the customer header still opens the edit form directly")
	}
	// Cancelar a edição volta para os detalhes.
	if !strings.Contains(tpl, `@click="cancelForm()"`) {
		t.Error("the customer form does not cancel through cancelForm")
	}
}

// Todo limite que a tela lê de WTT.limits existe no servidor (window.BOOT.limits).
func TestCustomerBilling_LimitsTheScreenReadsExist(t *testing.T) {
	limits := validate.Limits()
	var missing []string
	for _, file := range []string{"static/pages/org.js", "static/pages/project.js"} {
		for _, m := range regexp.MustCompile(`WTT\.limits\.(\w+)`).FindAllStringSubmatch(staticFile(t, file), -1) {
			if _, ok := limits[m[1]]; !ok {
				missing = append(missing, file+": "+m[1])
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("limits the screen reads that validate.Limits() does not have: %v", missing)
	}
	// O servidor confere pelos mesmos tetos que a tela lê.
	if limits["term_days"] != validate.MaxTermDays || limits["rate_bp"] != validate.MaxRateBP {
		t.Errorf("limits term_days/rate_bp = %d/%d, want %d/%d", limits["term_days"], limits["rate_bp"], validate.MaxTermDays, validate.MaxRateBP)
	}
}
