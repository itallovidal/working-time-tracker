package organization

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // o binário valida fusos mesmo onde o sistema não tem a base instalada
	"unicode/utf8"

	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/validate"
)

var (
	ErrNameRequired       = errors.New("informe o nome")
	ErrInvalidFoundedYear = errors.New("o ano de fundação deve ficar entre 1900 e o ano atual")
	ErrInvalidSize        = errors.New("porte inválido: use 1-10, 11-50, 51-200, 201-500 ou 500+")
	ErrInvalidURL         = errors.New("endereço inválido: use um link http ou https, por exemplo https://exemplo.com.br")
	ErrInvalidEmail       = errors.New("informe um email de contato válido")
	ErrInvalidPhone       = errors.New("telefone inválido: use números, espaços, +, parênteses e hífen")
	ErrInvalidCNPJ        = errors.New("CNPJ inválido: confira os números e os dígitos verificadores")
	ErrInvalidTimezone    = errors.New("fuso horário inválido: use um nome como America/Sao_Paulo")
	ErrInvalidWeeklyHours = errors.New("a jornada semanal deve ficar entre 1 e 168 horas")
	ErrInvalidSprint      = errors.New("a sprint padrão precisa ter entre 1 e 90 dias")
	ErrInvalidCurrency    = errors.New("moeda inválida: use BRL, USD ou EUR")
)

var (
	sizes        = map[string]bool{"1-10": true, "11-50": true, "51-200": true, "201-500": true, "500+": true}
	currencies   = map[string]bool{"BRL": true, "USD": true, "EUR": true}
	phonePattern = regexp.MustCompile(`^[0-9+() .-]{8,32}$`)
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
		return errors.New("exclua todos os projetos antes de excluir a organização")
	}
	return s.store.Delete(id)
}

func apply(org *Organization, in UpdateInput) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return ErrNameRequired
		}
		if err := maxLen("o nome", name, 120); err != nil {
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
		{&org.Description, in.Description, "a descrição", 2000},
		{&org.Industry, in.Industry, "o segmento", 100},
		{&org.LegalName, in.LegalName, "a razão social", 200},
		{&org.AddressLine1, in.AddressLine1, "o endereço", 200},
		{&org.AddressLine2, in.AddressLine2, "o complemento", 200},
		{&org.City, in.City, "a cidade", 100},
		{&org.State, in.State, "o estado", 100},
		{&org.PostalCode, in.PostalCode, "o CEP", 16},
		{&org.Country, in.Country, "o país", 100},
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
		if err := maxLen("o resumo", v, 160); err != nil {
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
		if v != "" && !phonePattern.MatchString(v) {
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
	if in.WeeklyHours != nil {
		if *in.WeeklyHours < 0 || *in.WeeklyHours > 168 {
			return ErrInvalidWeeklyHours
		}
		org.WeeklyHours = nilIfZero(*in.WeeklyHours)
	}
	if in.DefaultSprintDays != nil {
		if *in.DefaultSprintDays < 0 || *in.DefaultSprintDays > 90 {
			return ErrInvalidSprint
		}
		org.DefaultSprintDays = nilIfZero(*in.DefaultSprintDays)
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

func maxLen(label, v string, max int) error {
	if utf8.RuneCountInString(v) > max {
		return fmt.Errorf("%s pode ter até %d caracteres", label, max)
	}
	return nil
}

func nilIfZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
