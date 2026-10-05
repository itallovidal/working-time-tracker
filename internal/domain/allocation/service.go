package allocation

import (
	"errors"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
)

var (
	ErrRateRequired   = errors.New("informe o valor por hora (pay_rate_cents)")
	ErrInvalidRate    = errors.New("o valor por hora deve ficar entre 0 e 1.000.000,00")
	ErrPersonNotInOrg = errors.New("pessoa não encontrada nesta organização")
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

// ids converte os dois IDs de uma vez. Um ID malformado vale como "não encontrado".
func ids(projectID, personID string) (uuid.UUID, uuid.UUID, error) {
	prj, err := uuid.Parse(projectID)
	if err != nil {
		return uuid.Nil, uuid.Nil, database.ErrNotFound
	}
	per, err := uuid.Parse(personID)
	if err != nil {
		return uuid.Nil, uuid.Nil, database.ErrNotFound
	}
	return prj, per, nil
}

// Set vincula a pessoa ao projeto com o valor dado, ou troca o valor de quem
// já está vinculado. Zero vale: é alguém que trabalha no projeto sem receber
// por hora.
func (s *Service) Set(projectID, personID string, payRateCents int) (*Allocation, error) {
	if payRateCents < 0 || payRateCents > MaxRateCents {
		return nil, ErrInvalidRate
	}
	prj, per, err := ids(projectID, personID)
	if err != nil {
		return nil, ErrPersonNotInOrg
	}
	inOrg, err := s.store.PersonInProjectOrganization(per, prj)
	if err != nil {
		return nil, err
	}
	if !inOrg {
		return nil, ErrPersonNotInOrg
	}
	if err := s.store.Set(prj, per, payRateCents); err != nil {
		return nil, err
	}
	return s.store.Get(prj, per)
}

func (s *Service) Remove(projectID, personID string) error {
	prj, per, err := ids(projectID, personID)
	if err != nil {
		return err
	}
	return s.store.Delete(prj, per)
}

func (s *Service) ListByProject(projectID string) ([]Allocation, error) {
	prj, err := uuid.Parse(projectID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	return s.store.ListByProject(prj)
}

// Get devolve o vínculo da pessoa no projeto, ou database.ErrNotFound.
func (s *Service) Get(projectID, personID string) (*Allocation, error) {
	prj, per, err := ids(projectID, personID)
	if err != nil {
		return nil, err
	}
	return s.store.Get(prj, per)
}

func (s *Service) ListByPerson(personID string) ([]Allocation, error) {
	per, err := uuid.Parse(personID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	return s.store.ListByPerson(per)
}
