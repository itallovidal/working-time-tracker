package allocation

import (
	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/validate"
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

// Set põe a pessoa no projeto com o valor dado, ou troca o valor de quem já
// está nele. O valor tem o piso de validate.MinRateCents: ninguém trabalha de graça, e só o dono entra com 0
// (a checagem do dono vem antes da do valor). Para tirar a pessoa do projeto, o caminho é collaborator.Service.Remove, que
// apaga o valor e os times juntos: não há como ficar no projeto sem valor.
func (s *Service) Set(projectID, personID string, payRateCents int) (*Allocation, error) {
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
	// O dono não recebe valor por hora: o que ele tira do projeto é a margem, e as horas
	// dele valem o valor cobrado. Quem o adiciona ao projeto não escolhe um valor.
	if owner, err := s.store.IsOwner(per); err != nil {
		return nil, err
	} else if owner {
		payRateCents = 0
	} else if validate.Rate("pay_rate_cents", payRateCents) != nil {
		return nil, ErrInvalidRate.With("field", "pay_rate_cents")
	}
	if err := s.store.Set(prj, per, payRateCents); err != nil {
		return nil, err
	}
	return s.store.Get(prj, per)
}

// SetPreset dá à pessoa o grupo de permissões do projeto, que já tem de estar nele. O
// grupo troca as permissões dela de uma vez: as que vieram de um grupo anterior saem.
func (s *Service) SetPreset(projectID, personID, presetID string) (*Allocation, error) {
	preset, ok := permission.PresetByID(presetID)
	if !ok {
		return nil, ErrInvalidPreset
	}
	prj, per, err := ids(projectID, personID)
	if err != nil {
		return nil, ErrPersonNotInOrg
	}
	if err := s.store.SetPreset(prj, per, preset.ID, preset.Permissions); err != nil {
		return nil, err
	}
	return s.store.Get(prj, per)
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
