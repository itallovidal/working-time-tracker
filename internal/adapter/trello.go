package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const defaultTrelloBaseURL = "https://api.trello.com/1"

var trelloDescriptor = Descriptor{
	Type:  "trello",
	Label: "Trello",
	Metadata: []Field{
		// A chave é a do app (TRELLO_API_KEY), guardada pela conexão: a tela não a mostra nem a deixa trocar.
		{Key: "api_key", Required: true, Internal: true},
		{Key: "board_id", Required: true, Summary: true, Picker: "select", NameKey: "board_name"},
		// O nome do quadro, que a tela guarda ao escolher na lista para o cartão mostrar no lugar do id.
		{Key: "board_name", Hidden: true},
	},
	// Cada cartão do quadro é uma tarefa (as listas ficam de fora por ora), e a tarefa criada aqui vira um
	// cartão sozinha. O Trello não expõe o e-mail dos membros, então o responsável não se liga, e não filtra
	// os cartões por data, então toda rodada olha o quadro inteiro.
	Sync: true,
	Caps: SyncCaps{Deadline: true, AutoPublish: true},
	// A pessoa autoriza no site do Trello e volta (trello_auth.go), sem token para colar.
	Auth: AuthOAuth,
}

// TrelloIntegration fala com a API do Trello. BaseURL e Client são opcionais e
// existem para apontar o adapter para um servidor fake nos testes.
type TrelloIntegration struct {
	BaseURL string
	Client  *http.Client
}

func (t *TrelloIntegration) baseURL() string {
	if t.BaseURL == "" {
		return defaultTrelloBaseURL
	}
	return strings.TrimRight(t.BaseURL, "/")
}

func (t *TrelloIntegration) client() *http.Client {
	if t.Client == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return t.Client
}

// trelloMetadata são os campos que só o Trello tem. A chave identifica o Power-Up e
// pode ser pública; o segredo é o token.
type trelloMetadata struct {
	APIKey    string
	BoardID   string
	BoardName string // opcional: o nome legível do quadro
}

// trelloBoardNameLimit é o tamanho máximo do nome do quadro guardado.
const trelloBoardNameLimit = 200

var (
	// Ids, links curtos e chaves do Trello só têm letras e dígitos.
	trelloID       = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	trelloBoardURL = regexp.MustCompile(`trello\.com/b/([A-Za-z0-9]+)`)
	trelloCardURL  = regexp.MustCompile(`trello\.com/c/([A-Za-z0-9]+)`)
)

func parseTrelloMetadata(raw map[string]any) (*trelloMetadata, error) {
	fields, err := trelloDescriptor.fields(raw)
	if err != nil {
		return nil, err
	}
	key := fields["api_key"]
	if !trelloID.MatchString(key) {
		return nil, ErrTrelloInvalidKey
	}
	board := fields["board_id"]
	if m := trelloBoardURL.FindStringSubmatch(board); m != nil {
		board = m[1]
	}
	if !trelloID.MatchString(board) {
		return nil, ErrTrelloInvalidBoard
	}
	name := fields["board_name"]
	if r := []rune(name); len(r) > trelloBoardNameLimit {
		name = string(r[:trelloBoardNameLimit])
	}
	return &trelloMetadata{APIKey: key, BoardID: board, BoardName: name}, nil
}

func (t *TrelloIntegration) Descriptor() Descriptor {
	return trelloDescriptor
}

func (t *TrelloIntegration) CheckMetadata(raw map[string]any) (map[string]any, error) {
	meta, err := parseTrelloMetadata(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"api_key": meta.APIKey, "board_id": meta.BoardID}
	if meta.BoardName != "" {
		out["board_name"] = meta.BoardName
	}
	return out, nil
}

// get manda a chave e o token no cabeçalho, para o token não aparecer na URL.
func (t *TrelloIntegration) get(conn Connection, meta *trelloMetadata, path string) (*http.Response, error) {
	return t.do(context.Background(), conn, meta.APIKey, "GET", path, nil)
}

// do faz uma requisição à API. target é o caminho, já com a query. O corpo, quando há, vai em JSON. Não
// segue redirecionamento: o net/http transformaria um PUT em GET ao seguir um 301 e daria "200" sem ter
// gravado nada.
func (t *TrelloIntegration) do(ctx context.Context, conn Connection, apiKey, method, target string, body any) (*http.Response, error) {
	token, err := trelloDescriptor.token(conn)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(token, "\"\\ \t\r\n") {
		return nil, ErrInvalidToken.With("provider", "Trello")
	}
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.baseURL()+target, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, apiKey, token))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "working-time-tracker")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := *t.client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderUnreachable.With("provider", "Trello").Wrap(err)
	}
	return resp, nil
}

// withQuery junta a query ao caminho.
func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

func (t *TrelloIntegration) Validate(conn Connection) error {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return err
	}

	resp, err := t.get(conn, meta, "/boards/"+meta.BoardID+"?fields=name")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		// O Trello responde 401 tanto para credencial errada quanto para quadro que o token não vê.
		return ErrTrelloNoAccessBoard
	case http.StatusBadRequest, http.StatusNotFound:
		return ErrTrelloBoardMissing
	default:
		return ErrProviderStatus.With("provider", "Trello", "status", resp.StatusCode)
	}
}

func (t *TrelloIntegration) FetchItemDetails(conn Connection, itemID string) (*ItemDetails, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	card := strings.TrimSpace(itemID)
	if m := trelloCardURL.FindStringSubmatch(card); m != nil {
		card = m[1]
	}
	if !trelloID.MatchString(card) {
		return nil, ErrTrelloInvalidCard
	}

	// Uma chamada só: o cartão com o quadro dele aninhado.
	resp, err := t.get(conn, meta, "/cards/"+card+"?fields=name,closed,due,dueComplete,shortUrl,idBoard&board=true&board_fields=shortLink")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTrelloNoAccessCard
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, ErrItemNotFound.With("item", card)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrProviderStatus.With("provider", "Trello", "status", resp.StatusCode)
	}

	var body struct {
		Name        string  `json:"name"`
		Closed      bool    `json:"closed"`
		Due         *string `json:"due"`
		DueComplete bool    `json:"dueComplete"`
		ShortURL    string  `json:"shortUrl"`
		IDBoard     string  `json:"idBoard"`
		Board       struct {
			ID        string `json:"id"`
			ShortLink string `json:"shortLink"`
		} `json:"board"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, ErrUnexpectedResponse.With("provider", "Trello").Wrap(err)
	}

	// O quadro foi configurado pelo id ou pelo link curto: o cartão tem de ser dele.
	sameBoard := meta.BoardID == body.Board.ShortLink ||
		strings.EqualFold(meta.BoardID, body.Board.ID) || strings.EqualFold(meta.BoardID, body.IDBoard)
	if !sameBoard {
		return nil, ErrTrelloCardOtherBoard.With("card", card)
	}

	// Fechado é o cartão arquivado ou o que teve a data de entrega marcada como concluída: as duas fecham a
	// tarefa. As listas ficam de fora por ora.
	state := "open"
	if body.Closed || (body.Due != nil && body.DueComplete) {
		state = "closed"
	}
	return &ItemDetails{
		Title: body.Name,
		State: state,
		URL:   body.ShortURL,
	}, nil
}
