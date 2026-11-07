package auth

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
)

// O modo solo no cadastro e nos convites: a organização nasce solo quando o cadastro pede, e uma organização solo não
// convida ninguém.

func soloSignup(t *testing.T, svc *Service, email string) *Identity {
	t.Helper()
	id, _, err := svc.Signup(SignupInput{OrganizationName: "Solo", Name: "Ana", Email: email, Password: "senha-forte-1", Solo: true})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	return id
}

func TestSignup_SoloCreatesTheOrganizationInSoloMode(t *testing.T) {
	svc, _ := newService(t)
	id := soloSignup(t, svc, "ana@solo.com")

	if !id.OrganizationSolo || !id.IsOwner {
		t.Errorf("identity = %+v, want the owner of a solo organization", id)
	}
	org, err := testClient.Organization.Get(bg, id.OrganizationID)
	if err != nil || !org.Solo {
		t.Fatalf("organization = %+v (%v), want solo", org, err)
	}
	// A identidade que o login e a sessão devolvem também diz.
	if logged, _, err := svc.Login("ana@solo.com", "senha-forte-1"); err != nil || !logged.OrganizationSolo {
		t.Errorf("login identity = %+v (%v), want OrganizationSolo", logged, err)
	}
}

// Sem o campo, a organização nasce com equipe, como sempre.
func TestSignup_WithoutSoloKeepsTheTeamMode(t *testing.T) {
	svc, _ := newService(t)
	id, _ := mustSignup(t, svc, "ana@acme.com")
	if id.OrganizationSolo {
		t.Errorf("identity = %+v, want a team organization", id)
	}
	if org, _ := testClient.Organization.Get(bg, id.OrganizationID); org == nil || org.Solo {
		t.Errorf("organization = %+v, want not solo", org)
	}
}

func TestClerkSignup_SoloCreatesTheOrganizationInSoloMode(t *testing.T) {
	svc, fake := newClerkService(t)
	token := clerkUser(fake, "user_solo", "ana@solo.com", "Ana")
	res, err := svc.ClerkSignup(bg, token, ClerkSignupInput{OrganizationName: "Solo", Solo: true})
	if err != nil {
		t.Fatalf("clerk signup: %v", err)
	}
	if !res.Identity.OrganizationSolo {
		t.Errorf("identity = %+v, want a solo organization", res.Identity)
	}
	if org, _ := testClient.Organization.Get(bg, res.Identity.OrganizationID); org == nil || !org.Solo {
		t.Errorf("organization = %+v, want solo", org)
	}

	// Sem o campo, com equipe.
	svc2, fake2 := newClerkService(t)
	token2 := clerkUser(fake2, "user_team", "bia@acme.com", "Bia")
	if res2 := mustClerkSignup(t, svc2, token2, "Acme"); res2.Identity.OrganizationSolo {
		t.Errorf("identity = %+v, want a team organization", res2.Identity)
	}
}

// Numa organização solo, convidar para a organização ou para um projeto é recusado, com e sem e-mail, e nada é gravado.
func TestInvites_SoloOrganizationInvitesNoOne(t *testing.T) {
	svc, _ := newService(t)
	owner := soloSignup(t, svc, "ana@solo.com")

	if _, _, err := svc.CreateInvite(bg, owner, "", person.RoleMember); !errors.Is(err, organization.ErrSoloNoInvites) {
		t.Errorf("invite for the organization: err = %v, want ErrSoloNoInvites", err)
	}
	if _, _, err := svc.CreateInvite(bg, owner, "bia@solo.com", person.RoleMember); !errors.Is(err, organization.ErrSoloNoInvites) {
		t.Errorf("invite with an email: err = %v, want ErrSoloNoInvites", err)
	}
	setup := ProjectSetup{ProjectID: uuid.New(), PayRateCents: ptr(5000)}
	if _, _, err := svc.CreateProjectInvite(bg, owner, "bia@solo.com", setup); !errors.Is(err, organization.ErrSoloNoInvites) {
		t.Errorf("invite for a project: err = %v, want ErrSoloNoInvites", err)
	}
	if n := testClient.Invite.Query().CountX(bg); n != 0 {
		t.Errorf("%d invites were saved, want none", n)
	}
}

// O estado que vale é o do banco: desligar o modo solo (pelo PATCH da organização) libera o convite na hora, mesmo
// com a identidade de antes ainda na mão.
func TestInvites_TurningSoloOffAllowsThemAgain(t *testing.T) {
	svc, _ := newService(t)
	owner := soloSignup(t, svc, "ana@solo.com")
	if _, _, err := svc.CreateInvite(bg, owner, "", person.RoleMember); !errors.Is(err, organization.ErrSoloNoInvites) {
		t.Fatalf("while solo: err = %v, want ErrSoloNoInvites", err)
	}
	testClient.Organization.UpdateOneID(owner.OrganizationID).SetSolo(false).ExecX(bg)

	inv, token, err := svc.CreateInvite(bg, owner, "", person.RoleMember)
	if err != nil || inv == nil || token == "" {
		t.Errorf("after turning solo off: invite = %+v, token %q, err %v, want a link", inv, token, err)
	}
}

// Uma organização com equipe convida como sempre.
func TestInvites_TeamOrganizationIsUnchanged(t *testing.T) {
	svc, _ := newService(t)
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	if _, token, err := svc.CreateInvite(bg, owner, "", person.RoleMember); err != nil || token == "" {
		t.Errorf("invite in a team organization: token %q, err %v, want a link", token, err)
	}
}

func ptr[T any](v T) *T { return &v }
