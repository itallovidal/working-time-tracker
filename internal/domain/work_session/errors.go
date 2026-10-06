package work_session

import (
	"net/http"

	"working-time-tracker/internal/apperr"
)

// Erros do domínio. O código é o que a API devolve; o texto em cada idioma está
// em internal/i18n/locales, na chave errors.<código>.
var (
	ErrNoRate               = apperr.New("work_session.no_rate", http.StatusBadRequest)
	ErrTaskNotFound         = apperr.New("work_session.task_not_found", http.StatusBadRequest)
	ErrTaskOtherProject     = apperr.New("work_session.task_other_project", http.StatusBadRequest)
	ErrPersonNotFound       = apperr.New("work_session.person_not_found", http.StatusBadRequest)
	ErrPersonNotInOrg       = apperr.New("work_session.person_not_in_org", http.StatusBadRequest)
	ErrAlreadyOpen          = apperr.New("work_session.already_open", http.StatusBadRequest)
	ErrNotOpen              = apperr.New("work_session.not_open", http.StatusBadRequest)
	ErrFilterRequired       = apperr.New("work_session.filter_required", http.StatusBadRequest)
	ErrInvalidTaskFilter    = apperr.New("work_session.invalid_task_filter", http.StatusBadRequest)
	ErrInvalidPersonFilter  = apperr.New("work_session.invalid_person_filter", http.StatusBadRequest)
	ErrTaskRequired         = apperr.New("work_session.task_required", http.StatusBadRequest)
	ErrPersonRequired       = apperr.New("work_session.person_required", http.StatusBadRequest)
	ErrOtherPersonAdminOnly = apperr.New("work_session.other_person_admin_only", http.StatusForbidden)
)
