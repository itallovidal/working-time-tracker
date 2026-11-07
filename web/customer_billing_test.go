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
	if len(sent) != 10 {
		t.Errorf("billingBody sends %d keys, want the 10 terms: %v", len(sent), sent)
	}
	for _, m := range sent {
		if !known[m[1]] {
			t.Errorf("billingBody sends %q, which customer.Input does not accept (the API refuses unknown keys)", m[1])
		}
	}

	// Cada termo tem um campo na tela, com o erro embaixo dele: o modo e o valor por hora no modal de cliente, e o resto
	// no parcial customer_contract, que o modal inclui.
	all := templates(t)
	tpl := all["templates/pages/org_customers.gohtml"] + all["templates/partials/customer_contract.gohtml"]
	for _, m := range sent {
		for _, want := range []string{`data-field="` + m[1] + `"`, `{{template "field_error" "` + m[1] + `"}}`} {
			if !strings.Contains(tpl, want) {
				t.Errorf("org_customers.gohtml + customer_contract.gohtml lack %s", want)
			}
		}
	}
}

// O modo, o contrato e os prazos só existem no modo solo: ficam dentro do {{if .Me.OrganizationSolo}}. O valor por hora
// fica fora, porque todo cliente por hora tem um, na equipe também (Decisão 21).
func TestCustomerBilling_FieldsAreInsideTheSoloGate(t *testing.T) {
	tpl := templates(t)["templates/pages/org_customers.gohtml"]
	fieldset := strings.Index(tpl, `<legend>{{.T "customer.billing.title"}}</legend>`)
	if fieldset < 0 {
		t.Fatal("org_customers.gohtml has no billing fieldset")
	}
	gate := strings.Index(tpl[fieldset:], "{{if .Me.OrganizationSolo}}")
	if gate < 0 {
		t.Fatal("the billing mode is not behind .Me.OrganizationSolo")
	}
	gate += fieldset
	mode := strings.Index(tpl, `id="customer-billing-mode"`)
	if mode < gate {
		t.Error("customer-billing-mode is outside the solo gate")
	}
	rate := strings.Index(tpl, `id="customer-hourly-rate"`)
	if rate < 0 {
		t.Error("org_customers.gohtml has no field customer-hourly-rate")
	}
	// O valor por hora vem depois do fim do bloco do modo, fora do modo solo.
	if end := strings.Index(tpl[mode:], "{{end}}"); end < 0 || rate < mode+end {
		t.Error("customer-hourly-rate is inside the solo gate, but every hourly customer has a rate")
	}
	// O contrato vem do parcial, num bloco do modo solo depois do valor por hora.
	include := strings.Index(tpl, `{{template "customer_contract" (dict "Data" . "Model" "draft" "Id" "customer")}}`)
	if include < 0 {
		t.Error("org_customers.gohtml does not include customer_contract for the draft")
	} else if gated := strings.LastIndex(tpl[:include], "{{if .Me.OrganizationSolo}}"); gated < rate {
		t.Error("customer_contract is included outside a solo gate")
	}
	contract := templates(t)["templates/partials/customer_contract.gohtml"]
	for _, id := range []string{"monthly-fee", "contract-start", "contract-end", "closing-day", "payment-days", "late-fee", "late-interest", "grace-days"} {
		if !strings.Contains(contract, `id="{{$id}}-`+id+`"`) {
			t.Errorf("customer_contract.gohtml has no field {{$id}}-%s", id)
		}
	}
	// A coluna Cobrança aparece nos dois modos, e os prazos dela só no solo.
	if !strings.Contains(tpl, `<th>{{.T "customer.billing.column"}}</th>`) {
		t.Error("the customers list has no Cobrança column")
	}
	if !strings.Contains(tpl, `{{if .Me.OrganizationSolo}}<div class="muted small" x-text="billingTerms(c)"></div>{{end}}`) {
		t.Error("the closing and due terms of the Cobrança column are not behind .Me.OrganizationSolo")
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
