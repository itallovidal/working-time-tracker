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
	proj, err := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Project", "", 0, nil, nil, nil)
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

	// O estado de um cartão é a lista em que ele está.
	res, err := svc.FetchItemDetails(id, "H0TZyzbK")
	if err != nil || res.Details == nil || res.Details.Title != "Corrigir login" || res.Details.State != "Em andamento" {
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
