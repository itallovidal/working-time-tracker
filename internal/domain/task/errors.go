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
	ErrDescriptionTooLong      = apperr.New("task.description_too_long", http.StatusBadRequest, "max")
	ErrAssigneeNotInTeam       = apperr.New("task.assignee_not_in_team", http.StatusBadRequest)
	ErrInvalidAssignee         = apperr.New("task.invalid_assignee", http.StatusBadRequest)
	ErrAlreadyAssigned         = apperr.New("task.already_assigned", http.StatusConflict)
	ErrIntegrationNotFound     = apperr.New("task.integration_not_found", http.StatusBadRequest)
	ErrIntegrationOtherProject = apperr.New("task.integration_other_project", http.StatusBadRequest)
	ErrNoExternalItem          = apperr.New("task.no_external_item", http.StatusBadRequest)
	ErrIntegrationsUnavailable = apperr.New("task.integrations_unavailable", http.StatusBadRequest)
	ErrLinkFieldsRequired      = apperr.New("task.link_fields_required", http.StatusBadRequest, "field")
	// ErrAlreadyLinked é a tarefa que já tem um item nesta integração: cada integração tem um só.
	ErrAlreadyLinked = apperr.New("task.already_linked", http.StatusConflict)
	// ErrItemTaken é o item que já está ligado a outra tarefa.
	ErrItemTaken             = apperr.New("task.item_taken", http.StatusConflict)
	ErrQueryTooLong          = apperr.New("task.query_too_long", http.StatusBadRequest, "max", "field")
	ErrInvalidAssigneeFilter = apperr.New("task.invalid_assignee_filter", http.StatusBadRequest)
	ErrInvalidDeadlineFilter = apperr.New("task.invalid_deadline_filter", http.StatusBadRequest)
	ErrInvalidPriority       = apperr.New("task.invalid_priority", http.StatusBadRequest)
	ErrInvalidPriorityFilter = apperr.New("task.invalid_priority_filter", http.StatusBadRequest)
	ErrInvalidStatus         = apperr.New("task.invalid_status", http.StatusBadRequest)
	ErrInvalidStatusFilter   = apperr.New("task.invalid_status_filter", http.StatusBadRequest)
	ErrInvalidLabelFilter    = apperr.New("task.invalid_label_filter", http.StatusBadRequest)
	ErrLabelOtherProject     = apperr.New("task.label_other_project", http.StatusBadRequest)
	ErrLabelNameRequired     = apperr.New("label.name_required", http.StatusBadRequest)
	ErrLabelNameTooLong      = apperr.New("label.name_too_long", http.StatusBadRequest, "max")
	ErrLabelNameTaken        = apperr.New("label.name_taken", http.StatusConflict)
	ErrLabelNotFound         = apperr.New("label.not_found", http.StatusNotFound)
	ErrInvalidPage           = apperr.New("task.invalid_page", http.StatusBadRequest)
	ErrInvalidPerPage        = apperr.New("task.invalid_per_page", http.StatusBadRequest)
)

// ErrDeadlineOutOfRange é o prazo antes de 1971: a tarefa guarda o "sem prazo" como uma data antiga, então uma
// data assim seria lida como sem prazo, em silêncio. Para tirar o prazo se manda null.
var ErrDeadlineOutOfRange = apperr.ErrFieldInvalid.With("field", "deadline")
