package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
)

// Session grava direto no banco uma sessão da pessoa na tarefa, do início ao fim (aberta,
// com end nulo), com os valores por hora dados. A sessão é do projeto da tarefa, e a tarefa
// cobre a sessão inteira, como depois de um clock-in sem mais nada. Serve aos testes que
// precisam de sessões com horários exatos, que o clock-in, com o relógio de agora, não dá.
func Session(t testing.TB, client *ent.Client, taskID, personID uuid.UUID, start time.Time, end *time.Time, pay, bill *int) *ent.WorkSession {
	t.Helper()
	ctx := context.Background()
	task, err := client.Task.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("session: task: %v", err)
	}
	session, err := client.WorkSession.Create().
		SetProjectID(task.ProjectID).SetPersonID(personID).
		SetStartAt(start).SetNillableEndAt(end).
		SetNillablePayRateCents(pay).SetNillableBillRateCents(bill).
		Save(ctx)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := client.WorkSessionTask.Create().
		SetSessionID(session.ID).SetTaskID(taskID).SetFromAt(start).SetNillableUntilAt(end).
		Exec(ctx); err != nil {
		t.Fatalf("session: task link: %v", err)
	}
	return session
}
