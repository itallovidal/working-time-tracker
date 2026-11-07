package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/testutil"
)

// O valor cobrado do projeto segue o modo de cobrança do cliente: o cliente por hora o exige, o mensal o recusa (o
// contrato é um valor por mês) e o projeto interno continua sem cliente e sem valor.

type billingFixture struct {
	svc     *project.Service
	orgID   string
	hourly  string // cliente por hora
	monthly string // cliente mensal
}

func newBillingFixture(t *testing.T) billingFixture {
	t.Helper()
	cleanup(t)
	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ctx := context.Background()
	testClient.Organization.UpdateOneID(org.ID).SetSolo(true).ExecX(ctx)
	hourly := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Por hora").SaveX(ctx).ID.String()
	monthly := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Mensal").
		SetBillingMode(customer.BillingMonthly).SetMonthlyFeeCents(630000).SetContractStart("2026-11-01").SaveX(ctx).ID.String()
	return billingFixture{svc: project.NewService(project.NewStore(testClient)), orgID: org.ID.String(), hourly: hourly, monthly: monthly}
}

func TestBillingMode_CreateFollowsTheCustomerMode(t *testing.T) {
	f := newBillingFixture(t)

	// Cliente por hora: o valor é obrigatório, como sempre, e tem o piso.
	_, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "A", CustomerID: f.hourly})
	if !errors.Is(err, project.ErrBillRateRequired) || fieldOf(err) != "bill_rate_cents" {
		t.Errorf("hourly customer without a rate: err = %v, want ErrBillRateRequired on bill_rate_cents", err)
	}
	_, err = f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "A", CustomerID: f.hourly, BillRateCents: ptr(999)})
	if !errors.Is(err, project.ErrInvalidBillRate) {
		t.Errorf("hourly customer under the floor: err = %v, want ErrInvalidBillRate", err)
	}
	hourlyProject, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "A", CustomerID: f.hourly, BillRateCents: ptr(15000)})
	if err != nil {
		t.Fatalf("hourly customer with a rate: %v", err)
	}
	if b, _ := f.svc.Billing(hourlyProject.ID.String()); b.BillRateCents == nil || *b.BillRateCents != 15000 ||
		b.CustomerBillingMode == nil || *b.CustomerBillingMode != customer.BillingHourly {
		t.Errorf("hourly billing = %+v", b)
	}

	// Cliente mensal: o valor é recusado, em qualquer quantia, e sem valor o projeto nasce.
	for _, rate := range []int{15000, 999, 0} {
		_, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "M", CustomerID: f.monthly, BillRateCents: ptr(rate)})
		if !errors.Is(err, project.ErrBillRateNotAllowed) || fieldOf(err) != "bill_rate_cents" {
			t.Errorf("monthly customer with rate %d: err = %v, want ErrBillRateNotAllowed on bill_rate_cents", rate, err)
		}
	}
	monthlyProject, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "M", CustomerID: f.monthly})
	if err != nil {
		t.Fatalf("monthly customer without a rate: %v", err)
	}
	if monthlyProject.Customer == nil || monthlyProject.Customer.Name != "Mensal" {
		t.Errorf("monthly project customer = %+v", monthlyProject.Customer)
	}
	b, err := f.svc.Billing(monthlyProject.ID.String())
	if err != nil || b.Customer == nil || b.BillRateCents != nil || b.CustomerBillingMode == nil || *b.CustomerBillingMode != customer.BillingMonthly {
		t.Errorf("monthly billing = %+v, %v; want the customer, no rate and its mode", b, err)
	}

	// O projeto interno não muda: sem cliente e sem valor, e um valor sem cliente continua recusado.
	internal, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "I"})
	if err != nil {
		t.Fatalf("internal project: %v", err)
	}
	_, err = f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "I", BillRateCents: ptr(15000)})
	if !errors.Is(err, project.ErrBillRateNeedsCustomer) {
		t.Errorf("internal project with a rate: err = %v, want ErrBillRateNeedsCustomer", err)
	}
	if b, _ := f.svc.Billing(internal.ID.String()); b.CustomerBillingMode != nil {
		t.Errorf("internal billing mode = %v, want none", *b.CustomerBillingMode)
	}
}

// Trocar o cliente do projeto revalida o valor contra o modo do cliente novo.
func TestBillingMode_SetBillingFollowsTheCustomerMode(t *testing.T) {
	f := newBillingFixture(t)
	p, err := f.svc.Create(f.orgID, "Projeto", "", 0, project.Routine{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := p.ID.String()

	// Interno para mensal: sem valor.
	_, err = f.svc.SetBilling(id, &f.monthly, ptr(15000))
	if !errors.Is(err, project.ErrBillRateNotAllowed) || fieldOf(err) != "bill_rate_cents" {
		t.Errorf("monthly customer with a rate: err = %v, want ErrBillRateNotAllowed", err)
	}
	if b, _ := f.svc.Billing(id); b.Customer != nil {
		t.Errorf("a refused billing changed the project: %+v", b)
	}
	b, err := f.svc.SetBilling(id, &f.monthly, nil)
	if err != nil || b.Customer == nil || b.Customer.Name != "Mensal" || b.BillRateCents != nil {
		t.Fatalf("monthly customer without a rate: %+v, %v", b, err)
	}

	// Mensal para por hora: o valor passa a ser obrigatório.
	_, err = f.svc.SetBilling(id, &f.hourly, nil)
	if !errors.Is(err, project.ErrBillRateRequired) {
		t.Errorf("hourly customer without a rate: err = %v, want ErrBillRateRequired", err)
	}
	b, err = f.svc.SetBilling(id, &f.hourly, ptr(12000))
	if err != nil || b.BillRateCents == nil || *b.BillRateCents != 12000 {
		t.Fatalf("hourly customer with a rate: %+v, %v", b, err)
	}

	// Por hora (com valor) para mensal: o valor antigo não passa para o cliente novo, tem de ir nulo.
	_, err = f.svc.SetBilling(id, &f.monthly, ptr(12000))
	if !errors.Is(err, project.ErrBillRateNotAllowed) {
		t.Errorf("monthly customer carrying the old rate: err = %v, want ErrBillRateNotAllowed", err)
	}
	if b, err = f.svc.SetBilling(id, &f.monthly, nil); err != nil || b.BillRateCents != nil {
		t.Errorf("hourly to monthly with a nil rate: %+v, %v", b, err)
	}

	// Voltar a interno: sem cliente e sem valor.
	if b, err = f.svc.SetBilling(id, nil, nil); err != nil || b.Customer != nil || b.BillRateCents != nil || b.CustomerBillingMode != nil {
		t.Errorf("back to internal: %+v, %v", b, err)
	}
}

// A sessão não guarda o cliente: as horas de um projeto são do cliente que o projeto tem hoje. Tirar do cliente o
// projeto que tem sessões, quando o cliente já tem recebíveis, jogaria as horas já cobradas nas propostas do cliente
// novo. Sem recebível, ou sem sessão, o projeto troca à vontade, e o mesmo cliente com outro valor também passa.
func TestBillingMode_MovingABilledProjectIsRefused(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()
	other := testClient.Customer.Create().SetOrganizationID(uuid.MustParse(f.orgID)).SetName("Outro").SaveX(ctx).ID.String()
	who, err := person.NewService(person.NewStore(testClient)).Create(f.orgID, "Ana", "ana@test.com")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// newProject cria um projeto do cliente por hora, com uma sessão de duas horas ou sem nenhuma.
	newProject := func(name string, withSession bool) string {
		t.Helper()
		p, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: name, CustomerID: f.hourly, BillRateCents: ptr(10000)})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if withSession {
			task := testClient.Task.Create().SetProjectID(p.ID).SetName("T").SaveX(ctx)
			start := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
			end := start.Add(2 * time.Hour)
			pay, bill := 0, 10000
			testutil.Session(t, testClient, task.ID, who.ID, start, &end, &pay, &bill)
		}
		return p.ID.String()
	}
	billed := newProject("Com horas", true)
	idle := newProject("Sem horas", false)
	customerOf := func(id string) string {
		t.Helper()
		b, err := f.svc.Billing(id)
		if err != nil {
			t.Fatalf("billing: %v", err)
		}
		if b.Customer == nil {
			return ""
		}
		return b.Customer.ID.String()
	}

	// Sem recebível o projeto com horas vai para outro cliente e volta.
	if _, err := f.svc.SetBilling(billed, &other, ptr(12000)); err != nil {
		t.Fatalf("move without receivables: %v", err)
	}
	if _, err := f.svc.SetBilling(billed, &f.hourly, ptr(10000)); err != nil {
		t.Fatalf("move back without receivables: %v", err)
	}

	rec := testClient.Receivable.Create().
		SetOrganizationID(uuid.MustParse(f.orgID)).SetCustomerID(uuid.MustParse(f.hourly)).SetKind("hourly").
		SetPeriodStart("2026-09-01").SetPeriodEnd("2026-09-30").SetSeconds(7200).
		SetCalculatedCents(20000).SetAmountCents(20000).SetCurrency("BRL").SetDueDate("2026-10-05").
		SaveX(ctx)

	// Com recebível no cliente, o projeto com horas não sai dele: nem para outro cliente, nem para interno.
	_, err = f.svc.SetBilling(billed, &other, ptr(12000))
	if !errors.Is(err, project.ErrCustomerLocked) || fieldOf(err) != "customer_id" {
		t.Errorf("move to another customer: err = %v, want ErrCustomerLocked on customer_id", err)
	}
	_, err = f.svc.SetBilling(billed, nil, nil)
	if !errors.Is(err, project.ErrCustomerLocked) || fieldOf(err) != "customer_id" {
		t.Errorf("back to internal: err = %v, want ErrCustomerLocked on customer_id", err)
	}
	if got := customerOf(billed); got != f.hourly {
		t.Errorf("customer = %s after refused moves, want the original %s", got, f.hourly)
	}

	// O mesmo cliente com outro valor passa: só o valor muda.
	if b, err := f.svc.SetBilling(billed, &f.hourly, ptr(11000)); err != nil || b.BillRateCents == nil || *b.BillRateCents != 11000 {
		t.Errorf("same customer, new rate: %+v, %v", b, err)
	}
	// O projeto sem sessões não tem horas para cobrar de novo e sai à vontade.
	if _, err := f.svc.SetBilling(idle, &other, ptr(12000)); err != nil {
		t.Errorf("move a project without sessions: %v", err)
	}

	// Desfeito o recebível, o projeto com horas pode sair.
	testClient.Receivable.DeleteOneID(rec.ID).ExecX(ctx)
	if _, err := f.svc.SetBilling(billed, &other, ptr(12000)); err != nil {
		t.Errorf("move after the receivable was undone: %v", err)
	}
	if got := customerOf(billed); got != other {
		t.Errorf("customer = %s, want %s after the move", got, other)
	}
}
