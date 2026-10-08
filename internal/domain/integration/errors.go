package integration

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound             = apperr.New("integration.not_found", http.StatusNotFound)
	ErrNameRequired         = apperr.New("integration.name_required", http.StatusBadRequest)
	ErrTypeRequired         = apperr.New("integration.type_required", http.StatusBadRequest)
	ErrTypeComingSoon       = apperr.New("integration.type_coming_soon", http.StatusBadRequest, "provider")
	ErrDisabled             = apperr.New("integration.disabled", http.StatusBadRequest)
	ErrNoCredential         = apperr.New("integration.no_credential", http.StatusBadRequest)
	ErrUnreadableCredential = apperr.New("integration.unreadable_credential", http.StatusBadRequest)
	// ErrNotConnectable e ErrNoRepositories são do tipo que não tem a capacidade pedida:
	// conectar por OAuth, listar repositórios.
	ErrNotConnectable = apperr.New("integration.not_connectable", http.StatusBadRequest)
	ErrNoRepositories = apperr.New("integration.no_repositories", http.StatusBadRequest)

	// A sincronização das issues: só o tipo que lê e escreve issues a tem, pede o repositório e a
	// integração ativa para ligar, e com ela ligada o repositório não troca (as issues ligadas
	// seriam de outro lugar).
	ErrSyncUnsupported  = apperr.New("integration.sync_unsupported", http.StatusBadRequest, "provider")
	ErrSyncNeedsRepo    = apperr.New("integration.sync_needs_repo", http.StatusBadRequest)
	ErrSyncNeedsEnabled = apperr.New("integration.sync_needs_enabled", http.StatusBadRequest)
	ErrSyncRepoLocked   = apperr.New("integration.sync_repo_locked", http.StatusBadRequest)
)
