package server_test

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
)

const errorDocsPath = "../../_docs/error-codes.md"

const errorDocsHeader = `# Códigos de erro da API

Este arquivo é gerado por ` + "`" + `internal/server/errors_doc_test.go` + "`" + ` a partir dos códigos declarados no
código (` + "`" + `apperr.New` + "`" + `) e dos textos de ` + "`" + `internal/i18n/locales` + "`" + `. Não edite a tabela à mão: depois de
mexer num código ou num texto, rode

` + "```" + `
UPDATE_ERROR_DOCS=1 go test ./internal/server -run ErrorCodesDoc
` + "```" + `

## Formato da resposta

A API **não manda mensagem**: manda um código estável, e quem chama mostra o texto no idioma
da pessoa. Toda resposta de erro tem este corpo, com o status HTTP da tabela:

` + "```" + `json
{ "error": { "code": "organization.field_too_long", "params": { "field": "summary", "max": 160 } } }
` + "```" + `

- ` + "`" + `code` + "`" + ` é ` + "`" + `domínio.motivo` + "`" + `, em minúsculas e com ` + "`" + `_` + "`" + `. Vale como contrato: um código existente não muda de
  significado nem some sem aviso.
- ` + "`" + `params` + "`" + ` só existe quando o texto precisa de valores, e traz exatamente os parâmetros da coluna
  *Parâmetros*. O parâmetro ` + "`" + `field` + "`" + ` é o nome de um campo da API; o cliente mostra o rótulo dele.
- Um erro que a API não previu vira ` + "`" + `internal.server_error` + "`" + ` (500), sem nenhum texto interno. O detalhe fica
  só no log do servidor.
- ` + "`" + `GET /api/tasks/:taskId/external-details` + "`" + ` responde 200 mesmo quando a plataforma falha: o campo
  ` + "`" + `error` + "`" + ` dessa resposta tem o mesmo formato (` + "`" + `{"code": "...", "params": {...}}` + "`" + `).

## Como o cliente mostra a mensagem

O navegador procura o texto em ` + "`" + `errors.<código>` + "`" + ` no catálogo do idioma (` + "`" + `WTT.errorText` + "`" + ` em ` + "`" + `app.js` + "`" + `),
troca os ` + "`" + `{{.parâmetro}}` + "`" + ` pelos valores e, para ` + "`" + `field` + "`" + `, usa o rótulo ` + "`" + `fields.<campo>` + "`" + `. Um código sem
texto mostra ` + "`" + `errors.unknown` + "`" + `. Outro cliente da API pode ter a própria tabela de textos.

## Como criar um código

1. Declare no ` + "`" + `errors.go` + "`" + ` do domínio: ` + "`" + `ErrX = apperr.New("dominio.motivo", http.StatusBadRequest, "param")` + "`" + `.
2. Devolva o erro no service (` + "`" + `return ErrX` + "`" + ` ou ` + "`" + `ErrX.With("param", valor)` + "`" + `); o handler responde com
   ` + "`" + `apperr.Respond(c, status, err)` + "`" + `.
3. Escreva o texto em ` + "`" + `errors.dominio.motivo` + "`" + ` nos dois ` + "`" + `locales/*.yaml` + "`" + `, só com os placeholders declarados.
4. Rode o comando acima e commite este arquivo. Os testes recusam um código sem texto, um texto sem código,
   um placeholder que o código não declara e este arquivo desatualizado.

## Códigos
`

func renderErrorDocs(t *testing.T) string {
	t.Helper()
	registered := apperr.Registered()
	pt, en := flatCatalog(t, "pt-BR"), flatCatalog(t, "en")
	codes := make([]string, 0, len(registered))
	for c := range registered {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	cell := func(s string) string { return "`" + strings.ReplaceAll(s, "|", "\\|") + "`" }
	var b strings.Builder
	b.WriteString(errorDocsHeader)
	domain := ""
	for _, code := range codes {
		d := code[:strings.Index(code, ".")]
		if d != domain {
			domain = d
			fmt.Fprintf(&b, "\n### %s\n\n| Código | Status | Parâmetros | pt-BR | en |\n|---|---|---|---|---|\n", d)
		}
		info := registered[code]
		params := "—"
		if len(info.Params) > 0 {
			params = "`" + strings.Join(info.Params, "`, `") + "`"
		}
		fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s |\n", code, info.Status, params, cell(pt["errors."+code]), cell(en["errors."+code]))
	}
	return b.String()
}

func TestErrorCodesDoc_IsUpToDate(t *testing.T) {
	want := renderErrorDocs(t)
	if os.Getenv("UPDATE_ERROR_DOCS") != "" {
		if err := os.WriteFile(errorDocsPath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(errorDocsPath)
	if err != nil {
		t.Fatalf("%v: run UPDATE_ERROR_DOCS=1 go test ./internal/server -run ErrorCodesDoc", err)
	}
	if string(got) != want {
		t.Error("_docs/error-codes.md is out of date: run UPDATE_ERROR_DOCS=1 go test ./internal/server -run ErrorCodesDoc")
	}
}
