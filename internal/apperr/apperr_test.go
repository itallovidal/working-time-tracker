package apperr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
)

var errSample = apperr.New("sample.too_long", http.StatusBadRequest, "field", "max")

func TestNew_RejectsBadCodes(t *testing.T) {
	for _, code := range []string{"", "nodot", "Upper.case", "a.b.c", "a.", ".b", "a b.c", "a.b-c", "1a.b"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%q) did not panic", code)
				}
			}()
			apperr.New(code, 400)
		}()
	}
	defer func() {
		if recover() == nil {
			t.Error("a repeated code did not panic")
		}
	}()
	apperr.New("sample.too_long", 400)
}

func TestError_IsAndWith(t *testing.T) {
	withParams := errSample.With("field", "summary", "max", 160)
	if withParams == errSample {
		t.Fatal("With must return a copy")
	}
	if errSample.Params != nil {
		t.Errorf("With changed the sentinel: %v", errSample.Params)
	}
	if !errors.Is(withParams, errSample) {
		t.Error("a copy with params is not the sentinel for errors.Is")
	}
	wrapped := fmt.Errorf("saving: %w", withParams)
	if !errors.Is(wrapped, errSample) || apperr.Code(wrapped) != "sample.too_long" {
		t.Error("a wrapped coded error lost its code")
	}
	if errors.Is(errSample, apperr.ErrInvalidBody) || apperr.Code(errors.New("plain")) != "" {
		t.Error("different codes must differ, and a plain error has no code")
	}
	if withParams.Error() != "sample.too_long" {
		t.Errorf("Error() = %q, want the code", withParams.Error())
	}

	cause := errors.New("connection refused")
	got := errSample.With("field", "x").Wrap(cause)
	if !errors.Is(got, cause) || !errors.Is(got, errSample) || got.Params["field"] != "x" {
		t.Error("Wrap/With must keep the cause, the code and the params")
	}
}

func respond(t *testing.T, status int, err error) (int, map[string]any, string) {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := apperr.Respond(c, status, err); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %q", rec.Body.String())
	}
	return rec.Code, body, rec.Body.String()
}

func TestRespond(t *testing.T) {
	code, body, raw := respond(t, 400, errSample.With("field", "summary", "max", 160))
	detail := body["error"].(map[string]any)
	if code != 400 || detail["code"] != "sample.too_long" || detail["params"].(map[string]any)["max"] != float64(160) {
		t.Errorf("coded error = %d %s", code, raw)
	}
	if strings.Contains(raw, "message") {
		t.Errorf("the response has a message: %s", raw)
	}

	// Sem parâmetros, o campo não aparece.
	_, _, raw = respond(t, 404, apperr.ErrNotFound)
	if strings.Contains(raw, "params") {
		t.Errorf("params should be omitted when empty: %s", raw)
	}

	// Um registro que não existe vira 404, mesmo que o handler peça outro status.
	if code, _, raw := respond(t, 400, database.ErrNotFound); code != 404 || !strings.Contains(raw, `"request.not_found"`) {
		t.Errorf("database.ErrNotFound = %d %s", code, raw)
	}

	// Um erro sem código é inesperado: 500, e nenhum texto interno sai.
	code, _, raw = respond(t, 400, errors.New("pq: connection to 10.0.0.5 refused"))
	if code != 500 || !strings.Contains(raw, `"internal.server_error"`) || strings.Contains(raw, "10.0.0.5") {
		t.Errorf("unexpected error = %d %s", code, raw)
	}
}
