package person

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/validate"
)

// NormalizeEmail tira espaços e deixa o email em minúsculas, para que
// "Ana@X.com" e "ana@x.com " sejam a mesma conta.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail confere o formato local@dominio.tld e o tamanho; a confirmação real seria por email.
func ValidEmail(email string) bool {
	return validate.EmailFormat(email)
}

// ParseEmail normaliza o e-mail de uma pessoa e confere o tamanho (até 255) e o formato local@dominio.tld. field é a
// chave do campo no corpo da requisição, para a tela mostrar o erro embaixo dele. Vazio é recusado como e-mail
// inválido: quem aceita o campo vazio confere antes.
func ParseEmail(field, raw string) (string, error) {
	email := NormalizeEmail(raw)
	if utf8.RuneCountInString(email) > validate.MaxEmail {
		return "", apperr.ErrFieldTooLong.With("field", field, "max", validate.MaxEmail)
	}
	if !ValidEmail(email) {
		return "", ErrInvalidEmail.With("field", field)
	}
	return email, nil
}

// parseProfile confere o nome e o e-mail de uma pessoa: o nome sem espaços nas pontas, de 1 a 120 caracteres, e o
// e-mail obrigatório.
func parseProfile(name, email string) (string, string, error) {
	name, err := validate.Text("name", name, validate.MaxName)
	if err != nil {
		return "", "", err
	}
	if name == "" {
		return "", "", ErrNameRequired.With("field", "name")
	}
	if NormalizeEmail(email) == "" {
		return "", "", ErrEmailRequired.With("field", "email")
	}
	email, err = ParseEmail("email", email)
	if err != nil {
		return "", "", err
	}
	return name, email, nil
}

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(orgID, name, email string) (*Person, error) {
	name, email, err := parseProfile(name, email)
	if err != nil {
		return nil, err
	}
	orgUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, ErrInvalidOrganization
	}
	inUse, err := s.store.EmailInUse(email, nil)
	if err != nil {
		return nil, err
	}
	if inUse {
		return nil, ErrEmailInUse.With("field", "email")
	}
	person := &Person{
		Name:           name,
		Email:          email,
		OrganizationID: orgUID,
	}
	if err := s.store.Create(person); err != nil {
		return nil, err
	}
	return person, nil
}

func (s *Service) ListByOrg(orgID string) ([]Person, error) {
	return s.store.ListByOrg(orgID)
}

// ListVisible lista as pessoas da organização que quem pede pode ver: com viewer, só ele e quem divide projeto
// com ele; nil lista todas (admins e quem cuida de pessoas).
func (s *Service) ListVisible(orgID string, viewer *uuid.UUID) ([]Person, error) {
	return s.store.ListByOrgScoped(orgID, viewer)
}

func (s *Service) Get(id string) (*Person, error) {
	return s.store.GetByID(id)
}

// GetVisible devolve a pessoa se quem pede pode vê-la, ou database.ErrNotFound: com viewer, só ele mesmo e quem
// divide projeto com ele; nil vê todas.
func (s *Service) GetVisible(id string, viewer *uuid.UUID) (*Person, error) {
	if viewer != nil {
		pid, err := uuid.Parse(id)
		if err != nil {
			return nil, database.ErrNotFound
		}
		ok, err := s.store.Visible(pid, *viewer)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, database.ErrNotFound
		}
	}
	return s.store.GetByID(id)
}

// FindByEmailInOrg devolve a pessoa da organização com este e-mail, ou database.ErrNotFound.
func (s *Service) FindByEmailInOrg(orgID uuid.UUID, email string) (*Person, error) {
	return s.store.FindByEmailInOrg(orgID, email)
}

func (s *Service) Update(id, name, email string) (*Person, error) {
	name, email, err := parseProfile(name, email)
	if err != nil {
		return nil, err
	}
	person, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	inUse, err := s.store.EmailInUse(email, &person.ID)
	if err != nil {
		return nil, err
	}
	if inUse {
		return nil, ErrEmailInUse.With("field", "email")
	}
	person.Name = name
	person.Email = email
	if err := s.store.Update(person); err != nil {
		return nil, err
	}
	return person, nil
}

// SetRole muda o papel de uma pessoa, sem deixar a organização sem admin. A
// checagem do último admin fica no store, na mesma transação da mudança.
func (s *Service) SetRole(id, role string) (*Person, error) {
	if role != RoleAdmin && role != RoleMember {
		return nil, ErrInvalidRole.With("field", "role")
	}
	return s.store.SetRole(id, role)
}

// SetWeeklyHours define a jornada semanal da pessoa. nil ou zero apaga.
func (s *Service) SetWeeklyHours(id string, hours *int) (*Person, error) {
	if hours != nil && (*hours < 0 || *hours > 168) {
		return nil, ErrInvalidWeekHours.With("field", "weekly_hours")
	}
	if hours != nil && *hours == 0 {
		hours = nil
	}
	return s.store.SetWeeklyHours(id, hours)
}

// SetPayment define a regra de pagamento da pessoa. Mensal pede o dia (1 a 31) e descarta o início; quinzenal
// pede um início válido (YYYY-MM-DD) e descarta o dia; frequência vazia apaga a regra.
func (s *Service) SetPayment(id string, in PaymentRule) (*Person, error) {
	switch in.Frequency {
	case "":
		return s.store.SetPayment(id, nil)
	case PaymentMonthly:
		if in.Day < 1 || in.Day > 31 {
			return nil, ErrInvalidPayDay.With("field", "day")
		}
		return s.store.SetPayment(id, &PaymentRule{Frequency: PaymentMonthly, Day: in.Day})
	case PaymentBiweekly:
		start := strings.TrimSpace(in.Start)
		t, err := time.Parse(time.DateOnly, start)
		if err != nil {
			return nil, ErrInvalidPayStart.With("field", "start")
		}
		return s.store.SetPayment(id, &PaymentRule{Frequency: PaymentBiweekly, Start: t.Format(time.DateOnly)})
	}
	return nil, ErrInvalidPayFrequency.With("field", "frequency")
}
