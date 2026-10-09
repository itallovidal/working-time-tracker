package project

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
)

// maxBillRateCents é o teto do valor cobrado por hora: 1.000.000,00.
const maxBillRateCents = 100_000_000

// timePattern é o horário da daily e da weekly: HH:MM, de 00:00 a 23:59.
var timePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

var weekdays = map[string]bool{
	"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
	"friday": true, "saturday": true, "sunday": true,
}

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

// optional normaliza um campo opcional vindo da API: nil fica nil, e texto
// vazio vira um ponteiro para "" (pedido explícito para apagar).
func optional(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	return &t
}

// validate confere os formatos: HH:MM nos horários e um dia da semana em inglês nos dias.
func (r Routine) validate() error {
	if r.DailyTime != nil && *r.DailyTime != "" && !timePattern.MatchString(*r.DailyTime) {
		return ErrInvalidDailyTime
	}
	if err := validateSlot(r.WeeklySyncDay, r.WeeklySyncTime, ErrInvalidWeeklyTime); err != nil {
		return err
	}
	return validateSlot(r.CustomerMeetingDay, r.CustomerMeetingTime, ErrInvalidMeetingTime)
}

// validateSlot confere um dia da semana e um horário, cada um só se vier preenchido.
func validateSlot(day, at *string, errTime error) error {
	if day != nil && *day != "" && !weekdays[strings.ToLower(*day)] {
		return ErrInvalidWeekday
	}
	if at != nil && *at != "" && !timePattern.MatchString(*at) {
		return errTime
	}
	return nil
}

// normalized apara os textos e põe os dias em minúsculas; nil continua nil.
func (r Routine) normalized() Routine {
	lower := func(v *string) *string {
		if v = optional(v); v != nil {
			l := strings.ToLower(*v)
			return &l
		}
		return nil
	}
	return Routine{
		DailyTime:           optional(r.DailyTime),
		WeeklySyncDay:       lower(r.WeeklySyncDay),
		WeeklySyncTime:      optional(r.WeeklySyncTime),
		CustomerMeetingDay:  lower(r.CustomerMeetingDay),
		CustomerMeetingTime: optional(r.CustomerMeetingTime),
	}
}

// mergeSlot aplica um dia e um horário (nil mantém, vazio apaga) sobre os do
// projeto. O horário não existe sem o dia: quem apagou o dia não deixa um horário
// solto, e quem manda um horário precisa ter o dia.
func mergeSlot(curDay, curTime, day, at *string, errWithoutDay error) (*string, *string, error) {
	if day != nil {
		curDay = nilIfEmpty(day)
	}
	if at != nil {
		curTime = nilIfEmpty(at)
	}
	if curDay == nil {
		if nilIfEmpty(at) != nil {
			return nil, nil, errWithoutDay
		}
		curTime = nil
	}
	return curDay, curTime, nil
}

func nilIfEmpty(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

// Create cria o projeto. Sprint zerada vira 14 dias. Daily, weekly e reunião com o
// cliente são opcionais: sem horário da daily o projeto não tem daily, e sem dia da
// weekly ou da reunião não tem uma nem outra (e então o horário delas não vale).
func (s *Service) Create(orgID, name, description string, sprintDurationDays int, routine Routine) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	orgUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, ErrInvalidOrganization
	}
	if sprintDurationDays == 0 {
		sprintDurationDays = 14
	}
	if sprintDurationDays < 1 || sprintDurationDays > 90 {
		return nil, ErrInvalidSprint
	}
	routine = routine.normalized()
	if err := routine.validate(); err != nil {
		return nil, err
	}
	project := &Project{
		OrganizationID:     orgUID,
		Name:               name,
		Description:        strings.TrimSpace(description),
		SprintDurationDays: sprintDurationDays,
		DailyTime:          nilIfEmpty(routine.DailyTime),
	}
	if project.WeeklySyncDay, project.WeeklySyncTime, err = mergeSlot(nil, nil, routine.WeeklySyncDay, routine.WeeklySyncTime, ErrWeeklyTimeWithoutDay); err != nil {
		return nil, err
	}
	if project.CustomerMeetingDay, project.CustomerMeetingTime, err = mergeSlot(nil, nil, routine.CustomerMeetingDay, routine.CustomerMeetingTime, ErrMeetingTimeWithoutDay); err != nil {
		return nil, err
	}
	if err := s.store.Create(project); err != nil {
		return nil, err
	}
	return project, nil
}

// ListByOrg lista os projetos da organização. Com member, só os em que essa pessoa está, que é o que quem não
// é admin vê; nil lista todos.
func (s *Service) ListByOrg(orgID string, member *uuid.UUID) ([]Project, error) {
	return s.store.ListByOrg(orgID, member)
}

// CountByOrg conta todos os projetos da organização.
func (s *Service) CountByOrg(orgID string) (int, error) {
	return s.store.CountByOrg(orgID, nil)
}

// Tamanho da página da lista de projetos quando quem chama não diz, e o maior que aceita.
const (
	DefaultPerPage = 10
	MaxPerPage     = 100
)

// ListPage devolve uma página da lista de projetos com o total. perPage zero vale
// DefaultPerPage e acima de MaxPerPage vale MaxPerPage; uma página além da última volta como
// a última, e a organização sem projetos volta como a página 1, vazia. Com member, só os projetos em que essa
// pessoa está; nil lista todos.
func (s *Service) ListPage(orgID string, member *uuid.UUID, page, perPage int) (*Page, error) {
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	perPage = min(perPage, MaxPerPage)
	total, err := s.store.CountByOrg(orgID, member)
	if err != nil {
		return nil, err
	}
	last := max(1, (total+perPage-1)/perPage)
	page = min(max(page, 1), last)
	items, err := s.store.ListPageByOrg(orgID, member, page, perPage)
	if err != nil {
		return nil, err
	}
	return &Page{Items: items, Total: total, Page: page, PerPage: perPage}, nil
}

func (s *Service) Get(id string) (*Project, error) {
	return s.store.GetByID(id)
}

// Update altera o projeto. Nos campos opcionais, nil mantém o valor atual e
// texto vazio apaga; sprintDurationDays igual a zero mantém a duração atual.
// Apagar o dia da weekly ou da reunião apaga também o horário.
func (s *Service) Update(id, name, description string, sprintDurationDays int, routine Routine) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	if sprintDurationDays < 0 || sprintDurationDays > 90 {
		return nil, ErrInvalidSprint
	}
	routine = routine.normalized()
	if err := routine.validate(); err != nil {
		return nil, err
	}
	project, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	project.Name = name
	project.Description = strings.TrimSpace(description)
	if sprintDurationDays > 0 {
		project.SprintDurationDays = sprintDurationDays
	}
	if routine.DailyTime != nil {
		project.DailyTime = nilIfEmpty(routine.DailyTime)
	}
	if project.WeeklySyncDay, project.WeeklySyncTime, err = mergeSlot(project.WeeklySyncDay, project.WeeklySyncTime, routine.WeeklySyncDay, routine.WeeklySyncTime, ErrWeeklyTimeWithoutDay); err != nil {
		return nil, err
	}
	if project.CustomerMeetingDay, project.CustomerMeetingTime, err = mergeSlot(project.CustomerMeetingDay, project.CustomerMeetingTime, routine.CustomerMeetingDay, routine.CustomerMeetingTime, ErrMeetingTimeWithoutDay); err != nil {
		return nil, err
	}
	if err := s.store.Update(project); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}

func (s *Service) Billing(projectID string) (*Billing, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	return s.store.Billing(uid)
}

// SetBilling substitui o cliente e o valor cobrado do projeto. nil (ou cliente
// vazio) apaga: é assim que um projeto volta a ser interno.
func (s *Service) SetBilling(projectID string, customerID *string, billRateCents *int) (*Billing, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	if billRateCents != nil && (*billRateCents < 0 || *billRateCents > maxBillRateCents) {
		return nil, ErrInvalidBillRate
	}
	var customerUID *uuid.UUID
	if customerID != nil && strings.TrimSpace(*customerID) != "" {
		cid, err := uuid.Parse(strings.TrimSpace(*customerID))
		if err != nil {
			return nil, ErrCustomerNotFound
		}
		ok, err := s.store.CustomerInProjectOrganization(cid, uid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrCustomerNotFound
		}
		customerUID = &cid
	}
	if err := s.store.SetBilling(uid, customerUID, billRateCents); err != nil {
		return nil, err
	}
	return s.store.Billing(uid)
}
