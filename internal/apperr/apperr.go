// Package apperr define os erros que a API devolve a quem a chama. Cada erro tem
// um código estável (como "auth.invalid_credentials") e, quando a mensagem precisa
// de valores, parâmetros. A API não manda texto: quem mostra a mensagem é o
// cliente, no idioma da pessoa, procurando "errors.<código>" no catálogo. Os
// códigos estão documentados em _docs/error-codes.md.
package apperr

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"sync"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/database"
)

// codeRe é o formato dos códigos: "domínio.motivo", em minúsculas e com "_".
var codeRe = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*\.[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Info descreve um código declarado: o status HTTP usual e os parâmetros que
// acompanham o erro.
type Info struct {
	Status int
	Params []string
}

var (
	mu       sync.Mutex
	registry = map[string]Info{}
)

// Error é um erro de negócio com código. O valor devolvido por New é um sentinela:
// compara-se com errors.Is, e With cria uma cópia com parâmetros, que continua
// igual ao sentinela para o errors.Is.
type Error struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`

	cause error
}

// New declara um código e devolve o sentinela dele. status é o status HTTP com
// que a API costuma responder; é usado só para documentar e testar, quem escolhe
// o status de uma resposta é o handler. params são os nomes dos parâmetros que o
// erro leva (ver With); os textos só podem usar esses placeholders. Um código
// repetido ou fora do formato derruba o programa na inicialização: é erro de quem
// escreveu o código.
func New(code string, status int, params ...string) *Error {
	if !codeRe.MatchString(code) {
		panic("apperr: código fora do formato domínio.motivo: " + code)
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[code]; dup {
		panic("apperr: código repetido: " + code)
	}
	registry[code] = Info{Status: status, Params: params}
	return &Error{Code: code}
}

// Error devolve o código, que é o que aparece nos logs.
func (e *Error) Error() string { return e.Code }

// Is compara pelo código, para um erro com parâmetros continuar igual ao sentinela.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// With devolve uma cópia do erro com os parâmetros dados, como pares nome/valor:
// ErrTooLong.With("field", "summary", "max", 160).
func (e *Error) With(pairs ...any) *Error {
	params := make(map[string]any, len(e.Params)+len(pairs)/2)
	for k, v := range e.Params {
		params[k] = v
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		if name, ok := pairs[i].(string); ok {
			params[name] = pairs[i+1]
		}
	}
	return &Error{Code: e.Code, Params: params, cause: e.cause}
}

// Wrap guarda a causa técnica (a falha de rede, por exemplo) para quem investiga
// com errors.Unwrap. A causa não vai para a resposta.
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

// Unwrap devolve a causa guardada por Wrap.
func (e *Error) Unwrap() error { return e.cause }

// Code é o código do erro, ou "" quando err não é um *Error.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Registered devolve uma cópia dos códigos declarados.
func Registered() map[string]Info {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]Info, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// Codes lista só os códigos, em ordem alfabética.
func Codes() []string {
	reg := Registered()
	out := make([]string, 0, len(reg))
	for code := range reg {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Códigos que não pertencem a um domínio só.
var (
	ErrInvalidBody  = New("request.invalid_body", http.StatusBadRequest)
	ErrJSONRequired = New("request.json_required", http.StatusUnsupportedMediaType)
	ErrNotFound     = New("request.not_found", http.StatusNotFound)
	ErrBodyTooLarge = New("request.body_too_large", http.StatusRequestEntityTooLarge)
	ErrInternal     = New("internal.server_error", http.StatusInternalServerError)
	ErrTooMany      = New("request.too_many_attempts", http.StatusTooManyRequests)

	// Os três erros de campo que servem a qualquer formulário. O parâmetro field é o nome do campo na API (o
	// rótulo dele está em fields.* no catálogo), para a tela mostrar o erro embaixo do campo certo.
	ErrFieldRequired = New("request.field_required", http.StatusBadRequest, "field")
	ErrFieldTooLong  = New("request.field_too_long", http.StatusBadRequest, "field", "max")
	ErrFieldInvalid  = New("request.field_invalid", http.StatusBadRequest, "field")
)

// body é o corpo de toda resposta de erro: {"error": {"code": "...", "params": {...}}}.
type body struct {
	Error detail `json:"error"`
}

type detail struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// Respond escreve a resposta de erro com o status dado. Um registro que não existe
// (database.ErrNotFound) vira "request.not_found" com 404. Qualquer outro erro sem
// código é inesperado: o detalhe vai para o log, e quem chamou recebe só
// "internal.server_error" com status 500, sem texto interno.
func Respond(c *echo.Context, status int, err error) error {
	var e *Error
	if errors.As(err, &e) {
		return c.JSON(status, body{Error: detail{Code: e.Code, Params: e.Params}})
	}
	if errors.Is(err, database.ErrNotFound) {
		return c.JSON(http.StatusNotFound, body{Error: detail{Code: ErrNotFound.Code}})
	}
	c.Logger().Error("unexpected error", "error", err)
	return c.JSON(http.StatusInternalServerError, body{Error: detail{Code: ErrInternal.Code}})
}

// BindError é o erro que um handler devolve quando c.Bind falha. O Binder do servidor já entrega um *Error (corpo
// grande demais, ou um tipo errado num campo, com o nome do campo); qualquer outra falha vira "request.invalid_body".
func BindError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return ErrInvalidBody
}
