package organization

import (
	"strings"
	"time"
	_ "time/tzdata" // o binário valida fusos mesmo onde o sistema não tem a base instalada
	"unicode/utf8"

	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/validate"
)

var (
	sizes      = map[string]bool{"1-10": true, "11-50": true, "51-200": true, "201-500": true, "500+": true}
	workModes  = map[string]bool{"remote": true, "hybrid": true, "onsite": true}
	currencies = map[string]bool{"BRL": true, "USD": true, "EUR": true}
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(name string) (*Organization, error) {
	if name == "" {
		return nil, ErrNameRequired
	}
	org := &Organization{Name: name}
	if err := s.store.Create(org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *Service) List() ([]Organization, error) {
	return s.store.List()
}

func (s *Service) Get(id string) (*Organization, error) {
	return s.store.GetByID(id)
}

// Update altera só os campos que vieram em in e valida cada um antes de gravar.
func (s *Service) Update(id string, in UpdateInput) (*Organization, error) {
	org, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := apply(org, in); err != nil {
		return nil, err
	}
	if err := s.store.Update(org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *Service) Delete(id string) error {
	hasProjects, err := s.store.HasActiveProjects(id)
	if err != nil {
		return err
	}
	if hasProjects {
		return ErrHasProjects
	}
	return s.store.Delete(id)
}

func apply(org *Organization, in UpdateInput) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return ErrNameRequired
		}
		if err := maxLen("name", name, 120); err != nil {
			return err
		}
		org.Name = name
	}

	// Textos livres: só o tamanho é conferido.
	for _, f := range []struct {
		dst   *string
		src   *string
		label string
		max   int
	}{
		{&org.Description, in.Description, "long_description", 2000},
		{&org.Industry, in.Industry, "industry", 100},
		{&org.LegalName, in.LegalName, "legal_name", 200},
		{&org.AddressLine1, in.AddressLine1, "address_line1", 200},
		{&org.AddressLine2, in.AddressLine2, "address_line2", 200},
		{&org.City, in.City, "city", 100},
		{&org.State, in.State, "state", 100},
		{&org.PostalCode, in.PostalCode, "postal_code", 16},
		{&org.Country, in.Country, "country", 100},
	} {
		if f.src == nil {
			continue
		}
		v := strings.TrimSpace(*f.src)
		if err := maxLen(f.label, v, f.max); err != nil {
			return err
		}
		*f.dst = v
	}

	if in.Summary != nil {
		// O resumo é uma linha só: quebras e espaços repetidos viram um espaço.
		v := strings.Join(strings.Fields(*in.Summary), " ")
		if err := maxLen("summary", v, 160); err != nil {
			return err
		}
		org.Summary = v
	}

	for _, f := range []struct{ dst, src *string }{
		{&org.Website, in.Website},
		{&org.LinkedinURL, in.LinkedinURL},
		{&org.InstagramURL, in.InstagramURL},
	} {
		if f.src == nil {
			continue
		}
		v := strings.TrimSpace(*f.src)
		if v != "" {
			normalized, ok := validate.HTTPURL(v)
			if !ok || utf8.RuneCountInString(normalized) > 255 {
				return ErrInvalidURL
			}
			v = normalized
		}
		*f.dst = v
	}

	if in.ContactEmail != nil {
		v := person.NormalizeEmail(*in.ContactEmail)
		if v != "" && (!person.ValidEmail(v) || utf8.RuneCountInString(v) > 255) {
			return ErrInvalidEmail
		}
		org.ContactEmail = v
	}
	if in.Phone != nil {
		v := strings.TrimSpace(*in.Phone)
		if v != "" && !validate.Phone(v) {
			return ErrInvalidPhone
		}
		org.Phone = v
	}
	if in.CNPJ != nil {
		v := strings.TrimSpace(*in.CNPJ)
		if v != "" {
			normalized, ok := validate.CNPJ(v)
			if !ok {
				return ErrInvalidCNPJ
			}
			v = normalized
		}
		org.CNPJ = v
	}
	if in.Size != nil {
		v := strings.TrimSpace(*in.Size)
		if v != "" && !sizes[v] {
			return ErrInvalidSize
		}
		org.Size = v
	}

	if in.FoundedYear != nil {
		if *in.FoundedYear != 0 && (*in.FoundedYear < 1900 || *in.FoundedYear > time.Now().Year()) {
			return ErrInvalidFoundedYear
		}
		org.FoundedYear = nilIfZero(*in.FoundedYear)
	}
	if in.WorkMode != nil {
		v := strings.ToLower(strings.TrimSpace(*in.WorkMode))
		if v != "" && !workModes[v] {
			return ErrInvalidWorkMode
		}
		org.WorkMode = v
	}

	// Fuso e moeda sempre têm valor: vazio volta para o padrão.
	if in.Timezone != nil {
		v := strings.TrimSpace(*in.Timezone)
		if v == "" {
			v = DefaultTimezone
		}
		if _, err := time.LoadLocation(v); err != nil || v == "Local" {
			return ErrInvalidTimezone
		}
		org.Timezone = v
	}
	if in.Currency != nil {
		v := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if v == "" {
			v = DefaultCurrency
		}
		if !currencies[v] {
			return ErrInvalidCurrency
		}
		org.Currency = v
	}
	return nil
}

// maxLen confere o tamanho de um campo de texto. field é o nome do campo na API,
// e o cliente mostra o rótulo dele.
func maxLen(field, v string, max int) error {
	if utf8.RuneCountInString(v) > max {
		return ErrFieldTooLong.With("field", field, "max", max)
	}
	return nil
}

func nilIfZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
