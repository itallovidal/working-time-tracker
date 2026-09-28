package page

import (
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
)

// Deps são os services que as páginas usam para montar o cabeçalho (nomes de
// projeto e tarefa). Os dados das listas vêm da API, não daqui.
type Deps struct {
	Projects *project.Service
	Tasks    *task.Service
}
