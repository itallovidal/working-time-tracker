package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/person"
)

// wantField confere o código do erro e o parâmetro field (a chave do campo no corpo da requisição).
func wantField(t *testing.T, name string, err error, code *apperr.Error, field string) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || !errors.Is(err, code) {
		t.Errorf("%s: err = %v, want %s", name, err, code.Code)
		return
	}
	if e.Params["field"] != field {
		t.Errorf("%s: field = %v, want %q", name, e.Params["field"], field)
	}
}

func email(n int) string { return strings.Repeat("a", n-len("@b.co")) + "@b.co" }

func TestSignup_FieldRules(t *testing.T) {
	svc, _ := newService(t)
	ok := func(in SignupInput) SignupInput {
		base := SignupInput{OrganizationName: "Org", Name: "Ana", Email: "novo@acme.com", Password: "senha-forte-1"}
		if in.OrganizationName != "" {
			base.OrganizationName = in.OrganizationName
		}
		if in.Name != "" {
			base.Name = in.Name
		}
		if in.Email != "" {
			base.Email = in.Email
		}
		return base
	}

	cases := []struct {
		name  string
		in    SignupInput
		code  *apperr.Error
		field string
	}{
		{"organização vazia", SignupInput{OrganizationName: "   ", Name: "A", Email: "a@b.co", Password: "senha-forte-1"}, ErrOrgNameRequired, "organization_name"},
		{"organização acima do teto", ok(SignupInput{OrganizationName: strings.Repeat("o", 121)}), apperr.ErrFieldTooLong, "organization_name"},
		{"nome só com espaços", SignupInput{OrganizationName: "Org", Name: "   ", Email: "a@b.co", Password: "senha-forte-1"}, ErrNameRequired, "name"},
		{"nome acima do teto", ok(SignupInput{Name: strings.Repeat("n", 121)}), apperr.ErrFieldTooLong, "name"},
		{"nome com acentos acima do teto", ok(SignupInput{Name: strings.Repeat("ã", 121)}), apperr.ErrFieldTooLong, "name"},
		{"e-mail sem domínio", ok(SignupInput{Email: "ana@acme"}), person.ErrInvalidEmail, "email"},
		{"e-mail com espaço no meio", ok(SignupInput{Email: "an a@acme.com"}), person.ErrInvalidEmail, "email"},
		{"e-mail acima do teto", ok(SignupInput{Email: email(256)}), apperr.ErrFieldTooLong, "email"},
		{"senha curta", SignupInput{OrganizationName: "Org", Name: "A", Email: "a@b.co", Password: "curta"}, ErrWeakPassword, "password"},
		{"senha acima de 72 bytes", SignupInput{OrganizationName: "Org", Name: "A", Email: "a@b.co", Password: strings.Repeat("x", 73)}, ErrLongPassword, "password"},
		{"país desconhecido", SignupInput{OrganizationName: "Org", Name: "A", Email: "a@b.co", Password: "senha-forte-1", Country: "Atlantida"}, ErrInvalidCountry, "country"},
	}
	for _, tc := range cases {
		_, _, err := svc.Signup(tc.in)
		wantField(t, tc.name, err, tc.code, tc.field)
	}
}

func TestSignup_AtTheLimitsAndTrimmed(t *testing.T) {
	svc, _ := newService(t)
	id, _, err := svc.Signup(SignupInput{
		OrganizationName: "  " + strings.Repeat("o", 120) + "  ",
		Name:             " " + strings.Repeat("ã", 120) + " ",
		Email:            "  " + strings.ToUpper(email(255)) + " ",
		Password:         "senha-forte-1",
	})
	if err != nil {
		t.Fatalf("signup at the limits: %v", err)
	}
	if id.OrganizationName != strings.Repeat("o", 120) || id.Name != strings.Repeat("ã", 120) || id.Email != email(255) {
		t.Errorf("values were not trimmed and lowercased: %q %q %q", id.OrganizationName, id.Name, id.Email)
	}
}

func TestSignup_EmailInUseCarriesTheField(t *testing.T) {
	svc, _ := newService(t)
	mustSignup(t, svc, "ana@acme.com")
	_, _, err := svc.Signup(SignupInput{OrganizationName: "Org", Name: "A", Email: "ANA@acme.com", Password: "senha-forte-1"})
	wantField(t, "e-mail repetido", err, ErrEmailInUse, "email")
}

// O login não confere formato nem tamanho (só a tela exige os campos preenchidos): qualquer coisa que não confere é o
// mesmo "e-mail ou senha incorretos", sem dizer qual dos dois errou.
func TestLogin_DoesNotValidateFormat(t *testing.T) {
	svc, _ := newService(t)
	mustSignup(t, svc, "ana@acme.com")
	for _, c := range [][2]string{{"ana", "x"}, {email(300), "x"}, {"  ", "x"}, {"ana@acme.com", ""}} {
		if _, _, err := svc.Login(c[0], c[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("login %.20q / %q: err = %v, want invalid credentials", c[0], c[1], err)
		}
	}
}

func TestChangePassword_Fields(t *testing.T) {
	svc, _ := newService(t)
	id, token := mustSignup(t, svc, "ana@acme.com")
	cases := []struct {
		name, current, next string
		code                *apperr.Error
		field               string
	}{
		{"atual errada", "errada-errada", "outra-senha-1", ErrWrongPassword, "current_password"},
		{"nova igual à atual", "senha-forte-1", "senha-forte-1", ErrSamePassword, "new_password"},
		{"nova curta", "senha-forte-1", "curta", ErrWeakPassword, "new_password"},
		{"nova acima de 72 bytes", "senha-forte-1", strings.Repeat("y", 73), ErrLongPassword, "new_password"},
	}
	for _, tc := range cases {
		err := svc.ChangePassword(id, token, tc.current, tc.next)
		wantField(t, tc.name, err, tc.code, tc.field)
	}
	// A atual errada vem antes de "igual": quem não sabe a senha não descobre nada por esse erro.
	err := svc.ChangePassword(id, token, "errada-errada", "errada-errada")
	wantField(t, "atual errada e igual", err, ErrWrongPassword, "current_password")

	if err := svc.ChangePassword(id, token, "senha-forte-1", "senha-forte-2"); err != nil {
		t.Fatalf("a valid change: %v", err)
	}
}

func TestCreateInvite_EmailFieldRules(t *testing.T) {
	svc, _ := newService(t)
	id, _ := mustSignup(t, svc, "ana@acme.com")
	ctx := context.Background()
	cases := []struct {
		name, email, role string
		code              *apperr.Error
		field             string
	}{
		{"e-mail sem arroba", "bia", "member", person.ErrInvalidEmail, "email"},
		{"e-mail acima do teto", email(256), "member", apperr.ErrFieldTooLong, "email"},
		{"e-mail de conta que já existe", "ANA@acme.com", "member", ErrAccountExists, "email"},
		{"papel desconhecido", "", "owner", person.ErrInvalidRole, "role"},
	}
	for _, tc := range cases {
		_, _, err := svc.CreateInvite(ctx, id, tc.email, tc.role)
		wantField(t, tc.name, err, tc.code, tc.field)
	}
	// No teto, e sem e-mail (só o link), o convite sai.
	for _, e := range []string{email(255), "", "  "} {
		if _, _, err := svc.CreateInvite(ctx, id, e, "member"); err != nil {
			t.Errorf("invite with %.20q: %v", e, err)
		}
	}
}

func TestClerkSignup_NamesAreLimited(t *testing.T) {
	for _, tc := range []struct {
		name, typed, fromClerk, email, want string
	}{
		{"o que a pessoa digitou, aparado", "  Ana  ", "Clerk", "a@b.co", "Ana"},
		{"o do Clerk", "", "Clerk", "a@b.co", "Clerk"},
		{"o do e-mail", "", "", "ana@b.co", "ana"},
		{"o do Clerk cortado no teto", "", strings.Repeat("c", 200), "a@b.co", strings.Repeat("c", 120)},
		{"o do e-mail cortado no teto", "", "", strings.Repeat("e", 200) + "@b.co", strings.Repeat("e", 120)},
	} {
		if got := accountName(tc.typed, tc.fromClerk, tc.email); got != tc.want {
			t.Errorf("%s: accountName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClerkSignup_FieldRules(t *testing.T) {
	svc, fake := newClerkService(t)
	token := clerkUser(fake, "user_1", "ana@acme.com", "Ana")
	cases := []struct {
		name  string
		in    ClerkSignupInput
		code  *apperr.Error
		field string
	}{
		{"organização vazia", ClerkSignupInput{OrganizationName: "  "}, ErrOrgNameRequired, "organization_name"},
		{"organização acima do teto", ClerkSignupInput{OrganizationName: strings.Repeat("o", 121)}, apperr.ErrFieldTooLong, "organization_name"},
		{"nome acima do teto", ClerkSignupInput{OrganizationName: "Org", Name: strings.Repeat("n", 121)}, apperr.ErrFieldTooLong, "name"},
	}
	for _, tc := range cases {
		_, err := svc.ClerkSignup(bg, token, tc.in)
		wantField(t, tc.name, err, tc.code, tc.field)
	}
	// No teto e com espaços nas pontas passa, aparado.
	res, err := svc.ClerkSignup(bg, token, ClerkSignupInput{OrganizationName: " " + strings.Repeat("o", 120) + " ", Name: " Ana "})
	if err != nil {
		t.Fatalf("signup at the limit: %v", err)
	}
	if res.Identity.OrganizationName != strings.Repeat("o", 120) || res.Identity.Name != "Ana" {
		t.Errorf("identity = %q / %q, want trimmed values", res.Identity.OrganizationName, res.Identity.Name)
	}
}
