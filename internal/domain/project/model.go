package project

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/organization"
)

type Project struct {
	ID                 uuid.UUID                 `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrganizationID     uuid.UUID                 `gorm:"type:uuid;not null;index" json:"organization_id"`
	Organization       organization.Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"organization,omitempty"`
	Name               string                    `gorm:"not null" json:"name"`
	Description        string                    `json:"description"`
	GithubRepoURL      *string                   `json:"github_repo_url,omitempty"`
	GitlabRepoURL      *string                   `json:"gitlab_repo_url,omitempty"`
	SprintDurationDays int                       `gorm:"default:14" json:"sprint_duration_days"`
	DailyTime          *string                   `json:"daily_time,omitempty"`
	WeeklySyncDay      *string                   `json:"weekly_sync_day,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
}
