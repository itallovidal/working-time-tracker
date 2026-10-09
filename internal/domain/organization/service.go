package organization

import (
	"strings"
	"time"
	_ "time/tzdata" // o binário valida fusos mesmo onde o sistema não tem a base instalada
	"unicode/utf8"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/country"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/validate"
)

var (
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
		{&org.LegalName, in.LegalName, "legal_name", 200},
		{&org.AddressLine1, in.AddressLine1, "address_line1", 200},
		{&org.AddressLine2, in.AddressLine2, "address_line2", 200},
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

	// País: o código do cadastro, e vazio volta para o padrão. Vem antes do que depende dele (o fuso e a moeda padrão).
	if in.Country != nil {
		code := country.Default
		if v := strings.TrimSpace(*in.Country); v != "" {
			parsed, ok := country.Parse(v)
			if !ok {
				return ErrInvalidCountry
			}
			code = parsed
		}
		org.Country = code
	}
	profile := profileOf(org.Country)

	for _, f := range []struct{ dst, src *string }{
		{&org.Website, in.Website},
		{&org.LinkedinURL, in.LinkedinURL},
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

	// Os documentos fiscais, um por país do cadastro. Cada um é conferido pela regra do país dono dele, qualquer que seja o
	// país da organização: o PATCH {"country":"US","ein":"..."} não depende da ordem dos campos, e trocar de país não
	// apaga o documento do outro.
	refs := legalIDRefs(org, in)
	for _, c := range country.All() {
		ref, ok := refs[c.LegalID.Field]
		if !ok || ref.src == nil {
			continue
		}
		v := strings.TrimSpace(*ref.src)
		if v != "" {
			normalized, ok := c.LegalID.Normalize(v)
			if !ok {
				return ref.err
			}
			v = normalized
		}
		*ref.dst = v
	}

	if in.WorkMode != nil {
		v := strings.ToLower(strings.TrimSpace(*in.WorkMode))
		if v != "" && !workModes[v] {
			return ErrInvalidWorkMode
		}
		org.WorkMode = v
	}

	// Fuso e moeda sempre têm valor: vazio volta para o padrão do país.
	if in.Timezone != nil {
		v := strings.TrimSpace(*in.Timezone)
		if v == "" {
			v = profile.Timezone
		}
		if _, err := time.LoadLocation(v); err != nil || v == "Local" {
			return ErrInvalidTimezone
		}
		org.Timezone = v
	}
	if in.Currency != nil {
		v := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if v == "" {
			v = profile.Currency
		}
		if !currencies[v] {
			return ErrInvalidCurrency
		}
		org.Currency = v
	}
	return nil
}

// profileOf devolve as regras do país de uma organização. Um código que o cadastro não conhece (um dado antigo, escrito à
// mão) cai no país padrão, para a organização continuar editável.
func profileOf(code string) *country.Country {
	if c, ok := country.Get(code); ok {
		return c
	}
	return country.MustGet(country.Default)
}

// legalIDRef liga um documento fiscal à coluna da organização, ao valor que veio no PATCH e ao erro dele.
type legalIDRef struct {
	dst *string
	src *string
	err *apperr.Error
}

// legalIDRefs liga o documento de cada país do cadastro (country.LegalID.Field) à coluna da organização. É a lista a
// mexer quando um país com documento novo entra; um teste confere que nenhum documento do cadastro ficou de fora.
func legalIDRefs(org *Organization, in UpdateInput) map[string]legalIDRef {
	return map[string]legalIDRef{
		"cnpj": {&org.CNPJ, in.CNPJ, ErrInvalidCNPJ},
		"ein":  {&org.EIN, in.EIN, ErrInvalidEIN},
	}
}

// maxLen confere o tamanho de um campo de texto. field é o nome do campo na API,
// e o cliente mostra o rótulo dele.
func maxLen(field, v string, max int) error {
	if utf8.RuneCountInString(v) > max {
		return ErrFieldTooLong.With("field", field, "max", max)
	}
	return nil
}
