package validate_test

import (
	"testing"

	"working-time-tracker/internal/validate"
)

func TestCNPJ(t *testing.T) {
	valid := map[string]string{
		"11.222.333/0001-81": "11222333000181",
		"11222333000181":     "11222333000181",
		// Exemplo alfanumérico da Receita Federal.
		"12.ABC.345/01DE-35": "12ABC34501DE35",
		"12.abc.345/01de-35": "12ABC34501DE35",
	}
	for in, want := range valid {
		got, ok := validate.CNPJ(in)
		if !ok || got != want {
			t.Errorf("CNPJ(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"",
		"11.222.333/0001-80", // dígito verificador errado
		"12.ABC.345/01DE-34",
		"00.000.000/0000-00",
		"1122233300018",      // 13 caracteres
		"112223330001811",    // 15 caracteres
		"11.222.333/0001-8A", // letra no dígito verificador
		"11_222_333_0001_81",
	} {
		if got, ok := validate.CNPJ(in); ok {
			t.Errorf("CNPJ(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestHTTPURL(t *testing.T) {
	valid := map[string]string{
		"https://acme.com.br":       "https://acme.com.br",
		"http://acme.com/sobre?a=1": "http://acme.com/sobre?a=1",
		"acme.com.br":               "https://acme.com.br",
		"  www.acme.com/contato ":   "https://www.acme.com/contato",
		"HTTPS://Acme.com":          "https://Acme.com",
	}
	for in, want := range valid {
		got, ok := validate.HTTPURL(in)
		if !ok || got != want {
			t.Errorf("HTTPURL(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"",
		"javascript:alert(1)",
		"javascript://acme.com/%0aalert(1)",
		"data:text/html,<script>alert(1)</script>",
		"ftp://acme.com",
		"https://",
		"https://localhost",
		"https://user:senha@acme.com",
		"acme .com",
		"https://acme.com/\tx",
	} {
		if got, ok := validate.HTTPURL(in); ok {
			t.Errorf("HTTPURL(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestEIN(t *testing.T) {
	for in, want := range map[string]string{
		"12-3456789":   "123456789",
		"123456789":    "123456789",
		" 12 3456789 ": "123456789",
		"98-7654321":   "987654321",
	} {
		if got, ok := validate.EIN(in); !ok || got != want {
			t.Errorf("EIN(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", "12-345678", "12-34567890", "00-1234567", // curto, longo, prefixo que não existe
		"AB-3456789", "12/3456789", "12-345678A",
	} {
		if got, ok := validate.EIN(in); ok {
			t.Errorf("EIN(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestCEP(t *testing.T) {
	for in, want := range map[string]string{
		"88010-000":  "88010-000",
		"88010000":   "88010-000",
		"88.010-000": "88010-000",
		" 01310 100": "01310-100",
	} {
		if got, ok := validate.CEP(in); !ok || got != want {
			t.Errorf("CEP(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "8801-000", "880100000", "88010-00A", "88010/000"} {
		if got, ok := validate.CEP(in); ok {
			t.Errorf("CEP(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestZIP(t *testing.T) {
	for in, want := range map[string]string{
		"10001":       "10001",
		"10001-1234":  "10001-1234",
		"100011234":   "10001-1234",
		" 10001 1234": "10001-1234",
		"10001-":      "10001",
	} {
		if got, ok := validate.ZIP(in); !ok || got != want {
			t.Errorf("ZIP(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "1000", "100011", "10001-123", "ABCDE", "10001-12AB"} {
		if got, ok := validate.ZIP(in); ok {
			t.Errorf("ZIP(%q) = %q, true; want invalid", in, got)
		}
	}
}

func TestGenericTaxID(t *testing.T) {
	for in, want := range map[string]string{
		"DE 123456789":     "DE 123456789",
		"  DE   123456789": "DE 123456789",
		"CHE-123.456.789":  "CHE-123.456.789",
		"B-12345678/9":     "B-12345678/9",
		"NIF_PT501964843":  "NIF_PT501964843",
	} {
		if got, ok := validate.GenericTaxID(in); !ok || got != want {
			t.Errorf("GenericTaxID(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", "   ", "12345678901234567890123456789012345", // vazio, só espaço, 35 caracteres
		"12<345", "12;345", "12'345",
	} {
		if got, ok := validate.GenericTaxID(in); ok {
			t.Errorf("GenericTaxID(%q) = %q, true; want invalid", in, got)
		}
	}
}
