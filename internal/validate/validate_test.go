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
