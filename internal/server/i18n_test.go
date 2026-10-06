package server_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/i18n"
)

// getPage faz um GET como o navegador faria: com a sessão, o cookie de idioma e o
// Accept-Language que o teste quiser.
func getPage(e *echo.Echo, path, session, langCookie, acceptLanguage string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if session != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	}
	if langCookie != "" {
		req.AddCookie(&http.Cookie{Name: i18n.CookieName, Value: langCookie})
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLanguage_PageFollowsCookieThenHeaderThenDefault(t *testing.T) {
	e := newServer(t)
	cases := []struct {
		name, cookie, accept string
		wantLang, wantText   string
	}{
		{"default", "", "", "pt-BR", "Use o email e a senha da sua conta."},
		{"cookie en", "en", "", "en", "Use the email and password of your account."},
		{"header en", "", "en-US,en;q=0.9", "en", "Use the email and password of your account."},
		{"cookie beats header", "pt-BR", "en-US,en;q=0.9", "pt-BR", "Use o email e a senha da sua conta."},
		{"invalid cookie is ignored", "xx", "en", "en", "Use the email and password of your account."},
		{"unsupported header", "", "fr-FR,fr;q=0.9", "pt-BR", "Use o email e a senha da sua conta."},
	}
	for _, tc := range cases {
		rec := getPage(e, "/login", "", tc.cookie, tc.accept)
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", tc.name, rec.Code)
		}
		if !strings.Contains(body, `<html lang="`+tc.wantLang+`">`) {
			t.Errorf("%s: want lang=%q in %.120s", tc.name, tc.wantLang, body)
		}
		if !strings.Contains(body, tc.wantText) {
			t.Errorf("%s: page does not contain %q", tc.name, tc.wantText)
		}
		if vary := strings.Join(rec.Header().Values("Vary"), ","); !strings.Contains(vary, "Cookie") || !strings.Contains(vary, "Accept-Language") {
			t.Errorf("%s: Vary = %q", tc.name, vary)
		}
	}
}

// Em inglês a página não pode ter sobra de português nos textos que já foram migrados.
func TestLanguage_EnglishPagesHaveNoPortugueseLeftovers(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	pages := []struct{ path, session string }{
		{"/login", ""}, {"/signup", ""}, {"/invite/abc", ""},
		{"/profile", admin.session}, {"/no-such-page", admin.session}, // 404 só aparece para quem está logado
	}
	leftovers := []string{"Entrar", "Sair", "Senha", "Seu perfil", "Voltar ao início", "Fechar", "Página não encontrada", "Parar"}
	for _, p := range pages {
		body := getPage(e, p.path, p.session, "en", "").Body.String()
		if !strings.Contains(body, `<html lang="en">`) {
			t.Errorf("%s: not in English", p.path)
		}
		for _, word := range leftovers {
			if strings.Contains(body, ">"+word+"<") || strings.Contains(body, `"`+word+`"`) {
				t.Errorf("%s: still has %q", p.path, word)
			}
		}
	}
	if body := getPage(e, "/profile", admin.session, "en", "").Body.String(); !strings.Contains(body, "Sign out") {
		t.Error("topbar is not in English")
	}
	if body := getPage(e, "/no-such-page", admin.session, "en", "").Body.String(); !strings.Contains(body, "Page not found") {
		t.Error("404 page is not in English")
	}
}

func TestLanguage_TitleIsTranslated(t *testing.T) {
	e := newServer(t)
	if body := getPage(e, "/signup", "", "en", "").Body.String(); !strings.Contains(body, "<title>Create account · Working Time Tracker</title>") {
		t.Errorf("english title missing: %.300s", body)
	}
	if body := getPage(e, "/signup", "", "", "").Body.String(); !strings.Contains(body, "<title>Criar conta · Working Time Tracker</title>") {
		t.Errorf("portuguese title missing: %.300s", body)
	}
}

func TestLanguage_ToggleIsOnEveryPage(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	for _, path := range []string{"/login", "/profile"} {
		session := ""
		if path == "/profile" {
			session = admin.session
		}
		body := getPage(e, path, session, "", "").Body.String()
		for _, want := range []string{
			`class="lang-switch"`,
			`href="/lang/pt-BR?next=%2f`, // o caminho da página vai escapado em next
			`href="/lang/en?next=%2f`,
			`aria-current="true"`,
		} {
			if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
				t.Errorf("%s: toggle is missing %q", path, want)
			}
		}
	}
	// A opção marcada é a do idioma atual.
	body := getPage(e, "/login", "", "en", "").Body.String()
	if !strings.Contains(body, `lang="en" hreflang="en" title="English" aria-current="true"`) {
		t.Errorf("english is not the current option: %s", body)
	}
}

func TestLanguage_SwitchSetsCookieAndRedirects(t *testing.T) {
	e := newServer(t)
	cases := []struct {
		path       string
		wantCookie string // vazio: não grava
		wantTo     string
	}{
		{"/lang/en?next=/projects/abc/tasks", "en", "/projects/abc/tasks"},
		{"/lang/pt-BR?next=%2Fprofile", "pt-BR", "/profile"},
		{"/lang/EN", "en", "/"},
		{"/lang/en?next=//evil.example", "en", "/"},
		{"/lang/en?next=https://evil.example", "en", "/"},
		{"/lang/xx?next=/profile", "", "/profile"},
	}
	for _, tc := range cases {
		rec := getPage(e, tc.path, "", "", "")
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.wantTo {
			t.Errorf("%s = %d to %q, want 303 to %q", tc.path, rec.Code, rec.Header().Get("Location"), tc.wantTo)
		}
		var got *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == i18n.CookieName {
				got = c
			}
		}
		switch {
		case tc.wantCookie == "" && got != nil:
			t.Errorf("%s set the language cookie to %q", tc.path, got.Value)
		case tc.wantCookie != "" && (got == nil || got.Value != tc.wantCookie):
			t.Errorf("%s: cookie = %v, want %q", tc.path, got, tc.wantCookie)
		case got != nil && (got.MaxAge < 86400 || got.Path != "/" || got.SameSite != http.SameSiteLaxMode):
			t.Errorf("%s: cookie attributes: %+v", tc.path, got)
		}
	}
}

func TestLanguage_Script(t *testing.T) {
	e := newServer(t)

	rec := getPage(e, "/i18n/en.js", "", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("GET /i18n/en.js = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "window.I18N=") || !strings.Contains(body, `"lang":"en"`) || !strings.Contains(body, "Sign in") {
		t.Errorf("unexpected script: %.200s", body)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("without ?v= the script must revalidate: etag=%q cache=%q", etag, rec.Header().Get("Cache-Control"))
	}

	// Com o hash certo na URL o navegador pode guardar para sempre.
	hash := strings.Trim(etag, `"`)
	rec = getPage(e, "/i18n/en.js?v="+hash, "", "", "")
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("with the right ?v= the script should be immutable, got %q", cc)
	}
	rec = getPage(e, "/i18n/en.js?v=outro", "", "", "")
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("a wrong ?v= must not be cached forever, got %q", cc)
	}

	// A página aponta para o script do idioma com o hash certo.
	if page := getPage(e, "/login", "", "en", "").Body.String(); !strings.Contains(page, `/i18n/en.js?v=`+hash) {
		t.Errorf("page does not load /i18n/en.js?v=%s", hash)
	}

	// Revalidação.
	req := httptest.NewRequest(http.MethodGet, "/i18n/en.js", nil)
	req.Header.Set("If-None-Match", etag)
	cond := httptest.NewRecorder()
	e.ServeHTTP(cond, req)
	if cond.Code != http.StatusNotModified {
		t.Errorf("If-None-Match = %d, want 304", cond.Code)
	}

	for _, path := range []string{"/i18n/xx.js", "/i18n/en", "/i18n/en.json", "/i18n/.js"} {
		if rec := getPage(e, path, "", "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

// O idioma é o da requisição, não o da pessoa que já está logada: quem trocou no
// toggle vê a escolha mesmo com sessão aberta.
func TestLanguage_LoggedInPersonFollowsTheCookie(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	pt := getPage(e, "/profile", admin.session, "", "").Body.String()
	en := getPage(e, "/profile", admin.session, "en", "").Body.String()
	if !strings.Contains(pt, "Dados pessoais") || strings.Contains(pt, "Personal details") {
		t.Error("profile should default to Portuguese")
	}
	if !strings.Contains(en, "Personal details") || strings.Contains(en, "Dados pessoais") {
		t.Error("profile should follow the English cookie")
	}
}

var (
	scriptBlock  = regexp.MustCompile(`(?s)<script.*?</script>`)
	accentedWord = regexp.MustCompile(`[\p{L}]*[À-ÿ][\p{L}]*`)
)

// portugueseLeftovers devolve as palavras com acento que sobraram no HTML visível.
// O inglês não tem acento, então qualquer uma é texto que ficou sem tradução. Os
// scripts (os dados iniciais da página) e o nome "Português" do toggle ficam de fora.
func portugueseLeftovers(body string) []string {
	body = scriptBlock.ReplaceAllString(body, "")
	var words []string
	for _, w := range accentedWord.FindAllString(body, -1) {
		if w != "Português" {
			words = append(words, w)
		}
	}
	return words
}

// migratedPages são as páginas cujo texto já vem do catálogo. A lista cresce a cada
// etapa da migração; uma página nova entra aqui junto com a sua tradução.
func migratedPages(admin account, projectID, taskID string) []string {
	return []string{
		"/profile",
		"/projects/" + projectID + "/tasks",
		"/projects/" + projectID + "/time-tracking",
		"/tasks/" + taskID,
		"/orgs/" + admin.orgID,
		"/orgs/" + admin.orgID + "/about",
		"/orgs/" + admin.orgID + "/settings",
		"/orgs/" + admin.orgID + "/people",
		"/orgs/" + admin.orgID + "/customers",
		"/orgs/" + admin.orgID + "/projects",
	}
}

func TestLanguage_MigratedPagesHaveNoPortugueseInEnglish(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Alpha")
	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Core"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+admin.id+`"}`, admin.session)
	rec = do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Login screen","assignee_id":"`+admin.id+`"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)

	for _, who := range []account{admin, member} {
		for _, path := range migratedPages(admin, projectID, taskID) {
			rec := getPage(e, path, who.session, "en", "")
			if rec.Code == http.StatusNotFound {
				continue // abas de gestão são só de admins
			}
			if rec.Code != http.StatusOK {
				t.Errorf("GET %s = %d", path, rec.Code)
				continue
			}
			if words := portugueseLeftovers(rec.Body.String()); len(words) > 0 {
				t.Errorf("GET %s in English still has: %v", path, words)
			}
			// E o mesmo caminho em português não pode ter perdido o acento por engano.
			if pt := getPage(e, path, who.session, "", "").Body.String(); len(portugueseLeftovers(pt)) == 0 {
				t.Errorf("GET %s in Portuguese has no accented word: is it rendering keys?", path)
			}
		}
	}
}
