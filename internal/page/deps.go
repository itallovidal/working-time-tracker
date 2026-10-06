package page

import (
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/i18n"
)

// Deps são os services que as páginas usam para montar o cabeçalho (nomes de
// projeto e tarefa, resumo da organização). Os dados das listas vêm da API, não
// daqui.
type Deps struct {
	Orgs     *organization.Service
	Projects *project.Service
	Tasks    *task.Service

	// I18n traduz os textos das páginas; CookieSecure marca o cookie de idioma.
	I18n         *i18n.Catalog
	CookieSecure bool
}
