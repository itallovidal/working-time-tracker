// Package page serve as páginas HTML da interface web. As páginas são cascas:
// o servidor resolve quem está logado e o contexto (org, projeto), e os
// componentes Alpine buscam e alteram os dados pela API JSON em /api.
package page

import (
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/country"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/i18n"
	"working-time-tracker/internal/validate"
)

// Crumb identifica um recurso no cabeçalho da página.
type Crumb struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Data é o que todo template recebe.
type Data struct {
	Title string
	// TitleKey é a chave do título no catálogo; o render a traduz e põe em Title.
	// TitleSuffix, se houver, vai depois: "Tarefas · Projeto Alfa".
	TitleKey    string
	TitleSuffix string
	Me          *auth.Identity
	Section     string                     // item ativo da barra superior: home, projects, people, customers, payments, organization, profile, help
	Org         *organization.Organization // só nas páginas da organização
	Project     *Crumb
	Task        *Crumb // só na página da tarefa, que tem cabeçalho próprio, sem as abas do projeto
	Person      *Crumb // só no perfil de um colaborador, aberto por um admin
	Management  bool   // a página é da área de Gestão: a barra de abas mostra as abas dela que a pessoa abre
	Tab         string // aba ativa do projeto (overview, tasks, time, teams, integrations, settings)
	Script      string // página em /static/pages/<Script>.js com os componentes Alpine
	Markdown    bool   // a página mostra ou escreve Markdown: carrega o marked e o DOMPurify (ligado em prepare)
	// Bare tira a barra superior e o ponto aberto de uma página de quem está logado: a tela de consentimento do OAuth
	// é só a pergunta, como o login.
	Bare  bool
	Props map[string]any
	// Access é o que a pessoa pode fazer no projeto da página (tudo, para os admins).
	Access permission.Set

	Lang string // idioma da requisição: pt-BR ou en
	Path string // caminho e query da página, para o toggle de idioma voltar para ela

	cat *i18n.Catalog
}

// T traduz uma chave do catálogo no idioma da página. Os argumentos são pares
// nome/valor: {{.T "session.in_progress" "name" .Name}}. Dentro de range ou with,
// use {{$.T ...}}. Texto dentro de expressões Alpine usa $t no navegador, não isto.
func (d Data) T(key string, args ...any) string {
	return d.cat.T(d.Lang, key, args...)
}

// LangOption é uma opção do toggle de idioma.
type LangOption struct {
	Code    string
	Short   string
	Name    string
	Current bool
}

// LangOptions lista os idiomas para o toggle, marcando o atual.
func (d Data) LangOptions() []LangOption {
	var out []LangOption
	for _, code := range i18n.Supported() {
		k := i18n.Key(code)
		out = append(out, LangOption{
			Code:    code,
			Short:   d.T("lang." + k + ".short"),
			Name:    d.T("lang." + k + ".name"),
			Current: code == d.Lang,
		})
	}
	return out
}

// I18nHash identifica a versão do catálogo, para a URL do script de textos.
func (d Data) I18nHash() string {
	return d.cat.Hash(d.Lang)
}

// Granted lista as permissões que quem olha tem na página, para o JavaScript esconder o
// que não pode usar: as do projeto da página e as da organização.
func (d Data) Granted() []string {
	out := []string{}
	if d.Project != nil {
		for _, k := range permission.ProjectKeys {
			if d.Access.Has(k) {
				out = append(out, k)
			}
		}
	}
	for _, k := range permission.OrganizationKeys {
		if d.Me.Can(k) {
			out = append(out, k)
		}
	}
	return out
}

// Boot vira window.BOOT na página: os dados iniciais que o JavaScript precisa.
func (d Data) Boot() map[string]any {
	boot := map[string]any{"me": d.Me, "lang": d.Lang, "can": d.Granted(), "limits": validate.Limits()}
	if d.Project != nil {
		boot["project"] = d.Project
	}
	if d.Person != nil {
		boot["person"] = d.Person
	}
	// O cadastro de países (rótulos, máscaras, estados) vai só às páginas que o desenham: o cadastro, a volta do Clerk e
	// as páginas da organização, que são as do script org.
	if d.cat != nil && (d.Script == "auth" || d.Script == "org") {
		boot["countries"] = country.Describe(d.T)
	}
	for k, v := range d.Props {
		boot[k] = v
	}
	return boot
}

func (d Data) OrgID() string {
	if d.Me == nil {
		return ""
	}
	return d.Me.OrganizationID.String()
}

func (d Data) IsAdmin() bool {
	return d.Me.IsAdmin()
}

// Can diz se quem olha pode fazer o que a permissão libera: as do projeto, no projeto da
// página, e as da organização, em qualquer uma.
func (d Data) Can(key string) bool {
	if slices.Contains(permission.OrganizationKeys, key) {
		return d.Me.Can(key)
	}
	return d.Access.Has(key)
}

// CanDo é Can só dentro da Gestão: fora dela as telas são de leitura, para todos.
func (d Data) CanDo(key string) bool {
	return d.Management && d.Can(key)
}

// CanDoAny diz se alguma das permissões vale na Gestão.
func (d Data) CanDoAny(keys ...string) bool {
	for _, k := range keys {
		if d.CanDo(k) {
			return true
		}
	}
	return false
}

// ManagementTab é uma aba da Gestão e as permissões do projeto que a abrem (basta uma).
type ManagementTab struct {
	ID   string
	Keys []string
}

// ManagementTabs são as abas da Gestão, na ordem da barra. A rota de cada aba (routes/pages.go), a barra de abas e a
// raiz da Gestão leem daqui, para ninguém ver uma aba que responde 404.
var ManagementTabs = []ManagementTab{
	{"overview", permission.ProjectKeys},
	{"teams", []string{permission.CollaboratorsManage, permission.TeamsManage, permission.RatesView, permission.RatesManage}},
	{"integrations", []string{permission.IntegrationsManage}},
	{"settings", []string{permission.ProjectEdit, permission.BillingView, permission.BillingManage}},
}

// TabKeys devolve as permissões que abrem a aba da Gestão de nome id (nenhuma, para uma aba que não existe).
func TabKeys(id string) []string {
	for _, t := range ManagementTabs {
		if t.ID == id {
			return t.Keys
		}
	}
	return nil
}

// OpensTab diz se quem olha abre a aba da Gestão de nome id. Numa organização no modo solo a aba Colaboradores não
// existe (a rota dela responde 404), por mais que a pessoa tenha as permissões dela.
func (d Data) OpensTab(id string) bool {
	if id == "teams" && d.Me != nil && d.Me.OrganizationSolo {
		return false
	}
	return d.Access.HasAny(TabKeys(id)...)
}

// ProjectTab é uma aba da barra do projeto: o endereço, o ícone, a chave do rótulo no catálogo, se é a página aberta e
// se vai para o fim da barra (o botão da Gestão e a volta dela).
type ProjectTab struct {
	ID      string
	Href    string
	Icon    string
	Label   string
	Current bool
	End     bool
}

// managementTabViews são o ícone e o rótulo de cada aba da Gestão (ManagementTabs).
var managementTabViews = map[string]struct{ icon, label string }{
	"overview":     {"chart-simple", "project.head.overview"},
	"teams":        {"people-group", "project.head.teams"},
	"integrations": {"plug", "project.head.integrations"},
	"settings":     {"gear", "project.head.settings"},
}

// outsideManagement diz se a aba da Gestão de nome id mora na barra principal do projeto, junto do Início e das
// Tarefas, em vez de dentro da Gestão. No modo solo a Visão geral é uma tela do dia a dia: a pessoa é a própria
// gestora, e a Gestão fica com Integrações e Configurações. A outra diferença da barra entre os modos fica em
// ProjectTabs: no solo, Minhas tarefas e a Lista de tarefas são uma aba só, Tarefas (Decisão 28).
func (d Data) outsideManagement(id string) bool {
	return id == "overview" && d.Solo()
}

// opensManagement diz se a pessoa abre alguma aba que mora dentro da Gestão: é quando o botão Gestão aparece.
func (d Data) opensManagement() bool {
	return d.firstManagementTab() != ""
}

// firstManagementTab é a primeira aba de dentro da Gestão que a pessoa abre, ou "" quando não abre nenhuma. A raiz da
// Gestão leva para ela.
func (d Data) firstManagementTab() string {
	for _, t := range ManagementTabs {
		if !d.outsideManagement(t.ID) && d.OpensTab(t.ID) {
			return t.ID
		}
	}
	return ""
}

// managementTab monta a aba da Gestão de nome id.
func (d Data) managementTab(id string) ProjectTab {
	v := managementTabViews[id]
	return ProjectTab{
		ID: id, Href: "/projects/" + d.Project.ID + "/management/" + id, Icon: v.icon, Label: v.label,
		Current: d.Management && d.Tab == id,
	}
}

// ProjectTabs é a barra de abas do projeto (partials/project_head). Dentro da Gestão: as abas dela que a pessoa abre e a
// volta para o projeto. Fora: Início, Minhas tarefas, Lista de tarefas, Colaboradores (fora do modo solo), as abas da
// Gestão que moram fora (outsideManagement) e o botão Gestão. No modo solo Minhas tarefas não existe: a aba das
// tarefas, Tarefas, junta as duas (Decisão 28). Uma aba da Gestão que mora fora mostra a barra de fora, com ela marcada.
func (d Data) ProjectTabs() []ProjectTab {
	if d.Project == nil {
		return nil
	}
	base := "/projects/" + d.Project.ID
	if d.Management && !d.outsideManagement(d.Tab) {
		var tabs []ProjectTab
		for _, t := range ManagementTabs {
			if !d.outsideManagement(t.ID) && d.OpensTab(t.ID) {
				tabs = append(tabs, d.managementTab(t.ID))
			}
		}
		return append(tabs, ProjectTab{ID: "back", Href: base + "/overview", Icon: "arrow-left", Label: "project.head.back", End: true})
	}
	current := func(id string) bool { return !d.Management && d.Tab == id }
	home := ProjectTab{ID: "home", Href: base + "/overview", Icon: "house", Label: "project.head.home", Current: current("home")}
	// No modo solo a organização é uma pessoa só: as tarefas dela e as do projeto são as mesmas, numa aba (Decisão 28),
	// e a aba Colaboradores (as pessoas e os times) não existe.
	if d.Solo() {
		tabs := []ProjectTab{home, {ID: "tasks", Href: base + "/tasks", Icon: "list-check", Label: "project.head.tasks_solo", Current: current("tasks")}}
		return d.appendOutsideTabs(tabs)
	}
	tabs := []ProjectTab{
		home,
		{ID: "mine", Href: base + "/my-tasks", Icon: "user-check", Label: "project.head.my_tasks", Current: current("mine")},
		{ID: "tasks", Href: base + "/tasks", Icon: "list-check", Label: "project.head.tasks", Current: current("tasks")},
		{ID: "teams", Href: base + "/collaborators", Icon: "people-group", Label: "project.head.teams", Current: current("teams")},
	}
	return d.appendOutsideTabs(tabs)
}

// appendOutsideTabs põe no fim da barra de fora da Gestão as abas dela que moram fora (outsideManagement) e o botão
// Gestão, para quem abre alguma aba de dentro.
func (d Data) appendOutsideTabs(tabs []ProjectTab) []ProjectTab {
	base := "/projects/" + d.Project.ID
	for _, t := range ManagementTabs {
		if d.outsideManagement(t.ID) && d.OpensTab(t.ID) {
			tabs = append(tabs, d.managementTab(t.ID))
		}
	}
	if d.Access.Manages() && d.opensManagement() {
		tabs = append(tabs, ProjectTab{ID: "management", Href: base + "/management", Icon: "sliders", Label: "project.head.management", End: true})
	}
	return tabs
}

// IsOwner diz se quem olha é o dono da organização.
func (d Data) IsOwner() bool {
	return d.Me != nil && d.Me.IsOwner
}

// Solo diz se quem olha está numa organização no modo solo. Vale também nas páginas públicas (a ajuda), onde não há
// ninguém logado e .Me.OrganizationSolo quebraria o template.
func (d Data) Solo() bool {
	return d.Me != nil && d.Me.OrganizationSolo
}

func (d Data) Initials() string {
	if d.Me == nil {
		return ""
	}
	return initialsOf(d.Me.Name)
}

// PersonInitials são as iniciais da pessoa do perfil aberto por um admin.
func (d Data) PersonInitials() string {
	if d.Person == nil {
		return ""
	}
	return initialsOf(d.Person.Name)
}

// initialsOf são as iniciais das duas primeiras palavras do nome: "Ana Souza" -> "AS".
func initialsOf(name string) string {
	out := ""
	for i, p := range strings.Fields(name) {
		if i == 2 {
			break
		}
		out += strings.ToUpper(string([]rune(p)[0]))
	}
	return out
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// markdownPages são as páginas que renderizam Markdown (a descrição da tarefa).
var markdownPages = map[string]bool{"project_tasks": true, "task_detail": true}

func (h *Handler) render(c *echo.Context, name string, d Data) error {
	d.Markdown = markdownPages[name]
	return c.Render(http.StatusOK, name, h.prepare(c, d))
}

// prepare preenche o que toda página tem em comum: quem está logado, o idioma e
// o título traduzido.
func (h *Handler) prepare(c *echo.Context, d Data) Data {
	d.Me = auth.CurrentPerson(c)
	d.Access = auth.ProjectPermissions(c)
	d.Lang = h.deps.I18n.Lang(c)
	d.Path = c.Request().URL.RequestURI()
	d.cat = h.deps.I18n
	if d.TitleKey != "" {
		d.Title = d.T(d.TitleKey)
		if d.TitleSuffix != "" {
			d.Title += " · " + d.TitleSuffix
		}
	}
	return d
}

// NotFound é a página de 404 para rotas do navegador.
func (h *Handler) NotFound(c *echo.Context) error {
	return c.Render(http.StatusNotFound, "notfound", h.prepare(c, Data{TitleKey: "titles.not_found"}))
}

// SetLanguage grava o idioma escolhido no toggle e volta para a página de onde
// a pessoa veio. Funciona sem JavaScript e nas telas sem login.
func (h *Handler) SetLanguage(c *echo.Context) error {
	if lang, ok := h.deps.I18n.Valid(c.Param("code")); ok {
		i18n.SetCookie(c, lang, h.deps.CookieSecure)
	}
	return c.Redirect(http.StatusSeeOther, safeNext(c.QueryParam("next")))
}

// I18nScript serve os textos do idioma para o JavaScript (window.I18N). O ?v=
// é o hash do conteúdo: com ele a URL nunca muda de significado e pode ser guardada
// para sempre; sem ele o navegador revalida a cada uso.
func (h *Handler) I18nScript(c *echo.Context) error {
	lang, ok := h.deps.I18n.Valid(strings.TrimSuffix(c.Param("file"), ".js"))
	if !ok || !strings.HasSuffix(c.Param("file"), ".js") {
		return echo.ErrNotFound
	}
	body, hash, _ := h.deps.I18n.Script(lang)
	res := c.Response()
	res.Header().Set("ETag", `"`+hash+`"`)
	if c.QueryParam("v") == hash {
		res.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		res.Header().Set("Cache-Control", "no-cache")
	}
	if c.Request().Header.Get("If-None-Match") == `"`+hash+`"` {
		return c.NoContent(http.StatusNotModified)
	}
	return c.Blob(http.StatusOK, "text/javascript; charset=utf-8", body)
}

// safeNext só aceita caminhos locais, para o ?next= do login não virar um
// redirecionamento para outro site. Caracteres de controle e barra invertida
// são recusados em qualquer posição: o navegador remove TAB e quebra de linha
// da URL e trata "\" como "/", então "/\t/host" chegaria como "//host".
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") ||
		strings.ContainsRune(next, '\\') || strings.IndexFunc(next, unicode.IsControl) >= 0 {
		return "/"
	}
	return next
}
