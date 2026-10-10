package team

import (
	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/validate"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

// teamName apara o nome e confere: obrigatório, de 1 a 120 caracteres. O erro diz o campo (name).
func teamName(name string) (string, error) {
	name, err := validate.Required("name", name, validate.MaxName)
	if apperr.Code(err) == apperr.ErrFieldRequired.Code {
		return "", ErrNameRequired.With("field", "name")
	}
	return name, err
}

func (s *Service) Create(projectID, name string) (*Team, error) {
	name, err := teamName(name)
	if err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	team := &Team{
		ProjectID: pid,
		Name:      name,
	}
	if err := s.store.Create(team); err != nil {
		return nil, err
	}
	return team, nil
}

func (s *Service) ListByProject(projectID string) ([]Team, error) {
	return s.store.ListByProject(projectID)
}

func (s *Service) Get(id string) (*Team, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id, name string) (*Team, error) {
	name, err := teamName(name)
	if err != nil {
		return nil, err
	}
	team, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	team.Name = name
	if err := s.store.Update(team); err != nil {
		return nil, err
	}
	return team, nil
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}
