package allocation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/validate"
	"working-time-tracker/testutil"
)

type fixture struct {
	svc        *allocation.Service
	projSvc    *project.Service
	personSvc  *person.Service
	orgID      string
	projectX   string
	projectY   string
	ana, bruno string
}

func setup(t *testing.T) fixture {
	t.Helper()
	testutil.Truncate(t, testDB)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	f := fixture{
		svc:       allocation.NewService(allocation.NewStore(testClient)),
		projSvc:   project.NewService(project.NewStore(testClient)),
		personSvc: person.NewService(person.NewStore(testClient)),
	}
	org, _ := orgSvc.Create("Org")
	f.orgID = org.ID.String()
	x, _ := f.projSvc.Create(f.orgID, "Projeto X", "", 0, project.Routine{})
	y, _ := f.projSvc.Create(f.orgID, "Projeto Y", "", 0, project.Routine{})
	ana, _ := f.personSvc.Create(f.orgID, "Ana", "ana@test.com")
	bruno, err := f.personSvc.Create(f.orgID, "Bruno", "bruno@test.com")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	f.projectX, f.projectY = x.ID.String(), y.ID.String()
	f.ana, f.bruno = ana.ID.String(), bruno.ID.String()
	return f
}

// O dono da organização não recebe valor por hora: o que ele tira do projeto é a margem,
// então qualquer valor que venha vira zero.
func TestService_Set_OwnerAlwaysZero(t *testing.T) {
	f := setup(t)
	testClient.Person.UpdateOneID(uuid.MustParse(f.ana)).SetIsOwner(true).ExecX(context.Background())

	a, err := f.svc.Set(f.projectX, f.ana, 9000)
	if err != nil {
		t.Fatalf("set the owner: %v", err)
	}
	if a.PayRateCents != 0 {
		t.Errorf("owner pay rate = %d, want 0", a.PayRateCents)
	}
	if b, _ := f.svc.Set(f.projectX, f.bruno, 9000); b.PayRateCents != 9000 {
		t.Errorf("someone else's rate = %d, want 9000", b.PayRateCents)
	}
}

// A checagem do dono vem antes da do valor: o dono entra com 0, que para os outros não vale.
func TestService_Set_OwnerEntersWithZero(t *testing.T) {
	f := setup(t)
	testClient.Person.UpdateOneID(uuid.MustParse(f.ana)).SetIsOwner(true).ExecX(context.Background())

	a, err := f.svc.Set(f.projectX, f.ana, 0)
	if err != nil || a.PayRateCents != 0 {
		t.Fatalf("owner with zero: a = %+v, err = %v, want zero and none", a, err)
	}
	if _, err := f.svc.Set(f.projectX, f.bruno, 0); !errors.Is(err, allocation.ErrInvalidRate) {
		t.Errorf("a member with zero: err = %v, want ErrInvalidRate", err)
	}
}

// A mesma pessoa tem um valor diferente em cada projeto.
func TestService_RatePerProject(t *testing.T) {
	f := setup(t)

	a, err := f.svc.Set(f.projectX, f.bruno, 2000)
	if err != nil {
		t.Fatalf("set X: %v", err)
	}
	if a.PayRateCents != 2000 || a.Person == nil || a.Person.Name != "Bruno" {
		t.Errorf("allocation = %+v", a)
	}
	if _, err := f.svc.Set(f.projectY, f.bruno, 2500); err != nil {
		t.Fatalf("set Y: %v", err)
	}

	mine, err := f.svc.ListByPerson(f.bruno)
	if err != nil {
		t.Fatalf("list by person: %v", err)
	}
	if len(mine) != 2 || mine[0].Project.Name != "Projeto X" || mine[0].PayRateCents != 2000 || mine[1].PayRateCents != 2500 {
		t.Errorf("list by person = %+v", mine)
	}
}

// Definir de novo troca o valor, sem criar um segundo vínculo.
func TestService_SetTwiceUpdates(t *testing.T) {
	f := setup(t)
	f.svc.Set(f.projectX, f.bruno, 2000)
	f.svc.Set(f.projectX, f.ana, 1000)

	a, err := f.svc.Set(f.projectX, f.bruno, 3000)
	if err != nil {
		t.Fatalf("second set: %v", err)
	}
	if a.PayRateCents != 3000 {
		t.Errorf("pay_rate_cents = %d, want 3000", a.PayRateCents)
	}
	list, _ := f.svc.ListByProject(f.projectX)
	if len(list) != 2 || list[0].Person.Name != "Ana" || list[0].PayRateCents != 1000 || list[1].PayRateCents != 3000 {
		t.Errorf("list by project = %+v", list)
	}
}

func TestService_Validation(t *testing.T) {
	f := setup(t)

	// O piso e o teto: 10,00 e 1.000.000,00 valem; zero, 9,99, -1 e o teto + 1 não, e o erro diz o campo.
	for _, cents := range []int{validate.MinRateCents, validate.MaxCents} {
		if _, err := f.svc.Set(f.projectX, f.bruno, cents); err != nil {
			t.Errorf("rate %d: err = %v, want none", cents, err)
		}
	}
	for _, cents := range []int{0, validate.MinRateCents - 1, -1, validate.MaxCents + 1} {
		_, err := f.svc.Set(f.projectX, f.bruno, cents)
		var e *apperr.Error
		if !errors.Is(err, allocation.ErrInvalidRate) || !errors.As(err, &e) || e.Params["field"] != "pay_rate_cents" {
			t.Errorf("rate %d: err = %v, want ErrInvalidRate with field pay_rate_cents", cents, err)
		}
	}
	if allocation.MaxRateCents != validate.MaxCents {
		t.Errorf("MaxRateCents = %d, want validate.MaxCents", allocation.MaxRateCents)
	}

	// Pessoa de outra organização não entra no projeto.
	other, _ := organization.NewService(organization.NewStore(testClient)).Create("Outra")
	outsider, _ := f.personSvc.Create(other.ID.String(), "Caio", "caio@outra.com")
	if _, err := f.svc.Set(f.projectX, outsider.ID.String(), 1000); !errors.Is(err, allocation.ErrPersonNotInOrg) {
		t.Errorf("outsider: err = %v, want ErrPersonNotInOrg", err)
	}
	if _, err := f.svc.Set(f.projectX, "não-é-uuid", 1000); !errors.Is(err, allocation.ErrPersonNotInOrg) {
		t.Errorf("bad id: err = %v, want ErrPersonNotInOrg", err)
	}
}

// Excluir o projeto leva os vínculos junto.
func TestService_ProjectDeleteCascades(t *testing.T) {
	f := setup(t)
	f.svc.Set(f.projectX, f.bruno, 2000)
	f.svc.Set(f.projectY, f.bruno, 2500)

	if err := f.projSvc.Delete(f.projectX); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	mine, _ := f.svc.ListByPerson(f.bruno)
	if len(mine) != 1 || mine[0].Project.Name != "Projeto Y" {
		t.Errorf("after project delete, list by person = %+v", mine)
	}
}
