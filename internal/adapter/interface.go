package adapter

import (
	"regexp"
	"strings"
	"working-time-tracker/internal/apperr"
)

type ItemDetails struct {
	Title string `json:"title"`
	State string `json:"state"`
	URL   string `json:"url"`
}

// ExternalDetailsResult é a resposta de external-details. Quando a plataforma falha, a
// resposta continua sendo 200, com details nulo e o motivo em Error, no mesmo formato
// ({code, params}) dos outros erros da API.
type ExternalDetailsResult struct {
	Details *ItemDetails  `json:"details"`
	Error   *apperr.Error `json:"error,omitempty"`
}

// Connection é a estrutura comum a todas as integrações: a credencial e os campos
// próprios da plataforma. É o que todo adapter recebe.
type Connection struct {
	Token    string
	Metadata map[string]any
}

// Field descreve um campo do metadata de um tipo de integração. Label, Placeholder e
// Hint ficam vazios aqui: o texto de cada idioma está no catálogo, em
// integration_types.<tipo>.fields.<campo>, e quem monta a resposta da página o
// preenche (internal/page).
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
// formulário e os rótulos a partir dele. Só o nome da plataforma (Label) vem do
// adapter; Description, TokenHint, ItemLabel, ItemPlaceholder e o texto dos campos
// são do catálogo, em integration_types.<tipo>.
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
	// ComingSoon marca o tipo que existe no adapter mas ainda não se oferece: a tela
	// mostra "em breve" e a API recusa criar uma integração nova dele. As que já
	// existem continuam funcionando e podem ser editadas.
	ComingSoon bool `json:"coming_soon"`
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
			return nil, ErrFieldNotText.With("field", f.Key, "provider", d.Label)
		}
		if value == "" {
			if f.Required {
				return nil, ErrFieldRequired.With("field", f.Key, "provider", d.Label)
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
		return "", ErrTokenRequired.With("provider", d.Label)
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
		return "", ErrInvalidIssueNumber
	}
	return id, nil
}
