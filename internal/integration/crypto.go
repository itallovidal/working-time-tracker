package integration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

func deriveKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}

func EncryptConfig(plaintext map[string]interface{}, key string) (map[string]interface{}, error) {
	data, err := json.Marshal(plaintext)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return map[string]interface{}{
		"encrypted_data": base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func DecryptConfig(encrypted map[string]interface{}, key string) (map[string]interface{}, error) {
	encData, ok := encrypted["encrypted_data"].(string)
	if !ok {
		return nil, errors.New("invalid encrypted config format")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encData)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return nil, err
	}
	return config, nil
}
