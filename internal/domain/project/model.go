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

// Routine são os horários que o projeto combina: a daily e a weekly do time e a
// reunião semanal com o cliente. Em cada campo, nil mantém o que o projeto já tem
// (na criação, deixa sem) e texto vazio apaga. O horário de uma weekly ou de uma
// reunião só existe junto com o dia.
type Routine struct {
	DailyTime           *string `json:"daily_time"`
	WeeklySyncDay       *string `json:"weekly_sync_day"`
	WeeklySyncTime      *string `json:"weekly_sync_time"`
	CustomerMeetingDay  *string `json:"customer_meeting_day"`
	CustomerMeetingTime *string `json:"customer_meeting_time"`
}

type Project struct {
	ID                 uuid.UUID `json:"id"`
	OrganizationID     uuid.UUID `json:"organization_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	GithubRepoURL      *string   `json:"github_repo_url,omitempty"`
	GitlabRepoURL      *string   `json:"gitlab_repo_url,omitempty"`
	SprintDurationDays int       `json:"sprint_duration_days"`
	DailyTime          *string   `json:"daily_time,omitempty"`
	WeeklySyncDay      *string   `json:"weekly_sync_day,omitempty"`
	WeeklySyncTime     *string   `json:"weekly_sync_time,omitempty"`
	// CustomerMeetingDay e CustomerMeetingTime são a reunião semanal com o cliente.
	CustomerMeetingDay  *string `json:"customer_meeting_day,omitempty"`
	CustomerMeetingTime *string `json:"customer_meeting_time,omitempty"`
	// Customer é nil em projetos internos.
	Customer *CustomerRef `json:"customer"`
	// MemberCount conta os colaboradores do projeto, sem repetir: quem está em
	// algum time dele ou tem valor por hora nele. TaskCount, as tarefas do projeto.
	MemberCount int       `json:"member_count"`
	TaskCount   int       `json:"task_count"`
	CreatedAt   time.Time `json:"created_at"`
}
