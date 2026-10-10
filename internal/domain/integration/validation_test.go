package integration_test

import (
	"errors"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/validate"
	"working-time-tracker/testutil"
)

// codeAndField devolve o código do erro e o parâmetro field.
func codeAndField(err error) (code, field string) {
	var e *apperr.Error
	if !errors.As(err, &e) {
		return "", ""
	}
	field, _ = e.Params["field"].(string)
	return e.Code, field
}

// O nome da integração é aparado e vai de 1 a 120 caracteres, na criação e na edição; o token é aparado e vai
// até 512.
func TestService_NameAndToken_Validation(t *testing.T) {
	svc, projectID := setup(t)
	base, err := svc.Create(projectID, "github", "Base", "ghp_test", githubMetadata(), true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := base.ID.String()

	names := []struct {
		name, in, wantCode, wantName string
	}{
		{"empty", "", "integration.name_required", ""},
		{"spaces only", "   ", "integration.name_required", ""},
		{"trimmed", "  Repo  ", "", "Repo"},
		{"at the limit", strings.Repeat("a", validate.MaxName), "", strings.Repeat("a", validate.MaxName)},
		{"limit plus one", strings.Repeat("a", validate.MaxName+1), "request.field_too_long", ""},
		{"limit counts characters", strings.Repeat("ç", validate.MaxName), "", strings.Repeat("ç", validate.MaxName)},
	}
	for _, tc := range names {
		t.Run("create "+tc.name, func(t *testing.T) {
			it, err := svc.Create(projectID, "github", tc.in, "ghp_test", githubMetadata(), true)
			if tc.wantCode == "" {
				if err != nil || it.DisplayName != tc.wantName {
					t.Fatalf("create = %+v, %v; want the name %q", it, err, tc.wantName)
				}
				return
			}
			if code, field := codeAndField(err); code != tc.wantCode || field != "display_name" {
				t.Errorf("err = %v (field %q), want %s on display_name", err, field, tc.wantCode)
			}
		})
		t.Run("edit "+tc.name, func(t *testing.T) {
			if _, err := svc.Update(id, "Base", "", nil, nil); err != nil {
				t.Fatal(err)
			}
			it, err := svc.Update(id, tc.in, "", nil, nil)
			switch {
			case tc.in == "":
				// Nome vazio na edição é "não mexer".
				if err != nil || it.DisplayName != "Base" {
					t.Errorf("edit with no name = %+v, %v; want the name kept", it, err)
				}
			case tc.wantCode == "":
				if err != nil || it.DisplayName != tc.wantName {
					t.Fatalf("edit = %+v, %v; want the name %q", it, err, tc.wantName)
				}
			default:
				code, field := codeAndField(err)
				wantCode := tc.wantCode
				if code != wantCode || field != "display_name" {
					t.Errorf("err = %v (field %q), want %s on display_name", err, field, wantCode)
				}
				// Um nome de espaços não é gravado.
				if got, _ := svc.Get(id); got.DisplayName != "Base" {
					t.Errorf("name after a refused edit = %q, want Base", got.DisplayName)
				}
			}
		})
	}

	tokens := []struct {
		name, in, wantCode string
	}{
		{"at the limit", strings.Repeat("t", validate.MaxToken), ""},
		{"limit plus one", strings.Repeat("t", validate.MaxToken+1), "request.field_too_long"},
		{"spaces do not count", " " + strings.Repeat("t", validate.MaxToken) + " ", ""},
	}
	for _, tc := range tokens {
		t.Run("token "+tc.name, func(t *testing.T) {
			_, err := svc.Create(projectID, "github", "Tok", tc.in, githubMetadata(), true)
			if tc.wantCode == "" {
				if err != nil {
					t.Errorf("create = %v, want accepted", err)
				}
			} else if code, field := codeAndField(err); code != tc.wantCode || field != "token" {
				t.Errorf("create = %v (field %q), want %s on token", err, field, tc.wantCode)
			}
			_, err = svc.Update(id, "", tc.in, nil, nil)
			if tc.wantCode == "" {
				if err != nil {
					t.Errorf("update = %v, want accepted", err)
				}
			} else if code, field := codeAndField(err); code != tc.wantCode || field != "token" {
				t.Errorf("update = %v (field %q), want %s on token", err, field, tc.wantCode)
			}
		})
	}
}

// O repositório, o projeto e o quadro são conferidos no formato e dizem o campo; um token que a plataforma
// recusa diz token.
func TestService_Connection_FieldsInTheErrors(t *testing.T) {
	svc, projectID := setup(t)
	longRepo := "owner/" + strings.Repeat("r", validate.MaxRepo)

	cases := []struct {
		name, kind string
		token      string
		meta       map[string]interface{}
		wantCode   string
		wantField  string
	}{
		{"github repo with spaces", "github", "ghp", map[string]interface{}{"repo": "not a repo"}, "integration.github_invalid_repo", "repo"},
		{"github repo without owner", "github", "ghp", map[string]interface{}{"repo": "repo"}, "integration.github_invalid_repo", "repo"},
		{"github repo of dots", "github", "ghp", map[string]interface{}{"repo": "owner/.."}, "integration.github_invalid_repo", "repo"},
		{"github repo over the limit", "github", "ghp", map[string]interface{}{"repo": longRepo}, "request.field_too_long", "repo"},
		{"github repo empty", "github", "ghp", map[string]interface{}{"repo": "   "}, "integration.field_required", "repo"},
		{"github repo not text", "github", "ghp", map[string]interface{}{"repo": 7}, "integration.field_not_text", "repo"},
		{"gitlab project with spaces", "gitlab", "glpat", map[string]interface{}{"project_url": "a b"}, "integration.gitlab_invalid_project", "project_url"},
		{"trello board with symbols", "trello", "tok", map[string]interface{}{"api_key": testutil.TrelloKey, "board_id": "###"}, "integration.trello_invalid_board", "board_id"},
		{"invalid token", "github", testutil.InvalidToken, githubMetadata(), "integration.invalid_token", "token"},
		{"token missing", "github", "   ", githubMetadata(), "integration.token_required", "token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(projectID, tc.kind, "Nome", tc.token, tc.meta, true)
			if code, field := codeAndField(err); code != tc.wantCode || field != tc.wantField {
				t.Errorf("err = %v (field %q), want %s on %s", err, field, tc.wantCode, tc.wantField)
			}
		})
	}

	// A repo com espaços nas pontas é aparado, e o endereço colado vira dono/repositório.
	it, err := svc.Create(projectID, "github", "Pontas", "ghp", map[string]interface{}{"repo": "  owner/repo  "}, true)
	if err != nil || it.Metadata["repo"] != "owner/repo" {
		t.Errorf("repo with spaces = %+v, %v; want it trimmed", it, err)
	}

	// Na edição, o formato também diz o campo.
	_, err = svc.Update(it.ID.String(), "", "", map[string]interface{}{"repo": "not a repo"}, nil)
	if code, field := codeAndField(err); code != "integration.github_invalid_repo" || field != "repo" {
		t.Errorf("edit with a bad repo = %v (field %q)", err, field)
	}
}
