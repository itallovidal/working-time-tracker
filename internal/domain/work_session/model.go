package work_session

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type WorkSession struct {
	ID        uuid.UUID  `json:"id"`
	TaskID    uuid.UUID  `json:"task_id"`
	Task      *Task      `json:"task,omitempty"`
	PersonID  uuid.UUID  `json:"person_id"`
	Person    *Person    `json:"person,omitempty"`
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
