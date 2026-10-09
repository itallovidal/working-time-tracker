package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// As boas-vindas do primeiro acesso: um modal da página inicial que só o dono vê, até dar baixa nele.

const onboardingMarkup = `x-data="onboarding"`

func TestOnboarding_OwnerSeesWelcomeUntilItIsDismissed(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	me := decode(t, do(e, "GET", "/api/auth/me", "", admin.session))
	if me["needs_onboarding"] != true {
		t.Fatalf("a new owner needs the welcome: me = %v", me)
	}
	home := getPage(e, "/orgs/"+admin.orgID, admin.session, "", "")
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), onboardingMarkup) {
		t.Fatalf("the owner's home carries the welcome modal: %d", home.Code)
	}

	if rec := do(e, "POST", "/api/auth/onboarding/complete", "", admin.session); rec.Code != http.StatusNoContent {
		t.Fatalf("complete = %d: %s", rec.Code, rec.Body.String())
	}
	me = decode(t, do(e, "GET", "/api/auth/me", "", admin.session))
	if me["needs_onboarding"] != false {
		t.Errorf("after the dismissal the welcome is gone: me = %v", me)
	}
	home = getPage(e, "/orgs/"+admin.orgID, admin.session, "", "")
	if strings.Contains(home.Body.String(), onboardingMarkup) {
		t.Error("the home no longer carries the welcome modal after the dismissal")
	}

	// O modal pode avisar duas vezes (ao chegar à última etapa e ao fechar): repetir não é erro.
	if rec := do(e, "POST", "/api/auth/onboarding/complete", "", admin.session); rec.Code != http.StatusNoContent {
		t.Errorf("complete again = %d, want 204", rec.Code)
	}
}

func TestOnboarding_OnlyTheOwnerSeesIt(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, owner, "bia@test.com", "member")
	admin := invite(t, e, owner, "caio@test.com", "admin")

	for name, who := range map[string]account{"member": member, "admin who is not the owner": admin} {
		me := decode(t, do(e, "GET", "/api/auth/me", "", who.session))
		if me["needs_onboarding"] != false {
			t.Errorf("%s: needs_onboarding = %v, want false", name, me["needs_onboarding"])
		}
		home := getPage(e, "/orgs/"+who.orgID, who.session, "", "")
		if home.Code != http.StatusOK || strings.Contains(home.Body.String(), onboardingMarkup) {
			t.Errorf("%s: the home must not carry the welcome modal (status %d)", name, home.Code)
		}
	}
	// A baixa de quem não é o dono não mexe no dono.
	if rec := do(e, "POST", "/api/auth/onboarding/complete", "", member.session); rec.Code != http.StatusNoContent {
		t.Fatalf("complete as a member = %d", rec.Code)
	}
	if me := decode(t, do(e, "GET", "/api/auth/me", "", owner.session)); me["needs_onboarding"] != true {
		t.Errorf("the owner still needs the welcome after a member's call: me = %v", me)
	}
}

func TestOnboarding_RequiresLogin(t *testing.T) {
	e := newServer(t)
	if rec := do(e, "POST", "/api/auth/onboarding/complete", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("complete without a session = %d, want 401", rec.Code)
	}
}
