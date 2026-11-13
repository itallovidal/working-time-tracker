package personaccount

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

// Store devolve o store, para quem só lê (a sincronização, a trava da tarefa).
func (s *Service) Store() *Store { return s.store }

// login é o formato de um login do GitHub e do GitLab: letras, números, ponto, hífen e sublinhado.
var login = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

// ValidLogin diz se l tem o formato de um login.
func ValidLogin(l string) bool { return login.MatchString(l) }

// Link grava a conta a com a precedência de Store.Link. O login é conferido aqui, a plataforma também.
func (s *Service) Link(a Account) (*Account, error) {
	if !ValidPlatform(a.Platform) {
		return nil, ErrInvalidPlatform
	}
	a.Username = strings.TrimSpace(a.Username)
	a.Email = strings.TrimSpace(a.Email)
	if a.Source != SourceSelf && !ValidLogin(a.Username) {
		return nil, ErrInvalidLogin
	}
	return s.store.Link(a)
}

// List devolve as contas de quem está logado.
func (s *Service) List(actor *auth.Identity) ([]Account, error) {
	return s.store.ForPerson(actor.PersonID)
}

// SetNoAccount grava que quem está logado não tem conta na plataforma. Quem já tem um login ligado fica com ele.
func (s *Service) SetNoAccount(actor *auth.Identity, platform string) (*Account, error) {
	return s.Link(Account{PersonID: actor.PersonID, OrganizationID: actor.OrganizationID, Platform: platform,
		Source: SourceSelf})
}

// Disconnect apaga a conta de quem está logado na plataforma.
func (s *Service) Disconnect(actor *auth.Identity, platform string) error {
	if !ValidPlatform(platform) {
		return ErrInvalidPlatform
	}
	if err := s.store.Delete(actor.PersonID, platform); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// Missing devolve as plataformas pedidas em que a pessoa ainda não tem um login ligado (vazio: tem todas).
func (s *Service) Missing(personID uuid.UUID, platforms []string) ([]string, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	linked, err := s.store.LinkedPlatforms([]uuid.UUID{personID})
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, p := range platforms {
		if !slices.Contains(linked[personID], p) {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

// --- handler ---

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func fail(c *echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidPlatform), errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrLoginTaken), errors.Is(err, ErrPersonLinked):
		status = http.StatusConflict
	case errors.Is(err, ErrInvalidLogin):
		status = http.StatusBadRequest
	}
	return apperr.Respond(c, status, err)
}

// List devolve as contas de quem está logado: GET /api/auth/accounts.
func (h *Handler) List(c *echo.Context) error {
	accounts, err := h.svc.List(auth.CurrentPerson(c))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, accounts)
}

// NoAccount grava que quem está logado não tem conta: POST /api/auth/accounts/:platform/no-account.
func (h *Handler) NoAccount(c *echo.Context) error {
	a, err := h.svc.SetNoAccount(auth.CurrentPerson(c), c.Param("platform"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, a)
}

// Disconnect apaga a conta de quem está logado: DELETE /api/auth/accounts/:platform.
func (h *Handler) Disconnect(c *echo.Context) error {
	if err := h.svc.Disconnect(auth.CurrentPerson(c), c.Param("platform")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
