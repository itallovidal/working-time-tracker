// Package collaborator junta os dois vínculos de uma pessoa com um projeto: o
// valor por hora (allocation), que libera o ponto, e os times, que deixam a
// pessoa ser responsável por tarefas. Colaborador do projeto é quem tem pelo
// menos um dos dois.
package collaborator

import "github.com/google/uuid"

type Person struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type Team struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Collaborator struct {
	Person Person `json:"person"`
	// Teams são os times deste projeto em que a pessoa está, por nome. Fica
	// vazio para quem tem valor e ainda não entrou em nenhum time.
	Teams []Team `json:"teams"`
	// PayRateCents é quanto a pessoa recebe por hora no projeto; nil quando ela
	// ainda não tem valor, ou quando quem pergunta não pode ver.
	PayRateCents *int `json:"pay_rate_cents"`
}
