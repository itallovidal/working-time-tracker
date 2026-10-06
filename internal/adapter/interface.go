package adapter

import (
	"fmt"
	"regexp"
	"strings"
)

type ItemDetails struct {
	Title string `json:"title"`
	State string `json:"state"`
	URL   string `json:"url"`
}

type ExternalDetailsResult struct {
	Details *ItemDetails `json:"details"`
	Error   *string      `json:"error,omitempty"`
}

// Connection é a estrutura comum a todas as integrações: a credencial e os campos
// próprios da plataforma. É o que todo adapter recebe.
type Connection struct {
	Token    string
	Metadata map[string]any
}

// Field descreve um campo do metadata de um tipo de integração.
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Hint        string `json:"hint,omitempty"`
	Required    bool   `json:"required"`
	// Summary marca o campo que identifica a conexão (o repositório, o quadro): é o
	// que o cartão da integração mostra.
	Summary bool `json:"summary"`
}

// Descriptor diz o que um tipo de integração é e o que ele pede. A tela desenha o
// formulário e os rótulos a partir dele.
type Descriptor struct {
	Type        string  `json:"type"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	TokenHint   string  `json:"token_hint,omitempty"`
	Metadata    []Field `json:"metadata"`
	// O item externo a que uma tarefa se vincula: uma issue, um cartão.
	ItemLabel       string `json:"item_label"`
	ItemPlaceholder string `json:"item_placeholder,omitempty"`
	ItemNumeric     bool   `json:"item_numeric"`
}

type Integration interface {
	Descriptor() Descriptor
	// CheckMetadata confere, sem falar com a plataforma, se o metadata tem tudo o que
	// este tipo precisa e devolve o que deve ser guardado: só os campos do tipo, já
	// normalizados.
	CheckMetadata(raw map[string]any) (map[string]any, error)
	// Validate consulta a plataforma para saber se a conexão funciona.
	Validate(conn Connection) error
	FetchItemDetails(conn Connection, itemID string) (*ItemDetails, error)
}

// fields lê do metadata os campos que o descritor declara, sem espaços nas pontas, e
// recusa quando falta um obrigatório. O que o tipo não declara fica de fora.
func (d Descriptor) fields(raw map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(d.Metadata))
	for _, f := range d.Metadata {
		var value string
		switch v := raw[f.Key].(type) {
		case nil:
		case string:
			value = strings.TrimSpace(v)
		default:
			return nil, fmt.Errorf("o campo \"%s\" do %s precisa ser um texto", f.Label, d.Label)
		}
		if value == "" {
			if f.Required {
				return nil, fmt.Errorf("informe o campo \"%s\" do %s", f.Label, d.Label)
			}
			continue
		}
		out[f.Key] = value
	}
	return out, nil
}

// token devolve a credencial da conexão ou o erro de quando ela não veio.
func (d Descriptor) token(conn Connection) (string, error) {
	token := strings.TrimSpace(conn.Token)
	if token == "" {
		return "", fmt.Errorf("informe o token do %s", d.Label)
	}
	return token, nil
}

// trimURL tira o esquema, o host e o que sobra nas pontas de um endereço colado no
// lugar de um identificador: "https://github.com/dono/repo.git/" vira "dono/repo".
func trimURL(value string, hosts ...string) string {
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "www.")
	for _, host := range hosts {
		value = strings.TrimPrefix(value, host+"/")
	}
	value = strings.TrimRight(value, "/")
	return strings.TrimSuffix(value, ".git")
}

var issueNumberPattern = regexp.MustCompile(`^[0-9]+$`)

// issueNumber confere o número de uma issue antes de ele entrar na URL da API.
// Aceita "42" e "#42".
func issueNumber(itemID string) (string, error) {
	id := strings.TrimPrefix(strings.TrimSpace(itemID), "#")
	if !issueNumberPattern.MatchString(id) {
		return "", fmt.Errorf("o número da issue precisa ter só dígitos")
	}
	return id, nil
}
