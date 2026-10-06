package collaborator

import (
	"github.com/google/uuid"

	"working-time-tracker/internal/database"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) ListByProject(projectID string) ([]Collaborator, error) {
	prj, err := uuid.Parse(projectID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	return s.store.ListByProject(prj)
}

// Remove tira a pessoa do projeto: do valor por hora e de todos os times dele.
// Um ID malformado vale como "não encontrado".
func (s *Service) Remove(projectID, personID string) error {
	prj, err := uuid.Parse(projectID)
	if err != nil {
		return database.ErrNotFound
	}
	per, err := uuid.Parse(personID)
	if err != nil {
		return database.ErrNotFound
	}
	return s.store.Remove(prj, per)
}
