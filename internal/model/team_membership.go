package model

import (
	"time"

	"github.com/google/uuid"
)

type TeamMembership struct {
	PersonID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_person_team"`
	Person    Person    `gorm:"foreignKey:PersonID;constraint:OnDelete:CASCADE"`
	TeamID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_person_team"`
	Team      Team      `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	CreatedAt time.Time
}
