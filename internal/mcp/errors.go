package mcp

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/i18n"
)

// Os erros que só o MCP dá. O texto em cada idioma está em internal/i18n/locales, na chave errors.<código>, como os
// da API.
var (
	ErrUnknownLabel      = apperr.New("mcp.unknown_label", http.StatusBadRequest, "label", "available")
	ErrInvalidDate       = apperr.New("mcp.invalid_date", http.StatusBadRequest, "field")
	ErrInvalidAssignee   = apperr.New("mcp.invalid_assignee", http.StatusBadRequest)
	ErrTaskNotInSession  = apperr.New("mcp.task_not_in_session", http.StatusBadRequest)
	ErrTaskAlreadyActive = apperr.New("mcp.task_already_active", http.StatusBadRequest)
	// ErrProjectRequired é o clock_out sem projeto quando há sessões abertas em mais de um.
	ErrProjectRequired = apperr.New("mcp.project_required", http.StatusBadRequest)
	// ErrProjectSyncOff é o sync_project num projeto sem nenhuma integração com a sincronização ligada. O
	// integration.sync_off do domínio fala de uma integração, o que confunde quem pediu o projeto inteiro.
	ErrProjectSyncOff = apperr.New("mcp.project_sync_off", http.StatusBadRequest)
	// ErrLabelsConflict é o update_task com labels (trocar todas) junto com add_labels ou remove_labels.
	ErrLabelsConflict = apperr.New("mcp.labels_conflict", http.StatusBadRequest)
	// ErrFindCriteria é a find_tasks sem issue e sem query.
	ErrFindCriteria = apperr.New("mcp.find_criteria_required", http.StatusBadRequest)
	// ErrInvalidIssue é a issue da find_tasks que não é a URL de uma issue ou de um cartão, nem owner/repo#12.
	ErrInvalidIssue = apperr.New("mcp.invalid_issue", http.StatusBadRequest)
)

// toolError é o erro que a ferramenta devolve ao Claude: o texto do catálogo (em inglês), o código entre parênteses
// e, quando há, os parâmetros e uma dica do que fazer. O SDK o põe no resultado com isError.
type toolError struct {
	code, text, hint string
	params           map[string]any
}

func (e *toolError) Error() string {
	var b strings.Builder
	b.WriteString(e.text)
	b.WriteString(" (")
	b.WriteString(e.code)
	if len(e.params) > 0 {
		keys := make([]string, 0, len(e.params))
		for k := range e.params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "; %s=%v", k, e.params[k])
		}
	}
	b.WriteString(")")
	if e.hint != "" {
		b.WriteString(". ")
		b.WriteString(e.hint)
	}
	return b.String()
}

// fail traduz um erro de um service para o erro da ferramenta. Um registro que não existe é request.not_found, como
// na API. Um erro sem código é inesperado: vai para o log, e o Claude recebe só internal.server_error.
func (s *server) fail(err error) error {
	return s.failWith(err, "")
}

// failWith é o fail com uma dica para o Claude (o que fazer em seguida).
func (s *server) failWith(err error, hint string) error {
	var te *toolError
	if errors.As(err, &te) {
		return te
	}
	var ae *apperr.Error
	switch {
	case errors.As(err, &ae):
	case errors.Is(err, database.ErrNotFound):
		ae = apperr.ErrNotFound
	default:
		s.Logger.Error("mcp: unexpected error", "error", err)
		ae = apperr.ErrInternal
	}
	args := make([]any, 0, 2*len(ae.Params))
	for k, v := range ae.Params {
		args = append(args, k, v)
	}
	// O texto vai em inglês, como as descrições das ferramentas e as dicas: é para o modelo, que responde à pessoa no
	// idioma dela.
	text := ae.Code
	if s.I18n != nil {
		text = s.I18n.T(i18n.EN, "errors."+ae.Code, args...)
	}
	return &toolError{code: ae.Code, text: text, hint: hint, params: ae.Params}
}
