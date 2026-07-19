package work_session

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/task"
)

type WorkSession struct {
	ID        uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID    uuid.UUID     `gorm:"type:uuid;not null;index" json:"task_id"`
	Task      task.Task     `gorm:"foreignKey:TaskID;constraint:OnDelete:CASCADE" json:"task,omitempty"`
	PersonID  uuid.UUID     `gorm:"type:uuid;not null;index" json:"person_id"`
	Person    person.Person `gorm:"foreignKey:PersonID;constraint:OnDelete:CASCADE" json:"person,omitempty"`
	StartAt   time.Time     `gorm:"not null" json:"start_at"`
	EndAt     *time.Time    `json:"end_at,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}
