package project

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/validate"
)

// Sprint: a duração vai de 1 a maxSprintDays dias. Sem valor, o projeto novo usa defaultSprintDays.
//
// Zero não é um valor válido de sprint. Na API, o campo ausente vale "não informado" (14 na criação, manter o atual
// na edição) e um 0 enviado é recusado nas duas rotas com project.invalid_sprint. Quem chama o serviço pelo Go,
// com um int, usa 0 como "não informado" na criação (Create), e a edição recebe um ponteiro: nil mantém.
const (
	defaultSprintDays = 14
	maxSprintDays     = 90
)

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
		return ErrInvalidDailyTime.With("field", "daily_time")
	}
	if err := validateSlot(r.WeeklySyncDay, r.WeeklySyncTime, "weekly_sync_day", "weekly_sync_time", ErrInvalidWeeklyTime); err != nil {
		return err
	}
	return validateSlot(r.CustomerMeetingDay, r.CustomerMeetingTime, "customer_meeting_day", "customer_meeting_time", ErrInvalidMeetingTime)
}

// validateSlot confere um dia da semana e um horário, cada um só se vier preenchido. dayField e timeField são as
// chaves do corpo, que o erro devolve.
func validateSlot(day, at *string, dayField, timeField string, errTime *apperr.Error) error {
	if day != nil && *day != "" && !weekdays[strings.ToLower(*day)] {
		return ErrInvalidWeekday.With("field", dayField)
	}
	if at != nil && *at != "" && !timePattern.MatchString(*at) {
		return errTime.With("field", timeField)
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
func mergeSlot(curDay, curTime, day, at *string, errWithoutDay *apperr.Error, timeField string) (*string, *string, error) {
	if day != nil {
		curDay = nilIfEmpty(day)
	}
	if at != nil {
		curTime = nilIfEmpty(at)
	}
	if curDay == nil {
		if nilIfEmpty(at) != nil {
			return nil, nil, errWithoutDay.With("field", timeField)
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

// nameAndDescription confere e apara o nome (obrigatório, 1 a 120) e a descrição (até 2.000), na criação e na edição.
func nameAndDescription(name string, description *string) (string, *string, error) {
	name, err := validate.Required("name", name, validate.MaxName)
	if err != nil {
		if apperr.Code(err) == apperr.ErrFieldRequired.Code {
			return "", nil, ErrNameRequired.With("field", "name")
		}
		return "", nil, err
	}
	if description == nil {
		return name, nil, nil
	}
	d, err := validate.Text("long_description", *description, validate.MaxDescription)
	if err != nil {
		return "", nil, err
	}
	return name, &d, nil
}

// meetingNeedsCustomer recusa a reunião com o cliente (um dia preenchido) quando o projeto não tem cliente.
func meetingNeedsCustomer(r Routine, hasCustomer bool) error {
	if !hasCustomer && r.CustomerMeetingDay != nil && *r.CustomerMeetingDay != "" {
		return ErrMeetingNeedsCustomer.With("field", "customer_meeting_day")
	}
	return nil
}

// CreateInput é o que a criação do projeto aceita, além da organização.
type CreateInput struct {
	Name        string
	Description string
	// SprintDurationDays zero vale 14.
	SprintDurationDays int
	Routine            Routine
	// CustomerID é o cliente do projeto novo (vazio: projeto interno). A reunião com o cliente só vale com ele.
	CustomerID string
}

// Create cria o projeto sem cliente (ver CreateWithCustomer). Sprint zerada vira 14 dias. Daily, weekly e
// reunião com o cliente são opcionais: sem horário da daily o projeto não tem daily, e sem dia da weekly ou da
// reunião não tem uma nem outra (e então o horário delas não vale). Um projeto sem cliente não aceita a
// reunião com o cliente.
func (s *Service) Create(orgID, name, description string, sprintDurationDays int, routine Routine) (*Project, error) {
	return s.CreateWithCustomer(orgID, CreateInput{
		Name: name, Description: description, SprintDurationDays: sprintDurationDays, Routine: routine,
	})
}

// CreateWithCustomer é Create com o cliente do projeto já escolhido, para a reunião com ele valer na criação.
func (s *Service) CreateWithCustomer(orgID string, in CreateInput) (*Project, error) {
	name, description, err := nameAndDescription(in.Name, &in.Description)
	if err != nil {
		return nil, err
	}
	orgUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, ErrInvalidOrganization
	}
	sprint := in.SprintDurationDays
	if sprint == 0 {
		sprint = defaultSprintDays
	}
	if sprint < 1 || sprint > maxSprintDays {
		return nil, ErrInvalidSprint.With("field", "sprint_duration_days")
	}
	routine := in.Routine.normalized()
	if err := routine.validate(); err != nil {
		return nil, err
	}
	var customerUID *uuid.UUID
	if cid := strings.TrimSpace(in.CustomerID); cid != "" {
		uid, err := uuid.Parse(cid)
		if err != nil {
			return nil, ErrCustomerNotFound.With("field", "customer_id")
		}
		ok, err := s.store.CustomerInOrganization(uid, orgUID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrCustomerNotFound.With("field", "customer_id")
		}
		customerUID = &uid
	}
	if err := meetingNeedsCustomer(routine, customerUID != nil); err != nil {
		return nil, err
	}
	project := &Project{
		OrganizationID:     orgUID,
		Name:               name,
		Description:        *description,
		SprintDurationDays: sprint,
		DailyTime:          nilIfEmpty(routine.DailyTime),
	}
	if customerUID != nil {
		project.Customer = &CustomerRef{ID: *customerUID}
	}
	if project.WeeklySyncDay, project.WeeklySyncTime, err = mergeSlot(nil, nil, routine.WeeklySyncDay, routine.WeeklySyncTime, ErrWeeklyTimeWithoutDay, "weekly_sync_time"); err != nil {
		return nil, err
	}
	if project.CustomerMeetingDay, project.CustomerMeetingTime, err = mergeSlot(nil, nil, routine.CustomerMeetingDay, routine.CustomerMeetingTime, ErrMeetingTimeWithoutDay, "customer_meeting_time"); err != nil {
		return nil, err
	}
	if err := s.store.Create(project); err != nil {
		return nil, err
	}
	if customerUID != nil {
		// Relê para o cliente voltar com o nome, como em qualquer outra resposta.
		return s.store.GetByID(project.ID.String())
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

// Update altera o projeto. Nos campos opcionais, nil mantém o valor atual e texto vazio apaga: a descrição é um
// *string pelo mesmo motivo (omitir mantém, "" apaga). sprintDurationDays nil mantém a duração atual; um valor
// fora de 1 a 90, inclusive 0, é recusado, igual à criação pela API. Apagar o dia da weekly ou da reunião apaga
// também o horário. Marcar a reunião com o cliente em um projeto sem cliente é recusado.
func (s *Service) Update(id, name string, description *string, sprintDurationDays *int, routine Routine) (*Project, error) {
	name, description, err := nameAndDescription(name, description)
	if err != nil {
		return nil, err
	}
	if sprintDurationDays != nil && (*sprintDurationDays < 1 || *sprintDurationDays > maxSprintDays) {
		return nil, ErrInvalidSprint.With("field", "sprint_duration_days")
	}
	routine = routine.normalized()
	if err := routine.validate(); err != nil {
		return nil, err
	}
	project, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := meetingNeedsCustomer(routine, project.Customer != nil); err != nil {
		return nil, err
	}
	project.Name = name
	if description != nil {
		project.Description = *description
	}
	if sprintDurationDays != nil {
		project.SprintDurationDays = *sprintDurationDays
	}
	if routine.DailyTime != nil {
		project.DailyTime = nilIfEmpty(routine.DailyTime)
	}
	if project.WeeklySyncDay, project.WeeklySyncTime, err = mergeSlot(project.WeeklySyncDay, project.WeeklySyncTime, routine.WeeklySyncDay, routine.WeeklySyncTime, ErrWeeklyTimeWithoutDay, "weekly_sync_time"); err != nil {
		return nil, err
	}
	if project.CustomerMeetingDay, project.CustomerMeetingTime, err = mergeSlot(project.CustomerMeetingDay, project.CustomerMeetingTime, routine.CustomerMeetingDay, routine.CustomerMeetingTime, ErrMeetingTimeWithoutDay, "customer_meeting_time"); err != nil {
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
	if billRateCents != nil && validate.Money("bill_rate_cents", *billRateCents) != nil {
		return nil, ErrInvalidBillRate.With("field", "bill_rate_cents")
	}
	var customerUID *uuid.UUID
	if customerID != nil && strings.TrimSpace(*customerID) != "" {
		cid, err := uuid.Parse(strings.TrimSpace(*customerID))
		if err != nil {
			return nil, ErrCustomerNotFound.With("field", "customer_id")
		}
		ok, err := s.store.CustomerInProjectOrganization(cid, uid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrCustomerNotFound.With("field", "customer_id")
		}
		customerUID = &cid
	}
	if err := s.store.SetBilling(uid, customerUID, billRateCents); err != nil {
		return nil, err
	}
	return s.store.Billing(uid)
}
