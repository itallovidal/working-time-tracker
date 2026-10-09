package customer

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/internal/country"
	"working-time-tracker/internal/domain/person"
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
	c := &Customer{OrganizationID: orgUID}
	if in.Name == nil {
		return nil, ErrNameRequired
	}
	// O país do cliente novo é o da organização, a menos que venha outro.
	if in.Country == nil || strings.TrimSpace(*in.Country) == "" {
		orgCountry, err := s.store.OrgCountry(orgUID)
		if err != nil {
			return nil, err
		}
		in.Country = &orgCountry
	}
	if err := apply(c, in); err != nil {
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
	if err := apply(c, in); err != nil {
		return nil, err
	}
	if err := s.store.Update(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Delete recusa o cliente que ainda tem projetos, para nenhum projeto perder o
// cliente sem alguém decidir isso.
func (s *Service) Delete(id string) error {
	c, err := s.store.GetByID(id)
	if err != nil {
		return err
	}
	if c.ProjectCount > 0 {
		return ErrHasProjects
	}
	return s.store.Delete(c.ID)
}

func apply(c *Customer, in Input) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return ErrNameRequired
		}
		if utf8.RuneCountInString(name) > 120 {
			return ErrNameTooLong
		}
		c.Name = name
	}
	// O país vem antes do documento, que se confere pela regra dele. Aceita os países do cadastro e qualquer código ISO.
	countryChanged := false
	if in.Country != nil {
		code, ok := country.CustomerCountry(*in.Country)
		if !ok {
			return ErrInvalidCountry
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
				return ErrInvalidDocument
			}
			v = normalized
		}
		c.Document = v
	case countryChanged && c.Document != "":
		// O documento guardado é conferido pelo país novo, e um que não serve é recusado: nada é apagado em silêncio.
		normalized, ok := country.NormalizeTaxID(c.Country, c.Document)
		if !ok {
			return ErrInvalidDocument
		}
		c.Document = normalized
	}
	if in.ContactName != nil {
		v := strings.TrimSpace(*in.ContactName)
		if utf8.RuneCountInString(v) > 120 {
			return ErrContactTooLong
		}
		c.ContactName = v
	}
	if in.ContactEmail != nil {
		v := person.NormalizeEmail(*in.ContactEmail)
		if v != "" && (!person.ValidEmail(v) || utf8.RuneCountInString(v) > 255) {
			return ErrInvalidEmail
		}
		c.ContactEmail = v
	}
	if in.ContactPhone != nil {
		v := strings.TrimSpace(*in.ContactPhone)
		if v != "" && !validate.Phone(v) {
			return ErrInvalidPhone
		}
		c.ContactPhone = v
	}
	return nil
}
