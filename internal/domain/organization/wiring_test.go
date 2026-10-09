package organization

import (
	"testing"

	"working-time-tracker/internal/country"
)

// Todo documento fiscal do cadastro de países precisa estar ligado a uma coluna da organização e a um erro: sem isso o
// campo seria aceito no PATCH e ignorado. Quem põe um país com documento novo é lembrado aqui.
func TestLegalIDRefs_CoverEveryCountry(t *testing.T) {
	refs := legalIDRefs(&Organization{}, UpdateInput{})
	for _, c := range country.All() {
		ref, ok := refs[c.LegalID.Field]
		if !ok || ref.dst == nil || ref.err == nil {
			t.Errorf("%s: the legal id %q is not wired in legalIDRefs", c.Code, c.LegalID.Field)
		}
	}
}
