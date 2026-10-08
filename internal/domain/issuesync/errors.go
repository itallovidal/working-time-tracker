package issuesync

import (
	"net/http"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/task"
)

// Erros da sincronização. Os dois primeiros voltam da API; os outros são códigos que a rodada deixa
// no vínculo da issue e na integração (last_sync_error), para o cartão da integração mostrar o motivo
// no idioma da pessoa.
var (
	// ErrSyncRunning é a rodada que já está em andamento nesta integração.
	ErrSyncRunning = apperr.New("integration.sync_running", http.StatusConflict)
	// ErrSyncOff é a integração que não está com a sincronização ligada (ou está desativada).
	ErrSyncOff = apperr.New("integration.sync_off", http.StatusBadRequest)

	// ErrAlreadyLinked é a tarefa que já tem um item nesta integração: não há o que postar nela.
	ErrAlreadyLinked = task.ErrAlreadyLinked
	// ErrPublishReadOnly: o token da integração não escreve no repositório, e a issue sairia sem as
	// etiquetas e o responsável (o GitHub os descarta sem avisar), então a tarefa não é postada.
	ErrPublishReadOnly = apperr.New("issue_sync.publish_read_only", http.StatusBadRequest)

	// ErrRemoveReadOnly: o token da integração não escreve no repositório ou quadro, então o item de uma tarefa
	// excluída continua lá.
	ErrRemoveReadOnly = apperr.New("issue_sync.remove_read_only", http.StatusBadRequest)
	// ErrRemoveOnlyClosed: a plataforma não deixou apagar o item (no GitHub, só o admin do repositório apaga uma
	// issue), então ele só foi fechado. Não é uma falha: vai no resultado da exclusão como aviso.
	ErrRemoveOnlyClosed = apperr.New("issue_sync.remove_only_closed", http.StatusBadRequest)

	// ErrReadOnly: o token não escreve no repositório (ou ele está arquivado), então as issues só
	// vêm para cá e as mudanças daqui ficam daqui.
	ErrReadOnly = apperr.New("issue_sync.read_only", http.StatusBadRequest)
	// ErrPushDiscarded: o GitHub respondeu 200 mas não gravou o que se mandou, como faz com quem não
	// tem permissão para mudar etiquetas e responsáveis.
	ErrPushDiscarded = apperr.New("issue_sync.push_discarded", http.StatusBadRequest)
	// ErrPushRejected: o GitHub recusou a mudança (por exemplo, um título grande demais).
	ErrPushRejected = apperr.New("issue_sync.push_rejected", http.StatusBadRequest)
	// ErrNoLogin: o responsável escolhido aqui não tem usuário do GitHub que se ache pelo e-mail.
	ErrNoLogin = apperr.New("issue_sync.no_login", http.StatusBadRequest)
	// ErrLabelRefused: o token não pode criar etiquetas no repositório.
	ErrLabelRefused = apperr.New("issue_sync.label_refused", http.StatusBadRequest)
)
