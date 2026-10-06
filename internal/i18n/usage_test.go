package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// As chaves aparecem no código sempre como texto literal, e é isso que a varredura
// procura:
//   - templates: {{.T "chave"}} e {{$.T "chave"}}
//   - navegador: $t('chave') e WTT.t('chave'), e t('chave') dentro de app.js
//   - páginas em Go: TitleKey: "chave"
//
// Chaves montadas em tempo de execução ('labels.role.' + valor) não são vistas
// por aqui; quem as monta fica coberto pelo teste da própria família.
var usagePatterns = map[string]*regexp.Regexp{
	".gohtml": regexp.MustCompile(`\.T\s+"([^"]+)"|\$t\(\s*'([^']+)'\s*[,)]`),
	".js":     regexp.MustCompile(`\$t\(\s*'([^']+)'\s*[,)]|WTT\.t\(\s*'([^']+)'\s*[,)]|(?:^|[^\w$.])t\(\s*'([^']+)'\s*[,)]`),
	".go":     regexp.MustCompile(`TitleKey:\s*"([^"]+)"`),
}

type usage struct{ file, key string }

func scan(t *testing.T, root string, exts ...string) []usage {
	t.Helper()
	var found []usage
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(path, ".min.js") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		ext := filepath.Ext(path)
		re, ok := usagePatterns[ext]
		if !ok {
			return nil
		}
		wanted := false
		for _, e := range exts {
			wanted = wanted || e == ext
		}
		if !wanted {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue // exemplos em comentários
			}
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				for _, key := range m[1:] {
					if key != "" {
						found = append(found, usage{path, key})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// Toda chave usada nos templates, no JavaScript e nas páginas em Go precisa existir
// em todos os idiomas.
func TestUsedKeysExist(t *testing.T) {
	c := catalog(t)
	var used []usage
	used = append(used, scan(t, "../../web/templates", ".gohtml")...)
	used = append(used, scan(t, "../../web/static", ".js")...)
	used = append(used, scan(t, "../page", ".go")...)
	if len(used) == 0 {
		t.Fatal("no translation keys found: the scan patterns are out of date")
	}
	for _, lang := range Supported() {
		flat := flatOf(t, c, lang)
		for _, u := range used {
			if _, ok := flat[u.key]; ok {
				continue
			}
			if _, ok := flat[u.key+"#other"]; ok {
				continue
			}
			t.Errorf("%s uses %q, which %s does not have", u.file, u.key, lang)
		}
	}
}

// O toggle monta as chaves lang.<idioma>.short e .name; cada idioma suportado precisa delas.
func TestLanguageToggleKeys(t *testing.T) {
	c := catalog(t)
	for _, lang := range Supported() {
		flat := flatOf(t, c, lang)
		for _, other := range Supported() {
			for _, field := range []string{"short", "name"} {
				key := "lang." + Key(other) + "." + field
				if flat[key] == "" {
					t.Errorf("%s is missing %q", lang, key)
				}
			}
		}
	}
}
