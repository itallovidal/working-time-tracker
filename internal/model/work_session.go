package model

import (
	"time"

	"github.com/google/uuid"
)

type WorkSession struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TaskID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	Task      Task       `gorm:"foreignKey:TaskID;constraint:OnDelete:CASCADE"`
	PersonID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	Person    Person     `gorm:"foreignKey:PersonID;constraint:OnDelete:CASCADE"`
	StartAt   time.Time  `gorm:"not null"`
	EndAt     *time.Time
	CreatedAt time.Time
}
