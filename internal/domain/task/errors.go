package task

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNotFound                = apperr.New("task.not_found", http.StatusNotFound)
	ErrNameRequired            = apperr.New("task.name_required", http.StatusBadRequest)
	ErrAssigneeRequired        = apperr.New("task.assignee_required", http.StatusBadRequest)
	ErrAssigneeNotInTeam       = apperr.New("task.assignee_not_in_team", http.StatusBadRequest)
	ErrInvalidAssignee         = apperr.New("task.invalid_assignee", http.StatusBadRequest)
	ErrIntegrationNotFound     = apperr.New("task.integration_not_found", http.StatusBadRequest)
	ErrIntegrationOtherProject = apperr.New("task.integration_other_project", http.StatusBadRequest)
	ErrNoExternalItem          = apperr.New("task.no_external_item", http.StatusBadRequest)
	ErrIntegrationsUnavailable = apperr.New("task.integrations_unavailable", http.StatusBadRequest)
	ErrLinkFieldsRequired      = apperr.New("task.link_fields_required", http.StatusBadRequest)
	ErrQueryTooLong            = apperr.New("task.query_too_long", http.StatusBadRequest, "max")
	ErrInvalidAssigneeFilter   = apperr.New("task.invalid_assignee_filter", http.StatusBadRequest)
	ErrInvalidDeadlineFilter   = apperr.New("task.invalid_deadline_filter", http.StatusBadRequest)
	ErrInvalidPage             = apperr.New("task.invalid_page", http.StatusBadRequest)
	ErrInvalidPerPage          = apperr.New("task.invalid_per_page", http.StatusBadRequest)
)
