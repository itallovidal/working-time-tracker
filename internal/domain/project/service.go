package project

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
)

// maxBillRateCents é o teto do valor cobrado por hora: 1.000.000,00.
const maxBillRateCents = 100_000_000

var dailyTimePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

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

func validateSchedule(dailyTime, weeklySyncDay *string) error {
	if dailyTime != nil && *dailyTime != "" && !dailyTimePattern.MatchString(*dailyTime) {
		return ErrInvalidDailyTime
	}
	if weeklySyncDay != nil && *weeklySyncDay != "" && !weekdays[strings.ToLower(*weeklySyncDay)] {
		return ErrInvalidWeekday
	}
	return nil
}

func validateWeeklyHours(v *int) error {
	if v != nil && (*v < 0 || *v > 168) {
		return ErrInvalidWeekHours
	}
	return nil
}

func nilIfZero(v *int) *int {
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

func nilIfEmpty(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

// Create cria o projeto. Sprint zerada vira 14 dias; jornada semanal nil ou zero
// fica sem valor.
func (s *Service) Create(orgID, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string, weeklyHours *int) (*Project, error) {
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
	if err := validateWeeklyHours(weeklyHours); err != nil {
		return nil, err
	}
	dailyTime, weeklySyncDay = optional(dailyTime), optional(weeklySyncDay)
	if err := validateSchedule(dailyTime, weeklySyncDay); err != nil {
		return nil, err
	}
	if weeklySyncDay != nil {
		lower := strings.ToLower(*weeklySyncDay)
		weeklySyncDay = &lower
	}
	project := &Project{
		OrganizationID:     orgUID,
		Name:               name,
		Description:        strings.TrimSpace(description),
		SprintDurationDays: sprintDurationDays,
		WeeklyHours:        nilIfZero(weeklyHours),
		DailyTime:          nilIfEmpty(dailyTime),
		WeeklySyncDay:      nilIfEmpty(weeklySyncDay),
	}
	if err := s.store.Create(project); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) ListByOrg(orgID string) ([]Project, error) {
	return s.store.ListByOrg(orgID)
}

func (s *Service) Get(id string) (*Project, error) {
	return s.store.GetByID(id)
}

// Update altera o projeto. Nos campos opcionais, nil mantém o valor atual e
// texto vazio (ou jornada zero) apaga; sprintDurationDays igual a zero mantém a
// duração atual.
func (s *Service) Update(id, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string, weeklyHours *int) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	if sprintDurationDays < 0 || sprintDurationDays > 90 {
		return nil, ErrInvalidSprint
	}
	if err := validateWeeklyHours(weeklyHours); err != nil {
		return nil, err
	}
	dailyTime, weeklySyncDay = optional(dailyTime), optional(weeklySyncDay)
	if err := validateSchedule(dailyTime, weeklySyncDay); err != nil {
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
	if weeklyHours != nil {
		project.WeeklyHours = nilIfZero(weeklyHours)
	}
	if dailyTime != nil {
		project.DailyTime = nilIfEmpty(dailyTime)
	}
	if weeklySyncDay != nil {
		lower := strings.ToLower(*weeklySyncDay)
		project.WeeklySyncDay = nilIfEmpty(&lower)
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
