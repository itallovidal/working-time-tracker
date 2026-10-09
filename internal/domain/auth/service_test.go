package auth

import (
	"errors"
	"testing"
	"time"

	"working-time-tracker/internal/domain/person"
	"working-time-tracker/testutil"
)

// clock é um relógio controlável para testar expiração.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newService(t *testing.T) (*Service, *clock) {
	t.Helper()
	testutil.Truncate(t, testDB)
	c := &clock{t: time.Now()}
	svc := NewService(NewStore(testClient))
	svc.now = c.now
	return svc, c
}

func mustSignup(t *testing.T, svc *Service, email string) (*Identity, string) {
	t.Helper()
	id, token, err := svc.Signup(SignupInput{OrganizationName: "Acme", Name: "Ana", Email: email, Password: "senha-forte-1"})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	return id, token
}

func TestSignup_CreatesAdminWithSession(t *testing.T) {
	svc, _ := newService(t)
	id, token := mustSignup(t, svc, " Ana@Acme.com ")

	if id.Role != person.RoleAdmin || id.Email != "ana@acme.com" || id.OrganizationName != "Acme" {
		t.Errorf("identity = %+v, want admin ana@acme.com in Acme", id)
	}
	if !id.IsOwner {
		t.Error("who creates the organization is its owner")
	}
	got, err := svc.Authenticate(token)
	if err != nil || got.PersonID != id.PersonID {
		t.Fatalf("authenticate new session: %v", err)
	}
}

func TestSignup_Validation(t *testing.T) {
	svc, _ := newService(t)
	mustSignup(t, svc, "ana@acme.com")

	cases := []struct {
		name string
		in   SignupInput
		want error
	}{
		{"email repetido", SignupInput{"Outra", "Bia", "ANA@acme.com", "senha-forte-1"}, ErrEmailInUse},
		{"senha curta", SignupInput{"Outra", "Bia", "bia@acme.com", "curta"}, ErrWeakPassword},
		{"sem organização", SignupInput{"  ", "Bia", "bia@acme.com", "senha-forte-1"}, ErrOrgNameRequired},
		{"sem nome", SignupInput{"Outra", "", "bia@acme.com", "senha-forte-1"}, ErrNameRequired},
		{"email inválido", SignupInput{"Outra", "Bia", "bia", "senha-forte-1"}, person.ErrInvalidEmail},
	}
	for _, tc := range cases {
		if _, _, err := svc.Signup(tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestLogin(t *testing.T) {
	svc, _ := newService(t)
	mustSignup(t, svc, "ana@acme.com")

	if _, _, err := svc.Login("ana@acme.com", "senha-errada"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: err = %v", err)
	}
	if _, _, err := svc.Login("ninguem@acme.com", "senha-forte-1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email: err = %v", err)
	}
	id, token, err := svc.Login("ANA@acme.com", "senha-forte-1")
	if err != nil || token == "" || id.Email != "ana@acme.com" {
		t.Fatalf("login: id=%+v err=%v", id, err)
	}
}

func TestSession_ExpiresAndLogout(t *testing.T) {
	svc, c := newService(t)
	_, token := mustSignup(t, svc, "ana@acme.com")

	c.advance(SessionTTL - time.Minute)
	if _, err := svc.Authenticate(token); err != nil {
		t.Fatalf("session should still be valid: %v", err)
	}
	c.advance(2 * time.Minute)
	if _, err := svc.Authenticate(token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expired session: err = %v, want ErrUnauthenticated", err)
	}

	_, token2, _ := svc.Login("ana@acme.com", "senha-forte-1")
	if err := svc.Logout(token2); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.Authenticate(token2); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("session after logout: err = %v", err)
	}
}

func TestInvite_AcceptOnce(t *testing.T) {
	svc, _ := newService(t)
	admin, _ := mustSignup(t, svc, "ana@acme.com")

	inv, token, err := svc.CreateInvite(bg, admin, "", person.RoleMember)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	info, err := svc.InviteInfo(token)
	if err != nil || info.OrganizationName != "Acme" || info.Role != person.RoleMember {
		t.Fatalf("invite info = %+v, err = %v", info, err)
	}
	if pending, _ := svc.ListInvites(admin.OrganizationID); len(pending) != 1 || pending[0].ID != inv.ID {
		t.Errorf("pending invites = %+v, want the new invite", pending)
	}

	member, _, err := svc.AcceptInvite(token, AcceptInviteInput{Name: "Bia", Email: "bia@acme.com", Password: "senha-forte-2"})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if member.OrganizationID != admin.OrganizationID || member.Role != person.RoleMember {
		t.Errorf("member = %+v, want member of the admin's org", member)
	}
	if member.IsOwner {
		t.Error("an invited person is never the owner")
	}

	if _, _, err := svc.AcceptInvite(token, AcceptInviteInput{Name: "Caio", Email: "caio@acme.com", Password: "senha-forte-3"}); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("second accept: err = %v, want ErrInviteInvalid", err)
	}
	if pending, _ := svc.ListInvites(admin.OrganizationID); len(pending) != 0 {
		t.Errorf("accepted invite still pending: %+v", pending)
	}
}

func TestInvite_EmailExpiryAndRevoke(t *testing.T) {
	svc, c := newService(t)
	admin, _ := mustSignup(t, svc, "ana@acme.com")

	_, token, _ := svc.CreateInvite(bg, admin, "Bia@Acme.com", person.RoleAdmin)
	if _, _, err := svc.AcceptInvite(token, AcceptInviteInput{Name: "Caio", Email: "caio@acme.com", Password: "senha-forte-3"}); !errors.Is(err, ErrInviteEmailMismatch) {
		t.Errorf("other email: err = %v, want ErrInviteEmailMismatch", err)
	}
	if _, _, err := svc.CreateInvite(bg, admin, "ana@acme.com", person.RoleMember); !errors.Is(err, ErrAccountExists) {
		t.Errorf("invite existing account: err = %v, want ErrAccountExists", err)
	}

	inv, revoked, _ := svc.CreateInvite(bg, admin, "", person.RoleMember)
	if err := svc.RevokeInvite(bg, inv.ID.String()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.InviteInfo(revoked); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("revoked invite: err = %v, want ErrInviteInvalid", err)
	}

	c.advance(InviteTTL + time.Minute)
	if _, err := svc.InviteInfo(token); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("expired invite: err = %v, want ErrInviteInvalid", err)
	}
}

func TestChangePassword_KeepsOnlyCurrentSession(t *testing.T) {
	svc, _ := newService(t)
	id, current := mustSignup(t, svc, "ana@acme.com")
	_, other, _ := svc.Login("ana@acme.com", "senha-forte-1")

	if err := svc.ChangePassword(id, current, "errada-000", "nova-senha-1"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong current password: err = %v", err)
	}
	if err := svc.ChangePassword(id, current, "senha-forte-1", "nova-senha-1"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := svc.Authenticate(current); err != nil {
		t.Errorf("current session should survive: %v", err)
	}
	if _, err := svc.Authenticate(other); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("other session should be closed: err = %v", err)
	}
	if _, _, err := svc.Login("ana@acme.com", "nova-senha-1"); err != nil {
		t.Errorf("login with new password: %v", err)
	}
}

func TestOnboarding_OnlyTheOwnerNeedsItAndTheFirstDismissalCounts(t *testing.T) {
	svc, c := newService(t)
	owner, token := mustSignup(t, svc, "ana@acme.com")
	if !owner.NeedsOnboarding {
		t.Error("a new owner needs the welcome")
	}
	if got, err := svc.Authenticate(token); err != nil || !got.NeedsOnboarding {
		t.Fatalf("the authenticated owner needs the welcome: %+v, err = %v", got, err)
	}

	// Quem entra por convite encontra a organização montada: não há boas-vindas para ele.
	_, inviteToken, err := svc.CreateInvite(bg, owner, "", person.RoleAdmin)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	guest, _, err := svc.AcceptInvite(inviteToken, AcceptInviteInput{Name: "Bia", Email: "bia@acme.com", Password: "senha-forte-2"})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if guest.NeedsOnboarding {
		t.Error("an invited person never needs the owner's welcome")
	}

	if err := svc.CompleteOnboarding(owner); err != nil {
		t.Fatalf("complete: %v", err)
	}
	first := c.now()
	if got, _ := svc.Authenticate(token); got.NeedsOnboarding {
		t.Error("after the dismissal the owner no longer needs the welcome")
	}

	// Repetir não muda a data: ela é a de quando a pessoa terminou.
	c.advance(time.Hour)
	if err := svc.CompleteOnboarding(owner); err != nil {
		t.Fatalf("complete again: %v", err)
	}
	p, err := testClient.Person.Get(bg, owner.PersonID)
	if err != nil {
		t.Fatalf("load the owner: %v", err)
	}
	if p.OnboardedAt == nil || !p.OnboardedAt.Truncate(time.Millisecond).Equal(first.Truncate(time.Millisecond)) {
		t.Errorf("onboarded_at = %v, want the first dismissal at %v", p.OnboardedAt, first)
	}
}
