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

// markedSHA256 é o sha256 de static/marked.min.js: lib/marked.umd.js do pacote npm
// marked@18.1.0, sem a última linha (o comentário sourceMappingURL, cujo .map não é servido).
const markedSHA256 = "7e0ef62ccce57ed740e7180947ed8fb8b344cf56c275f70ef403e6232c91322f"

// purifySHA256 é o sha256 de static/purify.min.js: dist/purify.min.js do pacote npm
// dompurify@3.4.16, sem a última linha (sourceMappingURL). O DOMPurify é a única defesa
// contra XSS na descrição da tarefa, que vira HTML no navegador: não edite o arquivo.
const purifySHA256 = "c2b2880796e279f9b76bed6e67aada76e421f1540491f9ef70589d7107b3e76c"

func TestVendoredLibsArePristine(t *testing.T) {
	for file, want := range map[string]string{
		"static/alpine.min.js": alpineSHA256,
		"static/marked.min.js": markedSHA256,
		"static/purify.min.js": purifySHA256,
	} {
		data, err := FS.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s sha256 = %s, want %s (arquivo alterado ou corrompido)", file, got, want)
		}
	}
}
