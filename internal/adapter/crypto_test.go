package adapter

import (
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := "test-key-32-bytes!!!!!"
	original := map[string]interface{}{
		"token": "ghp_test123",
		"repo":  "owner/repo",
	}

	encrypted, err := EncryptConfig(original, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if encrypted["encrypted_data"] == nil {
		t.Fatal("encrypted_data missing")
	}

	decrypted, err := DecryptConfig(encrypted, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if decrypted["token"] != original["token"] {
		t.Errorf("token mismatch: got %v, want %v", decrypted["token"], original["token"])
	}
	if decrypted["repo"] != original["repo"] {
		t.Errorf("repo mismatch: got %v, want %v", decrypted["repo"], original["repo"])
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1 := "test-key-32-bytes!!!!!"
	key2 := "wrong-key-32-bytes!!!!!!"
	original := map[string]interface{}{
		"token": "ghp_test123",
	}

	encrypted, err := EncryptConfig(original, key1)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	_, err = DecryptConfig(encrypted, key2)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
}

func TestDecryptInvalidFormat(t *testing.T) {
	invalid := map[string]interface{}{
		"encrypted_data": "not-base64!!!",
	}
	_, err := DecryptConfig(invalid, "any-key-32-bytes!!!!!!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestDecryptMissingKey(t *testing.T) {
	invalid := map[string]interface{}{
		"something": "value",
	}
	_, err := DecryptConfig(invalid, "any-key-32-bytes!!!!!!!!")
	if err == nil {
		t.Fatal("expected error for missing encrypted_data, got nil")
	}
}
