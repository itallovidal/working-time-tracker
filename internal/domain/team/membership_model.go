package team

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/person"
)

type TeamMembership struct {
	PersonID  uuid.UUID     `gorm:"type:uuid;not null;uniqueIndex:idx_person_team" json:"person_id"`
	Person    person.Person `gorm:"foreignKey:PersonID;constraint:OnDelete:CASCADE" json:"person,omitempty"`
	TeamID    uuid.UUID     `gorm:"type:uuid;not null;uniqueIndex:idx_person_team" json:"team_id"`
	Team      Team          `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE" json:"team,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}
