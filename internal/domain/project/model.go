package project

import (
	"time"

	"github.com/google/uuid"
)

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
	CreatedAt          time.Time `json:"created_at"`
}
