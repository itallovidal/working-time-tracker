package page

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/i18n"
	tmpl "working-time-tracker/internal/template"
	"working-time-tracker/web"
)

// O modo solo nas páginas: o que some do menu e das abas, e o modal de pessoa sem jornada nem regra de pagamento.

func identity(role string, owner, solo bool) *auth.Identity {
	return &auth.Identity{
		PersonID: uuid.New(), Name: "Ana Souza", Role: role, IsOwner: owner,
		Permissions:      permission.OrgKeysFor(role, owner),
		OrganizationID:   uuid.New(),
		OrganizationName: "Acme", OrganizationCurrency: "BRL", OrganizationSolo: solo,
	}
}

// render monta uma página inteira como o servidor faz, com o catálogo de verdade, para ler o HTML que sai.
func render(t *testing.T, name string, d Data) string {
	t.Helper()
	cat, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	r, err := tmpl.New(web.FS)
	if err != nil {
		t.Fatal(err)
	}
	d.cat, d.Lang = cat, i18n.PTBR
	d.Access = auth.BaseProjectSet(d.Me)
	var out strings.Builder
	if err := r.Render(nil, &out, name, d); err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return out.String()
}

// O modal Editar colaborador abre na lista de colaboradores e no perfil de um deles, páginas que o modo solo esconde
// (404); mesmo assim ele não oferece jornada semanal nem regra de pagamento a uma organização solo, e só a aba dos
// projetos fica. Fora do modo solo continua igual: o dono vê a jornada e a regra, o admin só a jornada.
func TestPersonEditModal_SoloHasNoWeeklyHoursNorPaymentRule(t *testing.T) {
	person := &Crumb{ID: uuid.NewString(), Name: "Bia"}
	page := func(me *auth.Identity) string {
		return render(t, "org_person", Data{Me: me, Person: person, Org: &organization.Organization{Solo: me.OrganizationSolo}, Section: "people", Script: "org"})
	}
	const weekly, rule, tabPayment, projects = `id="person-weekly-hours"`, `id="person-payment-frequency"`, `id="person-tab-payment"`, `id="person-panel-projects"`

	solo := page(identity("admin", true, true))
	for _, gone := range []string{weekly, rule, tabPayment} {
		if strings.Contains(solo, gone) {
			t.Errorf("the solo person modal still has %s", gone)
		}
	}
	if !strings.Contains(solo, projects) {
		t.Errorf("the solo person modal lost the projects panel")
	}

	owner := page(identity("admin", true, false))
	for _, want := range []string{weekly, rule, tabPayment, projects} {
		if !strings.Contains(owner, want) {
			t.Errorf("the team owner's person modal lost %s", want)
		}
	}
	admin := page(identity("admin", false, false))
	if !strings.Contains(admin, weekly) || strings.Contains(admin, rule) {
		t.Errorf("the team admin's person modal must keep the weekly hours and still hide the payment rule")
	}
}

// A aba Colaboradores do projeto (pessoas e times) não existe no modo solo, e a barra de abas não a oferece, na Gestão
// nem fora dela.
func TestOpensTab_TeamsIsHiddenInSolo(t *testing.T) {
	all := permission.Set{All: true}
	solo := Data{Me: identity("admin", true, true), Access: all}
	team := Data{Me: identity("admin", true, false), Access: all}
	if solo.OpensTab("teams") {
		t.Error("a solo organization must not open the teams tab")
	}
	for _, id := range []string{"overview", "integrations", "settings"} {
		if !solo.OpensTab(id) {
			t.Errorf("a solo organization must still open the %s tab", id)
		}
	}
	if !team.OpensTab("teams") {
		t.Error("a team organization must keep the teams tab")
	}
}

// Uma chave de permissão que o catálogo não conhece dá falso para quem não é o dono, sem quebrar: é o caso do link A
// receber (receivables.manage) até a permissão existir. O dono passa pela regra do projeto (que dá tudo a ele), e uma
// vez que a chave entra em permission.OrganizationKeys, pela do cargo; nos dois casos o dono a tem e os outros não.
func TestCan_UnknownKeyIsFalseForEveryoneButTheOwner(t *testing.T) {
	const key = "receivables.manage"
	for _, c := range []struct {
		who   string
		me    *auth.Identity
		allow bool
	}{
		{"owner", identity("admin", true, true), true},
		{"admin", identity("admin", false, false), false},
		{"member", identity("member", false, false), false},
	} {
		d := Data{Me: c.me, Access: auth.BaseProjectSet(c.me)}
		if got := d.Can(key); got != c.allow {
			t.Errorf("%s: Can(%q) = %v, want %v", c.who, key, got, c.allow)
		}
	}
}
