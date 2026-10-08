package task

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
)

// Excluir uma tarefa pode levar junto o item dela em cada plataforma (a issue do GitHub, o cartão do Trello).
// O domínio da tarefa não conhece as plataformas: quem sabe falar com elas se registra com SetRemover.

// remoteTimeout é quanto dura a parte da exclusão que fala com as plataformas. Ela corre depois que a tarefa já
// foi excluída, então não acompanha o pedido: a pessoa que fecha a aba não deixa o cartão pela metade.
const remoteTimeout = 30 * time.Second

// RemoteItem é o item de uma plataforma a que a tarefa excluída estava ligada.
type RemoteItem struct {
	IntegrationID uuid.UUID
	// Provider é o tipo da integração ("github", "trello"); ItemID, a chave do item na plataforma.
	Provider string
	ItemID   string
}

// RemoteResult é o que se fez com o item de uma plataforma quando a tarefa foi excluída. A exclusão da tarefa
// vale de qualquer jeito: o que falha aqui só vira aviso.
type RemoteResult struct {
	IntegrationID uuid.UUID `json:"integration_id"`
	Provider      string    `json:"provider"`
	// Outcome é o que aconteceu com o item: deleted, archived, closed ou gone. Vazio quando falhou.
	Outcome string `json:"outcome,omitempty"`
	// Problem é o motivo de o item não ter sido tratado, ou de ter sido tratado com menos do que se pediu (a
	// issue que só foi fechada).
	Problem *apperr.Error `json:"problem,omitempty"`
}

// Remover tira os itens das plataformas, um resultado para cada um.
type Remover interface {
	Remove(ctx context.Context, items []RemoteItem) []RemoteResult
}

// SetRemover registra quem trata os itens das plataformas ao excluir uma tarefa. Sem ele, só a tarefa sai.
func (s *Service) SetRemover(r Remover) { s.remover = r }

// Delete exclui a tarefa e, nas integrações pedidas em removeIn a que ela estava ligada, o item dela. As
// integrações a que a tarefa não está ligada são ignoradas. Os itens são lidos antes de a tarefa sair, porque
// depois o vínculo some dela; e só são tratados depois, para uma falha lá não deixar a tarefa viva (e o item
// fechado, que a próxima sincronização fecharia aqui também).
func (s *Service) Delete(ctx context.Context, id string, removeIn []uuid.UUID) ([]RemoteResult, error) {
	var items []RemoteItem
	if len(removeIn) > 0 && s.remover != nil {
		t, err := s.taskStore.GetByID(id)
		if err != nil {
			return nil, err
		}
		for _, l := range t.Links {
			if !slices.Contains(removeIn, l.IntegrationID) {
				continue
			}
			item := RemoteItem{IntegrationID: l.IntegrationID, ItemID: l.ItemID}
			if l.Integration != nil {
				item.Provider = l.Integration.Type
			}
			items = append(items, item)
		}
	}
	if err := s.taskStore.Delete(id); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []RemoteResult{}, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), remoteTimeout)
	defer cancel()
	return s.remover.Remove(ctx, items), nil
}
