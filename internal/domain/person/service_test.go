package person_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/testutil"
)

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	p, err := svc.Create(org.ID.String(), "John", "john@test.com")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if p.Name != "John" {
		t.Errorf("name = %q, want %q", p.Name, "John")
	}
	if p.Email != "john@test.com" {
		t.Errorf("email = %q, want %q", p.Email, "john@test.com")
	}
}

func TestService_Create_DuplicateEmail(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	svc.Create(org.ID.String(), "John", "john@test.com")
	_, err := svc.Create(org.ID.String(), "Jane", "john@test.com")
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "", "john@test.com")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Create_EmptyEmail(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "John", "")
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
}

func TestService_ListByOrg(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	svc.Create(org.ID.String(), "John", "john@test.com")
	svc.Create(org.ID.String(), "Jane", "jane@test.com")

	persons, err := svc.ListByOrg(org.ID.String())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(persons) != 2 {
		t.Errorf("got %d persons, want 2", len(persons))
	}
}

// O email identifica a conta no login, então não pode se repetir nem entre organizações.
func TestService_Create_EmailUniqueAcrossOrgs(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	orgA, _ := orgSvc.Create("Org A")
	orgB, _ := orgSvc.Create("Org B")

	if _, err := svc.Create(orgA.ID.String(), "John", "john@test.com"); err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	if _, err := svc.Create(orgB.ID.String(), "John", " John@Test.com "); err != person.ErrEmailInUse {
		t.Fatalf("expected ErrEmailInUse for the same email (any case) in another org, got %v", err)
	}
}

func TestService_Update_EmailInUse(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	svc.Create(org.ID.String(), "Ana", "ana@test.com")
	bia, _ := svc.Create(org.ID.String(), "Bia", "bia@test.com")

	if _, err := svc.Update(bia.ID.String(), "Bia", "ana@test.com"); err != person.ErrEmailInUse {
		t.Fatalf("expected ErrEmailInUse, got %v", err)
	}
	// Manter o próprio email não conta como duplicado.
	if _, err := svc.Update(bia.ID.String(), "Bia Souza", "bia@test.com"); err != nil {
		t.Fatalf("keeping own email should work: %v", err)
	}
}

func TestService_SetRole_LastAdmin(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	ana, _ := svc.Create(org.ID.String(), "Ana", "ana@test.com")
	bia, _ := svc.Create(org.ID.String(), "Bia", "bia@test.com")

	if _, err := svc.SetRole(ana.ID.String(), person.RoleAdmin); err != nil {
		t.Fatalf("promote ana: %v", err)
	}
	if _, err := svc.SetRole(ana.ID.String(), person.RoleMember); err != person.ErrLastAdmin {
		t.Fatalf("expected ErrLastAdmin, got %v", err)
	}
	if _, err := svc.SetRole(bia.ID.String(), person.RoleAdmin); err != nil {
		t.Fatalf("promote bia: %v", err)
	}
	if _, err := svc.SetRole(ana.ID.String(), person.RoleMember); err != nil {
		t.Fatalf("with two admins, demoting one should work: %v", err)
	}
}

// Com dois admins, dois rebaixamentos simultâneos não podem passar os dois:
// um deles precisa receber ErrLastAdmin e a org continua com um admin.
func TestService_SetRole_ConcurrentDemotionsKeepOneAdmin(t *testing.T) {
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	for i := 0; i < 20; i++ {
		cleanup(t)
		org, _ := orgSvc.Create("Org")
		ana, _ := svc.Create(org.ID.String(), "Ana", "ana@test.com")
		bia, _ := svc.Create(org.ID.String(), "Bia", "bia@test.com")
		svc.SetRole(ana.ID.String(), person.RoleAdmin)
		svc.SetRole(bia.ID.String(), person.RoleAdmin)

		errs := make([]error, 2)
		var wg sync.WaitGroup
		for j, id := range []string{ana.ID.String(), bia.ID.String()} {
			wg.Add(1)
			go func(j int, id string) {
				defer wg.Done()
				_, errs[j] = svc.SetRole(id, person.RoleMember)
			}(j, id)
		}
		wg.Wait()

		persons, err := svc.ListByOrg(org.ID.String())
		if err != nil {
			t.Fatalf("list persons: %v", err)
		}
		admins := 0
		for _, p := range persons {
			if p.Role == person.RoleAdmin {
				admins++
			}
		}
		if admins != 1 {
			t.Fatalf("round %d: %d admins left (errors: %v, %v), want 1", i, admins, errs[0], errs[1])
		}
		if !(errs[0] == nil && errs[1] == person.ErrLastAdmin) && !(errs[0] == person.ErrLastAdmin && errs[1] == nil) {
			t.Fatalf("round %d: errors = %v, %v; want one success and one ErrLastAdmin", i, errs[0], errs[1])
		}
	}
}

// A org só pode ser excluída sem projetos. Quando isso acontece, as pessoas dela vão junto.
func TestOrganizationDelete_CascadesPersons(t *testing.T) {
	cleanup(t)
	ctx := context.Background()
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	other, _ := orgSvc.Create("Outra Org")
	svc.Create(org.ID.String(), "Ana", "ana@test.com")
	svc.Create(other.ID.String(), "Bia", "bia@test.com")

	if err := orgSvc.Delete(org.ID.String()); err != nil {
		t.Fatalf("delete organization with persons failed: %v", err)
	}
	if n := testClient.Person.Query().CountX(ctx); n != 1 {
		t.Errorf("persons: %d rows, want 1 (only the other organization's person)", n)
	}
}

// A jornada semanal é da pessoa e é opcional: nil ou zero apagam.
func TestService_SetWeeklyHours(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	ana, _ := svc.Create(org.ID.String(), "Ana", "ana@test.com")
	bia, _ := svc.Create(org.ID.String(), "Bia", "bia@test.com")
	id := ana.ID.String()

	hours := func(id string) *int {
		t.Helper()
		p, err := svc.Get(id)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		return p.WeeklyHours
	}
	if h := hours(id); h != nil {
		t.Errorf("weekly_hours of a new person = %d, want none", *h)
	}

	forty, zero := 40, 0
	updated, err := svc.SetWeeklyHours(id, &forty)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if updated.WeeklyHours == nil || *updated.WeeklyHours != 40 {
		t.Errorf("returned weekly_hours = %v, want 40", updated.WeeklyHours)
	}
	if h := hours(id); h == nil || *h != 40 {
		t.Errorf("weekly_hours = %v, want 40", h)
	}
	if h := hours(bia.ID.String()); h != nil {
		t.Errorf("weekly_hours of another person = %d, want none", *h)
	}

	// Mudar nome e email mantém a jornada.
	if _, err := svc.Update(id, "Ana Souza", "ana@test.com"); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if h := hours(id); h == nil || *h != 40 {
		t.Errorf("weekly_hours after a profile update = %v, want 40", h)
	}

	for _, bad := range []int{-1, 169} {
		if _, err := svc.SetWeeklyHours(id, &bad); err != person.ErrInvalidWeekHours {
			t.Errorf("set %d hours: err = %v, want ErrInvalidWeekHours", bad, err)
		}
	}

	for name, none := range map[string]*int{"zero": &zero, "nil": nil} {
		if _, err := svc.SetWeeklyHours(id, &forty); err != nil {
			t.Fatalf("set failed: %v", err)
		}
		if _, err := svc.SetWeeklyHours(id, none); err != nil {
			t.Fatalf("clearing with %s failed: %v", name, err)
		}
		if h := hours(id); h != nil {
			t.Errorf("weekly_hours after clearing with %s = %d, want none", name, *h)
		}
	}

	if _, err := svc.SetWeeklyHours(uuid.NewString(), &forty); err != database.ErrNotFound {
		t.Errorf("unknown person: err = %v, want ErrNotFound", err)
	}
}
