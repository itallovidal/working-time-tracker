package integration

import (
	"reflect"
	"testing"

	"working-time-tracker/internal/adapter"
)

func TestKeepInternal(t *testing.T) {
	d := adapter.Descriptor{Metadata: []adapter.Field{
		{Key: "api_key", Required: true, Internal: true},
		{Key: "board_id", Required: true, Summary: true},
	}}
	stored := map[string]interface{}{"api_key": "appkey", "board_id": "old"}

	// O campo interno vem do que estava guardado, e quem edita não o troca; o resto passa como veio.
	in := map[string]interface{}{"api_key": "other", "board_id": "new"}
	got := keepInternal(d, in, stored)
	if want := map[string]interface{}{"api_key": "appkey", "board_id": "new"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keepInternal = %v, want %v", got, want)
	}
	if in["api_key"] != "other" {
		t.Errorf("the incoming map was changed: %v", in)
	}

	// Sem nada guardado para o campo interno, vale o que veio (a integração criada pela API com a chave).
	if got := keepInternal(d, in, map[string]interface{}{"board_id": "old"}); got["api_key"] != "other" {
		t.Errorf("with nothing stored the incoming key must stand, got %v", got)
	}

	// Um tipo sem campo interno devolve o metadata como chegou.
	plain := adapter.Descriptor{Metadata: []adapter.Field{{Key: "repo", Summary: true}}}
	if got := keepInternal(plain, map[string]interface{}{"repo": "a/b"}, map[string]interface{}{"repo": "c/d"}); got["repo"] != "a/b" {
		t.Errorf("a type with no internal field must pass the metadata through, got %v", got)
	}
}

func TestMergeMetadata(t *testing.T) {
	base := map[string]interface{}{"a": "1", "b": "2"}
	got := mergeMetadata(base, map[string]interface{}{"b": "app", "c": "3"})
	if want := map[string]interface{}{"a": "1", "b": "app", "c": "3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mergeMetadata = %v, want %v", got, want)
	}
	if base["b"] != "2" {
		t.Errorf("the base map was changed: %v", base)
	}
	if got := mergeMetadata(nil, nil); got == nil || len(got) != 0 {
		t.Errorf("mergeMetadata(nil, nil) = %v, want an empty, non-nil map", got)
	}
}
