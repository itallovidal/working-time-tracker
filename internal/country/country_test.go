package country

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"working-time-tracker/internal/i18n"
)

var twoLetters = regexp.MustCompile(`^[A-Z]{2}$`)

// Todo país do cadastro precisa de todos os textos, nos dois idiomas: a tela monta rótulos e dicas a partir deles, e
// uma chave que falta apareceria como o nome da chave.
func TestRegistry_HasTextsInEveryLanguage(t *testing.T) {
	cat, err := i18n.Load()
	if err != nil {
		t.Fatalf("load the catalogs: %v", err)
	}
	for _, lang := range i18n.Supported() {
		keys := []string{genericKey}
		for _, c := range All() {
			keys = append(keys, textKeys(c)...)
		}
		for _, key := range keys {
			if !cat.Has(lang, key) {
				t.Errorf("%s has no text for %q", lang, key)
			}
		}
	}
}

func TestRegistry_EntriesAreConsistent(t *testing.T) {
	codes, fields := map[string]bool{}, map[string]bool{}
	for _, c := range All() {
		if !twoLetters.MatchString(c.Code) || codes[c.Code] {
			t.Errorf("code %q is not two upper-case letters, or repeats", c.Code)
		}
		codes[c.Code] = true
		if c.LegalID.Field == "" || fields[c.LegalID.Field] || c.LegalID.Mask == "" || c.LegalID.Normalize == nil {
			t.Errorf("%s: the legal id needs its own field, a mask and a validator: %+v", c.Code, c.LegalID)
		}
		fields[c.LegalID.Field] = true
		if len(c.Currency) != 3 || c.Currency != strings.ToUpper(c.Currency) {
			t.Errorf("%s: currency %q is not a three-letter code", c.Code, c.Currency)
		}
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			t.Errorf("%s: timezone %q: %v", c.Code, c.Timezone, err)
		}
		if len(c.Langs) == 0 || c.Address.Postal.Mask == "" || c.Address.Postal.Normalize == nil {
			t.Errorf("%s: needs its languages and a postal code rule", c.Code)
		}
		seen := map[string]bool{}
		for _, s := range c.Address.States {
			if !twoLetters.MatchString(s.Code) || s.Name == "" || seen[s.Code] {
				t.Errorf("%s: state %+v is malformed or repeats", c.Code, s)
			}
			seen[s.Code] = true
		}
		for _, a := range c.Aliases {
			if a != strings.ToLower(a) {
				t.Errorf("%s: alias %q must be lower case", c.Code, a)
			}
		}
		if !isoSet[c.Code] {
			t.Errorf("%s is not an ISO 3166-1 code", c.Code)
		}
	}
	if got := len(MustGet("BR").Address.States); got != 27 {
		t.Errorf("BR has %d states, want the 26 states and the Federal District", got)
	}
	if got := len(MustGet("US").Address.States); got != 59 {
		t.Errorf("US has %d states and territories, want 50 + DC + 5 territories + 3 military", got)
	}
}

func TestISO(t *testing.T) {
	codes := ISO()
	if len(codes) != 249 {
		t.Errorf("ISO has %d codes, want the 249 of ISO 3166-1", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if !twoLetters.MatchString(c) || seen[c] {
			t.Errorf("code %q is malformed or repeats", c)
		}
		seen[c] = true
	}
	if !seen["BR"] || !seen["US"] {
		t.Error("ISO lacks BR or US")
	}
	// A cópia não deixa quem chama mexer na lista.
	codes[0] = "XX"
	if ISO()[0] == "XX" {
		t.Error("ISO returns the internal slice")
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{
		"BR": "BR", "br": "BR", " us ": "US", "US": "US",
		"Brasil": "BR", "BRAZIL": "BR", "brasil": "BR",
		"EUA": "US", "USA": "US", "U.S.A.": "US", "United States": "US", "Estados Unidos da América": "US",
	} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "  ", "Portugal", "DE", "Brasilia", "BRA"} {
		if got, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestForLang(t *testing.T) {
	for in, want := range map[string]string{
		"pt-BR": "BR", "pt": "BR", "en": "US", "en-US": "US", "EN": "US", "fr": "BR", "": "BR",
	} {
		if got := ForLang(in); got != want {
			t.Errorf("ForLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCustomerCountry(t *testing.T) {
	for in, want := range map[string]string{"br": "BR", " us ": "US", "de": "DE", "PT": "PT", "JP": "JP"} {
		if got, ok := CustomerCountry(in); !ok || got != want {
			t.Errorf("CustomerCountry(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "ZZ", "BRA", "B", "Brasil", "XK"} {
		if got, ok := CustomerCountry(in); ok {
			t.Errorf("CustomerCountry(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestNormalizeTaxID(t *testing.T) {
	cases := []struct {
		code, in, want string
		ok             bool
	}{
		{"BR", "11.222.333/0001-81", "11222333000181", true},
		{"BR", "11.222.333/0001-80", "", false}, // dígito verificador errado
		{"BR", "12-3456789", "", false},         // um EIN não é um CNPJ
		{"US", "12-3456789", "123456789", true},
		{"US", "00-3456789", "", false},
		{"US", "11.222.333/0001-81", "", false}, // um CNPJ não é um EIN
		{"DE", "DE 123456789", "DE 123456789", true},
		{"DE", "  DE   123456789  ", "DE 123456789", true},
		{"DE", "DE<123>", "", false},
		{"JP", "", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeTaxID(c.code, c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("NormalizeTaxID(%q, %q) = %q, %v; want %q, %v", c.code, c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeStateAndPostal(t *testing.T) {
	br, us := MustGet("BR"), MustGet("US")
	for _, c := range []struct {
		country  *Country
		in, want string
		ok       bool
	}{
		{br, "sp", "SP", true}, {br, "São Paulo", "SP", true}, {br, "são paulo", "SP", true},
		{br, "  RJ ", "RJ", true}, {br, "", "", true}, {br, "XX", "", false}, {br, "Sao Paulo", "", false},
		{us, "tx", "TX", true}, {us, "Texas", "TX", true}, {us, "DC", "DC", true}, {us, "SP", "", false},
	} {
		if got, ok := c.country.NormalizeState(c.in); ok != c.ok || got != c.want {
			t.Errorf("%s.NormalizeState(%q) = %q, %v; want %q, %v", c.country.Code, c.in, got, ok, c.want, c.ok)
		}
	}
	for _, c := range []struct {
		country  *Country
		in, want string
		ok       bool
	}{
		{br, "88010000", "88010-000", true}, {br, "88010-000", "88010-000", true}, {br, "", "", true}, {br, "10001", "", false},
		{us, "10001", "10001", true}, {us, "10001-1234", "10001-1234", true}, {us, "88010-000", "", false},
	} {
		if got, ok := c.country.NormalizePostal(c.in); ok != c.ok || got != c.want {
			t.Errorf("%s.NormalizePostal(%q) = %q, %v; want %q, %v", c.country.Code, c.in, got, ok, c.want, c.ok)
		}
	}
	// Sem lista, o estado é texto livre, com teto.
	free := &Country{}
	if got, ok := free.NormalizeState("  Bavaria "); !ok || got != "Bavaria" {
		t.Errorf("free state = %q, %v; want Bavaria, true", got, ok)
	}
	if _, ok := free.NormalizeState(strings.Repeat("x", 101)); ok {
		t.Error("a free state over 100 characters must be invalid")
	}
}

func TestDescribe_IsWhatTheBrowserReceives(t *testing.T) {
	r := Describe(func(key string, _ ...any) string { return "<" + key + ">" })
	if len(r.List) != len(All()) || r.List[0].Code != "BR" || r.List[1].Code != "US" {
		t.Fatalf("list = %+v, want BR then US", r.List)
	}
	br := r.List[0]
	if br.LegalID.Field != "cnpj" || br.LegalID.Mask != "**.***.***/****-99" || br.LegalID.Label != "<countries.br.legal_id.label>" {
		t.Errorf("BR legal id = %+v", br.LegalID)
	}
	if len(br.State.Options) != 27 || br.Postal.Mask != "99999-999" || br.Currency != "BRL" {
		t.Errorf("BR address/defaults = %+v", br)
	}
	if r.Generic.LegalID.Label != "<countries.generic.legal_id.label>" || len(r.ISO) != 249 {
		t.Errorf("generic/iso = %+v / %d", r.Generic, len(r.ISO))
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"legal_id"`, `"line2_label"`, `"options"`, `"iso"`, `"generic"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the JSON lacks %s", key)
		}
	}
}
