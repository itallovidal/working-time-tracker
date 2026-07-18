package model

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID                 uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID     uuid.UUID    `gorm:"type:uuid;not null;index"`
	Organization       Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE"`
	Name               string       `gorm:"not null"`
	Description        string
	GithubRepoURL      *string
	GitlabRepoURL      *string
	SprintDurationDays int     `gorm:"default:14"`
	DailyTime          *string
	WeeklySyncDay      *string
	CreatedAt          time.Time
}
