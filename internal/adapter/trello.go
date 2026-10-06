package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultTrelloBaseURL = "https://api.trello.com/1"

var trelloDescriptor = Descriptor{
	Type:  "trello",
	Label: "Trello",
	Metadata: []Field{
		{Key: "api_key", Required: true},
		{Key: "board_id", Required: true, Summary: true},
	},
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
	APIKey  string
	BoardID string
}

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
	return &trelloMetadata{APIKey: key, BoardID: board}, nil
}

func (t *TrelloIntegration) Descriptor() Descriptor {
	return trelloDescriptor
}

func (t *TrelloIntegration) CheckMetadata(raw map[string]any) (map[string]any, error) {
	meta, err := parseTrelloMetadata(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"api_key": meta.APIKey, "board_id": meta.BoardID}, nil
}

// get manda a chave e o token no cabeçalho, para o token não aparecer na URL.
func (t *TrelloIntegration) get(conn Connection, meta *trelloMetadata, path string) (*http.Response, error) {
	token, err := trelloDescriptor.token(conn)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(token, "\"\\ \t\r\n") {
		return nil, ErrInvalidToken.With("provider", "Trello")
	}
	req, err := http.NewRequest("GET", t.baseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, meta.APIKey, token))
	req.Header.Set("Accept", "application/json")

	resp, err := t.client().Do(req)
	if err != nil {
		return nil, ErrProviderUnreachable.With("provider", "Trello").Wrap(err)
	}
	return resp, nil
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

	// Uma chamada só: o cartão com a lista e o quadro dele aninhados.
	resp, err := t.get(conn, meta, "/cards/"+card+"?fields=name,closed,shortUrl,idBoard&list=true&board=true&board_fields=shortLink")
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
		Name     string `json:"name"`
		Closed   bool   `json:"closed"`
		ShortURL string `json:"shortUrl"`
		IDBoard  string `json:"idBoard"`
		List     struct {
			Name string `json:"name"`
		} `json:"list"`
		Board struct {
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

	// O estado de um cartão é a lista em que ele está.
	state := body.List.Name
	if body.Closed {
		state = "arquivado"
	} else if state == "" {
		state = "aberto"
	}
	return &ItemDetails{
		Title: body.Name,
		State: state,
		URL:   body.ShortURL,
	}, nil
}
