package model

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time
}
