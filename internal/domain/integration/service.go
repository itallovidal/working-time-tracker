package integration

import (
	"errors"
	"reflect"
	"strings"

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

// redact tira a credencial (criptografada) da integração antes de ela sair do
// service, guardando só se havia uma.
func redact(it *Integration) {
	it.HasToken = len(it.Credentials) > 0
	it.Credentials = nil
}

// seal criptografa o token, o único segredo de uma integração.
func (s *Service) seal(token string) (map[string]interface{}, error) {
	return adapter.EncryptConfig(map[string]interface{}{"token": token}, s.encryptKey)
}

// open devolve o token guardado.
func (s *Service) open(it *Integration) (string, error) {
	if len(it.Credentials) == 0 {
		return "", errors.New("a integração está sem credencial: edite-a e informe o token")
	}
	secrets, err := adapter.DecryptConfig(it.Credentials, s.encryptKey)
	if err != nil {
		return "", errors.New("não foi possível ler a credencial guardada: edite a integração e informe o token de novo")
	}
	token, _ := secrets["token"].(string)
	return token, nil
}

// Create recebe a estrutura comum a todos os tipos: o token e, em metadata, os
// campos próprios da plataforma. O adapter do tipo confere o metadata e valida a
// conexão na plataforma antes de qualquer coisa ser guardada.
func (s *Service) Create(projectID, integrationType, displayName, token string, metadata map[string]interface{}, enabled bool) (*Integration, error) {
	if displayName == "" {
		return nil, errors.New("informe o nome da integração")
	}

	impl, err := adapter.GetIntegration(integrationType)
	if err != nil {
		return nil, err
	}

	meta, err := impl.CheckMetadata(metadata)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if err := impl.Validate(adapter.Connection{Token: token, Metadata: meta}); err != nil {
		return nil, err
	}

	credentials, err := s.seal(token)
	if err != nil {
		return nil, err
	}

	it := &Integration{
		ProjectID:   uuid.MustParse(projectID),
		Type:        integrationType,
		DisplayName: displayName,
		Credentials: credentials,
		Metadata:    meta,
		Enabled:     enabled,
	}
	if err := s.store.Create(it); err != nil {
		return nil, err
	}
	redact(it)
	return it, nil
}

func (s *Service) ListByProject(projectID string) ([]Integration, error) {
	integrations, err := s.store.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	for i := range integrations {
		redact(&integrations[i])
	}
	return integrations, nil
}

func (s *Service) Get(id string) (*Integration, error) {
	it, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	redact(it)
	return it, nil
}

// Update muda só o que veio: token vazio mantém o guardado e metadata nulo mantém o
// atual. A plataforma só é consultada quando a conexão muda (token novo ou metadata
// diferente), então renomear ou desativar não depende de o token ainda valer.
func (s *Service) Update(id, displayName, token string, metadata map[string]interface{}, enabled *bool) (*Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}

	if displayName != "" {
		existing.DisplayName = displayName
	}

	token = strings.TrimSpace(token)
	if token != "" || metadata != nil {
		impl, err := adapter.GetIntegration(existing.Type)
		if err != nil {
			return nil, err
		}
		meta := existing.Metadata
		if metadata != nil {
			if meta, err = impl.CheckMetadata(metadata); err != nil {
				return nil, err
			}
		}
		if token != "" || !reflect.DeepEqual(meta, existing.Metadata) {
			if token == "" {
				if token, err = s.open(existing); err != nil {
					return nil, err
				}
			}
			if err := impl.Validate(adapter.Connection{Token: token, Metadata: meta}); err != nil {
				return nil, err
			}
			if existing.Credentials, err = s.seal(token); err != nil {
				return nil, err
			}
			existing.Metadata = meta
		}
	}

	if enabled != nil {
		existing.Enabled = *enabled
	}

	if err := s.store.Update(existing); err != nil {
		return nil, err
	}
	redact(existing)
	return existing, nil
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}

// FetchItemDetails nunca falha por causa da plataforma nem da credencial: nesses
// casos devolve details nulo com o motivo.
func (s *Service) FetchItemDetails(integrationID, itemID string) (*adapter.ExternalDetailsResult, error) {
	existing, err := s.store.GetByID(integrationID)
	if err != nil {
		return nil, err
	}
	unavailable := func(msg string) (*adapter.ExternalDetailsResult, error) {
		return &adapter.ExternalDetailsResult{Details: nil, Error: &msg}, nil
	}
	if !existing.Enabled {
		return unavailable("a integração está desativada")
	}

	impl, err := adapter.GetIntegration(existing.Type)
	if err != nil {
		return nil, err
	}

	token, err := s.open(existing)
	if err != nil {
		return unavailable(err.Error())
	}

	details, err := impl.FetchItemDetails(adapter.Connection{Token: token, Metadata: existing.Metadata}, itemID)
	if err != nil {
		return unavailable(err.Error())
	}

	return &adapter.ExternalDetailsResult{
		Details: details,
	}, nil
}
