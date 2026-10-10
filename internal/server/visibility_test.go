package server_test

import (
	"net/http"
	"testing"
)

// weeklyHoursOf lê a jornada da pessoa na lista de pessoas da organização, como quem tem a sessão.
func weeklyHoursOf(t *testing.T, list []map[string]any, personID string) any {
	t.Helper()
	for _, p := range list {
		if p["id"] == personID {
			return p["weekly_hours"]
		}
	}
	t.Fatalf("person %s is not in the list", personID)
	return nil
}

// A jornada semanal é da pessoa: ela, o dono e os admins a veem, e o administrador de projeto a vê nas pessoas do
// projeto dele. Um colega comum recebe weekly_hours: null, na lista e no detalhe; quem administra outro projeto,
// também.
func TestVisibility_WeeklyHoursOnlyToWhoManagesThem(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, owner, "gabi@test.com", "member")
	bia := invite(t, e, owner, "bia@test.com", "member")
	caio := invite(t, e, owner, "caio@test.com", "member")
	elsewhere := invite(t, e, owner, "edu@test.com", "member")
	alfa := createEmptyProject(t, e, owner, "Alfa")
	beta := createEmptyProject(t, e, owner, "Beta")
	withPreset(t, e, owner, alfa, manager.id, "manager")
	withPreset(t, e, owner, alfa, bia.id, "member")
	withPreset(t, e, owner, alfa, caio.id, "member")
	withPreset(t, e, owner, beta, elsewhere.id, "manager")
	if code := status(e, "PATCH", "/api/persons/"+bia.id+"/weekly-hours", `{"weekly_hours":30}`, owner.session); code != http.StatusOK {
		t.Fatalf("set weekly hours = %d", code)
	}

	list := func(who account) []map[string]any {
		t.Helper()
		rec := do(e, "GET", "/api/orgs/"+owner.orgID+"/persons", "", who.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("list persons = %d: %s", rec.Code, rec.Body.String())
		}
		return decodeList(t, rec)
	}
	for name, c := range map[string]struct {
		who  account
		want any
	}{
		"the person":                      {bia, float64(30)},
		"the owner":                       {owner, float64(30)},
		"the project administrator":       {manager, float64(30)},
		"a colleague":                     {caio, nil},
		"an administrator of another one": {elsewhere, nil},
	} {
		if got := weeklyHoursOf(t, list(c.who), bia.id); got != c.want {
			t.Errorf("%s sees Bia's weekly hours as %v, want %v", name, got, c.want)
		}
		rec := do(e, "GET", "/api/persons/"+bia.id, "", c.who.session)
		if rec.Code == http.StatusOK {
			if got := decode(t, rec)["weekly_hours"]; got != c.want {
				t.Errorf("%s GET the person: weekly_hours = %v, want %v", name, got, c.want)
			}
		}
	}
	// A própria jornada vem do /me/overview e não muda.
	if got := decode(t, do(e, "GET", "/api/orgs/"+owner.orgID+"/me/overview", "", bia.session))["weekly_hours"]; got != float64(30) {
		t.Errorf("Bia's own overview weekly_hours = %v, want 30", got)
	}
}

// A lista de permissões de cada pessoa, no projeto, só vai para ela, para o dono e para os admins; o grupo (o papel)
// continua à vista de quem vê a lista.
func TestVisibility_PermissionListOfTheProject(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, owner, "gabi@test.com", "member")
	bia := invite(t, e, owner, "bia@test.com", "member")
	prj := createEmptyProject(t, e, owner, "Alfa")
	withPreset(t, e, owner, prj, manager.id, "manager")
	withPreset(t, e, owner, prj, bia.id, "member")

	permissionsOf := func(who account, personID string) (preset string, perms []any, found bool) {
		t.Helper()
		rec := do(e, "GET", "/api/projects/"+prj+"/allocations", "", who.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("list allocations = %d: %s", rec.Code, rec.Body.String())
		}
		for _, a := range decodeList(t, rec) {
			if a["person_id"] == personID {
				p, _ := a["permissions"].([]any)
				return a["preset"].(string), p, true
			}
		}
		return "", nil, false
	}
	// O gerente vê a linha da Bia (o papel), mas não a lista dela; vê a própria lista; o dono vê todas.
	if preset, perms, ok := permissionsOf(manager, bia.id); !ok || preset != "member" || len(perms) != 0 {
		t.Errorf("manager sees Bia as preset %q with permissions %v (found %v), want member and none", preset, perms, ok)
	}
	if preset, perms, _ := permissionsOf(manager, manager.id); preset != "manager" || len(perms) == 0 {
		t.Errorf("manager sees own preset %q with permissions %v, want manager and a list", preset, perms)
	}
	if preset, perms, _ := permissionsOf(owner, manager.id); preset != "manager" || len(perms) == 0 {
		t.Errorf("owner sees the manager as preset %q with permissions %v, want manager and a list", preset, perms)
	}
}

// Excluir o projeto apaga as horas e os valores dele, e tem a permissão própria project.delete: o administrador de
// projeto, que edita, não exclui; o dono, o admin e o grupo admin do projeto sim.
func TestVisibility_DeletingAProjectIsItsOwnPermission(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, owner, "gabi@test.com", "member")
	helena := invite(t, e, owner, "helena@test.com", "admin")
	prj := createEmptyProject(t, e, owner, "Alfa")
	withPreset(t, e, owner, prj, manager.id, "manager")

	if code := status(e, "PATCH", "/api/projects/"+prj, `{"name":"Alfa 2"}`, manager.session); code != http.StatusOK {
		t.Errorf("manager editing = %d, want 200", code)
	}
	if code := status(e, "DELETE", "/api/projects/"+prj, "", manager.session); code != http.StatusForbidden {
		t.Errorf("manager deleting = %d, want 403", code)
	}
	if code := status(e, "DELETE", "/api/projects/"+prj, "", helena.session); code != http.StatusNoContent {
		t.Errorf("admin deleting = %d, want 204", code)
	}
}
