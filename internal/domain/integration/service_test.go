package integration_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/testutil"
)

const testKey = "test-32-byte-encryption-key!!!!"

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

// setup limpa o banco e devolve o service com um projeto para receber integrações.
func setup(t *testing.T) (*integration.Service, string) {
	t.Helper()
	cleanup(t)
	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	proj, err := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Project", "", 0, project.Routine{})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return integration.NewService(integration.NewStore(testClient), testKey), proj.ID.String()
}

func githubMetadata() map[string]interface{} {
	return map[string]interface{}{"repo": "owner/repo"}
}

func TestService_Create(t *testing.T) {
	svc, projectID := setup(t)

	// O repositório pode vir como endereço: o que fica guardado é dono/repositorio.
	it, err := svc.Create(projectID, "github", "My GitHub", "ghp_test",
		map[string]interface{}{"repo": "https://github.com/owner/repo", "ignored": "x"}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if it.DisplayName != "My GitHub" {
		t.Errorf("display_name = %q, want %q", it.DisplayName, "My GitHub")
	}
	if it.Credentials != nil || !it.HasToken {
		t.Errorf("Credentials = %v, HasToken = %v; want nil and true", it.Credentials, it.HasToken)
	}
	if want := githubMetadata(); !reflect.DeepEqual(it.Metadata, want) {
		t.Errorf("metadata = %v, want %v", it.Metadata, want)
	}

	// No banco, o metadata fica em claro e o blob criptografado guarda só o token.
	row := testClient.Integration.GetX(context.Background(), it.ID)
	if want := githubMetadata(); !reflect.DeepEqual(row.Metadata, want) {
		t.Errorf("stored metadata = %v, want %v", row.Metadata, want)
	}
	secrets, err := adapter.DecryptConfig(row.Credentials, testKey)
	if err != nil {
		t.Fatalf("decrypt stored credentials: %v", err)
	}
	if want := map[string]interface{}{"token": "ghp_test"}; !reflect.DeepEqual(secrets, want) {
		t.Errorf("stored secrets = %v, want %v", secrets, want)
	}
}

func TestService_Create_InvalidType(t *testing.T) {
	svc, projectID := setup(t)

	_, err := svc.Create(projectID, "invalid-type", "Bad", "token", nil, true)
	if err == nil {
		t.Fatal("expected error for invalid integration type, got nil")
	}
}

// Sem o campo que o tipo pede no metadata, a plataforma nem é consultada.
func TestService_Create_MissingMetadata(t *testing.T) {
	svc, projectID := setup(t)

	before := platformCalls.Load()
	cases := []struct {
		typ      string
		metadata map[string]interface{}
		wantErr  string
	}{
		{"github", nil, "integration.field_required"},
		{"github", map[string]interface{}{"project_url": "group/project"}, "integration.field_required"},
		{"gitlab", map[string]interface{}{}, "integration.field_required"},
		{"trello", map[string]interface{}{"api_key": testutil.TrelloKey}, "integration.field_required"},
	}
	for _, tc := range cases {
		_, err := svc.Create(projectID, tc.typ, "Sem metadata", "token", tc.metadata, true)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s with %v: error = %v, want one containing %q", tc.typ, tc.metadata, err, tc.wantErr)
		}
	}
	if after := platformCalls.Load(); after != before {
		t.Errorf("%d platform call(s) for metadata that should be refused before them", after-before)
	}
	if list, _ := svc.ListByProject(projectID); len(list) != 0 {
		t.Errorf("%d integration(s) saved after refused creations", len(list))
	}
}

func TestService_Get_NoCredentials(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "My GitHub", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	got, err := svc.Get(created.ID.String())
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Credentials != nil {
		t.Error("credentials should be nil in get response (the token must never be returned)")
	}
	if want := githubMetadata(); !reflect.DeepEqual(got.Metadata, want) {
		t.Errorf("metadata = %v, want %v", got.Metadata, want)
	}
}

func TestService_List_NoCredentials(t *testing.T) {
	svc, projectID := setup(t)

	if _, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), true); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	list, err := svc.ListByProject(projectID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list has %d item(s), want 1", len(list))
	}
	for _, item := range list {
		if item.Credentials != nil || !item.HasToken {
			t.Errorf("Credentials = %v, HasToken = %v; want nil and true", item.Credentials, item.HasToken)
		}
		if want := githubMetadata(); !reflect.DeepEqual(item.Metadata, want) {
			t.Errorf("metadata = %v, want %v", item.Metadata, want)
		}
	}
}

// Renomear, desativar ou reenviar o mesmo metadata não consulta a plataforma: a
// edição não pode depender de o token ainda valer.
func TestService_Update_WithoutTouchingTheConnection(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "Old Name", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created.ID.String()

	before := platformCalls.Load()
	disabled := false
	updated, err := svc.Update(id, "New Name", "", nil, &disabled)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.DisplayName != "New Name" || updated.Enabled || !updated.HasToken {
		t.Errorf("updated = %+v, want the new name, disabled and still with a token", updated)
	}
	// O modal manda o metadata sempre; igual ao guardado (mesmo em outra forma), não revalida.
	same := map[string]interface{}{"repo": "https://github.com/owner/repo/"}
	if _, err := svc.Update(id, "", "  ", same, nil); err != nil {
		t.Fatalf("update with the same metadata failed: %v", err)
	}
	if after := platformCalls.Load(); after != before {
		t.Errorf("%d platform call(s) for edits that do not change the connection", after-before)
	}
	got, _ := svc.Get(id)
	if got.DisplayName != "New Name" || !reflect.DeepEqual(got.Metadata, githubMetadata()) {
		t.Errorf("after updates: %+v", got)
	}
}

// Trocar o metadata sem mandar token valida a conexão nova com o token guardado.
func TestService_Update_MetadataKeepsTheToken(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created.ID.String()

	before := platformCalls.Load()
	updated, err := svc.Update(id, "", "", map[string]interface{}{"repo": "owner/other"}, nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if platformCalls.Load() == before {
		t.Error("changing the metadata did not validate the new connection on the platform")
	}
	if want := map[string]interface{}{"repo": "owner/other"}; !reflect.DeepEqual(updated.Metadata, want) || !updated.HasToken {
		t.Errorf("updated = %+v, want metadata %v and a token", updated, want)
	}
	// O repositório novo não tem a issue 42: a resposta é "não encontrado", não "token inválido".
	res, err := svc.FetchItemDetails(id, "42")
	if err != nil || res.Details != nil || res.Error == nil || res.Error.Code != "integration.item_not_found" {
		t.Errorf("fetch on the new repository = %+v, %v", res, err)
	}

	// Um repositório que a plataforma não conhece é recusado, e o guardado continua.
	if _, err := svc.Update(id, "", "", map[string]interface{}{"repo": "owner/missing"}, nil); err == nil {
		t.Error("expected error for a repository the platform does not know")
	}
	if _, err := svc.Update(id, "", "", map[string]interface{}{}, nil); err == nil || !strings.Contains(err.Error(), "integration.field_required") {
		t.Errorf("empty metadata: error = %v, want the missing field", err)
	}
	got, _ := svc.Get(id)
	if want := map[string]interface{}{"repo": "owner/other"}; !reflect.DeepEqual(got.Metadata, want) {
		t.Errorf("metadata after refused updates = %v, want %v", got.Metadata, want)
	}
}

func TestService_Update_NewToken(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created.ID.String()

	// Token novo inválido: recusado, e o token antigo continua valendo.
	if _, err := svc.Update(id, "", testutil.InvalidToken, nil, nil); err == nil {
		t.Fatal("expected error for an invalid new token")
	}
	if res, err := svc.FetchItemDetails(id, "42"); err != nil || res.Details == nil {
		t.Fatalf("fetch after a refused token = %+v, %v; want the details with the old token", res, err)
	}

	if _, err := svc.Update(id, "", "ghp_new", nil, nil); err != nil {
		t.Fatalf("update with a new token failed: %v", err)
	}
	row := testClient.Integration.GetX(context.Background(), created.ID)
	secrets, err := adapter.DecryptConfig(row.Credentials, testKey)
	if err != nil || secrets["token"] != "ghp_new" {
		t.Errorf("stored secrets = %v (%v), want the new token", secrets, err)
	}
}

func TestService_Delete(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "Test", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	err = svc.Delete(created.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = svc.Get(created.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_Create_InvalidToken(t *testing.T) {
	svc, projectID := setup(t)

	_, err := svc.Create(projectID, "github", "GitHub", testutil.InvalidToken, githubMetadata(), true)
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
	if _, err := svc.Create(projectID, "github", "GitHub", "", githubMetadata(), true); err == nil || !strings.Contains(err.Error(), "integration.token_required") {
		t.Errorf("create without token: error = %v", err)
	}
}

func TestService_FetchItemDetails(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	found, err := svc.FetchItemDetails(created.ID.String(), "42")
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if found.Details == nil || found.Details.Title != "Corrigir login" || found.Details.State != "open" {
		t.Errorf("details = %+v, want title %q and state %q", found.Details, "Corrigir login", "open")
	}

	// Item inexistente: a chamada não falha, só devolve details nulo com a mensagem de erro.
	missing, err := svc.FetchItemDetails(created.ID.String(), "999")
	if err != nil {
		t.Fatalf("fetch of missing item should degrade gracefully, got error: %v", err)
	}
	if missing.Details != nil || missing.Error == nil {
		t.Errorf("expected nil details and an error message, got %+v", missing)
	}
}

func TestService_HasTokenAndDisabledFetch(t *testing.T) {
	svc, projectID := setup(t)

	// Criada desativada: has_token tem que refletir a credencial, não o enabled.
	created, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := svc.Get(created.ID.String())
	if !got.HasToken || got.Credentials != nil {
		t.Errorf("HasToken = %v, Credentials = %v; want true and nil", got.HasToken, got.Credentials)
	}

	res, err := svc.FetchItemDetails(created.ID.String(), "42")
	if err != nil {
		t.Fatalf("fetch on disabled integration should degrade gracefully, got %v", err)
	}
	if res.Details != nil || res.Error == nil {
		t.Errorf("disabled integration returned %+v, want nil details and an error message", res)
	}
}

// Uma linha de antes do metadata guardava token e repositório juntos no blob. O token
// continua legível; falta o metadata, e editar a integração uma vez resolve.
func TestService_RowFromBeforeMetadata(t *testing.T) {
	svc, projectID := setup(t)

	blob, err := adapter.EncryptConfig(map[string]interface{}{"token": "ghp_test", "repo": "owner/repo"}, testKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	row := testClient.Integration.Create().SetProjectID(uuid.MustParse(projectID)).
		SetType("github").SetDisplayName("Antiga").SetCredentials(blob).SaveX(context.Background())
	id := row.ID.String()

	got, err := svc.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.HasToken || got.Metadata == nil || len(got.Metadata) != 0 {
		t.Errorf("HasToken = %v, Metadata = %#v; want true and an empty object", got.HasToken, got.Metadata)
	}

	res, err := svc.FetchItemDetails(id, "42")
	if err != nil || res.Details != nil || res.Error == nil || !(res.Error.Code == "integration.field_required" && res.Error.Params["field"] == "repo") {
		t.Fatalf("fetch before the edit = %+v, %v; want the missing field as the reason", res, err)
	}

	if _, err := svc.Update(id, "", "", githubMetadata(), nil); err != nil {
		t.Fatalf("update with the metadata and no token: %v", err)
	}
	if res, err := svc.FetchItemDetails(id, "42"); err != nil || res.Details == nil {
		t.Errorf("fetch after the edit = %+v, %v; want the details", res, err)
	}
	// A edição regrava o blob só com o token.
	secrets, err := adapter.DecryptConfig(testClient.Integration.GetX(context.Background(), row.ID).Credentials, testKey)
	if want := map[string]interface{}{"token": "ghp_test"}; err != nil || !reflect.DeepEqual(secrets, want) {
		t.Errorf("stored secrets = %v (%v), want %v", secrets, err, want)
	}
}

// Com outra chave de criptografia a credencial fica ilegível: a busca explica o
// motivo em vez de falhar, e a edição pede o token de novo.
func TestService_UnreadableCredentials(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "github", "GitHub", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.ID.String()
	rotated := integration.NewService(integration.NewStore(testClient), "another-encryption-key")

	res, err := rotated.FetchItemDetails(id, "42")
	if err != nil || res.Details != nil || res.Error == nil || res.Error.Code != "integration.unreadable_credential" {
		t.Errorf("fetch = %+v, %v; want a reason that asks for the token again", res, err)
	}
	if _, err := rotated.Update(id, "", "", map[string]interface{}{"repo": "owner/other"}, nil); err == nil {
		t.Error("expected error when the metadata changes and the stored token cannot be read")
	}
	if _, err := rotated.Update(id, "", "ghp_new", nil, nil); err != nil {
		t.Fatalf("update with a new token: %v", err)
	}
	if res, err := rotated.FetchItemDetails(id, "42"); err != nil || res.Details == nil {
		t.Errorf("fetch after the new token = %+v, %v; want the details", res, err)
	}
}

func TestService_GitLab(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "gitlab", "GitLab", "glpat-test",
		map[string]interface{}{"project_url": "https://gitlab.com/group/project"}, true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if want := map[string]interface{}{"project_url": "group/project"}; !reflect.DeepEqual(created.Metadata, want) {
		t.Errorf("metadata = %v, want %v", created.Metadata, want)
	}

	res, err := svc.FetchItemDetails(created.ID.String(), "7")
	if err != nil || res.Details == nil || res.Details.Title != "Ajustar relatório" || res.Details.State != "opened" {
		t.Errorf("fetch = %+v, %v", res, err)
	}
}

func TestService_Trello(t *testing.T) {
	svc, projectID := setup(t)

	created, err := svc.Create(projectID, "trello", "Quadro do app", "trello-token", map[string]interface{}{
		"api_key":  testutil.TrelloKey,
		"board_id": "https://trello.com/b/" + testutil.TrelloBoardShortLink + "/app",
	}, true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := map[string]interface{}{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardShortLink}
	if !reflect.DeepEqual(created.Metadata, want) || !created.HasToken {
		t.Errorf("created = %+v, want metadata %v and a token", created, want)
	}
	id := created.ID.String()

	// O estado de um cartão é aberto ou fechado (arquivado ou com a data de entrega concluída); as listas ficam de fora.
	res, err := svc.FetchItemDetails(id, "H0TZyzbK")
	if err != nil || res.Details == nil || res.Details.Title != "Corrigir login" || res.Details.State != "open" {
		t.Errorf("fetch = %+v, %v", res, err)
	}
	other, err := svc.FetchItemDetails(id, "OutroQdr")
	if err != nil || other.Details != nil || other.Error == nil || other.Error.Code != "integration.trello_card_other_board" {
		t.Errorf("fetch of a card from another board = %+v, %v", other, err)
	}

	if _, err := svc.Create(projectID, "trello", "Token errado", testutil.InvalidToken, want, true); err == nil {
		t.Error("expected error for an invalid Trello token")
	}
}

// Uma conexão por OAuth nasce desativada e sem repositório, com o token cifrado e o nome
// sugerido pelo login. Escolher o repositório (um Update com o metadata) a valida contra o
// token guardado e é o que a deixa pronta.
func TestService_Connect(t *testing.T) {
	svc, projectID := setup(t)

	it, err := svc.Connect(projectID, "github", testutil.GitHubOAuthToken)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if want := "GitHub · @" + testutil.GitHubLogin; it.DisplayName != want {
		t.Errorf("display_name = %q, want %q", it.DisplayName, want)
	}
	if it.Enabled || !it.HasToken || it.Credentials != nil || len(it.Metadata) != 0 {
		t.Errorf("connected = enabled %v, has_token %v, credentials %v, metadata %v; want disabled, with token, no credentials, no metadata",
			it.Enabled, it.HasToken, it.Credentials, it.Metadata)
	}
	row := testClient.Integration.GetX(context.Background(), it.ID)
	secrets, err := adapter.DecryptConfig(row.Credentials, testKey)
	if err != nil || secrets["token"] != testutil.GitHubOAuthToken {
		t.Errorf("stored secrets = %v, %v, want the OAuth token", secrets, err)
	}

	// Sem repositório, a integração aparece incompleta: os itens vinculados não são buscados.
	if res, err := svc.FetchItemDetails(it.ID.String(), "42"); err != nil || res.Details != nil || res.Error == nil {
		t.Errorf("fetch on a disabled, incomplete integration = %+v, %v, want details nil with a reason", res, err)
	}

	// Escolher o repositório valida a conexão com o token guardado e ativa a integração.
	enabled := true
	done, err := svc.Update(it.ID.String(), "", "", githubMetadata(), &enabled)
	if err != nil || !done.Enabled || !reflect.DeepEqual(done.Metadata, githubMetadata()) {
		t.Fatalf("finish = %+v, %v, want enabled with the repository", done, err)
	}
	if res, err := svc.FetchItemDetails(it.ID.String(), "42"); err != nil || res.Details == nil || res.Details.Title != "Corrigir login" {
		t.Errorf("fetch after finishing = %+v, %v", res, err)
	}
	// Um repositório que o token não enxerga é recusado, e a integração continua como estava.
	if _, err := svc.Update(it.ID.String(), "", "", map[string]interface{}{"repo": "owner/missing"}, nil); err == nil ||
		!strings.Contains(err.Error(), "integration.github_repo_not_found") {
		t.Errorf("update with a repository the token cannot see: %v", err)
	}
}

// Um tipo que traz do app parte do metadata (a chave do app, no Trello) a recebe na conexão e na
// reconexão; o Connect e o Reauthorize de sempre não mexem nele.
func TestService_ConnectWithAppFields(t *testing.T) {
	svc, projectID := setup(t)

	it, err := svc.ConnectWith(projectID, "github", testutil.GitHubOAuthToken, map[string]interface{}{"app_key": "k1"})
	if err != nil {
		t.Fatalf("connect with: %v", err)
	}
	if !reflect.DeepEqual(it.Metadata, map[string]interface{}{"app_key": "k1"}) {
		t.Errorf("metadata after connecting = %v, want the app field", it.Metadata)
	}

	again, err := svc.ReauthorizeWith(it.ID.String(), testutil.GitHubSecondOAuthToken, map[string]interface{}{"app_key": "k2"})
	if err != nil || again.Metadata["app_key"] != "k2" {
		t.Fatalf("reauthorize with = %+v, %v, want the app field replaced", again, err)
	}
	plain, err := svc.Reauthorize(it.ID.String(), testutil.GitHubOAuthToken)
	if err != nil || plain.Metadata["app_key"] != "k2" {
		t.Errorf("a plain reauthorize = %+v, %v, want the metadata untouched", plain, err)
	}
	// Um token que a plataforma recusa não troca nada.
	if _, err := svc.ReauthorizeWith(it.ID.String(), testutil.InvalidToken, map[string]interface{}{"app_key": "k3"}); err == nil {
		t.Error("a rejected token must fail")
	}
	if got, _ := svc.Get(it.ID.String()); got.Metadata["app_key"] != "k2" {
		t.Errorf("a rejected reauthorize changed the metadata: %v", got.Metadata)
	}
}

func TestService_Connect_Refused(t *testing.T) {
	svc, projectID := setup(t)

	if _, err := svc.Connect(projectID, "github", testutil.InvalidToken); err == nil || !strings.Contains(err.Error(), "integration.invalid_token") {
		t.Errorf("connect with a token GitHub rejects: %v", err)
	}
	// O GitLab não se conecta por autorização. (O Trello sabe dizer quem o token representa e, no fim da
	// Sprint 69, também se conecta; aqui sem a chave do app a conexão não chega a existir.)
	for _, typ := range []string{"gitlab"} {
		if _, err := svc.Connect(projectID, typ, "x"); err == nil || !strings.Contains(err.Error(), "integration.not_connectable") {
			t.Errorf("connect %s: %v, want integration.not_connectable", typ, err)
		}
	}
	if _, err := svc.Connect(projectID, "nope", "x"); err == nil || !strings.Contains(err.Error(), "integration.unsupported_type") {
		t.Errorf("connect an unknown type: %v", err)
	}
	if list, err := svc.ListByProject(projectID); err != nil || len(list) != 0 {
		t.Errorf("refused connections left %d integration(s) behind (%v)", len(list), err)
	}
}

// Reconectar troca o token e nada mais, e um token que não vale (ou que não enxerga o
// repositório) não chega a trocar.
func TestService_Reauthorize(t *testing.T) {
	svc, projectID := setup(t)
	it, err := svc.Connect(projectID, "github", testutil.GitHubOAuthToken)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	enabled := true
	if _, err := svc.Update(it.ID.String(), "Meu repositório", "", githubMetadata(), &enabled); err != nil {
		t.Fatalf("finish: %v", err)
	}

	again, err := svc.Reauthorize(it.ID.String(), testutil.GitHubSecondOAuthToken)
	if err != nil {
		t.Fatalf("reauthorize: %v", err)
	}
	if again.DisplayName != "Meu repositório" || !again.Enabled || !reflect.DeepEqual(again.Metadata, githubMetadata()) || again.Credentials != nil {
		t.Errorf("reauthorized = %+v, want the name, state and repository untouched and no credentials", again)
	}
	row := testClient.Integration.GetX(context.Background(), it.ID)
	if secrets, err := adapter.DecryptConfig(row.Credentials, testKey); err != nil || secrets["token"] != testutil.GitHubSecondOAuthToken {
		t.Errorf("stored secrets = %v, %v, want the new token", secrets, err)
	}
	// O token novo é o que lista: o fake responde outros repositórios para ele.
	if repos, err := svc.Repositories(it.ID.String()); err != nil || len(repos) != 1 || repos[0].FullName != "owner/second" {
		t.Errorf("repositories after reconnecting = %v, %v, want the ones of the new token", repos, err)
	}

	if _, err := svc.Reauthorize(it.ID.String(), testutil.InvalidToken); err == nil || !strings.Contains(err.Error(), "integration.invalid_token") {
		t.Errorf("reauthorize with an invalid token: %v", err)
	}
	row = testClient.Integration.GetX(context.Background(), it.ID)
	if secrets, _ := adapter.DecryptConfig(row.Credentials, testKey); secrets["token"] != testutil.GitHubSecondOAuthToken {
		t.Errorf("a refused reconnection changed the stored token to %v", secrets["token"])
	}
	if _, err := svc.Reauthorize(uuid.NewString(), testutil.GitHubOAuthToken); err == nil {
		t.Error("reauthorize an integration that does not exist: no error")
	}
}

func TestService_Repositories(t *testing.T) {
	svc, projectID := setup(t)
	it, err := svc.Connect(projectID, "github", testutil.GitHubOAuthToken)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	repos, err := svc.Repositories(it.ID.String())
	if err != nil || len(repos) != 2 || repos[0].FullName != "owner/repo" || !repos[0].Private {
		t.Errorf("repositories = %v, %v", repos, err)
	}

	// Sem credencial guardada (uma integração de antes) não há com o que listar.
	bare := testClient.Integration.Create().SetProjectID(uuid.MustParse(projectID)).SetType("github").
		SetDisplayName("Sem credencial").SaveX(context.Background())
	if _, err := svc.Repositories(bare.ID.String()); err == nil || !strings.Contains(err.Error(), "integration.no_credential") {
		t.Errorf("repositories without a credential: %v", err)
	}
	// Um tipo que não lista repositórios.
	gl, err := svc.Create(projectID, "gitlab", "GitLab", "glpat-test", map[string]interface{}{"project_url": "group/project"}, true)
	if err != nil {
		t.Fatalf("create gitlab: %v", err)
	}
	if _, err := svc.Repositories(gl.ID.String()); err == nil || !strings.Contains(err.Error(), "integration.no_repositories") {
		t.Errorf("repositories of a GitLab integration: %v", err)
	}
}
