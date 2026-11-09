package payment

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/work_session"
)

// A pessoa trabalha em dois projetos ao mesmo tempo (Decisão 27): o A das 12h às 15h a 20,00/h e o B das 12h às 16h a
// 30,00/h. Ela recebe cada sessão pelo valor do projeto dela (3 × 20 + 4 × 30 = 180,00); as horas do período são 4,
// não 7, e cada projeto fica com o tempo inteiro dele (3h e 4h).
func TestWithin_ParallelProjectsCountTheHoursOnce(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 11, 9, h, 0, 0, 0, time.UTC) }
	end := func(h int) *time.Time { v := at(h); return &v }
	rate := func(c int) *int { return &c }
	ana := uuid.New()
	projectA, projectB := uuid.New(), uuid.New()
	sessions := []work_session.WorkSession{
		{PersonID: ana, ProjectID: projectA, StartAt: at(12), EndAt: end(15), PayRateCents: rate(2000)},
		{PersonID: ana, ProjectID: projectB, StartAt: at(12), EndAt: end(16), PayRateCents: rate(3000)},
	}
	perProject := map[uuid.UUID]*sum{}
	total := within(sessions, at(0), at(24), at(20), false, perProject)
	if total.seconds != 4*3600 || total.cents != 3*2000+4*3000 {
		t.Errorf("period: %v seconds and %d cents, want %v and %d", total.seconds, total.cents, 4*3600, 3*2000+4*3000)
	}
	if perProject[projectA].seconds != 3*3600 || perProject[projectB].seconds != 4*3600 {
		t.Errorf("per project: A %v and B %v seconds, want 3h and 4h", perProject[projectA].seconds, perProject[projectB].seconds)
	}
}
