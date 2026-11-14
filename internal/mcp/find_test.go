package mcp

import "testing"

// A find_tasks aceita a URL como a pessoa a copia do navegador (com ou sem https://, com o nome do cartão ou um
// comentário no fim) e a forma curta; e lê do mesmo jeito a URL que o vínculo guarda.
func TestParseItemRef(t *testing.T) {
	cases := []struct {
		in   string
		want itemRef
		ok   bool
	}{
		{"https://github.com/Acme/Site/issues/12", itemRef{"github.com", "acme/site", "12"}, true},
		{"github.com/acme/site/issues/12#issuecomment-99", itemRef{"github.com", "acme/site", "12"}, true},
		{"https://www.github.com/acme/site/issues/12/", itemRef{"github.com", "acme/site", "12"}, true},
		{"https://gitlab.com/grupo/sub/app/-/issues/7", itemRef{"gitlab.com", "grupo/sub/app", "7"}, true},
		{"https://gitlab.empresa.com/grupo/app/-/work_items/7?tab=x", itemRef{"gitlab.empresa.com", "grupo/app", "7"}, true},
		{"https://trello.com/c/AbCd1234", itemRef{"trello.com", "", "AbCd1234"}, true},
		{"https://trello.com/c/AbCd1234/12-corrigir-login", itemRef{"trello.com", "", "AbCd1234"}, true},
		{"Acme/Site#12", itemRef{"", "acme/site", "12"}, true},
		{"grupo/sub/app#7", itemRef{"", "grupo/sub/app", "7"}, true},
		{"#12", itemRef{}, false},
		{"site#12", itemRef{}, false},
		{"acme/site#doze", itemRef{}, false},
		{"https://github.com/acme/site/pull/12", itemRef{}, false},
		{"https://github.com/acme/site", itemRef{}, false},
		{"https://trello.com/b/xyz/quadro", itemRef{}, false},
		{"corrigir login", itemRef{}, false},
		{"", itemRef{}, false},
	}
	for _, tc := range cases {
		got, ok := parseItemRef(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseItemRef(%q) = %+v, %v; want %+v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
