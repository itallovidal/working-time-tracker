package auth

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrInvalidCredentials  = apperr.New("auth.invalid_credentials", http.StatusUnauthorized)
	ErrUnauthenticated     = apperr.New("auth.unauthenticated", http.StatusUnauthorized)
	ErrInviteInvalid       = apperr.New("auth.invite_invalid", http.StatusNotFound)
	ErrInviteEmailMismatch = apperr.New("auth.invite_email_mismatch", http.StatusBadRequest)
	ErrWrongPassword       = apperr.New("auth.wrong_password", http.StatusBadRequest)
	ErrAccountExists       = apperr.New("auth.account_exists", http.StatusConflict)
	ErrNameRequired        = apperr.New("auth.name_required", http.StatusBadRequest)
	ErrOrgNameRequired     = apperr.New("auth.org_name_required", http.StatusBadRequest)
	ErrWeakPassword        = apperr.New("auth.weak_password", http.StatusBadRequest)
	ErrLongPassword        = apperr.New("auth.long_password", http.StatusBadRequest)
	ErrAdminOnly           = apperr.New("auth.admin_only", http.StatusForbidden)
	ErrOwnerOnly           = apperr.New("auth.owner_only", http.StatusForbidden)
	ErrPermissionRequired  = apperr.New("auth.permission_required", http.StatusForbidden)
	ErrOwnProfileOnly      = apperr.New("auth.own_profile_only", http.StatusForbidden)

	// O login pelo Clerk.
	ErrClerkDisabled        = apperr.New("auth.clerk_disabled", http.StatusNotFound)
	ErrClerkTokenInvalid    = apperr.New("auth.clerk_token_invalid", http.StatusUnauthorized)
	ErrClerkEmailUnverified = apperr.New("auth.clerk_email_unverified", http.StatusForbidden)
	ErrClerkAccountLinked   = apperr.New("auth.clerk_account_linked", http.StatusConflict)
	ErrClerkUnavailable     = apperr.New("auth.clerk_unavailable", http.StatusBadGateway)
	ErrClerkInviteFailed    = apperr.New("auth.clerk_invite_failed", http.StatusBadGateway)
)
