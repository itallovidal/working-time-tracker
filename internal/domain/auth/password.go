package auth

import (
	"errors"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 8
	// bcrypt ignora tudo depois de 72 bytes, então senhas maiores são recusadas.
	maxPasswordBytes = 72
)

var (
	ErrWeakPassword = errors.New("a senha precisa ter pelo menos 8 caracteres")
	ErrLongPassword = errors.New("a senha pode ter no máximo 72 caracteres")
)

func HashPassword(password string) (string, error) {
	if len([]rune(password)) < MinPasswordLength {
		return "", ErrWeakPassword
	}
	if len(password) > maxPasswordBytes {
		return "", ErrLongPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// burnPasswordCheck gasta o mesmo tempo de uma verificação real. É usado quando
// o email não existe, para que o tempo de resposta não revele quais contas existem.
func burnPasswordCheck(password string) {
	dummyHashOnce.Do(func() {
		h, _ := bcrypt.GenerateFromPassword([]byte("senha-que-nunca-confere"), bcrypt.DefaultCost)
		dummyHash = string(h)
	})
	CheckPassword(dummyHash, password)
}
