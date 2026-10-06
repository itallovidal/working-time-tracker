package integration

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/integration"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(it *Integration) error {
	created, err := s.client.Integration.Create().
		SetProjectID(it.ProjectID).
		SetType(it.Type).
		SetDisplayName(it.DisplayName).
		SetCredentials(it.Credentials).
		SetMetadata(it.Metadata).
		SetEnabled(it.Enabled).
		Save(context.Background())
	if err != nil {
		return err
	}
	it.ID = created.ID
	it.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) ListByProject(projectID string) ([]Integration, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	integrations, err := s.client.Integration.Query().
		Where(integration.ProjectIDEQ(uid)).
		Order(ent.Desc(integration.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainIntegrations(integrations), nil
}

func (s *Store) GetByID(id string) (*Integration, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	it, err := s.client.Integration.Get(context.Background(), uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainIntegration(it), nil
}

func (s *Store) Update(it *Integration) error {
	_, err := s.client.Integration.UpdateOneID(it.ID).
		SetDisplayName(it.DisplayName).
		SetCredentials(it.Credentials).
		SetMetadata(it.Metadata).
		SetEnabled(it.Enabled).
		Save(context.Background())
	return err
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Integration.DeleteOneID(uid).Exec(context.Background())
}

func toDomainIntegration(e *ent.Integration) *Integration {
	if e == nil {
		return nil
	}
	// Uma linha de antes do metadata tem a coluna nula: a resposta leva um objeto vazio.
	metadata := e.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return &Integration{
		ID:          e.ID,
		ProjectID:   e.ProjectID,
		Type:        e.Type,
		DisplayName: e.DisplayName,
		Credentials: e.Credentials,
		Metadata:    metadata,
		Enabled:     e.Enabled,
		CreatedAt:   e.CreatedAt,
	}
}

func toDomainIntegrations(es []*ent.Integration) []Integration {
	result := make([]Integration, len(es))
	for i, e := range es {
		result[i] = *toDomainIntegration(e)
	}
	return result
}
