// Package personaccount guarda a conta de cada pessoa no GitHub e no GitLab: o login dela lá, ou a resposta de que
// ela não tem uma. A sincronização das issues liga o responsável da issue à pessoa por aqui (antes do e-mail
// público), e a trava da tarefa ("só quem tem conta no GitHub") confere aqui quem pode ser o responsável.
package personaccount

import (
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
)

// As plataformas em que uma pessoa tem conta. São as que ligam o responsável da issue (Caps.Assignee).
const (
	GitHub = "github"
	GitLab = "gitlab"
)

// Platforms são as plataformas com conta, na ordem da tela.
var Platforms = []string{GitHub, GitLab}

// ValidPlatform diz se p é uma plataforma com conta.
func ValidPlatform(p string) bool { return slices.Contains(Platforms, p) }

// De onde veio o vínculo. A conta autorizada pela própria pessoa (SourceOAuth) vence as outras; a de quem conectou
// uma integração (SourceIntegration) também foi autorizada, mas pode ser de um robô; o admin (SourceAdmin) e o e-mail
// público (SourceEmail) não provam nada. SourceSelf é a pessoa dizendo que não tem conta.
const (
	SourceOAuth       = "oauth"
	SourceIntegration = "integration"
	SourceAdmin       = "admin"
	SourceEmail       = "email"
	SourceSelf        = "self"
)

// verified diz se a conta foi autorizada na plataforma: um admin não a troca por outra.
func verified(source string) bool { return source == SourceOAuth || source == SourceIntegration }

// Account é a conta de uma pessoa numa plataforma. Sem Username e com NoAccount, a pessoa disse que não tem uma.
type Account struct {
	PersonID       uuid.UUID `json:"person_id"`
	OrganizationID uuid.UUID `json:"-"`
	Platform       string    `json:"platform"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	NoAccount      bool      `json:"no_account"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
}

// Linked diz se a conta tem um login: é o que a trava da tarefa e a sincronização usam.
func (a *Account) Linked() bool { return a != nil && a.Username != "" }

// Erros do domínio. O texto de cada código está em internal/i18n/locales, na chave errors.<código>.
var (
	ErrInvalidPlatform = apperr.New("account.invalid_platform", http.StatusNotFound)
	ErrNotFound        = apperr.New("account.not_found", http.StatusNotFound)
	// O login já é de outra pessoa da organização, que o autorizou na plataforma.
	ErrLoginTaken = apperr.New("account.login_taken", http.StatusConflict, "login")
	// A pessoa já tem outra conta autorizada nesta plataforma: um admin não a troca.
	ErrPersonLinked = apperr.New("account.person_linked", http.StatusConflict, "login")
	ErrInvalidLogin = apperr.New("account.invalid_login", http.StatusBadRequest)
)
