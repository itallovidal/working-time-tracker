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

	// OAuthConfigured diz, por tipo de integração, se o servidor tem o app OAuth dele
	// cadastrado. A tela de integrações avisa quando o botão Conectar não pode funcionar.
	OAuthConfigured map[string]bool

	// Clerk são os dados que as páginas de entrada precisam para montar o Clerk no navegador. Nulo, o login
	// pelo Clerk está desligado e as páginas ficam como eram.
	Clerk *Clerk
}

// Clerk é o que o navegador precisa para carregar o clerk-js: a chave pública do app (ela não é segredo) e o
// endereço do Frontend API, de onde o script vem.
type Clerk struct {
	PublishableKey string `json:"publishableKey"`
	FrontendAPI    string `json:"frontendApi"`
}
