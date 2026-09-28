package page

import "testing"

// O ?next= do login só pode apontar para um caminho deste site. O navegador
// remove TAB e quebra de linha antes de ler a URL, então "/\t/host" viraria
// "//host", um endereço de outro site.
func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"/projects/x/tasks?only=mine": "/projects/x/tasks?only=mine",
		"/%09/x":                      "/%09/x",
		"":                            "/",
		"//evil.example":              "/",
		"/\\evil.example":             "/",
		"/\t/evil.example":            "/",
		"/\n/evil.example":            "/",
		"/\r/evil.example":            "/",
		"/x\\/evil.example":           "/",
		"https://evil.example":        "/",
		"javascript:alert(1)":         "/",
	}
	for next, want := range cases {
		if got := safeNext(next); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", next, got, want)
		}
	}
}
