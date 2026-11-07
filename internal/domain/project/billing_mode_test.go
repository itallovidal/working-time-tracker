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

// A cobrança mora no cliente (Decisão 21): o projeto só guarda um acréscimo, por hora com cliente por hora (o valor
// cobrado é o do cliente mais ele) e por mês com cliente mensal (soma à mensalidade). O projeto interno não tem cliente
// nem acréscimo.

type billingFixture struct {
	svc     *project.Service
	orgID   string
	hourly  string // cliente por hora, a 150,00
	monthly string // cliente mensal
	norate  string // cliente por hora antigo, ainda sem valor por hora
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
	hourly := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Por hora").SetHourlyRateCents(15000).SaveX(ctx).ID.String()
	monthly := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Mensal").
		SetBillingMode(customer.BillingMonthly).SetMonthlyFeeCents(630000).SetContractStart("2026-11-01").SaveX(ctx).ID.String()
	norate := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Sem valor").SaveX(ctx).ID.String()
	return billingFixture{svc: project.NewService(project.NewStore(testClient)), orgID: org.ID.String(), hourly: hourly, monthly: monthly, norate: norate}
}

func TestBillingMode_CreateFollowsTheCustomerMode(t *testing.T) {
	f := newBillingFixture(t)

	// Cliente por hora: sem acréscimo, o projeto cobra o valor do cliente; com acréscimo, os dois somados.
	plain, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "A", CustomerID: f.hourly})
	if err != nil {
		t.Fatalf("hourly customer without an extra: %v", err)
	}
	if b, _ := f.svc.Billing(plain.ID.String()); b.BillRateCents == nil || *b.BillRateCents != 15000 || b.BillExtraCents != 0 ||
		b.CustomerBillingMode == nil || *b.CustomerBillingMode != customer.BillingHourly {
		t.Errorf("hourly billing = %+v, want the customer's 150.00 and no extra", b)
	}
	extra, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "B", CustomerID: f.hourly, BillExtraCents: 2000})
	if err != nil {
		t.Fatalf("hourly customer with an extra: %v", err)
	}
	if b, _ := f.svc.Billing(extra.ID.String()); b.BillRateCents == nil || *b.BillRateCents != 17000 || b.BillExtraCents != 2000 {
		t.Errorf("hourly billing with an extra = %+v, want 170.00 with 20.00 of extra", b)
	}
	for _, bad := range []int{-1, -2000} {
		_, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "C", CustomerID: f.hourly, BillExtraCents: bad})
		if !errors.Is(err, project.ErrInvalidBillExtra) || fieldOf(err) != "bill_extra_cents" {
			t.Errorf("negative extra %d: err = %v, want ErrInvalidBillExtra on bill_extra_cents", bad, err)
		}
	}

	// Cliente por hora sem valor (um cliente antigo): o projeto não teria o que cobrar.
	_, err = f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "N", CustomerID: f.norate})
	if !errors.Is(err, project.ErrCustomerWithoutRate) || fieldOf(err) != "customer_id" {
		t.Errorf("customer without a rate: err = %v, want ErrCustomerWithoutRate on customer_id", err)
	}

	// Cliente mensal: sem valor por hora, e o acréscimo é por mês.
	monthlyProject, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "M", CustomerID: f.monthly, BillExtraCents: 80000})
	if err != nil {
		t.Fatalf("monthly customer with an extra: %v", err)
	}
	if monthlyProject.Customer == nil || monthlyProject.Customer.Name != "Mensal" {
		t.Errorf("monthly project customer = %+v", monthlyProject.Customer)
	}
	b, err := f.svc.Billing(monthlyProject.ID.String())
	if err != nil || b.Customer == nil || b.BillRateCents != nil || b.BillExtraCents != 80000 ||
		b.CustomerBillingMode == nil || *b.CustomerBillingMode != customer.BillingMonthly {
		t.Errorf("monthly billing = %+v, %v; want the customer, no hourly rate, the 800.00 extra and its mode", b, err)
	}

	// O projeto interno não tem cliente nem acréscimo, e um acréscimo sem cliente é recusado.
	internal, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "I"})
	if err != nil {
		t.Fatalf("internal project: %v", err)
	}
	_, err = f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "I", BillExtraCents: 1000})
	if !errors.Is(err, project.ErrBillExtraNeedsCustomer) || fieldOf(err) != "bill_extra_cents" {
		t.Errorf("internal project with an extra: err = %v, want ErrBillExtraNeedsCustomer", err)
	}
	if b, _ := f.svc.Billing(internal.ID.String()); b.CustomerBillingMode != nil || b.BillRateCents != nil || b.BillExtraCents != 0 {
		t.Errorf("internal billing = %+v, want no mode, rate or extra", b)
	}
}

// Trocar o cliente do projeto troca a cobrança: o valor vem do cliente novo, e o acréscimo vale no modo dele.
func TestBillingMode_SetBillingFollowsTheCustomerMode(t *testing.T) {
	f := newBillingFixture(t)
	p, err := f.svc.Create(f.orgID, "Projeto", "", 0, project.Routine{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := p.ID.String()

	// Interno para mensal, com acréscimo por mês.
	b, err := f.svc.SetBilling(id, &f.monthly, 50000)
	if err != nil || b.Customer == nil || b.Customer.Name != "Mensal" || b.BillRateCents != nil || b.BillExtraCents != 50000 {
		t.Fatalf("monthly customer with an extra: %+v, %v", b, err)
	}

	// Mensal para por hora: o valor por hora é o do cliente mais o acréscimo que veio.
	b, err = f.svc.SetBilling(id, &f.hourly, 1000)
	if err != nil || b.BillRateCents == nil || *b.BillRateCents != 16000 || b.BillExtraCents != 1000 {
		t.Fatalf("hourly customer with an extra: %+v, %v", b, err)
	}

	// Um cliente sem valor é recusado, e o projeto fica como estava.
	_, err = f.svc.SetBilling(id, &f.norate, 0)
	if !errors.Is(err, project.ErrCustomerWithoutRate) || fieldOf(err) != "customer_id" {
		t.Errorf("customer without a rate: err = %v, want ErrCustomerWithoutRate", err)
	}
	if b, _ := f.svc.Billing(id); b.Customer == nil || b.Customer.ID.String() != f.hourly {
		t.Errorf("a refused billing changed the project: %+v", b)
	}

	// Voltar a interno com acréscimo é recusado; sem, o projeto fica sem cliente, valor e acréscimo.
	_, err = f.svc.SetBilling(id, nil, 1000)
	if !errors.Is(err, project.ErrBillExtraNeedsCustomer) {
		t.Errorf("internal with an extra: err = %v, want ErrBillExtraNeedsCustomer", err)
	}
	if b, err = f.svc.SetBilling(id, nil, 0); err != nil || b.Customer != nil || b.BillRateCents != nil || b.BillExtraCents != 0 || b.CustomerBillingMode != nil {
		t.Errorf("back to internal: %+v, %v", b, err)
	}
}

// Reajustar o valor do cliente muda o valor cobrado de todos os projetos dele, cada um com o seu acréscimo.
func TestBillingMode_TheRateFollowsTheCustomer(t *testing.T) {
	f := newBillingFixture(t)
	a, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "A", CustomerID: f.hourly})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	bp, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: "B", CustomerID: f.hourly, BillExtraCents: 2500})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	testClient.Customer.UpdateOneID(uuid.MustParse(f.hourly)).SetHourlyRateCents(18000).ExecX(context.Background())
	for id, want := range map[string]int{a.ID.String(): 18000, bp.ID.String(): 20500} {
		if b, err := f.svc.Billing(id); err != nil || b.BillRateCents == nil || *b.BillRateCents != want {
			t.Errorf("billing after the customer's raise = %+v, %v; want %d", b, err, want)
		}
	}
}

// A sessão não guarda o cliente: as horas de um projeto são do cliente que o projeto tem hoje. Tirar do cliente o
// projeto que tem sessões, quando o cliente já tem recebíveis, jogaria as horas já cobradas nas propostas do cliente
// novo. Sem recebível, ou sem sessão, o projeto troca à vontade, e o mesmo cliente com outro acréscimo também passa.
func TestBillingMode_MovingABilledProjectIsRefused(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()
	other := testClient.Customer.Create().SetOrganizationID(uuid.MustParse(f.orgID)).SetName("Outro").SetHourlyRateCents(12000).SaveX(ctx).ID.String()
	who, err := person.NewService(person.NewStore(testClient)).Create(f.orgID, "Ana", "ana@test.com")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// newProject cria um projeto do cliente por hora, com uma sessão de duas horas ou sem nenhuma.
	newProject := func(name string, withSession bool) string {
		t.Helper()
		p, err := f.svc.CreateWithCustomer(f.orgID, project.CreateInput{Name: name, CustomerID: f.hourly})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if withSession {
			task := testClient.Task.Create().SetProjectID(p.ID).SetName("T").SaveX(ctx)
			start := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
			end := start.Add(2 * time.Hour)
			pay, bill := 0, 15000
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
	if _, err := f.svc.SetBilling(billed, &other, 0); err != nil {
		t.Fatalf("move without receivables: %v", err)
	}
	if _, err := f.svc.SetBilling(billed, &f.hourly, 0); err != nil {
		t.Fatalf("move back without receivables: %v", err)
	}

	rec := testClient.Receivable.Create().
		SetOrganizationID(uuid.MustParse(f.orgID)).SetCustomerID(uuid.MustParse(f.hourly)).SetKind("hourly").
		SetPeriodStart("2026-09-01").SetPeriodEnd("2026-09-30").SetSeconds(7200).
		SetCalculatedCents(20000).SetAmountCents(20000).SetCurrency("BRL").SetDueDate("2026-10-05").
		SaveX(ctx)

	// Com recebível no cliente, o projeto com horas não sai dele: nem para outro cliente, nem para interno.
	_, err = f.svc.SetBilling(billed, &other, 0)
	if !errors.Is(err, project.ErrCustomerLocked) || fieldOf(err) != "customer_id" {
		t.Errorf("move to another customer: err = %v, want ErrCustomerLocked on customer_id", err)
	}
	_, err = f.svc.SetBilling(billed, nil, 0)
	if !errors.Is(err, project.ErrCustomerLocked) || fieldOf(err) != "customer_id" {
		t.Errorf("back to internal: err = %v, want ErrCustomerLocked on customer_id", err)
	}
	if got := customerOf(billed); got != f.hourly {
		t.Errorf("customer = %s after refused moves, want the original %s", got, f.hourly)
	}

	// O mesmo cliente com outro acréscimo passa: só o acréscimo muda.
	if b, err := f.svc.SetBilling(billed, &f.hourly, 1000); err != nil || b.BillRateCents == nil || *b.BillRateCents != 16000 {
		t.Errorf("same customer, new extra: %+v, %v", b, err)
	}
	// O projeto sem sessões não tem horas para cobrar de novo e sai à vontade.
	if _, err := f.svc.SetBilling(idle, &other, 0); err != nil {
		t.Errorf("move a project without sessions: %v", err)
	}

	// Desfeito o recebível, o projeto com horas pode sair.
	testClient.Receivable.DeleteOneID(rec.ID).ExecX(ctx)
	if _, err := f.svc.SetBilling(billed, &other, 0); err != nil {
		t.Errorf("move after the receivable was undone: %v", err)
	}
	if got := customerOf(billed); got != other {
		t.Errorf("customer = %s, want %s after the move", got, other)
	}
}
