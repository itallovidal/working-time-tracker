package project

import (
	"time"

	"github.com/google/uuid"
)

// CustomerRef é o cliente de um projeto, só com o que todo membro pode ver.
type CustomerRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Billing é o lado comercial do projeto: quem contratou e quanto paga por hora.
// Fica fora de Project porque só admins leem e alteram.
type Billing struct {
	Customer      *CustomerRef `json:"customer"`
	BillRateCents *int         `json:"bill_rate_cents"`
}

type Project struct {
	ID                 uuid.UUID `json:"id"`
	OrganizationID     uuid.UUID `json:"organization_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	GithubRepoURL      *string   `json:"github_repo_url,omitempty"`
	GitlabRepoURL      *string   `json:"gitlab_repo_url,omitempty"`
	SprintDurationDays int       `json:"sprint_duration_days"`
	// WeeklyHours é a jornada semanal esperada no projeto; nil quando não foi informada.
	WeeklyHours   *int    `json:"weekly_hours"`
	DailyTime     *string `json:"daily_time,omitempty"`
	WeeklySyncDay *string `json:"weekly_sync_day,omitempty"`
	// Customer é nil em projetos internos.
	Customer  *CustomerRef `json:"customer"`
	CreatedAt time.Time    `json:"created_at"`
}
