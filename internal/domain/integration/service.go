package integration

import (
	"errors"
	"reflect"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
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
		return "", ErrNoCredential
	}
	secrets, err := adapter.DecryptConfig(it.Credentials, s.encryptKey)
	if err != nil {
		return "", ErrUnreadableCredential
	}
	token, _ := secrets["token"].(string)
	return token, nil
}

// Create recebe a estrutura comum a todos os tipos: o token e, em metadata, os
// campos próprios da plataforma. O adapter do tipo confere o metadata e valida a
// conexão na plataforma antes de qualquer coisa ser guardada.
func (s *Service) Create(projectID, integrationType, displayName, token string, metadata map[string]interface{}, enabled bool) (*Integration, error) {
	if displayName == "" {
		return nil, ErrNameRequired
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

// accountLookup acha o adapter do tipo e a capacidade de dizer quem o token representa,
// que é o que uma conexão por OAuth precisa.
func accountLookup(integrationType string) (adapter.Integration, adapter.AccountLookup, error) {
	impl, err := adapter.GetIntegration(integrationType)
	if err != nil {
		return nil, nil, err
	}
	lookup, ok := impl.(adapter.AccountLookup)
	if !ok {
		return nil, nil, ErrNotConnectable
	}
	return impl, lookup, nil
}

// Connect guarda uma conexão feita por OAuth: o token que a plataforma acabou de dar.
// A integração nasce desativada e sem os campos da plataforma (o repositório): a pessoa
// os escolhe editando, e é aí que ela é ativada e a conexão validada contra o que foi
// escolhido. O nome sugerido leva o login de quem autorizou.
func (s *Service) Connect(projectID, integrationType, token string) (*Integration, error) {
	impl, lookup, err := accountLookup(integrationType)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	login, err := lookup.Account(adapter.Connection{Token: token})
	if err != nil {
		return nil, err
	}
	credentials, err := s.seal(token)
	if err != nil {
		return nil, err
	}

	it := &Integration{
		ProjectID:   uuid.MustParse(projectID),
		Type:        integrationType,
		DisplayName: impl.Descriptor().Label + " · @" + login,
		Credentials: credentials,
		Metadata:    map[string]interface{}{},
		Enabled:     false,
	}
	if err := s.store.Create(it); err != nil {
		return nil, err
	}
	redact(it)
	return it, nil
}

// Reauthorize troca o token de uma integração que já existe pelo de uma nova conexão
// OAuth. Se ela já tem o repositório, o token novo é validado contra ele antes de
// valer: quem reconecta com outra conta, que não enxerga o repositório, é avisado em
// vez de ficar com uma integração quebrada.
func (s *Service) Reauthorize(id, token string) (*Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	impl, lookup, err := accountLookup(existing.Type)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if _, err := lookup.Account(adapter.Connection{Token: token}); err != nil {
		return nil, err
	}
	if meta, err := impl.CheckMetadata(existing.Metadata); err == nil {
		if err := impl.Validate(adapter.Connection{Token: token, Metadata: meta}); err != nil {
			return nil, err
		}
	}
	if existing.Credentials, err = s.seal(token); err != nil {
		return nil, err
	}
	if err := s.store.Update(existing); err != nil {
		return nil, err
	}
	redact(existing)
	return existing, nil
}

// Repositories lista o que o token guardado da integração enxerga. O token fica no
// servidor: quem chama recebe só os nomes.
func (s *Service) Repositories(id string) ([]adapter.Repository, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	impl, err := adapter.GetIntegration(existing.Type)
	if err != nil {
		return nil, err
	}
	lister, ok := impl.(adapter.RepositoryLister)
	if !ok {
		return nil, ErrNoRepositories
	}
	token, err := s.open(existing)
	if err != nil {
		return nil, err
	}
	return lister.ListRepositories(adapter.Connection{Token: token, Metadata: existing.Metadata})
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
	unavailable := func(err error) (*adapter.ExternalDetailsResult, error) {
		return &adapter.ExternalDetailsResult{Details: nil, Error: coded(err)}, nil
	}
	if !existing.Enabled {
		return unavailable(ErrDisabled)
	}

	impl, err := adapter.GetIntegration(existing.Type)
	if err != nil {
		return nil, err
	}

	token, err := s.open(existing)
	if err != nil {
		return unavailable(err)
	}

	details, err := impl.FetchItemDetails(adapter.Connection{Token: token, Metadata: existing.Metadata}, itemID)
	if err != nil {
		return unavailable(err)
	}

	return &adapter.ExternalDetailsResult{
		Details: details,
	}, nil
}

// coded devolve o erro de uma integração com código: o que já veio com código segue
// como está, e qualquer outra falha vira um erro interno, sem o texto técnico.
func coded(err error) *apperr.Error {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e
	}
	return apperr.ErrInternal
}
