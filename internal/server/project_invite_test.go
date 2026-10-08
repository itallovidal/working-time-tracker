package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/projectinvite"
	"working-time-tracker/internal/domain/team"
)

// Convidar por um projeto: o convite guarda o valor, o grupo e o time, e quando a pessoa aceita ela já está no
// projeto com tudo isso. Antes de aceitar, o convite aparece na lista do projeto e na da organização.
func TestProjectInvite_JoinsTheProjectWhenAccepted(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Time A"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)

	rec = do(e, "POST", "/api/projects/"+projectID+"/invites",
		`{"email":"Leo@Test.com","pay_rate_cents":25000,"team_id":"`+teamID+`","preset":"manager"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project invite = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	project, _ := body["project"].(map[string]any)
	if body["email"] != "leo@test.com" || body["role"] != "member" || body["delivery"] != "link" || body["token"] == "" ||
		project["project_id"] != projectID || project["pay_rate_cents"] != float64(25000) || project["team_id"] != teamID || project["preset"] != "manager" {
		t.Fatalf("invite = %v, want leo as a member, for the project with 250,00, the team and the manager preset", body)
	}
	token := body["token"].(string)

	// Antes do aceite: nas duas listas, com o nome do projeto e do time.
	list := decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/invites", "", admin.session))
	if len(list) != 1 {
		t.Fatalf("project invites = %v, want one", list)
	}
	if p, _ := list[0]["project"].(map[string]any); p["project_name"] != "Projeto Alfa" || p["team_name"] != "Time A" || p["pay_rate_cents"] != float64(25000) {
		t.Errorf("project invite = %v, want the names and the rate", list[0])
	}
	orgList := decodeList(t, do(e, "GET", "/api/orgs/"+admin.orgID+"/invites", "", admin.session))
	if p, _ := orgList[0]["project"].(map[string]any); len(orgList) != 1 || p["project_name"] != "Projeto Alfa" {
		t.Errorf("organization invites = %v, want the one with the project name", orgList)
	}

	// A pessoa aceita com e-mail e senha: entra na organização e, com ela, no projeto.
	rec = do(e, "POST", "/api/auth/invites/"+token+"/accept", `{"name":"Leo","email":"leo@test.com","password":"senha-forte-2"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("accept = %d: %s", rec.Code, rec.Body.String())
	}
	personID := decode(t, rec)["id"].(string)

	var found map[string]any
	for _, a := range decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/allocations", "", admin.session)) {
		if a["person_id"] == personID {
			found = a
		}
	}
	if found == nil || found["pay_rate_cents"] != float64(25000) || found["preset"] != "manager" {
		t.Fatalf("allocation of the new person = %v, want 250,00 and the manager preset", found)
	}
	inTeam := false
	for _, m := range decodeList(t, do(e, "GET", "/api/teams/"+teamID+"/members", "", admin.session)) {
		inTeam = inTeam || m["person_id"] == personID
	}
	if !inTeam {
		t.Error("the new person must be in the team of the invite")
	}
	// Aceito, o convite sai da lista de pendentes.
	if list := decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/invites", "", admin.session)); len(list) != 0 {
		t.Errorf("project invites after accepting = %v, want none", list)
	}
}

// O mesmo vale quando a pessoa entra pelo Clerk com o token do convite.
func TestClerk_ProjectInviteAppliesWhenJoiningThroughClerk(t *testing.T) {
	e, fake := newClerkServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	rec := do(e, "POST", "/api/projects/"+projectID+"/invites", `{"email":"caio@test.com","pay_rate_cents":9000}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project invite = %d: %s", rec.Code, rec.Body.String())
	}
	token := decode(t, rec)["token"].(string)
	fake.AddUser("user_c", "caio@test.com", "Caio", true)

	rec = clerkDo(e, "/api/auth/clerk/login", `{"invite_token":"`+token+`"}`, fake.Token("user_c"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("clerk login with the invite token = %d: %s", rec.Code, rec.Body.String())
	}
	personID := decode(t, rec)["identity"].(map[string]any)["id"].(string)
	got := false
	for _, a := range decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/allocations", "", admin.session)) {
		got = got || (a["person_id"] == personID && a["pay_rate_cents"] == float64(9000))
	}
	if !got {
		t.Error("the person who joined through Clerk must be in the project with 90,00")
	}
}

// Quem convida precisa do que pedem juntos o convite da organização e pôr alguém no projeto com valor, e não pode
// dar um grupo que tenha permissão que ele mesmo não tem.
func TestProjectInvite_PermissionsAndValidation(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	otherProject := createEmptyProject(t, e, admin, "Projeto Beta")
	rec := do(e, "POST", "/api/projects/"+otherProject+"/teams", `{"name":"Time de outro projeto"}`, admin.session)
	foreignTeam := decode(t, rec)["id"].(string)

	manager := invite(t, e, admin, "gabi@test.com", "member")
	withPreset(t, e, admin, projectID, manager.id, "manager")
	plain := invite(t, e, admin, "caio@test.com", "member")
	allocate(t, e, admin, projectID, plain.id, 1000)
	existing := "caio@test.com"

	code := func(by account, body string) int {
		return do(e, "POST", "/api/projects/"+projectID+"/invites", body, by.session).Code
	}

	valid := `{"email":"novo@test.com","pay_rate_cents":5000}`
	// Quem é só colaborador do projeto não convida; o gerente do projeto, sem a permissão de pessoas da
	// organização, também não.
	if got := code(plain, valid); got != http.StatusForbidden {
		t.Errorf("plain collaborator = %d, want 403", got)
	}
	if got := code(manager, valid); got != http.StatusForbidden {
		t.Errorf("project manager without people.manage = %d, want 403", got)
	}
	// Com a permissão de pessoas, o gerente convida, mas só dá os grupos que cabem nas permissões dele.
	do(e, "PATCH", "/api/persons/"+manager.id+"/permissions", `{"permissions":["people.manage"]}`, admin.session)
	if got := code(manager, valid); got != http.StatusCreated {
		t.Errorf("manager with people.manage = %d, want 201", got)
	}
	if got := code(manager, `{"email":"admin@test.com","pay_rate_cents":5000,"preset":"admin"}`); got != http.StatusForbidden {
		t.Errorf("a preset above the manager's own permissions = %d, want 403", got)
	}

	for name, tc := range map[string]struct {
		body string
		want int
		code string
	}{
		"no rate":         {`{"email":"a1@test.com"}`, http.StatusBadRequest, "allocation.rate_required"},
		"negative rate":   {`{"email":"a2@test.com","pay_rate_cents":-1}`, http.StatusBadRequest, "allocation.invalid_rate"},
		"huge rate":       {`{"email":"a3@test.com","pay_rate_cents":100000001}`, http.StatusBadRequest, "allocation.invalid_rate"},
		"unknown preset":  {`{"email":"a4@test.com","pay_rate_cents":5000,"preset":"deus"}`, http.StatusBadRequest, "allocation.invalid_preset"},
		"team elsewhere":  {`{"email":"a5@test.com","pay_rate_cents":5000,"team_id":"` + foreignTeam + `"}`, http.StatusBadRequest, "projectinvite.team_not_in_project"},
		"team not a uuid": {`{"email":"a6@test.com","pay_rate_cents":5000,"team_id":"x"}`, http.StatusBadRequest, "projectinvite.team_not_in_project"},
		"no email":        {`{"pay_rate_cents":5000}`, http.StatusBadRequest, "person.invalid_email"},
		"bad email":       {`{"email":"nao-e-email","pay_rate_cents":5000}`, http.StatusBadRequest, "person.invalid_email"},
		"existing person": {`{"email":"` + existing + `","pay_rate_cents":5000}`, http.StatusConflict, "auth.account_exists"},
	} {
		rec := do(e, "POST", "/api/projects/"+projectID+"/invites", tc.body, admin.session)
		if rec.Code != tc.want || errorCode(t, rec) != tc.code {
			t.Errorf("%s = %d %s, want %d %s", name, rec.Code, rec.Body.String(), tc.want, tc.code)
		}
	}

	// Outra organização não alcança o projeto.
	stranger := signup(t, e, "Outra", "zeca@outra.com")
	if got := code(stranger, valid); got != http.StatusNotFound {
		t.Errorf("another organization's admin = %d, want 404", got)
	}
	if got := status(e, "GET", "/api/projects/"+projectID+"/invites", "", stranger.session); got != http.StatusNotFound {
		t.Errorf("list from another organization = %d, want 404", got)
	}
	// A lista do projeto é de quem cuida das pessoas dele.
	if got := status(e, "GET", "/api/projects/"+projectID+"/invites", "", plain.session); got != http.StatusForbidden {
		t.Errorf("list by a plain collaborator = %d, want 403", got)
	}
}

// Se o projeto some antes do aceite, o convite segue valendo, só para a organização.
func TestProjectInvite_ProjectDeletedBeforeAcceptingLeavesAPlainInvite(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	rec := do(e, "POST", "/api/projects/"+projectID+"/invites", `{"email":"leo@test.com","pay_rate_cents":5000}`, admin.session)
	token := decode(t, rec)["token"].(string)

	if got := status(e, "DELETE", "/api/projects/"+projectID, "", admin.session); got != http.StatusNoContent {
		t.Fatalf("delete project = %d", got)
	}
	orgList := decodeList(t, do(e, "GET", "/api/orgs/"+admin.orgID+"/invites", "", admin.session))
	if len(orgList) != 1 || orgList[0]["project"] != nil {
		t.Errorf("organization invites = %v, want the invite without a project", orgList)
	}
	rec = do(e, "POST", "/api/auth/invites/"+token+"/accept", `{"name":"Leo","email":"leo@test.com","password":"senha-forte-2"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("accept after the project is gone = %d: %s", rec.Code, rec.Body.String())
	}
}

// O que não deu certo no projeto não derruba a conta: a pessoa entra na organização, e o que deu certo antes
// (o valor) fica.
func TestProjectInvite_ApplierKeepsWhatWorkedWhenTheTeamFails(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	person := invite(t, e, admin, "leo@test.com", "member")

	applier := projectinvite.NewApplier(allocation.NewService(allocation.NewStore(testClient)), team.NewMembershipService(team.NewMembershipStore(testClient)))
	ghostTeam := uuid.New()
	rate := 7000
	err := applier.Apply(context.Background(), uuid.MustParse(person.id), auth.ProjectSetup{
		ProjectID: uuid.MustParse(projectID), PayRateCents: &rate, TeamID: &ghostTeam,
	})
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatal("a team that does not exist must be reported")
	}
	got := false
	for _, a := range decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/allocations", "", admin.session)) {
		got = got || (a["person_id"] == person.id && a["pay_rate_cents"] == float64(7000))
	}
	if !got {
		t.Error("the rate must stay even though the team failed")
	}
}
