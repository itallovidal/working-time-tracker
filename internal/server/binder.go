package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
)

// maxBodyBytes é o maior corpo JSON que a API lê: 1 MiB, muito acima do maior campo (a descrição de uma tarefa, 64 KiB).
const maxBodyBytes = 1 << 20

// rejectUnknownFields liga o erro para chave desconhecida. Fica ligado depois que toda tela manda só as chaves que a API
// conhece (o teste de chaves em web/ confere isso).
const rejectUnknownFields = true

// strictBinder lê o corpo JSON de forma estrita: campo desconhecido é erro (uma chave digitada errada não é mais
// ignorada em silêncio), o corpo tem teto, e um valor do tipo errado devolve "request.invalid_body" com o campo. Um
// corpo vazio deixa o destino como está, como o binder padrão.
type strictBinder struct{}

func (strictBinder) Bind(c *echo.Context, target any) error {
	req := c.Request()
	if req.ContentLength == 0 && req.Body != nil && req.TransferEncoding == nil {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(c.Response(), req.Body, maxBodyBytes))
	if rejectUnknownFields {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return apperr.ErrBodyTooLarge
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return apperr.ErrInvalidBody.With("field", typeErr.Field)
		}
		return apperr.ErrInvalidBody
	}
	return nil
}
