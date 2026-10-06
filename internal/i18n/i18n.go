// Package i18n carrega os catálogos de texto (YAML) e traduz por chave. Todo
// texto que a pessoa lê mora em locales/<idioma>.yaml; o código só conhece as
// chaves. O mesmo YAML vai para o navegador (ver Script), então o servidor e o
// JavaScript falam a mesma língua.
package i18n

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFS embed.FS

// Idiomas suportados. O primeiro é o padrão: vale sem cookie e sem
// Accept-Language que combine, e é para onde a tradução cai quando falta uma chave.
const (
	PTBR    = "pt-BR"
	EN      = "en"
	Default = PTBR
)

var supported = []string{PTBR, EN}

// Supported lista os idiomas na ordem em que o toggle os mostra.
func Supported() []string { return append([]string(nil), supported...) }

// Key transforma um código de idioma na forma usada nas chaves do YAML
// ("pt-BR" vira "pt_br").
func Key(lang string) string {
	return strings.ToLower(strings.ReplaceAll(lang, "-", "_"))
}

type script struct {
	body []byte
	hash string
}

// Catalog guarda as traduções de todos os idiomas, já carregadas.
type Catalog struct {
	locs    map[string]*goi18n.Localizer
	trees   map[string]map[string]any
	scripts map[string]script
	matcher language.Matcher
}

// Load lê os YAML embutidos. Um catálogo com erro faz a inicialização falhar,
// e não a primeira requisição.
func Load() (*Catalog, error) {
	return LoadFS(localeFS, "locales")
}

// LoadFS lê <dir>/<idioma>.yaml de fsys para cada idioma suportado.
func LoadFS(fsys fs.FS, dir string) (*Catalog, error) {
	bundle := goi18n.NewBundle(language.MustParse(Default))
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	c := &Catalog{
		locs:    map[string]*goi18n.Localizer{},
		trees:   map[string]map[string]any{},
		scripts: map[string]script{},
	}
	tags := make([]language.Tag, 0, len(supported))
	for _, lang := range supported {
		name := dir + "/" + lang + ".yaml"
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("i18n: %w", err)
		}
		if _, err := bundle.ParseMessageFileBytes(raw, lang+".yaml"); err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", name, err)
		}
		var tree map[string]any
		if err := yaml.Unmarshal(raw, &tree); err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", name, err)
		}
		c.trees[lang] = tree

		body, err := json.Marshal(map[string]any{"lang": lang, "messages": tree})
		if err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", name, err)
		}
		sum := sha256.Sum256(body)
		c.scripts[lang] = script{
			body: append(append([]byte("window.I18N="), body...), ';', '\n'),
			hash: hex.EncodeToString(sum[:])[:12],
		}
		c.locs[lang] = goi18n.NewLocalizer(bundle, lang, Default)
		tags = append(tags, language.MustParse(lang))
	}
	c.matcher = language.NewMatcher(tags)
	return c, nil
}

// Valid diz se o código é um idioma suportado, ignorando maiúsculas.
func (c *Catalog) Valid(lang string) (string, bool) {
	for _, s := range supported {
		if strings.EqualFold(s, lang) {
			return s, true
		}
	}
	return "", false
}

// Match escolhe o idioma da requisição: o cookie, se tiver um idioma
// suportado; senão o Accept-Language; senão o padrão.
func (c *Catalog) Match(cookie, acceptLanguage string) string {
	if lang, ok := c.Valid(cookie); ok {
		return lang
	}
	if acceptLanguage != "" {
		if tags, _, err := language.ParseAcceptLanguage(acceptLanguage); err == nil && len(tags) > 0 {
			if _, idx, conf := c.matcher.Match(tags...); conf != language.No {
				return supported[idx]
			}
		}
	}
	return Default
}

// T traduz a chave. Os argumentos são pares nome/valor para os placeholders
// ({{.name}}); o par "count" também escolhe a forma do plural (one/other). Chave
// que não existe volta como o próprio texto da chave, para o erro aparecer na
// tela e nos testes em vez de sumir.
func (c *Catalog) T(lang, key string, args ...any) string {
	loc, ok := c.locs[lang]
	if !ok {
		loc = c.locs[Default]
	}
	cfg := &goi18n.LocalizeConfig{MessageID: key}
	if data := pairs(args); len(data) > 0 {
		cfg.TemplateData = data
		if n, ok := data["count"]; ok {
			cfg.PluralCount = n
		}
	}
	s, err := loc.Localize(cfg)
	if err != nil {
		return key
	}
	return s
}

func pairs(args []any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	m := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			m[k] = args[i+1]
		}
	}
	return m
}

// Script devolve o catálogo do idioma como JavaScript (window.I18N = {...}) e
// um hash do conteúdo, para o navegador guardar em cache e renovar só quando mudar.
func (c *Catalog) Script(lang string) (body []byte, hash string, ok bool) {
	s, ok := c.scripts[lang]
	return s.body, s.hash, ok
}

// Hash é o hash do script do idioma, para a URL do <script>.
func (c *Catalog) Hash(lang string) string {
	return c.scripts[lang].hash
}

// Has diz se a chave tem texto no idioma. Serve a textos opcionais, como o
// placeholder de um campo, que só existe em alguns.
func (c *Catalog) Has(lang, key string) bool {
	var node any = c.trees[lang]
	for _, part := range strings.Split(key, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return false
		}
		if node, ok = m[part]; !ok {
			return false
		}
	}
	_, ok := node.(string)
	return ok
}

// Tree devolve o YAML do idioma já lido, para os testes conferirem as chaves.
func (c *Catalog) Tree(lang string) map[string]any {
	return c.trees[lang]
}
