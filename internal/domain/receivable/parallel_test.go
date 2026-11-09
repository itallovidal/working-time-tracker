package receivable

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/work_session"
)

// Duas sessões da mesma pessoa ao mesmo tempo, em dois projetos do mesmo cliente (Decisão 26): o projeto A das 12h às
// 15h a 20,00/h e o B das 12h às 16h a 30,00/h. O cliente paga cada sessão pelo valor do projeto dela (3 × 20 + 4 ×
// 30 = 180,00), e as horas do cliente são 4, não 7. Outra pessoa na mesma hora soma as horas dela.
func TestSumWithin_ParallelProjectsCountTheHoursOnce(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 11, 9, h, 0, 0, 0, time.UTC) }
	end := func(h int) *time.Time { v := at(h); return &v }
	rate := func(c int) *int { return &c }
	ana, bia := uuid.New(), uuid.New()
	projectA, projectB := uuid.New(), uuid.New()
	sessions := []work_session.WorkSession{
		{PersonID: ana, ProjectID: projectA, StartAt: at(12), EndAt: end(15), BillRateCents: rate(2000)},
		{PersonID: ana, ProjectID: projectB, StartAt: at(12), EndAt: end(16), BillRateCents: rate(3000)},
	}
	seconds, cents := sumWithin(sessions, at(0), at(24), at(20))
	if seconds != 4*3600 || cents != 3*2000+4*3000 {
		t.Errorf("one person in two projects: %v seconds and %d cents, want %v and %d", seconds, cents, 4*3600, 3*2000+4*3000)
	}

	sessions = append(sessions, work_session.WorkSession{PersonID: bia, ProjectID: projectA, StartAt: at(12), EndAt: end(13), BillRateCents: rate(2000)})
	seconds, cents = sumWithin(sessions, at(0), at(24), at(20))
	if seconds != 5*3600 || cents != 3*2000+4*3000+2000 {
		t.Errorf("with another person: %v seconds and %d cents, want %v and %d", seconds, cents, 5*3600, 3*2000+4*3000+2000)
	}
}
