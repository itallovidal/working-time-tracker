package person_test

import (
	"errors"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/organization"
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

func address(n int) string { return strings.Repeat("a", n-len("@b.co")) + "@b.co" }

func TestService_ProfileFieldRules(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	first, err := svc.Create(org.ID.String(), "Ana", "ana@test.com")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, personName, email string
		code                    *apperr.Error
		field                   string
	}{
		{"nome vazio", "", "x@test.com", person.ErrNameRequired, "name"},
		{"nome só com espaços", "   ", "x@test.com", person.ErrNameRequired, "name"},
		{"nome acima do teto", strings.Repeat("n", 121), "x@test.com", apperr.ErrFieldTooLong, "name"},
		{"nome com acentos acima do teto", strings.Repeat("é", 121), "x@test.com", apperr.ErrFieldTooLong, "name"},
		{"e-mail vazio", "X", "  ", person.ErrEmailRequired, "email"},
		{"e-mail sem arroba", "X", "xtest.com", person.ErrInvalidEmail, "email"},
		{"e-mail sem ponto no domínio", "X", "x@test", person.ErrInvalidEmail, "email"},
		{"e-mail com espaço", "X", "x y@test.com", person.ErrInvalidEmail, "email"},
		{"e-mail acima do teto", "X", address(256), apperr.ErrFieldTooLong, "email"},
		{"e-mail em uso, em outra caixa", "X", "ANA@test.com", person.ErrEmailInUse, "email"},
	}
	for _, tc := range cases {
		_, err := svc.Create(org.ID.String(), tc.personName, tc.email)
		wantField(t, "create/"+tc.name, err, tc.code, tc.field)
		if tc.code != person.ErrEmailInUse {
			_, err = svc.Update(first.ID.String(), tc.personName, tc.email)
			wantField(t, "update/"+tc.name, err, tc.code, tc.field)
		}
	}

	// No teto, com espaços nas pontas e em maiúsculas: aparado e em minúsculas.
	p, err := svc.Create(org.ID.String(), "  "+strings.Repeat("n", 120)+" ", " "+strings.ToUpper(address(255))+"  ")
	if err != nil {
		t.Fatalf("create at the limits: %v", err)
	}
	if p.Name != strings.Repeat("n", 120) || p.Email != address(255) {
		t.Errorf("stored %q / %q, want trimmed and lowercase", p.Name, p.Email)
	}
	got, err := svc.Update(first.ID.String(), " Ana Souza ", " ANA@test.com ")
	if err != nil || got.Name != "Ana Souza" || got.Email != "ana@test.com" {
		t.Errorf("update = %+v, %v; want trimmed values and no conflict with itself", got, err)
	}
}

func TestService_RoleFields(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	p, _ := svc.Create(org.ID.String(), "Ana", "ana@test.com")

	_, err := svc.SetRole(p.ID.String(), "owner")
	wantField(t, "papel", err, person.ErrInvalidRole, "role")
}

func TestService_WeeklyHoursAndPaymentFields(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	p, _ := svc.Create(org.ID.String(), "Ana", "ana@test.com")
	id := p.ID.String()

	for _, h := range []int{-1, 169} {
		_, err := svc.SetWeeklyHours(id, &h)
		wantField(t, "jornada", err, person.ErrInvalidWeekHours, "weekly_hours")
	}
	for _, h := range []int{0, 1, 168} {
		if _, err := svc.SetWeeklyHours(id, &h); err != nil {
			t.Errorf("weekly hours %d: %v", h, err)
		}
	}

	cases := []struct {
		name  string
		rule  person.PaymentRule
		code  *apperr.Error
		field string
	}{
		{"frequência desconhecida", person.PaymentRule{Frequency: "yearly"}, person.ErrInvalidPayFrequency, "frequency"},
		{"dia 0", person.PaymentRule{Frequency: "monthly", Day: 0}, person.ErrInvalidPayDay, "day"},
		{"dia 32", person.PaymentRule{Frequency: "monthly", Day: 32}, person.ErrInvalidPayDay, "day"},
		{"início vazio", person.PaymentRule{Frequency: "biweekly"}, person.ErrInvalidPayStart, "start"},
		{"início fora do formato", person.PaymentRule{Frequency: "biweekly", Start: "01/10/2026"}, person.ErrInvalidPayStart, "start"},
	}
	for _, tc := range cases {
		_, err := svc.SetPayment(id, tc.rule)
		wantField(t, tc.name, err, tc.code, tc.field)
	}
}
