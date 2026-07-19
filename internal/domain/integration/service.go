package integration

import (
	"errors"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
)

type Service struct {
	store      *Store
	encryptKey string
}

func NewService(store *Store, encryptKey string) *Service {
	return &Service{store: store, encryptKey: encryptKey}
}

func (s *Service) Create(projectID, integrationType, displayName string, config map[string]interface{}, enabled bool) (*Integration, error) {
	if displayName == "" {
		return nil, errors.New("display_name is required")
	}

	impl, err := adapter.GetIntegration(integrationType)
	if err != nil {
		return nil, err
	}

	if err := impl.ValidateConfig(config); err != nil {
		return nil, err
	}

	encrypted, err := adapter.EncryptConfig(config, s.encryptKey)
	if err != nil {
		return nil, err
	}

	it := &Integration{
		ProjectID:   uuid.MustParse(projectID),
		Type:        integrationType,
		DisplayName: displayName,
		Config:      encrypted,
		Enabled:     enabled,
	}
	if err := s.store.Create(it); err != nil {
		return nil, err
	}
	it.Config = nil
	return it, nil
}

func (s *Service) ListByProject(projectID string) ([]Integration, error) {
	integrations, err := s.store.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	for i := range integrations {
		integrations[i].Config = nil
	}
	return integrations, nil
}

func (s *Service) Get(id string) (*Integration, error) {
	it, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	it.Config = nil
	return it, nil
}

func (s *Service) Update(id, displayName string, config map[string]interface{}, enabled *bool) (*Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}

	if displayName != "" {
		existing.DisplayName = displayName
	}

	if config != nil {
		impl, err := adapter.GetIntegration(existing.Type)
		if err != nil {
			return nil, err
		}
		if err := impl.ValidateConfig(config); err != nil {
			return nil, err
		}
		encrypted, err := adapter.EncryptConfig(config, s.encryptKey)
		if err != nil {
			return nil, err
		}
		existing.Config = encrypted
	}

	if enabled != nil {
		existing.Enabled = *enabled
	}

	if err := s.store.Update(existing); err != nil {
		return nil, err
	}
	existing.Config = nil
	return existing, nil
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}

func (s *Service) ValidateConfig(integrationType string, config map[string]interface{}) error {
	impl, err := adapter.GetIntegration(integrationType)
	if err != nil {
		return err
	}
	return impl.ValidateConfig(config)
}

func (s *Service) FetchItemDetails(integrationID, itemID string) (*adapter.ExternalDetailsResult, error) {
	existing, err := s.store.GetByID(integrationID)
	if err != nil {
		return nil, err
	}

	decrypted, err := adapter.DecryptConfig(existing.Config, s.encryptKey)
	if err != nil {
		return nil, err
	}

	impl, err := adapter.GetIntegration(existing.Type)
	if err != nil {
		return nil, err
	}

	details, err := impl.FetchItemDetails(decrypted, itemID)
	if err != nil {
		errMsg := err.Error()
		return &adapter.ExternalDetailsResult{
			Details: nil,
			Error:   &errMsg,
		}, nil
	}

	return &adapter.ExternalDetailsResult{
		Details: details,
	}, nil
}
