// Package collaborator mostra quem está num projeto. A pessoa entra com o valor
// por hora dela (allocation), que libera o ponto, e só então pode entrar nos
// times, que a deixam ser responsável por tarefas.
//
// Antes dessa regra dava para pôr num time alguém sem valor. Quem ficou assim
// num banco antigo continua na lista, sem valor, até um admin definir o valor
// ou tirar a pessoa do projeto.
package collaborator

import "github.com/google/uuid"

type Person struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	// IsOwner marca o dono da organização: o valor dele no projeto é o cobrado, e não um
	// valor pago.
	IsOwner bool `json:"is_owner"`
}

type Team struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Collaborator struct {
	Person Person `json:"person"`
	// Teams são os times deste projeto em que a pessoa está, por nome. Fica
	// vazio para quem ainda não entrou em nenhum time.
	Teams []Team `json:"teams"`
	// Preset é o grupo de permissões da pessoa neste projeto (member, manager, finance,
	// admin ou custom). Quem entrou num time sem valor, de antes, fica em member.
	Preset string `json:"preset"`
	// PayRateCents é quanto a pessoa recebe por hora no projeto; nil quando quem
	// pergunta não pode ver, ou em quem entrou num time antes de o valor ser
	// obrigatório.
	PayRateCents *int `json:"pay_rate_cents"`
}
