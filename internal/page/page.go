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

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/i18n"
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
	Section     string                     // item ativo da barra superior: home, organization, profile
	Org         *organization.Organization // só nas páginas da organização
	Project     *Crumb
	Task        *Crumb // só na página da tarefa, que tem cabeçalho próprio, sem as abas do projeto
	Management  bool   // a página é da área de Gestão (só admins): a barra de abas mostra as abas dela
	Tab         string // aba ativa do projeto (overview, tasks, time, teams, integrations, settings) ou da organização (about, people, customers, projects)
	Script      string // página em /static/pages/<Script>.js com os componentes Alpine
	Markdown    bool   // a página mostra ou escreve Markdown: carrega o marked e o DOMPurify (ligado em prepare)
	Props       map[string]any
	// Access é o que a pessoa pode fazer no projeto da página (tudo, para os admins).
	Access permission.Set

	Lang string // idioma da requisição: pt-BR ou en
	Path string // caminho e query da página, para o toggle de idioma voltar para ela

	// IDPrefix vai na frente dos id dos campos de um parcial que a página repete (os campos de
	// atributos da tarefa estão no modal Editar e no de atualização rápida): dois id iguais na
	// mesma página quebram o `for` dos rótulos. Vem de WithIDPrefix; o padrão é vazio.
	IDPrefix string

	cat *i18n.Catalog
}

// WithIDPrefix devolve os mesmos dados com um prefixo para os id dos campos de um parcial:
// {{template "task_attrs" (.WithIDPrefix "quick-")}}.
func (d Data) WithIDPrefix(prefix string) Data {
	d.IDPrefix = prefix
	return d
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
	boot := map[string]any{"me": d.Me, "lang": d.Lang, "can": d.Granted()}
	if d.Project != nil {
		boot["project"] = d.Project
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

// IsOwner diz se quem olha é o dono da organização.
func (d Data) IsOwner() bool {
	return d.Me != nil && d.Me.IsOwner
}

func (d Data) Initials() string {
	if d.Me == nil {
		return ""
	}
	parts := strings.Fields(d.Me.Name)
	out := ""
	for i, p := range parts {
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
