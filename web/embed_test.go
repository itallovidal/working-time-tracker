package web

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// alpineSHA256 é o sha256 de dist/cdn.min.js do pacote npm alpinejs@3.14.8.
// A cópia vendorizada já esteve corrompida sem ninguém notar: nenhuma tela com
// Alpine funcionava. Ao atualizar o Alpine, troque o arquivo e este hash juntos.
const alpineSHA256 = "b600e363d99d95444db54acbfb2deffec9ae792aa99a09229bcda078e5b55643"

func TestVendoredAlpineIsPristine(t *testing.T) {
	data, err := FS.ReadFile("static/alpine.min.js")
	if err != nil {
		t.Fatalf("read alpine: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != alpineSHA256 {
		t.Fatalf("static/alpine.min.js sha256 = %s, want %s (arquivo alterado ou corrompido)", got, alpineSHA256)
	}
}
