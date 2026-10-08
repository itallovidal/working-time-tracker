package adapter

import (
	"net/url"
	"strings"
)

const defaultTrelloSiteURL = "https://trello.com"

// TrelloAuth é a autorização do Trello: a pessoa vai ao site dele, escolhe Permitir e volta para o
// RedirectURL com o token no fragmento da URL (#token=...), que o servidor nunca vê — quem o lê é a página
// de retorno, no navegador. APIKey é a chave do app (a do .env); ela é pública, o segredo é o token. Sem
// os dois, ou sem o endereço de retorno, o botão avisa que a conexão não está configurada.
type TrelloAuth struct {
	APIKey      string
	AppName     string // o nome que a tela de autorização do Trello mostra
	RedirectURL string // PUBLIC_URL mais o caminho da página de retorno
	SiteURL     string // vazio é trello.com; só se muda para um servidor fake
}

// Configured diz se o servidor tem tudo para mandar a pessoa autorizar.
func (a *TrelloAuth) Configured() bool {
	return a != nil && a.APIKey != "" && a.RedirectURL != ""
}

func (a *TrelloAuth) siteURL() string {
	if a.SiteURL == "" {
		return defaultTrelloSiteURL
	}
	return strings.TrimRight(a.SiteURL, "/")
}

// AuthorizeURL é o endereço da tela de autorização. O Trello não tem o parâmetro state: o nosso viaja na
// query do return_url, que ele devolve como está, com o token depois do "#". O acesso pedido é leitura e
// escrita nos quadros, sem prazo (a pessoa o revoga na conta do Trello).
func (a *TrelloAuth) AuthorizeURL(state string) string {
	name := a.AppName
	if name == "" {
		name = "Working Time Tracker"
	}
	q := url.Values{
		"key":             {a.APIKey},
		"name":            {name},
		"scope":           {"read,write"},
		"expiration":      {"never"},
		"response_type":   {"token"},
		"callback_method": {"fragment"},
		"return_url":      {a.RedirectURL + "?state=" + url.QueryEscape(state)},
	}
	return a.siteURL() + "/1/authorize?" + q.Encode()
}
