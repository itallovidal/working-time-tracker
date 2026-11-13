package issuesync

import (
	"errors"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/personaccount"
)

type Handler struct {
	syncer *Syncer
}

func NewHandler(syncer *Syncer) *Handler { return &Handler{syncer: syncer} }

// Sync é o botão Sincronizar agora: faz uma rodada completa e responde o que ela fez. Com outra rodada
// em andamento na mesma integração, responde 409 em vez de esperar.
func (h *Handler) Sync(c *echo.Context) error {
	sum, err := h.syncer.SyncNow(c.Request().Context(), parseID(c.Param("integrationId")))
	switch {
	case err == nil:
		return c.JSON(200, sum)
	case errors.Is(err, ErrSyncRunning):
		return apperr.Respond(c, 409, err)
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, 404, err)
	}
	return apperr.Respond(c, 400, err)
}

// SyncProject é o botão Sincronizar das listas de tarefas: uma rodada completa em cada integração do
// projeto com a sincronização ligada. É de qualquer pessoa do projeto, como mudar uma tarefa, que já
// leva a mudança ao GitHub. Responde 409 se todas as integrações já estão numa rodada.
func (h *Handler) SyncProject(c *echo.Context) error {
	sum, err := h.syncer.SyncProject(c.Request().Context(), parseID(c.Param("projectId")))
	switch {
	case err == nil:
		return c.JSON(200, sum)
	case errors.Is(err, ErrSyncRunning):
		return apperr.Respond(c, 409, err)
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, 404, err)
	}
	return apperr.Respond(c, 400, err)
}

// SyncTask é o botão Sincronizar da tela da tarefa: sincroniza o item dela (o da integração do query
// integration_id, ou todos) e responde o que mudou. Com uma rodada em andamento na integração, responde 409
// em vez de esperar.
func (h *Handler) SyncTask(c *echo.Context) error {
	var integrationID *uuid.UUID
	if raw := c.QueryParam("integration_id"); raw != "" {
		id := parseID(raw)
		integrationID = &id
	}
	sum, err := h.syncer.SyncTaskNow(c.Request().Context(), parseID(c.Param("taskId")), integrationID)
	switch {
	case err == nil:
		return c.JSON(200, sum)
	case errors.Is(err, ErrSyncRunning):
		return apperr.Respond(c, 409, err)
	case errors.Is(err, database.ErrNotFound), errors.Is(err, adapter.ErrIssueGone):
		return apperr.Respond(c, 404, err)
	}
	return apperr.Respond(c, 400, err)
}

// linkBody é o corpo de POST /api/integrations/:integrationId/accounts.
type linkBody struct {
	Login    string `json:"login"`
	PersonID string `json:"person_id"`
}

// LinkLogin é o admin ligando o login de um responsável das issues a uma pessoa do projeto, no cartão da integração.
// Responde a integração como ficou, com a lista dos logins sem pessoa já sem este.
func (h *Handler) LinkLogin(c *echo.Context) error {
	var body linkBody
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	personID, err := uuid.Parse(body.PersonID)
	if err != nil {
		return apperr.Respond(c, 400, ErrLinkNotInProject)
	}
	it, err := h.syncer.LinkLogin(c.Request().Context(), parseID(c.Param("integrationId")), body.Login, personID)
	switch {
	case err == nil:
		return c.JSON(200, it)
	case errors.Is(err, personaccount.ErrLoginTaken), errors.Is(err, personaccount.ErrPersonLinked):
		return apperr.Respond(c, 409, err)
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, 404, err)
	}
	return apperr.Respond(c, 400, err)
}

// publishBody é o corpo de POST /api/tasks/:taskId/publish.
type publishBody struct {
	IntegrationID string `json:"integration_id"`
}

// Publish posta uma tarefa como issue nova: é o passo Integrações do modal Nova tarefa. Qualquer pessoa
// que veja a tarefa pode, como no botão Sincronizar dela.
func (h *Handler) Publish(c *echo.Context) error {
	var body publishBody
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, 400, apperr.BindError(err))
	}
	published, err := h.syncer.Publish(c.Request().Context(), parseID(c.Param("taskId")), parseID(body.IntegrationID))
	switch {
	case err == nil:
		return c.JSON(200, published)
	case errors.Is(err, ErrAlreadyLinked):
		return apperr.Respond(c, 409, err)
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, 404, err)
	}
	return apperr.Respond(c, 400, err)
}

// parseID lê o id da rota; um id que não é um UUID não é de nenhuma integração.
func parseID(raw string) uuid.UUID {
	id, _ := uuid.Parse(raw)
	return id
}
