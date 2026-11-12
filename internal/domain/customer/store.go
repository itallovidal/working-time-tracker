package customer

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entcustomer "working-time-tracker/ent/customer"
	entorganization "working-time-tracker/ent/organization"
	entreceivable "working-time-tracker/ent/receivable"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(c *Customer) error {
	created, err := s.client.Customer.Create().
		SetOrganizationID(c.OrganizationID).
		SetName(c.Name).
		SetCountry(c.Country).
		SetDocument(c.Document).
		SetContactName(c.ContactName).
		SetContactEmail(c.ContactEmail).
		SetContactPhone(c.ContactPhone).
		SetBillingMode(c.BillingMode).
		SetNillableHourlyRateCents(c.HourlyRateCents).
		SetNillableMonthlyFeeCents(c.MonthlyFeeCents).
		SetNillableContractStart(c.ContractStart).
		SetNillableContractEnd(c.ContractEnd).
		SetNillableEstimatedWeeklyHours(c.EstimatedWeeklyHours).
		SetBillingCycle(c.BillingCycle).
		SetNillableCycleAnchor(c.CycleAnchor).
		SetClosingDay(c.ClosingDay).
		SetPaymentDays(c.PaymentDays).
		SetLateFeeBp(c.LateFeeBP).
		SetLateInterestBpMonth(c.LateInterestBPMonth).
		SetGraceDays(c.GraceDays).
		Save(context.Background())
	if err != nil {
		return err
	}
	c.ID = created.ID
	c.CreatedAt = created.CreatedAt
	return nil
}

// ListByOrg lista os clientes em ordem alfabética, com a contagem de projetos.
func (s *Store) ListByOrg(orgID string) ([]Customer, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.client.Customer.Query().
		Where(entcustomer.OrganizationIDEQ(uid)).
		WithProjects().
		Order(ent.Asc(entcustomer.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]Customer, len(customers))
	for i, c := range customers {
		result[i] = *toDomain(c)
	}
	return result, nil
}

func (s *Store) GetByID(id string) (*Customer, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, database.ErrNotFound
	}
	c, err := s.client.Customer.Query().
		Where(entcustomer.IDEQ(uid)).
		WithProjects().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomain(c), nil
}

// Update grava o cliente como está: um campo opcional nil é apagado no banco. Quem decide entre manter e apagar é o
// service.
func (s *Store) Update(c *Customer) error {
	q := s.client.Customer.UpdateOneID(c.ID).
		SetName(c.Name).
		SetCountry(c.Country).
		SetDocument(c.Document).
		SetContactName(c.ContactName).
		SetContactEmail(c.ContactEmail).
		SetContactPhone(c.ContactPhone).
		SetBillingMode(c.BillingMode).
		SetBillingCycle(c.BillingCycle).
		SetClosingDay(c.ClosingDay).
		SetPaymentDays(c.PaymentDays).
		SetLateFeeBp(c.LateFeeBP).
		SetLateInterestBpMonth(c.LateInterestBPMonth).
		SetGraceDays(c.GraceDays)
	if c.CycleAnchor != nil {
		q = q.SetCycleAnchor(*c.CycleAnchor)
	} else {
		q = q.ClearCycleAnchor()
	}
	if c.HourlyRateCents != nil {
		q = q.SetHourlyRateCents(*c.HourlyRateCents)
	} else {
		q = q.ClearHourlyRateCents()
	}
	if c.MonthlyFeeCents != nil {
		q = q.SetMonthlyFeeCents(*c.MonthlyFeeCents)
	} else {
		q = q.ClearMonthlyFeeCents()
	}
	if c.ContractStart != nil {
		q = q.SetContractStart(*c.ContractStart)
	} else {
		q = q.ClearContractStart()
	}
	if c.ContractEnd != nil {
		q = q.SetContractEnd(*c.ContractEnd)
	} else {
		q = q.ClearContractEnd()
	}
	if c.EstimatedWeeklyHours != nil {
		q = q.SetEstimatedWeeklyHours(*c.EstimatedWeeklyHours)
	} else {
		q = q.ClearEstimatedWeeklyHours()
	}
	_, err := q.Save(context.Background())
	return err
}

// OrgCountry devolve o país da organização, que é o do cliente novo quando ninguém escolhe outro. Organização que não
// existe é ErrInvalidOrganization.
func (s *Store) OrgCountry(orgID uuid.UUID) (string, error) {
	code, err := s.client.Organization.Query().
		Where(entorganization.IDEQ(orgID)).
		Select(entorganization.FieldCountry).
		String(context.Background())
	if ent.IsNotFound(err) {
		return "", ErrInvalidOrganization
	}
	return code, err
}

// OrgSolo diz se a organização trabalha no modo solo, que é quando o cliente pode ser mensal. Organização que não
// existe é ErrInvalidOrganization.
func (s *Store) OrgSolo(orgID uuid.UUID) (bool, error) {
	solo, err := s.client.Organization.Query().
		Where(entorganization.IDEQ(orgID)).
		Select(entorganization.FieldSolo).
		Bool(context.Background())
	if ent.IsNotFound(err) {
		return false, ErrInvalidOrganization
	}
	return solo, err
}

// OrgTimezone devolve o fuso da organização, onde se conta o "hoje" da âncora do ciclo de 14 dias. Organização que não
// existe é ErrInvalidOrganization.
func (s *Store) OrgTimezone(orgID uuid.UUID) (string, error) {
	tz, err := s.client.Organization.Query().
		Where(entorganization.IDEQ(orgID)).
		Select(entorganization.FieldTimezone).
		String(context.Background())
	if ent.IsNotFound(err) {
		return "", ErrInvalidOrganization
	}
	return tz, err
}

// HasReceivables diz se o cliente tem algum recebível, confirmado ou recebido. Enquanto tiver, não pode ser excluído.
func (s *Store) HasReceivables(id uuid.UUID) (bool, error) {
	return s.client.Receivable.Query().
		Where(entreceivable.CustomerIDEQ(id)).
		Exist(context.Background())
}

// LastPeriodEnd devolve o último dia do período mais recente que o cliente já tem em recebíveis (YYYY-MM-DD), e false
// quando ele não tem nenhum. Os dias se comparam como texto.
func (s *Store) LastPeriodEnd(id uuid.UUID) (string, bool, error) {
	ends, err := s.client.Receivable.Query().
		Where(entreceivable.CustomerIDEQ(id)).
		Order(ent.Desc(entreceivable.FieldPeriodEnd)).
		Limit(1).
		Select(entreceivable.FieldPeriodEnd).
		Strings(context.Background())
	if err != nil || len(ends) == 0 {
		return "", false, err
	}
	return ends[0], true, nil
}

func (s *Store) Delete(id uuid.UUID) error {
	return s.client.Customer.DeleteOneID(id).Exec(context.Background())
}

func toDomain(e *ent.Customer) *Customer {
	return &Customer{
		ID:             e.ID,
		OrganizationID: e.OrganizationID,
		Name:           e.Name,
		Country:        e.Country,
		Document:       e.Document,
		ContactName:    e.ContactName,
		ContactEmail:   e.ContactEmail,
		ContactPhone:   e.ContactPhone,
		ProjectCount:   len(e.Edges.Projects),
		CreatedAt:      e.CreatedAt,

		BillingMode:          e.BillingMode,
		HourlyRateCents:      e.HourlyRateCents,
		MonthlyFeeCents:      e.MonthlyFeeCents,
		ContractStart:        e.ContractStart,
		ContractEnd:          e.ContractEnd,
		EstimatedWeeklyHours: e.EstimatedWeeklyHours,
		BillingCycle:         e.BillingCycle,
		CycleAnchor:          e.CycleAnchor,
		ClosingDay:           e.ClosingDay,
		PaymentDays:          e.PaymentDays,
		LateFeeBP:            e.LateFeeBp,
		LateInterestBPMonth:  e.LateInterestBpMonth,
		GraceDays:            e.GraceDays,
	}
}
