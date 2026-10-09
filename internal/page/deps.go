package page

import (
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/i18n"
)

// Deps são os services que as páginas usam para montar o cabeçalho (nomes de
// projeto, tarefa e pessoa, resumo da organização). Os dados das listas vêm da API, não
// daqui.
type Deps struct {
	Orgs     *organization.Service
	Projects *project.Service
	Tasks    *task.Service
	People   *person.Service

	// I18n traduz os textos das páginas; CookieSecure marca o cookie de idioma.
	I18n         *i18n.Catalog
	CookieSecure bool

	// OAuthConfigured diz, por tipo de integração, se o servidor tem o app OAuth dele
	// cadastrado. A tela de integrações avisa quando o botão Conectar não pode funcionar.
	OAuthConfigured map[string]bool

	// Clerk são os dados que as páginas de entrada precisam para montar o Clerk no navegador. Nulo, o login
	// pelo Clerk está desligado e as páginas ficam como eram.
	Clerk *Clerk
	// InviteByEmail diz que o convite sai por e-mail (o Clerk está ligado): a tela de colaboradores e a ajuda
	// falam em enviar o convite, e não só em copiar o link.
	InviteByEmail bool
}

// Clerk é o que o navegador precisa para carregar o clerk-js: a chave pública do app (ela não é segredo) e o
// endereço do Frontend API, de onde o script vem.
type Clerk struct {
	PublishableKey string `json:"publishableKey"`
	FrontendAPI    string `json:"frontendApi"`
}
