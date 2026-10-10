package customer

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

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
	c := &Customer{OrganizationID: orgUID}
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
