package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

// loginFrom tenta entrar com uma senha errada por um proxy (o endereço da conexão é sempre o dele), dizendo no
// X-Real-IP quem chamou, e devolve o status.
func loginFrom(e *echo.Echo, clientIP string) int {
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"ninguem@example.com","password":"errada123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:5000"
	if clientIP != "" {
		req.Header.Set("X-Real-IP", clientIP)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

// Atrás de um proxy, o limite de tentativas do login conta por quem chamou, no cabeçalho que o proxy escreve: quem
// errou dez vezes fica de fora, e outra pessoa atrás do mesmo proxy ainda tenta. Sem o cabeçalho configurado, o que
// vem nele não conta, e quem chama não escapa do limite trocando o IP que diz ter.
func TestAuthRateLimit_ClientIPHeader(t *testing.T) {
	testutil.Truncate(t, testDB)

	behindProxy, err := server.New(testClient, server.Options{EncryptKey: "test-key", ClientIPHeader: "X-Real-IP"})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	for i := range 10 {
		if code := loginFrom(behindProxy, "203.0.113.7"); code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d of the same person was refused before the limit", i+1)
		}
	}
	if code := loginFrom(behindProxy, "203.0.113.7"); code != http.StatusTooManyRequests {
		t.Errorf("11th attempt of the same person = %d, want 429", code)
	}
	if code := loginFrom(behindProxy, "198.51.100.9"); code == http.StatusTooManyRequests {
		t.Errorf("another person behind the same proxy was refused (429): the limit is shared")
	}

	direct, err := server.New(testClient, server.Options{EncryptKey: "test-key"})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	for i := range 10 {
		loginFrom(direct, fmt.Sprintf("203.0.113.%d", i+1))
	}
	if code := loginFrom(direct, "198.51.100.9"); code != http.StatusTooManyRequests {
		t.Errorf("without CLIENT_IP_HEADER a made-up X-Real-IP escaped the limit (%d, want 429)", code)
	}
}
