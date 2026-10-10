package validate

import (
	"bytes"
	"encoding/json"
)

// Optional distingue os três estados de um campo de PATCH: ausente (Set falso: manter o que está), null (Set
// verdadeiro e Value nulo: apagar) e com valor. Um *T comum não separa ausente de null.
type Optional[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON marca o campo como enviado, mesmo quando o valor é null.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// MarshalJSON escreve o valor, ou null quando não há.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}
