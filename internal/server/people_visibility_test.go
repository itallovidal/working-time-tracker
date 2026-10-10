package server_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// Quem não é admin, não cuida de pessoas e não pode pôr gente em projeto só vê a si mesmo e quem está em algum
// projeto dele, na lista da organização e pelo id; os outros dão 404, como uma pessoa que não existe. O dono, os
// admins e quem tem collaborators.manage em algum projeto (o administrador de projeto) veem todas (precisam achar
// quem entra).
func TestPeople_MemberSeesOnlyColleagues(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member")
	dora := invite(t, e, admin, "dora@test.com", "member")
	eli := invite(t, e, admin, "eli@test.com", "member")   // não está em projeto nenhum
	gabi := invite(t, e, admin, "gabi@test.com", "member") // gerente do Alfa
	pam := invite(t, e, admin, "pam@test.com", "admin")    // admin que não é o dono
	other := signup(t, e, "Outra", "zed@outra.com")
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	beta := createEmptyProject(t, e, admin, "Projeto Beta")
	allocate(t, e, admin, alfa, bia.id, 2000)
	allocate(t, e, admin, alfa, caio.id, 2000)
	allocate(t, e, admin, beta, dora.id, 2000)
	withPreset(t, e, admin, alfa, gabi.id, "manager")

	list := "/api/orgs/" + admin.orgID + "/persons"
	ids := func(session string) []string {
		t.Helper()
		rec := do(e, "GET", list, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET persons = %d: %s", rec.Code, rec.Body.String())
		}
		var out []string
		for _, p := range decodeList(t, rec) {
			out = append(out, p["id"].(string))
		}
		slices.Sort(out)
		return out
	}
	sorted := func(accounts ...account) []string {
		var out []string
		for _, a := range accounts {
			out = append(out, a.id)
		}
		slices.Sort(out)
		return out
	}
	everyone := sorted(admin, bia, caio, dora, eli, gabi, pam)

	// A lista: o membro vê quem divide projeto com ele (e o gerente do projeto, que também está nele), e a si mesmo.
	for who, c := range map[string]struct {
		session string
		want    []string
	}{
		"Bia (member of the Alfa)":    {bia.session, sorted(bia, caio, gabi)},
		"Caio (member of the Alfa)":   {caio.session, sorted(bia, caio, gabi)},
		"Dora (alone in the Beta)":    {dora.session, sorted(dora)},
		"Eli (in no project)":         {eli.session, sorted(eli)},
		"admin":                       {admin.session, everyone},
		"Pam (admin)":                 {pam.session, everyone},
		"Gabi (collaborators.manage)": {gabi.session, everyone},
	} {
		if got := ids(c.session); !slices.Equal(got, c.want) {
			t.Errorf("%s lists %d people, want %d", who, len(got), len(c.want))
		}
	}
	// O que não é dele não aparece na resposta, nem o e-mail.
	if body := do(e, "GET", list, "", bia.session).Body.String(); strings.Contains(body, "dora@test.com") || strings.Contains(body, "eli@test.com") {
		t.Errorf("Bia's list leaks people who share no project with her: %s", body)
	}

	// Pelo id: quem é visível abre; os outros são 404.
	person := func(session string, who account) int {
		return status(e, "GET", "/api/persons/"+who.id, "", session)
	}
	for _, c := range []struct {
		viewer  string
		session string
		target  account
		name    string
		want    int
	}{
		{"Bia", bia.session, bia, "herself", http.StatusOK},
		{"Bia", bia.session, caio, "Caio (same project)", http.StatusOK},
		{"Bia", bia.session, dora, "Dora (another project)", http.StatusNotFound},
		{"Bia", bia.session, eli, "Eli (no project)", http.StatusNotFound},
		{"Bia", bia.session, admin, "the owner", http.StatusNotFound},
		{"Dora", dora.session, bia, "Bia", http.StatusNotFound},
		{"Eli", eli.session, eli, "herself", http.StatusOK},
		{"Gabi", gabi.session, dora, "Dora", http.StatusOK},
		{"Pam", pam.session, eli, "Eli", http.StatusOK},
		{"admin", admin.session, eli, "Eli", http.StatusOK},
	} {
		if got := person(c.session, c.target); got != c.want {
			t.Errorf("%s GET /persons/%s = %d, want %d", c.viewer, c.name, got, c.want)
		}
	}
	// Outra organização continua sendo 404, e sem sessão, 401.
	if got := status(e, "GET", list, "", other.session); got != http.StatusNotFound {
		t.Errorf("another org GET persons = %d, want 404", got)
	}
	if got := status(e, "GET", list, "", ""); got != http.StatusUnauthorized {
		t.Errorf("GET persons without session = %d, want 401", got)
	}
	// O próprio perfil e a página dela seguem abrindo para quem não vê mais ninguém.
	if got := status(e, "GET", "/profile", "", eli.session); got != http.StatusOK {
		t.Errorf("Eli's profile page = %d, want 200", got)
	}
}

// A regra de pagamento de uma pessoa é dinheiro: só vai para ela mesma e para o dono. Um colega de projeto e um admin
// que não é o dono recebem payment: null, na lista e pelo id.
func TestPeople_PaymentRuleIsHiddenFromColleagues(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member")
	pam := invite(t, e, admin, "pam@test.com", "admin")
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	allocate(t, e, admin, alfa, bia.id, 2000)
	allocate(t, e, admin, alfa, caio.id, 2000)
	if rec := do(e, "PATCH", "/api/persons/"+bia.id+"/payment", `{"frequency":"monthly","day":5}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set rule = %d", rec.Code)
	}
	for _, c := range []struct {
		who     string
		session string
		visible bool
	}{{"bia", bia.session, true}, {"admin", admin.session, true}, {"admin who is not the owner", pam.session, false}, {"colleague", caio.session, false}} {
		byID := do(e, "GET", "/api/persons/"+bia.id, "", c.session)
		if byID.Code != http.StatusOK {
			t.Fatalf("%s: GET person = %d", c.who, byID.Code)
		}
		got := []any{decode(t, byID)["payment"]}
		for _, p := range decodeList(t, do(e, "GET", "/api/orgs/"+admin.orgID+"/persons", "", c.session)) {
			if p["id"] == bia.id {
				got = append(got, p["payment"])
			}
		}
		for _, g := range got {
			if (g != nil) != c.visible {
				t.Errorf("%s sees payment %v, visible want %v", c.who, g, c.visible)
			}
		}
	}
}
