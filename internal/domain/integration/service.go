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
	return s.ConnectWith(projectID, integrationType, token, nil)
}

// ConnectWith é o Connect de um tipo que traz do app, e não da pessoa, parte do metadata (a chave do app
// no Trello): app entra na consulta de quem o token representa e fica guardado no metadata da integração.
func (s *Service) ConnectWith(projectID, integrationType, token string, app map[string]interface{}) (*Integration, error) {
	impl, lookup, err := accountLookup(integrationType)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	login, err := lookup.Account(adapter.Connection{Token: token, Metadata: app})
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
		Metadata:    mergeMetadata(nil, app),
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
	return s.ReauthorizeWith(id, token, nil)
}

// ReauthorizeWith é o Reauthorize de um tipo que traz do app parte do metadata: app vale sobre o que a
// integração tinha (reconectar com a chave atual do app) e fica guardado.
func (s *Service) ReauthorizeWith(id, token string, app map[string]interface{}) (*Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	impl, lookup, err := accountLookup(existing.Type)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	merged := mergeMetadata(existing.Metadata, app)
	if _, err := lookup.Account(adapter.Connection{Token: token, Metadata: merged}); err != nil {
		return nil, err
	}
	if meta, err := impl.CheckMetadata(merged); err == nil {
		if err := impl.Validate(adapter.Connection{Token: token, Metadata: meta}); err != nil {
			return nil, err
		}
	}
	if existing.Credentials, err = s.seal(token); err != nil {
		return nil, err
	}
	if len(app) > 0 {
		existing.Metadata = merged
	}
	if err := s.store.Update(existing); err != nil {
		return nil, err
	}
	redact(existing)
	return existing, nil
}

// mergeMetadata junta os campos do app aos que a integração já tem; os do app valem. Devolve sempre um
// mapa novo (nunca nulo), para o chamador poder guardá-lo.
func mergeMetadata(base, app map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(app))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range app {
		out[k] = v
	}
	return out
}

// keepInternal devolve o metadata que chegou na edição com os campos internos do tipo (os que a conexão
// guarda por conta própria) trocados pelo que já estava guardado: quem edita não os muda. Sem campo
// interno no tipo, o metadata sai como chegou.
func keepInternal(d adapter.Descriptor, incoming, stored map[string]interface{}) map[string]interface{} {
	var out map[string]interface{}
	for _, f := range d.Metadata {
		if !f.Internal {
			continue
		}
		if v, ok := stored[f.Key]; ok && v != "" {
			if out == nil {
				out = mergeMetadata(incoming, nil)
			}
			out[f.Key] = v
		}
	}
	if out == nil {
		return incoming
	}
	return out
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
		if err := s.fillSync(&integrations[i]); err != nil {
			return nil, err
		}
	}
	return integrations, nil
}

func (s *Service) Get(id string) (*Integration, error) {
	it, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	redact(it)
	if err := s.fillSync(it); err != nil {
		return nil, err
	}
	return it, nil
}

// fillSync calcula o que só a tela da integração com a sincronização ligada mostra.
func (s *Service) fillSync(it *Integration) error {
	if !it.SyncIssues {
		return nil
	}
	n, err := s.store.Unmatched(it.ID)
	it.SyncUnmatched = n
	return err
}

// Connection devolve a credencial e os campos da plataforma de uma integração, prontos para falar com
// ela. É o único jeito de o token sair do service, e só em memória: quem chama é a sincronização, que
// nunca o guarda nem o devolve.
func (s *Service) Connection(id string) (adapter.Connection, *Integration, error) {
	it, err := s.store.GetByID(id)
	if err != nil {
		return adapter.Connection{}, nil, err
	}
	token, err := s.open(it)
	if err != nil {
		return adapter.Connection{}, nil, err
	}
	redact(it)
	return adapter.Connection{Token: token, Metadata: it.Metadata}, it, nil
}

// ListSyncing lista as integrações ativas com a sincronização das issues ligada.
func (s *Service) ListSyncing() ([]Integration, error) {
	integrations, err := s.store.ListSyncing()
	if err != nil {
		return nil, err
	}
	for i := range integrations {
		redact(&integrations[i])
	}
	return integrations, nil
}

// RecordSync guarda o resultado de uma rodada de sincronização na integração.
func (s *Service) RecordSync(id uuid.UUID, r SyncResult) error {
	return s.store.SetSyncResult(id, r)
}

// EditInput é o que a edição de uma integração pode mudar; o que não vem fica como está: token vazio
// mantém o guardado, metadata nulo mantém o atual.
type EditInput struct {
	DisplayName string
	Token       string
	Metadata    map[string]interface{}
	Enabled     *bool
	// SyncIssues liga ou desliga a sincronização das issues com as tarefas.
	SyncIssues *bool
}

// Update muda só o que veio: token vazio mantém o guardado e metadata nulo mantém o atual. A
// plataforma só é consultada quando a conexão muda (token novo ou metadata diferente), então renomear
// ou desativar não depende de o token ainda valer.
func (s *Service) Update(id, displayName, token string, metadata map[string]interface{}, enabled *bool) (*Integration, error) {
	return s.Edit(id, EditInput{DisplayName: displayName, Token: token, Metadata: metadata, Enabled: enabled})
}

// Edit é o Update com tudo o que a edição pode trazer. Ligar a sincronização das issues pede um
// tipo que as lê e escreve, o repositório já escolhido e a integração ativa; com ela ligada o
// repositório não troca (desligue, troque, ligue), e trocá-lo com ela desligada esquece o vínculo
// das issues do repositório antigo.
func (s *Service) Edit(id string, in EditInput) (*Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	// O tipo só é preciso para conferir a conexão e a sincronização: renomear ou ativar não depende dele.
	impl, implErr := adapter.GetIntegration(existing.Type)

	if in.DisplayName != "" {
		existing.DisplayName = in.DisplayName
	}

	wasSyncing := existing.SyncIssues
	repoChanged := false
	token := strings.TrimSpace(in.Token)
	if token != "" || in.Metadata != nil {
		if implErr != nil {
			return nil, implErr
		}
		meta := existing.Metadata
		desc := impl.Descriptor()
		// O que identifica a conexão (o repositório, o quadro) é o que a sincronização não deixa trocar.
		summary := desc.SummaryKey()
		if in.Metadata != nil {
			if meta, err = impl.CheckMetadata(keepInternal(desc, in.Metadata, existing.Metadata)); err != nil {
				return nil, err
			}
		}
		if token != "" || !reflect.DeepEqual(meta, existing.Metadata) {
			if in.SyncIssues == nil || *in.SyncIssues {
				if wasSyncing && meta[summary] != existing.Metadata[summary] {
					return nil, ErrSyncRepoLocked
				}
			}
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
			repoChanged = meta[summary] != existing.Metadata[summary]
			existing.Metadata = meta
		}
	}

	if in.Enabled != nil {
		existing.Enabled = *in.Enabled
	}

	if in.SyncIssues != nil {
		turningOn := *in.SyncIssues && !wasSyncing
		if turningOn {
			if implErr != nil {
				return nil, implErr
			}
			if _, ok := impl.(adapter.IssueSyncer); !ok {
				return nil, ErrSyncUnsupported.With("provider", impl.Descriptor().Label)
			}
			if _, err := impl.CheckMetadata(existing.Metadata); err != nil {
				return nil, ErrSyncNeedsRepo
			}
			if !existing.Enabled {
				return nil, ErrSyncNeedsEnabled
			}
			if len(existing.Credentials) == 0 {
				return nil, ErrNoCredential
			}
		}
		existing.SyncIssues = *in.SyncIssues
	}

	if err := s.store.Update(existing); err != nil {
		return nil, err
	}
	if repoChanged {
		if err := s.store.ResetSync(existing.ID); err != nil {
			return nil, err
		}
	}
	saved, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return saved, nil
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
