package service

import (
	"errors"
	"working-time-tracker/internal/integration"
	"working-time-tracker/internal/model"
	"working-time-tracker/internal/store"

	"github.com/google/uuid"
)

type IntegrationService struct {
	store      *store.IntegrationStore
	encryptKey string
}

func NewIntegrationService(store *store.IntegrationStore, encryptKey string) *IntegrationService {
	return &IntegrationService{store: store, encryptKey: encryptKey}
}

func (s *IntegrationService) Create(projectID, integrationType, displayName string, config map[string]interface{}, enabled bool) (*model.Integration, error) {
	if displayName == "" {
		return nil, errors.New("display_name is required")
	}

	impl, err := integration.GetIntegration(integrationType)
	if err != nil {
		return nil, err
	}

	if err := impl.ValidateConfig(config); err != nil {
		return nil, err
	}

	encrypted, err := integration.EncryptConfig(config, s.encryptKey)
	if err != nil {
		return nil, err
	}

	integration := &model.Integration{
		ProjectID:   uuid.MustParse(projectID),
		Type:        integrationType,
		DisplayName: displayName,
		Config:      encrypted,
		Enabled:     enabled,
	}
	if err := s.store.Create(integration); err != nil {
		return nil, err
	}
	integration.Config = nil
	return integration, nil
}

func (s *IntegrationService) ListByProject(projectID string) ([]model.Integration, error) {
	integrations, err := s.store.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	for i := range integrations {
		integrations[i].Config = nil
	}
	return integrations, nil
}

func (s *IntegrationService) Get(id string) (*model.Integration, error) {
	integration, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	integration.Config = nil
	return integration, nil
}

func (s *IntegrationService) Update(id, displayName string, config map[string]interface{}, enabled *bool) (*model.Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}

	if displayName != "" {
		existing.DisplayName = displayName
	}

	if config != nil {
		impl, err := integration.GetIntegration(existing.Type)
		if err != nil {
			return nil, err
		}
		if err := impl.ValidateConfig(config); err != nil {
			return nil, err
		}
		encrypted, err := integration.EncryptConfig(config, s.encryptKey)
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

func (s *IntegrationService) Delete(id string) error {
	return s.store.Delete(id)
}

func (s *IntegrationService) ValidateConfig(integrationType string, config map[string]interface{}) error {
	impl, err := integration.GetIntegration(integrationType)
	if err != nil {
		return err
	}
	return impl.ValidateConfig(config)
}

func (s *IntegrationService) FetchItemDetails(integrationID, itemID string) (*integration.ExternalDetailsResult, error) {
	existing, err := s.store.GetByID(integrationID)
	if err != nil {
		return nil, err
	}

	decrypted, err := integration.DecryptConfig(existing.Config, s.encryptKey)
	if err != nil {
		return nil, err
	}

	impl, err := integration.GetIntegration(existing.Type)
	if err != nil {
		return nil, err
	}

	details, err := impl.FetchItemDetails(decrypted, itemID)
	if err != nil {
		errMsg := err.Error()
		return &integration.ExternalDetailsResult{
			Details: nil,
			Error:   &errMsg,
		}, nil
	}

	return &integration.ExternalDetailsResult{
		Details: details,
	}, nil
}
