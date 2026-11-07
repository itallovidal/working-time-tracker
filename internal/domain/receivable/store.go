package receivable

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entcustomer "working-time-tracker/ent/customer"
	entorganization "working-time-tracker/ent/organization"
	entproject "working-time-tracker/ent/project"
	entreceivable "working-time-tracker/ent/receivable"
	entsession "working-time-tracker/ent/worksession"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/work_session"
)

// Store lê e grava os recebíveis. Os clientes, a organização e as sessões, que é de onde saem os períodos e as
// propostas, são lidos direto do banco: este domínio não muda nenhum deles.
type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// Org é o que o domínio precisa da organização: se ela trabalha no modo solo, o fuso dos períodos e a moeda.
type Org struct {
	Solo     bool
	Timezone string
	Currency string
}

// Org lê a organização. Um ID malformado ou que não existe é database.ErrNotFound.
func (s *Store) Org(orgID string) (Org, error) {
	id, err := uuid.Parse(orgID)
	if err != nil {
		return Org{}, database.ErrNotFound
	}
	o, err := s.client.Organization.Query().Where(entorganization.IDEQ(id)).Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return Org{}, database.ErrNotFound
		}
		return Org{}, err
	}
	return Org{Solo: o.Solo, Timezone: o.Timezone, Currency: o.Currency}, nil
}

// Customers lista os clientes da organização, em ordem alfabética, com os projetos de cada um (o acréscimo mensal de
// cada projeto soma à mensalidade).
func (s *Store) Customers(orgID uuid.UUID) ([]*ent.Customer, error) {
	return s.client.Customer.Query().
		Where(entcustomer.OrganizationIDEQ(orgID)).
		WithProjects().
		Order(ent.Asc(entcustomer.FieldName), ent.Asc(entcustomer.FieldID)).
		All(context.Background())
}

// Customer lê um cliente da organização, com os projetos dele. De outra organização ou inexistente, é
// database.ErrNotFound.
func (s *Store) Customer(orgID, id uuid.UUID) (*ent.Customer, error) {
	c, err := s.client.Customer.Query().
		Where(entcustomer.IDEQ(id), entcustomer.OrganizationIDEQ(orgID)).
		WithProjects().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

// SessionsByCustomer devolve as sessões de todos os projetos de cada cliente, agrupadas por cliente. Só os campos que
// cortam o tempo e o valor entram; as tarefas não fazem falta. O valor cobrado é o congelado em cada sessão (nulo no
// projeto de um cliente mensal, que não tem valor por hora).
func (s *Store) SessionsByCustomer(orgID uuid.UUID, customerIDs []uuid.UUID) (map[uuid.UUID][]work_session.WorkSession, error) {
	out := map[uuid.UUID][]work_session.WorkSession{}
	if len(customerIDs) == 0 {
		return out, nil
	}
	projects, err := s.client.Project.Query().
		Where(entproject.OrganizationIDEQ(orgID), entproject.CustomerIDIn(customerIDs...)).
		Select(entproject.FieldID, entproject.FieldCustomerID).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	customerOf := make(map[uuid.UUID]uuid.UUID, len(projects))
	ids := make([]uuid.UUID, 0, len(projects))
	for _, p := range projects {
		if p.CustomerID != nil {
			customerOf[p.ID] = *p.CustomerID
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.client.WorkSession.Query().
		Where(entsession.ProjectIDIn(ids...)).
		Order(ent.Asc(entsession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		cid := customerOf[r.ProjectID]
		out[cid] = append(out[cid], work_session.WorkSession{
			ID: r.ID, ProjectID: r.ProjectID, PersonID: r.PersonID,
			StartAt: r.StartAt, EndAt: r.EndAt, OwnerHours: r.OwnerHours,
			PayRateCents: r.PayRateCents, BillRateCents: r.BillRateCents,
		})
	}
	return out, nil
}

// List devolve os recebíveis da organização, todos: são poucos por cliente, um por período.
func (s *Store) List(orgID uuid.UUID) ([]*ent.Receivable, error) {
	return s.client.Receivable.Query().
		Where(entreceivable.OrganizationIDEQ(orgID)).
		All(context.Background())
}

// Get lê um recebível da organização. De outra organização, malformado ou inexistente, é database.ErrNotFound.
func (s *Store) Get(orgID uuid.UUID, id string) (*ent.Receivable, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, database.ErrNotFound
	}
	r, err := s.client.Receivable.Query().
		Where(entreceivable.IDEQ(uid), entreceivable.OrganizationIDEQ(orgID)).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

// Exists diz se o cliente já tem recebível do período.
func (s *Store) Exists(customerID uuid.UUID, start, end string) (bool, error) {
	return s.client.Receivable.Query().
		Where(entreceivable.CustomerIDEQ(customerID), entreceivable.PeriodStartEQ(start), entreceivable.PeriodEndEQ(end)).
		Exist(context.Background())
}

// Create grava a confirmação. Outra confirmação do mesmo cliente e período, gravada ao mesmo tempo, esbarra no índice
// único: ErrAlreadyConfirmed.
func (s *Store) Create(orgID uuid.UUID, r *ent.Receivable) (*ent.Receivable, error) {
	create := s.client.Receivable.Create().
		SetOrganizationID(orgID).
		SetCustomerID(r.CustomerID).
		SetKind(r.Kind).
		SetPeriodStart(r.PeriodStart).
		SetPeriodEnd(r.PeriodEnd).
		SetSeconds(r.Seconds).
		SetCalculatedCents(r.CalculatedCents).
		SetAmountCents(r.AmountCents).
		SetCurrency(r.Currency).
		SetDueDate(r.DueDate).
		SetInvoiceNumber(r.InvoiceNumber).
		SetNote(r.Note).
		SetConfirmedAt(r.ConfirmedAt)
	created, err := create.Save(context.Background())
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrAlreadyConfirmed
		}
		return nil, err
	}
	return created, nil
}

// Update grava a edição, só enquanto o recebível não foi recebido: a condição vai na própria instrução, então um
// recebimento que chega no meio não é sobrescrito. Sem linha mudada, ou o recebível não existe (database.ErrNotFound)
// ou já foi recebido (ErrAlreadyReceived).
func (s *Store) Update(orgID, id uuid.UUID, in editFields) error {
	up := s.client.Receivable.Update().
		Where(entreceivable.IDEQ(id), entreceivable.OrganizationIDEQ(orgID), entreceivable.ReceivedOnIsNil())
	if in.amount != nil {
		up = up.SetAmountCents(*in.amount)
	}
	if in.dueDate != nil {
		up = up.SetDueDate(*in.dueDate)
	}
	if in.invoice != nil {
		up = up.SetInvoiceNumber(*in.invoice)
	}
	if in.note != nil {
		up = up.SetNote(*in.note)
	}
	n, err := up.Save(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return s.whyNotChanged(orgID, id, ErrAlreadyReceived)
	}
	return nil
}

// editFields são os campos de uma edição; nil mantém o que está.
type editFields struct {
	amount  *int
	dueDate *string
	invoice *string
	note    *string
}

// Receive grava o recebimento, se o recebível ainda não foi recebido e continua com o valor confirmado que quem chama
// leu (amount): as condições vão na instrução, então dois recebimentos ao mesmo tempo não se sobrescrevem (o segundo é
// ErrAlreadyReceived) e uma edição do valor que chega no meio não é recebida às cegas (ErrChanged).
func (s *Store) Receive(orgID, id uuid.UUID, amount int, on string, cents, fee, interest int) error {
	n, err := s.client.Receivable.Update().
		Where(
			entreceivable.IDEQ(id), entreceivable.OrganizationIDEQ(orgID),
			entreceivable.ReceivedOnIsNil(), entreceivable.AmountCentsEQ(amount),
		).
		SetReceivedOn(on).
		SetReceivedCents(cents).
		SetLateFeeCents(fee).
		SetInterestCents(interest).
		Save(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return s.whyNotReceived(orgID, id)
	}
	return nil
}

// whyNotReceived explica um recebimento que não mudou linha nenhuma: o recebível não existe na organização, já foi
// recebido, ou foi editado depois de lido (ErrChanged).
func (s *Store) whyNotReceived(orgID, id uuid.UUID) error {
	r, err := s.Get(orgID, id.String())
	if err != nil {
		return err
	}
	if r.ReceivedOn != nil {
		return ErrAlreadyReceived
	}
	return ErrChanged
}

// Unreceive desfaz o recebimento: o recebível volta a estar em aberto. Sem recebimento a desfazer, ErrNotReceived.
func (s *Store) Unreceive(orgID, id uuid.UUID) error {
	n, err := s.client.Receivable.Update().
		Where(entreceivable.IDEQ(id), entreceivable.OrganizationIDEQ(orgID), entreceivable.ReceivedOnNotNil()).
		ClearReceivedOn().
		ClearReceivedCents().
		SetLateFeeCents(0).
		SetInterestCents(0).
		Save(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return s.whyNotChanged(orgID, id, ErrNotReceived)
	}
	return nil
}

// Delete desfaz a confirmação, se o recebível não foi recebido: o período volta a ser uma proposta.
func (s *Store) Delete(orgID, id uuid.UUID) error {
	n, err := s.client.Receivable.Delete().
		Where(entreceivable.IDEQ(id), entreceivable.OrganizationIDEQ(orgID), entreceivable.ReceivedOnIsNil()).
		Exec(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return s.whyNotChanged(orgID, id, ErrAlreadyReceived)
	}
	return nil
}

// whyNotChanged explica uma instrução condicional que não mudou linha nenhuma: o recebível não existe na organização,
// ou existe e o estado dele não deixa (o erro dado).
func (s *Store) whyNotChanged(orgID, id uuid.UUID, state error) error {
	exists, err := s.client.Receivable.Query().
		Where(entreceivable.IDEQ(id), entreceivable.OrganizationIDEQ(orgID)).
		Exist(context.Background())
	if err != nil {
		return err
	}
	if !exists {
		return database.ErrNotFound
	}
	return state
}
