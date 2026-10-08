package adapter

import (
	"context"
	"regexp"
	"strings"
	"time"

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
	// Sync diz que o tipo sabe sincronizar as issues do repositório com as tarefas (IssueSyncer): a
	// tela só oferece a caixa para quem a tem.
	Sync bool `json:"sync"`
	// Auth diz como a pessoa dá acesso: AuthToken (padrão, ela cola um token) ou
	// AuthOAuth (ela autoriza no site da plataforma e volta; não há campo de token).
	Auth string `json:"auth,omitempty"`
	// Configured é preenchido pela página: o servidor tem o app OAuth deste tipo
	// cadastrado. Só faz sentido para AuthOAuth.
	Configured bool `json:"configured"`
}

// Os modos de dar acesso a uma plataforma (Descriptor.Auth).
const (
	AuthToken = "token"
	AuthOAuth = "oauth"
)

// Repository é um repositório que a conexão enxerga, para a pessoa escolher em vez de
// digitar o nome.
type Repository struct {
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
}

// RepositoryLister é a capacidade opcional de um tipo que sabe listar o que a conexão
// enxerga. A API de repositórios responde 400 para o tipo que não a tem.
type RepositoryLister interface {
	ListRepositories(conn Connection) ([]Repository, error)
}

// AccountLookup é a capacidade opcional de dizer quem o token representa. A conexão
// OAuth a usa para conferir o token recém-obtido e para nomear a integração.
type AccountLookup interface {
	Account(conn Connection) (login string, err error)
}

// IssueRepo é o repositório como a sincronização de issues o enxerga.
type IssueRepo struct {
	FullName string
	Archived bool
	// CanPush diz se o token escreve nas issues. Sem isso a sincronização só traz as issues para cá.
	CanPush bool
}

// Issue é uma issue da plataforma, no que a sincronização usa dela.
type Issue struct {
	Number      int
	Title       string
	Body        string
	State       string // "open" ou "closed"
	StateReason string // "completed", "not_planned", "reopened" ou vazio
	Labels      []string
	Assignees   []string // logins
	URL         string
	UpdatedAt   time.Time
	// PullRequest marca o número que é de um pull request. A lista nunca traz um; a leitura de um
	// número só o traz se o número for de um.
	PullRequest bool
}

// ListIssuesOptions diz quais issues listar.
type ListIssuesOptions struct {
	State string // "open" (padrão) ou "all"
	// Since limita às issues mexidas depois deste instante; zero é sem limite.
	Since time.Time
	// ByUpdated ordena pelas mexidas há mais tempo primeiro; senão, pelas mais antigas primeiro.
	ByUpdated bool
}

// IssueList é o resultado de uma listagem. Quando ela para no meio (limite de requisições, erro de
// rede), o erro vem junto com as issues que já tinham chegado.
type IssueList struct {
	Issues []Issue
	// ServerTime é o relógio da plataforma quando a listagem começou: o ponto de onde a próxima
	// rodada incremental pode continuar sem perder o que mudou durante esta.
	ServerTime time.Time
}

// IssuePatch é a mudança numa issue; um campo nulo fica como está. Labels e Assignees substituem o
// conjunto inteiro, como a plataforma faz.
type IssuePatch struct {
	Title       *string
	Body        *string
	State       *string
	StateReason *string
	Labels      *[]string
	Assignees   *[]string
}

// NewIssue é o que se manda para criar uma issue.
type NewIssue struct {
	Title     string
	Body      string
	Labels    []string
	Assignees []string // logins
}

// IssueSyncer é a capacidade opcional de um tipo que sabe ler e escrever as issues do repositório
// da integração, para a sincronização com as tarefas. Todos os métodos aceitam o contexto, porque a
// rodada é de longa duração e pode ser cancelada.
type IssueSyncer interface {
	Repo(ctx context.Context, conn Connection) (*IssueRepo, error)
	// ListIssues percorre todas as páginas. As issues que são pull requests ficam de fora.
	ListIssues(ctx context.Context, conn Connection, opts ListIssuesOptions) (*IssueList, error)
	// GetIssue devolve ErrIssueGone quando a issue foi apagada ou transferida.
	GetIssue(ctx context.Context, conn Connection, number int) (*Issue, error)
	// UpdateIssue aplica a mudança e devolve a issue como ficou, que pode ser diferente do pedido:
	// a plataforma descarta sem avisar o que o token não pode mudar.
	UpdateIssue(ctx context.Context, conn Connection, number int, patch IssuePatch) (*Issue, error)
	// CreateIssue cria a issue e devolve como ela ficou, que pode ser diferente do pedido: quem não tem
	// permissão de escrita no repositório tem as etiquetas e os responsáveis descartados sem aviso.
	CreateIssue(ctx context.Context, conn Connection, issue NewIssue) (*Issue, error)
	ListLabels(ctx context.Context, conn Connection) ([]string, error)
	// CreateLabel cria a etiqueta; uma que já existe não é erro.
	CreateLabel(ctx context.Context, conn Connection, name string) error
	// UserEmail devolve o e-mail público do usuário, ou vazio se ele não publica um (ou não existe).
	UserEmail(ctx context.Context, conn Connection, login string) (string, error)
	// FindLoginByEmail devolve o login do único usuário que publica este e-mail, ou vazio se não
	// há nenhum ou há mais de um.
	FindLoginByEmail(ctx context.Context, conn Connection, email string) (string, error)
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
