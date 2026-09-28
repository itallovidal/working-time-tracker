package project

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNameRequired     = errors.New("informe o nome do projeto")
	ErrInvalidSprint    = errors.New("a sprint precisa ter entre 1 e 90 dias")
	ErrInvalidDailyTime = errors.New("o horário da daily deve estar no formato HH:MM, por exemplo 09:30")
	ErrInvalidWeekday   = errors.New("dia da weekly inválido: use monday, tuesday, wednesday, thursday, friday, saturday ou sunday")
)

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

func nilIfEmpty(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

func (s *Service) Create(orgID, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	orgUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, errors.New("organização inválida")
	}
	if sprintDurationDays == 0 {
		sprintDurationDays = 14
	}
	if sprintDurationDays < 1 || sprintDurationDays > 90 {
		return nil, ErrInvalidSprint
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
// texto vazio apaga; sprintDurationDays igual a zero mantém a duração atual.
func (s *Service) Update(id, name, description string, sprintDurationDays int, dailyTime, weeklySyncDay *string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	if sprintDurationDays < 0 || sprintDurationDays > 90 {
		return nil, ErrInvalidSprint
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
