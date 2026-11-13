package integration

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/validate"
)

// Os campos do corpo que os erros de validação apontam: a chave JSON, que é também a chave de fields.* no catálogo.
const (
	fieldDisplayName = "display_name"
	fieldToken       = "token"
)

// cleanName apara o nome da integração e confere o tamanho. Vazio, ou só espaços, é recusado.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNameRequired.With("field", fieldDisplayName)
	}
	return validate.Required(fieldDisplayName, name, validate.MaxName)
}

// cleanToken apara o token e confere o tamanho. Vazio é aceito aqui: a plataforma é quem diz que falta.
func cleanToken(token string) (string, error) {
	return validate.Text(fieldToken, token, validate.MaxToken)
}

// withField devolve o erro de uma plataforma com o campo que o causou: o token, o repositório (o campo que
// identifica a conexão: repo, project_url, board_id) ou a chave do app. Um erro que já diz o campo, ou que não é
// de um campo, segue como está.
func withField(err error, desc adapter.Descriptor) error {
	var e *apperr.Error
	if !errors.As(err, &e) {
		return err
	}
	if _, has := e.Params["field"]; has {
		return err
	}
	switch e.Code {
	case adapter.ErrInvalidToken.Code, adapter.ErrTokenRequired.Code:
		return e.With("field", fieldToken)
	case adapter.ErrGitHubInvalidRepo.Code, adapter.ErrGitHubRepoNotFound.Code,
		adapter.ErrGitLabInvalidProject.Code, adapter.ErrGitLabForbidden.Code, adapter.ErrGitLabProjectMissing.Code,
		adapter.ErrTrelloInvalidBoard.Code, adapter.ErrTrelloNoAccessBoard.Code, adapter.ErrTrelloBoardMissing.Code:
		if key := desc.SummaryKey(); key != "" {
			return e.With("field", key)
		}
	case adapter.ErrTrelloInvalidKey.Code:
		return e.With("field", "api_key")
	}
	return err
}

// checkMetadata confere o metadata da plataforma: o campo que identifica a conexão (repositório, projeto,
// quadro) tem um teto antes de o adaptador conferir o formato, e os erros dele dizem o campo.
func checkMetadata(impl adapter.Integration, raw map[string]interface{}) (map[string]interface{}, error) {
	desc := impl.Descriptor()
	if key := desc.SummaryKey(); key != "" {
		if v, ok := raw[key].(string); ok && utf8.RuneCountInString(strings.TrimSpace(v)) > validate.MaxRepo {
			return nil, apperr.ErrFieldTooLong.With("field", key, "max", validate.MaxRepo)
		}
	}
	meta, err := impl.CheckMetadata(raw)
	return meta, withField(err, desc)
}

type Service struct {
	store      *Store
	encryptKey string

	mu sync.RWMutex
	// refreshers renova, por tipo, o token que vence (o do OAuth do GitLab).
	refreshers map[string]adapter.TokenRefresher
}

// refreshMargin é quanto antes de vencer o token é renovado: uma rodada da sincronização que começa com ele
// ainda termina antes de ele vencer.
const refreshMargin = 10 * time.Minute

func NewService(store *Store, encryptKey string) *Service {
	return &Service{store: store, encryptKey: encryptKey}
}

// redact tira a credencial (criptografada) da integração antes de ela sair do
// service, guardando só se havia uma.
func redact(it *Integration) {
	it.HasToken = len(it.Credentials) > 0
	it.Credentials = nil
}

// SetRefresher diz quem renova o token que vence de um tipo: o app OAuth do GitLab. Sem ele, o token que vence
// é usado como está, e a plataforma recusa quando ele vencer.
func (s *Service) SetRefresher(integrationType string, r adapter.TokenRefresher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshers == nil {
		s.refreshers = map[string]adapter.TokenRefresher{}
	}
	s.refreshers[integrationType] = r
}

func (s *Service) refresher(integrationType string) adapter.TokenRefresher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.refreshers[integrationType]
}

// seal criptografa o token, o único segredo de uma integração.
func (s *Service) seal(token string) (map[string]interface{}, error) {
	return s.sealCredential(adapter.Credential{Token: token})
}

// sealCredential criptografa a credencial inteira: o token e, no token que vence, o de renovação e o vencimento.
func (s *Service) sealCredential(cred adapter.Credential) (map[string]interface{}, error) {
	secrets := map[string]interface{}{"token": cred.Token}
	if cred.RefreshToken != "" {
		secrets["refresh_token"] = cred.RefreshToken
		if !cred.ExpiresAt.IsZero() {
			secrets["expires_at"] = cred.ExpiresAt.UTC().Format(time.RFC3339)
		}
	}
	return adapter.EncryptConfig(secrets, s.encryptKey)
}

// unseal lê a credencial guardada.
func (s *Service) unseal(credentials map[string]interface{}) (adapter.Credential, error) {
	if len(credentials) == 0 {
		return adapter.Credential{}, ErrNoCredential
	}
	secrets, err := adapter.DecryptConfig(credentials, s.encryptKey)
	if err != nil {
		return adapter.Credential{}, ErrUnreadableCredential
	}
	var cred adapter.Credential
	cred.Token, _ = secrets["token"].(string)
	cred.RefreshToken, _ = secrets["refresh_token"].(string)
	if at, ok := secrets["expires_at"].(string); ok {
		cred.ExpiresAt, _ = time.Parse(time.RFC3339, at)
	}
	return cred, nil
}

// credentialKeys são as chaves que a credencial guarda hoje. Uma linha de antes do metadata guardava também o
// repositório no blob.
var credentialKeys = map[string]bool{"token": true, "refresh_token": true, "expires_at": true}

// dropLegacy regrava a credencial sem as chaves de antes do metadata, se ela as tem, e diz se regravou. A
// credencial de hoje fica como está: regravá-la sem necessidade poria a cópia lida por cima de uma renovação.
func (s *Service) dropLegacy(it *Integration) (bool, error) {
	secrets, err := adapter.DecryptConfig(it.Credentials, s.encryptKey)
	if err != nil {
		return false, nil
	}
	for key := range secrets {
		if !credentialKeys[key] {
			cred, err := s.unseal(it.Credentials)
			if err != nil {
				return false, err
			}
			if it.Credentials, err = s.sealCredential(cred); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// expiring diz se o token vence logo: só o que tem como ser renovado.
func expiring(cred adapter.Credential) bool {
	return cred.RefreshToken != "" && !cred.ExpiresAt.IsZero() && time.Until(cred.ExpiresAt) < refreshMargin
}

// open devolve o token de acesso guardado. O que vence logo (o do OAuth do GitLab) é renovado antes: a renovação
// trava a linha da integração, relê a credencial (outra chamada pode ter acabado de renovar) e grava só ela. A
// credencial nova também vai para it.Credentials, para quem chamou não gravar a velha por cima.
func (s *Service) open(it *Integration) (string, error) {
	cred, err := s.unseal(it.Credentials)
	if err != nil {
		return "", err
	}
	refresher := s.refresher(it.Type)
	if !expiring(cred) || refresher == nil {
		return cred.Token, nil
	}
	var fresh adapter.Credential
	sealed, err := s.store.RefreshCredentials(it.ID, func(stored map[string]interface{}) (map[string]interface{}, error) {
		current, err := s.unseal(stored)
		if err != nil {
			return nil, err
		}
		if !expiring(current) {
			fresh = current
			return nil, nil
		}
		next, err := refresher.Refresh(current.RefreshToken)
		if err != nil {
			return nil, err
		}
		fresh = next
		return s.sealCredential(next)
	})
	if err != nil {
		// A plataforma fora do ar não estraga um token que ainda vale: ele é usado, e a próxima chamada tenta de
		// novo. O token de renovação recusado (revogado, já usado) é o token recusado.
		if !errors.Is(err, adapter.ErrInvalidToken) && time.Now().Before(cred.ExpiresAt) {
			return cred.Token, nil
		}
		return "", err
	}
	it.Credentials = sealed
	return fresh.Token, nil
}

// Create recebe a estrutura comum a todos os tipos: o token e, em metadata, os
// campos próprios da plataforma. O adapter do tipo confere o metadata e valida a
// conexão na plataforma antes de qualquer coisa ser guardada.
func (s *Service) Create(projectID, integrationType, displayName, token string, metadata map[string]interface{}, enabled bool) (*Integration, error) {
	displayName, err := cleanName(displayName)
	if err != nil {
		return nil, err
	}
	token, err = cleanToken(token)
	if err != nil {
		return nil, err
	}

	impl, err := adapter.GetIntegration(integrationType)
	if err != nil {
		return nil, err
	}

	meta, err := checkMetadata(impl, metadata)
	if err != nil {
		return nil, err
	}
	if err := impl.Validate(adapter.Connection{Token: token, Metadata: meta}); err != nil {
		return nil, withField(err, impl.Descriptor())
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
	return s.connect(projectID, integrationType, adapter.Credential{Token: token}, app)
}

// ConnectCredential é o Connect de um tipo cujo token vence (o do GitLab): guarda o par inteiro, para o
// servidor renovar o token sozinho.
func (s *Service) ConnectCredential(projectID, integrationType string, cred adapter.Credential) (*Integration, error) {
	return s.connect(projectID, integrationType, cred, nil)
}

func (s *Service) connect(projectID, integrationType string, cred adapter.Credential, app map[string]interface{}) (*Integration, error) {
	impl, lookup, err := accountLookup(integrationType)
	if err != nil {
		return nil, err
	}
	cred.Token = strings.TrimSpace(cred.Token)
	login, err := lookup.Account(adapter.Connection{Token: cred.Token, Metadata: app})
	if err != nil {
		return nil, err
	}
	credentials, err := s.sealCredential(cred)
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
	it.ConnectedLogin = login
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
	return s.reauthorize(id, adapter.Credential{Token: token}, app)
}

// ReauthorizeCredential é o Reauthorize de um tipo cujo token vence (o do GitLab): troca o par inteiro.
func (s *Service) ReauthorizeCredential(id string, cred adapter.Credential) (*Integration, error) {
	return s.reauthorize(id, cred, nil)
}

func (s *Service) reauthorize(id string, cred adapter.Credential, app map[string]interface{}) (*Integration, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	impl, lookup, err := accountLookup(existing.Type)
	if err != nil {
		return nil, err
	}
	cred.Token = strings.TrimSpace(cred.Token)
	merged := mergeMetadata(existing.Metadata, app)
	login, err := lookup.Account(adapter.Connection{Token: cred.Token, Metadata: merged})
	if err != nil {
		return nil, err
	}
	if meta, err := impl.CheckMetadata(merged); err == nil {
		if err := impl.Validate(adapter.Connection{Token: cred.Token, Metadata: meta}); err != nil {
			return nil, err
		}
	}
	if existing.Credentials, err = s.sealCredential(cred); err != nil {
		return nil, err
	}
	if len(app) > 0 {
		existing.Metadata = merged
	}
	if err := s.store.Update(existing, true); err != nil {
		return nil, err
	}
	redact(existing)
	existing.ConnectedLogin = login
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
	logins, err := s.store.Unmatched(it.ID)
	it.SyncUnmatched, it.SyncUnmatchedLogins = len(logins), logins
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

	// O nome vazio mantém o que está; um nome só de espaços não é um nome e é recusado, não gravado.
	if in.DisplayName != "" {
		if existing.DisplayName, err = cleanName(in.DisplayName); err != nil {
			return nil, err
		}
	}

	wasSyncing := existing.SyncIssues
	repoChanged := false
	// newCredential diz se a edição trouxe um token novo: só então a credencial é gravada. Gravá-la sempre poria
	// por cima, com a cópia lida no começo, um token que a renovação acabou de girar.
	newCredential := false
	token, err := cleanToken(in.Token)
	if err != nil {
		return nil, err
	}
	if token != "" || in.Metadata != nil {
		if implErr != nil {
			return nil, implErr
		}
		meta := existing.Metadata
		desc := impl.Descriptor()
		// O que identifica a conexão (o repositório, o quadro) é o que a sincronização não deixa trocar.
		summary := desc.SummaryKey()
		if in.Metadata != nil {
			if meta, err = checkMetadata(impl, keepInternal(desc, in.Metadata, existing.Metadata)); err != nil {
				return nil, err
			}
			// O nome legível guardado é do alvo antigo: se o alvo mudou e o nome não, ele não vale mais.
			if name := desc.SummaryNameKey(); name != "" && meta[summary] != existing.Metadata[summary] && meta[name] == existing.Metadata[name] {
				delete(meta, name)
			}
		}
		if token != "" || !reflect.DeepEqual(meta, existing.Metadata) {
			if in.SyncIssues == nil || *in.SyncIssues {
				if wasSyncing && meta[summary] != existing.Metadata[summary] {
					return nil, ErrSyncRepoLocked
				}
			}
			// Sem token novo, vale o guardado (renovado antes, se for o que vence), e a credencial não muda.
			check := token
			if check == "" {
				if check, err = s.open(existing); err != nil {
					return nil, err
				}
			}
			if err := impl.Validate(adapter.Connection{Token: check, Metadata: meta}); err != nil {
				return nil, withField(err, desc)
			}
			if token != "" {
				if existing.Credentials, err = s.seal(token); err != nil {
					return nil, err
				}
				newCredential = true
			} else if newCredential, err = s.dropLegacy(existing); err != nil {
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

	if err := s.store.Update(existing, newCredential); err != nil {
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
