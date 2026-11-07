package customer

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/country"
	"working-time-tracker/internal/validate"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(orgID string, in Input) (*Customer, error) {
	orgUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, ErrInvalidOrganization
	}
	c := &Customer{OrganizationID: orgUID, BillingMode: BillingHourly, ClosingDay: DefaultClosingDay}
	if in.Name == nil {
		return nil, ErrNameRequired.With("field", "name")
	}
	// O país do cliente novo é o da organização, a menos que venha outro.
	if in.Country == nil {
		in.Country = new(string)
	}
	if err := s.fillCountry(orgUID, &in); err != nil {
		return nil, err
	}
	if err := apply(c, in); err != nil {
		return nil, err
	}
	if err := s.applyBilling(c, in); err != nil {
		return nil, err
	}
	if err := s.store.Create(c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) ListByOrg(orgID string) ([]Customer, error) {
	return s.store.ListByOrg(orgID)
}

func (s *Service) Get(id string) (*Customer, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id string, in Input) (*Customer, error) {
	c, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	// País vazio vale o mesmo na criação e na edição: o país da organização.
	if err := s.fillCountry(c.OrganizationID, &in); err != nil {
		return nil, err
	}
	if err := apply(c, in); err != nil {
		return nil, err
	}
	if err := s.applyBilling(c, in); err != nil {
		return nil, err
	}
	if err := s.store.Update(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Delete recusa o cliente que ainda tem projetos, para nenhum projeto perder o cliente sem alguém decidir isso, e o
// que tem recebíveis: o dono os desfaz antes, na página A receber (a chave estrangeira também não os apaga junto).
func (s *Service) Delete(id string) error {
	c, err := s.store.GetByID(id)
	if err != nil {
		return err
	}
	if c.ProjectCount > 0 {
		return ErrHasProjects
	}
	has, err := s.store.HasReceivables(c.ID)
	if err != nil {
		return err
	}
	if has {
		return ErrHasReceivables
	}
	return s.store.Delete(c.ID)
}

// fillCountry troca um país enviado vazio (ou só espaços) pelo da organização, igual na criação e na edição. País
// ausente (nil) não é tocado: na criação quem chama já o trocou por vazio, e na edição ausente mantém o atual.
func (s *Service) fillCountry(orgID uuid.UUID, in *Input) error {
	if in.Country == nil || strings.TrimSpace(*in.Country) != "" {
		return nil
	}
	orgCountry, err := s.store.OrgCountry(orgID)
	if err != nil {
		return err
	}
	in.Country = &orgCountry
	return nil
}

// apply confere os campos e os aplica a c. Todo erro diz o campo (a chave do corpo) para a tela marcá-lo.
func apply(c *Customer, in Input) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return ErrNameRequired.With("field", "name")
		}
		if utf8.RuneCountInString(name) > validate.MaxName {
			return ErrNameTooLong.With("field", "name", "max", validate.MaxName)
		}
		c.Name = name
	}
	// O país vem antes do documento, que se confere pela regra dele. Aceita os países do cadastro e qualquer código ISO.
	countryChanged := false
	if in.Country != nil {
		code, ok := country.CustomerCountry(*in.Country)
		if !ok {
			return ErrInvalidCountry.With("field", "country")
		}
		countryChanged = code != c.Country
		c.Country = code
	}
	switch {
	case in.Document != nil:
		v := strings.TrimSpace(*in.Document)
		if v != "" {
			normalized, ok := country.NormalizeTaxID(c.Country, v)
			if !ok {
				return ErrInvalidDocument.With("field", "document")
			}
			v = normalized
		}
		c.Document = v
	case countryChanged && c.Document != "":
		// O documento guardado é conferido pelo país novo, e um que não serve é recusado: nada é apagado em silêncio.
		normalized, ok := country.NormalizeTaxID(c.Country, c.Document)
		if !ok {
			return ErrInvalidDocument.With("field", "country")
		}
		c.Document = normalized
	}
	if in.ContactName != nil {
		v := strings.TrimSpace(*in.ContactName)
		if utf8.RuneCountInString(v) > validate.MaxName {
			return ErrContactTooLong.With("field", "contact_name", "max", validate.MaxName)
		}
		c.ContactName = v
	}
	if in.ContactEmail != nil {
		// Formato e tamanho são erros separados (request.field_invalid e request.field_too_long).
		v, err := validate.Email("contact_email", *in.ContactEmail)
		if err != nil {
			return err
		}
		c.ContactEmail = v
	}
	if in.ContactPhone != nil {
		v := strings.TrimSpace(*in.ContactPhone)
		if v != "" && !validate.Phone(v) {
			return ErrInvalidPhone.With("field", "contact_phone")
		}
		c.ContactPhone = v
	}
	return nil
}

// applyBilling confere os termos comerciais de in e os aplica a c, que já traz o cliente atual (na criação, os
// padrões: hourly e fechamento no dia 31). Todo erro diz o campo, para a tela marcá-lo.
//
// O modo vale o de in, ou o que o cliente já tem. Trocar de modo é recusado quando o cliente tem projetos
// (ErrModeLocked), e o monthly só vale em organização solo. Um campo do outro modo, com valor, é recusado; e trocar de
// modo apaga os campos do modo que saiu, para o cliente nunca guardar os dois.
func (s *Service) applyBilling(c *Customer, in Input) error {
	mode := c.BillingMode
	if in.BillingMode != nil {
		m := strings.TrimSpace(*in.BillingMode)
		if m != BillingHourly && m != BillingMonthly {
			return ErrInvalidBillingMode.With("field", "billing_mode")
		}
		if m != c.BillingMode && c.ProjectCount > 0 {
			return ErrModeLocked.With("field", "billing_mode")
		}
		if m == BillingMonthly {
			solo, err := s.store.OrgSolo(c.OrganizationID)
			if err != nil {
				return err
			}
			if !solo {
				return ErrMonthlyNeedsSolo.With("field", "billing_mode")
			}
		}
		mode = m
	}
	if field := otherModeField(mode, in); field != "" {
		if mode == BillingHourly {
			return ErrFieldMonthlyOnly.With("field", field)
		}
		return ErrFieldHourlyOnly.With("field", field)
	}
	if mode != c.BillingMode {
		resetModeFields(c)
		c.BillingMode = mode
	}

	// Os campos seguem a ordem da tela: o valor ou a mensalidade, o contrato, o fechamento e o prazo, e a multa.
	if mode == BillingHourly {
		if err := applyDefaultRate(c, in.DefaultRateCents); err != nil {
			return err
		}
	} else if err := applyContract(c, in); err != nil {
		return err
	}
	if in.ClosingDay != nil {
		if *in.ClosingDay < 1 || *in.ClosingDay > 31 {
			return ErrInvalidClosingDay.With("field", "closing_day")
		}
		c.ClosingDay = *in.ClosingDay
	}
	if in.PaymentDays != nil {
		if *in.PaymentDays < 0 || *in.PaymentDays > validate.MaxTermDays {
			return ErrInvalidPaymentDays.With("field", "payment_days")
		}
		c.PaymentDays = *in.PaymentDays
	}
	if mode == BillingMonthly {
		return applyLateTerms(c, in)
	}
	return nil
}

// otherModeField devolve o primeiro campo de in que é do outro modo e vem com valor, ou "" quando não há. Zero e
// vazio não são valor: é o padrão do campo, e uma tela que manda tudo sempre não é recusada por isso.
func otherModeField(mode string, in Input) string {
	filled := func(v *string) bool { return v != nil && strings.TrimSpace(*v) != "" }
	nonZero := func(v *int) bool { return v != nil && *v != 0 }
	if mode == BillingMonthly {
		if in.DefaultRateCents.Value != nil {
			return "default_rate_cents"
		}
		return ""
	}
	switch {
	case in.MonthlyFeeCents.Value != nil:
		return "monthly_fee_cents"
	case filled(in.ContractStart):
		return "contract_start"
	case filled(in.ContractEnd):
		return "contract_end"
	case nonZero(in.LateFeeBP):
		return "late_fee_bp"
	case nonZero(in.LateInterestBPMonth):
		return "late_interest_bp_month"
	case nonZero(in.GraceDays):
		return "grace_days"
	}
	return ""
}

// resetModeFields apaga o que é de um modo só, na troca de modo: o valor por hora padrão e todo o contrato mensal.
func resetModeFields(c *Customer) {
	c.DefaultRateCents = nil
	c.MonthlyFeeCents, c.ContractStart, c.ContractEnd = nil, nil, nil
	c.LateFeeBP, c.LateInterestBPMonth, c.GraceDays = 0, 0, 0
}

// applyDefaultRate aplica o valor por hora padrão do cliente por hora: ausente mantém, null apaga e, com valor, vale o
// piso de validate.MinRateCents (e o teto).
func applyDefaultRate(c *Customer, in validate.Optional[int]) error {
	if !in.Set {
		return nil
	}
	if in.Value == nil {
		c.DefaultRateCents = nil
		return nil
	}
	if validate.Rate("default_rate_cents", *in.Value) != nil {
		return ErrInvalidDefaultRate.With("field", "default_rate_cents")
	}
	v := *in.Value
	c.DefaultRateCents = &v
	return nil
}

// applyContract aplica o contrato do cliente mensal: a mensalidade (maior que zero) e o início são obrigatórios, e o
// fim, quando existe, vem depois do início.
func applyContract(c *Customer, in Input) error {
	if in.MonthlyFeeCents.Set {
		if in.MonthlyFeeCents.Value == nil {
			c.MonthlyFeeCents = nil
		} else {
			if *in.MonthlyFeeCents.Value < 1 || *in.MonthlyFeeCents.Value > validate.MaxCents {
				return ErrInvalidMonthlyFee.With("field", "monthly_fee_cents")
			}
			v := *in.MonthlyFeeCents.Value
			c.MonthlyFeeCents = &v
		}
	}
	if c.MonthlyFeeCents == nil {
		return apperr.ErrFieldRequired.With("field", "monthly_fee_cents")
	}
	var err error
	if c.ContractStart, err = applyDay(c.ContractStart, in.ContractStart, "contract_start"); err != nil {
		return err
	}
	if c.ContractStart == nil {
		return apperr.ErrFieldRequired.With("field", "contract_start")
	}
	if c.ContractEnd, err = applyDay(c.ContractEnd, in.ContractEnd, "contract_end"); err != nil {
		return err
	}
	if c.ContractEnd != nil {
		start, errStart := time.Parse(time.DateOnly, *c.ContractStart)
		end, errEnd := time.Parse(time.DateOnly, *c.ContractEnd)
		if errStart != nil || errEnd != nil || !end.After(start) {
			return ErrInvalidContractEnd.With("field", "contract_end")
		}
	}
	return nil
}

// applyDay aplica um dia de calendário (YYYY-MM-DD): ausente mantém o atual, texto vazio apaga e o resto precisa ser
// uma data que existe.
func applyDay(current, in *string, field string) (*string, error) {
	if in == nil {
		return current, nil
	}
	v := strings.TrimSpace(*in)
	if v == "" {
		return nil, nil
	}
	if _, err := time.Parse(time.DateOnly, v); err != nil {
		return nil, apperr.ErrFieldInvalid.With("field", field)
	}
	return &v, nil
}

// applyLateTerms aplica a multa, os juros ao mês (em pontos-base) e a tolerância do cliente mensal.
func applyLateTerms(c *Customer, in Input) error {
	if in.LateFeeBP != nil {
		if *in.LateFeeBP < 0 || *in.LateFeeBP > validate.MaxRateBP {
			return ErrInvalidLateFee.With("field", "late_fee_bp")
		}
		c.LateFeeBP = *in.LateFeeBP
	}
	if in.LateInterestBPMonth != nil {
		if *in.LateInterestBPMonth < 0 || *in.LateInterestBPMonth > validate.MaxRateBP {
			return ErrInvalidLateInterest.With("field", "late_interest_bp_month")
		}
		c.LateInterestBPMonth = *in.LateInterestBPMonth
	}
	if in.GraceDays != nil {
		if *in.GraceDays < 0 || *in.GraceDays > validate.MaxTermDays {
			return ErrInvalidGraceDays.With("field", "grace_days")
		}
		c.GraceDays = *in.GraceDays
	}
	return nil
}
