package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TrelloCard é um cartão do Trello fake. Os campos são os que a sincronização lê e escreve.
type TrelloCard struct {
	// ID tem 24 caracteres hexadecimais, e os 8 primeiros são o instante de criação, como no Trello.
	// ShortLink é o link curto (8 caracteres). Vazios, o fake os cria.
	ID        string
	ShortLink string
	Board     string // o id do quadro; vazio é o quadro de sempre
	List      string // o id da lista; vazio é a primeira do quadro
	Name      string
	Desc      string
	Closed    bool // arquivado
	// Due é a data de entrega; o tempo zero é sem data. DueComplete é ela marcada como concluída.
	Due         time.Time
	DueComplete bool
	Labels      []string // ids de etiquetas do quadro
	Created     time.Time
	Activity    time.Time
}

// TrelloLabel é uma etiqueta do quadro: nome e cor (o Trello permite etiqueta só com cor).
type TrelloLabel struct {
	ID    string
	Name  string
	Color string
}

// TrelloRequest é uma requisição que o Trello fake recebeu, para o teste contar o que o sistema pediu.
type TrelloRequest struct {
	Method string
	Path   string // sem o /1 do começo
	Query  string
	Body   string
}

// Trello é um Trello falso com estado: quadros com listas, etiquetas e cartões, a conta do token, a
// autorização que devolve o token no fragmento da URL, e chaves para os defeitos que a sincronização
// precisa aguentar (token revogado, quadro que o token não escreve ou não vê, PUT que descarta campos
// sem avisar, cartão apagado ou movido, limite de requisições). Atende a API na raiz e também em /1,
// como o endereço de verdade. Exige a chave TrelloKey e o token no cabeçalho Authorization (com
// credencial na URL, responde 400). Só atende em loopback.
//
// Conhece o quadro TrelloBoardID (link curto TrelloBoardShortLink, lista "Em andamento", os cartões
// H0TZyzbK aberto e ArqUiv4d arquivado) e o quadro TrelloOtherBoardID (o cartão OutroQdr).
type Trello struct {
	mu       sync.Mutex
	boards   map[string]*trBoard
	cards    []*TrelloCard
	revoked  map[string]bool
	requests []TrelloRequest
	faults   []*trFault
	limited  time.Time
	denyAuth bool
	seq      int
	last     time.Time
	handler  *http.ServeMux
}

type trBoard struct {
	id, shortLink, name, org string
	closed                   bool
	private                  bool
	member                   bool // o token enxerga o quadro
	write                    bool // e escreve nele (senão é observador)
	lists                    []trList
	labels                   []*TrelloLabel
	discard                  map[string]bool
	labelError               int
}

type trList struct {
	id, name string
	pos      float64
}

type trFault struct {
	method, prefix string
	status         int
	times          int
}

// NewTrello devolve o Trello fake com os quadros e os cartões de sempre.
func NewTrello() *Trello {
	t := &Trello{boards: map[string]*trBoard{}, revoked: map[string]bool{}}
	t.handler = t.routes()
	t.AddBoard(TrelloBoardID, TrelloBoardShortLink, "App")
	t.boards[TrelloBoardID].org = "Acme"
	t.boards[TrelloBoardID].lists = []trList{{id: "5abbe4b7ddc1b351ef961477", name: "Em andamento", pos: 65536}}
	t.AddBoard(TrelloOtherBoardID, TrelloOtherBoardShortLink, "Outro quadro")
	created := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	t.AddCard(TrelloCard{ShortLink: "H0TZyzbK", Name: "Corrigir login", Created: created})
	t.AddCard(TrelloCard{ShortLink: "ArqUiv4d", Name: "Tela antiga", Closed: true, Created: created})
	t.AddCard(TrelloCard{ShortLink: "OutroQdr", Name: "De outro quadro", Board: TrelloOtherBoardID, Created: created})
	return t
}

// FakeTrello é o Trello fake como http.Handler, para quem só precisa do servidor.
func FakeTrello() http.Handler { return NewTrello() }

// bump devolve um instante maior que todos os anteriores, para a ordem das escritas ser a dos instantes.
func (t *Trello) bump() time.Time {
	now := time.Now().UTC().Truncate(time.Millisecond)
	if !now.After(t.last) {
		now = t.last.Add(time.Millisecond)
	}
	t.last = now
	return now
}

func (t *Trello) next() int {
	t.seq++
	return t.seq
}

func (t *Trello) board(id string) *trBoard {
	b := t.boards[id]
	if b == nil {
		panic("testutil: unknown fake Trello board " + id)
	}
	return b
}

// firstList é o id da lista de menor posição (a primeira do quadro), ou vazio se não há lista.
func (b *trBoard) firstList() string {
	id, pos := "", 0.0
	for i, l := range b.lists {
		if i == 0 || l.pos < pos {
			id, pos = l.id, l.pos
		}
	}
	return id
}

// AddBoard cria um quadro que o token enxerga e onde escreve, com uma lista "A fazer".
func (t *Trello) AddBoard(id, shortLink, name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.boards[id] = &trBoard{
		id: id, shortLink: shortLink, name: name, member: true, write: true, private: true,
		lists:   []trList{{id: fmt.Sprintf("%08x%016x", 1700000000, t.next()), name: "A fazer", pos: 65536}},
		discard: map[string]bool{},
	}
}

// AddList cria uma lista no fim do quadro, ou com esta posição (a lista de menor posição é a primeira).
func (t *Trello) AddList(board, name string, pos float64) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.board(board)
	id := fmt.Sprintf("%08x%016x", 1700000000, t.next())
	b.lists = append(b.lists, trList{id: id, name: name, pos: pos})
	return id
}

// ClearLists tira todas as listas do quadro: criar um cartão nele passa a ser impossível.
func (t *Trello) ClearLists(board string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.board(board).lists = nil
}

// AddLabel cria uma etiqueta no quadro e devolve o id dela.
func (t *Trello) AddLabel(board, name, color string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.addLabel(t.board(board), name, color)
}

func (t *Trello) addLabel(b *trBoard, name, color string) string {
	l := &TrelloLabel{ID: fmt.Sprintf("%08x%016x", 1700000000, t.next()), Name: name, Color: color}
	b.labels = append(b.labels, l)
	return l.ID
}

// Labels lista as etiquetas do quadro.
func (t *Trello) Labels(board string) []TrelloLabel {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []TrelloLabel
	for _, l := range t.board(board).labels {
		out = append(out, *l)
	}
	return out
}

// AddCard cria um cartão e devolve o id dele. O que vem vazio é preenchido: o quadro de sempre, a
// primeira lista, o instante de agora.
func (t *Trello) AddCard(c TrelloCard) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c.Board == "" {
		c.Board = TrelloBoardID
	}
	b := t.board(c.Board)
	if c.List == "" {
		c.List = b.firstList()
	}
	if c.Created.IsZero() {
		c.Created = t.bump()
	}
	if c.ID == "" {
		c.ID = fmt.Sprintf("%08x%016x", c.Created.Unix(), t.next())
	}
	if c.ShortLink == "" {
		c.ShortLink = fmt.Sprintf("Cd%06d", t.next())
	}
	c.Activity = t.bump()
	c.Labels = append([]string(nil), c.Labels...)
	t.cards = append(t.cards, &c)
	return c.ID
}

func (t *Trello) find(idOrShort string) *TrelloCard {
	for _, c := range t.cards {
		if c.ID == idOrShort || c.ShortLink == idOrShort {
			return c
		}
	}
	return nil
}

// Card devolve uma cópia do cartão (pelo id ou pelo link curto).
func (t *Trello) Card(idOrShort string) (TrelloCard, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c := t.find(idOrShort); c != nil {
		out := *c
		out.Labels = append([]string(nil), c.Labels...)
		return out, true
	}
	return TrelloCard{}, false
}

// Cards lista os cartões do quadro, na ordem em que nasceram.
func (t *Trello) Cards(board string) []TrelloCard {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []TrelloCard
	for _, c := range t.cards {
		if c.Board == board {
			cp := *c
			cp.Labels = append([]string(nil), c.Labels...)
			out = append(out, cp)
		}
	}
	return out
}

// EditCard é alguém mexendo no cartão pelo site do Trello: aplica a função e atualiza a última atividade.
func (t *Trello) EditCard(idOrShort string, edit func(*TrelloCard)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.find(idOrShort)
	if c == nil {
		panic("testutil: fake Trello has no card " + idOrShort)
	}
	edit(c)
	c.Activity = t.bump()
}

// DeleteCard apaga o cartão: o Trello passa a responder 404.
func (t *Trello) DeleteCard(idOrShort string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, c := range t.cards {
		if c.ID == idOrShort || c.ShortLink == idOrShort {
			t.cards = append(t.cards[:i], t.cards[i+1:]...)
			return
		}
	}
}

// MoveCard leva o cartão para a primeira lista de outro quadro.
func (t *Trello) MoveCard(idOrShort, board string) {
	t.EditCard(idOrShort, func(c *TrelloCard) {
		c.Board, c.List, c.Labels = board, "", nil
		if b := t.boards[board]; b != nil {
			c.List = b.firstList()
		}
	})
}

// SetWrite diz se o token escreve no quadro. Sem escrita ele é observador: os PUT e POST são recusados com 401.
func (t *Trello) SetWrite(board string, write bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.board(board).write = write
}

// SetVisible diz se o token enxerga o quadro. Sem isso o Trello responde 401 a tudo dele.
func (t *Trello) SetVisible(board string, visible bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.board(board).member = visible
}

// SetClosed fecha (arquiva) o quadro.
func (t *Trello) SetClosed(board string, closed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.board(board).closed = closed
}

// RevokeToken faz o Trello recusar este token, como quem o revogou na conta.
func (t *Trello) RevokeToken(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.revoked[token] = true
}

// Discard faz o PUT do quadro descartar estes campos sem avisar (idLabels, name, desc, closed, due):
// responde 200 com o cartão como estava.
func (t *Trello) Discard(board string, fields ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.board(board)
	b.discard = map[string]bool{}
	for _, f := range fields {
		b.discard[f] = true
	}
}

// LabelCreateStatus faz a criação de etiqueta devolver este status (401, por exemplo). Zero volta ao normal.
func (t *Trello) LabelCreateStatus(board string, status int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.board(board).labelError = status
}

// DenyAuthorization faz a autorização devolver a pessoa sem token, como quem clicou em Negar.
func (t *Trello) DenyAuthorization(deny bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.denyAuth = deny
}

// Fail faz as próximas `times` requisições que casam com o método e o prefixo do caminho responderem com
// este status, sem tocar no estado.
func (t *Trello) Fail(method, pathPrefix string, status, times int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.faults = append(t.faults, &trFault{method: method, prefix: pathPrefix, status: status, times: times})
}

// RateLimitedFor recusa toda requisição da API, por limite, durante este tempo.
func (t *Trello) RateLimitedFor(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.limited = time.Now().Add(d)
}

// Requests devolve o que o fake recebeu, na ordem. Reset zera o registro.
func (t *Trello) Requests() []TrelloRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]TrelloRequest(nil), t.requests...)
}

// Count conta as requisições do método cujo caminho começa com o prefixo.
func (t *Trello) Count(method, pathPrefix string) int {
	n := 0
	for _, r := range t.Requests() {
		if r.Method == method && strings.HasPrefix(r.Path, pathPrefix) {
			n++
		}
	}
	return n
}

// Writes conta as requisições que escrevem (tudo que não é GET).
func (t *Trello) Writes() int {
	n := 0
	for _, r := range t.Requests() {
		if r.Method != http.MethodGet {
			n++
		}
	}
	return n
}

// Reset apaga o registro de requisições.
func (t *Trello) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests = nil
}

// ServeHTTP atende a API e o site (a autorização) no mesmo endereço, na raiz e em /1.
func (t *Trello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if strings.HasPrefix(r.URL.Path, "/1/") {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/1")
	}

	t.mu.Lock()
	t.requests = append(t.requests, TrelloRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(body)})
	limited := t.limited
	var fault *trFault
	for _, f := range t.faults {
		if f.times > 0 && f.method == r.Method && strings.HasPrefix(r.URL.Path, f.prefix) {
			f.times--
			fault = f
			break
		}
	}
	t.mu.Unlock()

	w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
	if r.URL.Path != "/authorize" {
		if time.Now().Before(limited) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":"API_TOKEN_LIMIT_EXCEEDED","message":"Rate limit exceeded"}`)
			return
		}
		if fault != nil {
			http.Error(w, "injected failure", fault.status)
			return
		}
	}
	t.handler.ServeHTTP(w, r)
}

var trelloAuthHeader = regexp.MustCompile(`oauth_consumer_key="([^"]*)", oauth_token="([^"]*)"`)

func (t *Trello) routes() *http.ServeMux {
	mux := http.NewServeMux()
	// auth confere a chave e o token do cabeçalho e recusa credencial na URL.
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Has("key") || q.Has("token") {
				http.Error(w, "credentials in the URL", http.StatusBadRequest)
				return
			}
			m := trelloAuthHeader.FindStringSubmatch(r.Header.Get("Authorization"))
			if m == nil || m[1] != TrelloKey {
				http.Error(w, "invalid key", http.StatusUnauthorized)
				return
			}
			t.mu.Lock()
			bad := m[2] == "" || m[2] == InvalidToken || t.revoked[m[2]]
			t.mu.Unlock()
			if bad {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("GET /members/me", auth(t.getMe))
	mux.HandleFunc("GET /members/me/boards", auth(t.myBoards))
	mux.HandleFunc("GET /boards/{id}", auth(t.getBoard))
	mux.HandleFunc("GET /boards/{id}/lists", auth(t.boardLists))
	mux.HandleFunc("GET /boards/{id}/labels", auth(t.boardLabels))
	mux.HandleFunc("GET /boards/{id}/cards", auth(t.boardCards))
	mux.HandleFunc("GET /boards/{id}/cards/{filter}", auth(t.boardCards))
	mux.HandleFunc("GET /cards/{id}", auth(t.getCard))
	mux.HandleFunc("PUT /cards/{id}", auth(t.putCard))
	mux.HandleFunc("POST /cards", auth(t.postCard))
	mux.HandleFunc("POST /labels", auth(t.postLabel))
	mux.HandleFunc("GET /authorize", t.authorize)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "The requested resource was not found.", http.StatusNotFound)
	})
	return mux
}

var trelloIDPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func trJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func (t *Trello) getMe(w http.ResponseWriter, r *http.Request) {
	trJSON(w, map[string]any{"id": "5abbe4b7ddc1b351ef960001", "username": TrelloUsername, "fullName": "Octo Trello"})
}

func (t *Trello) myBoards(w http.ResponseWriter, r *http.Request) {
	withOrg := r.URL.Query().Get("organization") == "true"
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []map[string]any{}
	for _, b := range t.boards {
		if !b.member || b.closed {
			continue
		}
		level := "private"
		if !b.private {
			level = "org"
		}
		m := map[string]any{"id": b.id, "name": b.name, "shortLink": b.shortLink, "closed": b.closed, "prefs": map[string]any{"permissionLevel": level}}
		if withOrg && b.org != "" {
			m["organization"] = map[string]any{"displayName": b.org}
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	trJSON(w, out)
}

// boardFor acha o quadro pelo id ou pelo link curto e confere o que o token pode: 400 para o id que o Trello
// não entende, 404 para o que não existe, 401 para o quadro que o token não enxerga. Devolve nil depois de
// responder.
func (t *Trello) boardFor(w http.ResponseWriter, idOrShort string) *trBoard {
	if !trelloIDPattern.MatchString(idOrShort) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range t.boards {
		if b.id == idOrShort || b.shortLink == idOrShort {
			if !b.member {
				http.Error(w, "unauthorized board permission requested", http.StatusUnauthorized)
				return nil
			}
			cp := *b
			cp.lists = append([]trList(nil), b.lists...)
			cp.labels = append([]*TrelloLabel(nil), b.labels...)
			return &cp
		}
	}
	http.Error(w, "The requested resource was not found.", http.StatusNotFound)
	return nil
}

func (t *Trello) getBoard(w http.ResponseWriter, r *http.Request) {
	b := t.boardFor(w, r.PathValue("id"))
	if b == nil {
		return
	}
	out := map[string]any{"id": b.id, "name": b.name, "closed": b.closed, "shortLink": b.shortLink}
	if r.URL.Query().Get("memberships") != "" {
		memberships := []map[string]any{}
		if b.member {
			kind := "observer"
			if b.write {
				kind = "admin"
			}
			memberships = append(memberships, map[string]any{"idMember": "5abbe4b7ddc1b351ef960001", "memberType": kind, "deactivated": false})
		}
		out["memberships"] = memberships
	}
	trJSON(w, out)
}

func (t *Trello) boardLists(w http.ResponseWriter, r *http.Request) {
	b := t.boardFor(w, r.PathValue("id"))
	if b == nil {
		return
	}
	out := []map[string]any{}
	for _, l := range b.lists {
		out = append(out, map[string]any{"id": l.id, "name": l.name, "pos": l.pos, "closed": false, "idBoard": b.id})
	}
	trJSON(w, out)
}

func (t *Trello) boardLabels(w http.ResponseWriter, r *http.Request) {
	b := t.boardFor(w, r.PathValue("id"))
	if b == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []map[string]any{}
	for _, l := range b.labels {
		out = append(out, map[string]any{"id": l.ID, "name": l.Name, "color": nullable(l.Color), "idBoard": b.id})
	}
	trJSON(w, out)
}

// nullable devolve nulo para o texto vazio, como o Trello faz com a etiqueta sem cor.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

const trelloDue = "2006-01-02T15:04:05.000Z"

// cardJSON monta o cartão como o Trello o devolve. Quem chama já tem o cadeado.
func (t *Trello) cardJSON(c *TrelloCard, withBoard bool) map[string]any {
	b := t.boards[c.Board]
	labels := []map[string]any{}
	ids := []string{}
	if b != nil {
		for _, id := range c.Labels {
			for _, l := range b.labels {
				if l.ID == id {
					labels = append(labels, map[string]any{"id": l.ID, "name": l.Name, "color": nullable(l.Color)})
					ids = append(ids, l.ID)
				}
			}
		}
	}
	var due any
	if !c.Due.IsZero() {
		due = c.Due.UTC().Format(trelloDue)
	}
	out := map[string]any{
		"id": c.ID, "shortLink": c.ShortLink, "name": c.Name, "desc": c.Desc, "closed": c.Closed,
		"due": due, "dueComplete": c.DueComplete, "idList": c.List, "idBoard": c.Board,
		"idLabels": ids, "labels": labels,
		"shortUrl": "https://trello.com/c/" + c.ShortLink, "url": "https://trello.com/c/" + c.ShortLink + "/card",
		"dateLastActivity": c.Activity.UTC().Format(trelloDue),
	}
	if withBoard && b != nil {
		out["board"] = map[string]any{"id": b.id, "shortLink": b.shortLink}
	}
	return out
}

func (t *Trello) boardCards(w http.ResponseWriter, r *http.Request) {
	b := t.boardFor(w, r.PathValue("id"))
	if b == nil {
		return
	}
	filter := r.PathValue("filter")
	if filter == "" {
		filter = "open"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []map[string]any{}
	for _, c := range t.cards {
		if c.Board != b.id {
			continue
		}
		if (filter == "open" && c.Closed) || (filter == "closed" && !c.Closed) {
			continue
		}
		out = append(out, t.cardJSON(c, false))
	}
	trJSON(w, out)
}

// cardFor acha o cartão e confere o que o token pode ver; mesma regra de boardFor.
func (t *Trello) cardFor(w http.ResponseWriter, idOrShort string) (*TrelloCard, *trBoard) {
	if !trelloIDPattern.MatchString(idOrShort) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return nil, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.find(idOrShort)
	if c == nil {
		http.Error(w, "The requested resource was not found.", http.StatusNotFound)
		return nil, nil
	}
	b := t.boards[c.Board]
	if b == nil || !b.member {
		http.Error(w, "unauthorized card permission requested", http.StatusUnauthorized)
		return nil, nil
	}
	return c, b
}

func (t *Trello) getCard(w http.ResponseWriter, r *http.Request) {
	c, _ := t.cardFor(w, r.PathValue("id"))
	if c == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	trJSON(w, t.cardJSON(c, r.URL.Query().Get("board") == "true"))
}

// params junta o que veio na query, no formulário e no corpo JSON: o Trello aceita os três.
func params(r *http.Request) map[string]any {
	out := map[string]any{}
	for k, v := range r.URL.Query() {
		out[k] = v[0]
	}
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		var js map[string]any
		if json.Unmarshal(body, &js) == nil {
			for k, v := range js {
				out[k] = v
			}
		} else if form, err := url.ParseQuery(string(body)); err == nil {
			for k, v := range form {
				out[k] = v[0]
			}
		}
	}
	return out
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		b, _ := strconv.ParseBool(x)
		return b
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// labelIDs lê o idLabels, que vem como lista separada por vírgula (ou como lista JSON), e fica só com as
// etiquetas que o quadro tem.
func (b *trBoard) labelIDs(v any) []string {
	var raw []string
	switch x := v.(type) {
	case string:
		for _, s := range strings.Split(x, ",") {
			if s = strings.TrimSpace(s); s != "" {
				raw = append(raw, s)
			}
		}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	out := []string{}
	for _, id := range raw {
		for _, l := range b.labels {
			if l.ID == id {
				out = append(out, id)
			}
		}
	}
	return out
}

func (t *Trello) putCard(w http.ResponseWriter, r *http.Request) {
	c, _ := t.cardFor(w, r.PathValue("id"))
	if c == nil {
		return
	}
	p := params(r)
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.boards[c.Board]
	if !b.write {
		http.Error(w, "unauthorized card permission requested", http.StatusUnauthorized)
		return
	}
	skip := func(field string) bool { return b.discard[field] }
	if v, ok := p["name"]; ok && !skip("name") {
		name := asString(v)
		if strings.TrimSpace(name) == "" {
			http.Error(w, "invalid value for name", http.StatusBadRequest)
			return
		}
		c.Name = name
	}
	if v, ok := p["desc"]; ok && !skip("desc") {
		desc := asString(v)
		if len(desc) > 16384 {
			http.Error(w, "invalid value for desc", http.StatusBadRequest)
			return
		}
		c.Desc = desc
	}
	if v, ok := p["closed"]; ok && !skip("closed") {
		c.Closed = asBool(v)
	}
	if v, ok := p["dueComplete"]; ok && !skip("closed") {
		c.DueComplete = asBool(v)
	}
	if v, ok := p["due"]; ok && !skip("due") {
		s := asString(v)
		if s == "" || s == "null" {
			c.Due, c.DueComplete = time.Time{}, false
		} else {
			due, err := time.Parse(time.RFC3339, s)
			if err != nil {
				http.Error(w, "invalid value for due", http.StatusBadRequest)
				return
			}
			c.Due = due.UTC()
		}
	}
	if v, ok := p["idLabels"]; ok && !skip("idLabels") {
		c.Labels = b.labelIDs(v)
	}
	c.Activity = t.bump()
	trJSON(w, t.cardJSON(c, false))
}

func (t *Trello) postCard(w http.ResponseWriter, r *http.Request) {
	p := params(r)
	listID := asString(p["idList"])
	t.mu.Lock()
	defer t.mu.Unlock()
	var b *trBoard
	for _, candidate := range t.boards {
		for _, l := range candidate.lists {
			if l.id == listID {
				b = candidate
			}
		}
	}
	if b == nil {
		http.Error(w, "invalid value for idList", http.StatusBadRequest)
		return
	}
	if !b.member || !b.write {
		http.Error(w, "unauthorized board permission requested", http.StatusUnauthorized)
		return
	}
	created := t.bump()
	c := &TrelloCard{
		ID: fmt.Sprintf("%08x%016x", created.Unix(), t.next()), ShortLink: fmt.Sprintf("Cd%06d", t.next()),
		Board: b.id, List: listID, Name: asString(p["name"]), Desc: asString(p["desc"]), Created: created, Activity: created,
	}
	if s := asString(p["due"]); s != "" && s != "null" {
		if due, err := time.Parse(time.RFC3339, s); err == nil {
			c.Due = due.UTC()
		}
	}
	if !b.discard["idLabels"] {
		c.Labels = b.labelIDs(p["idLabels"])
	}
	t.cards = append(t.cards, c)
	trJSON(w, t.cardJSON(c, false))
}

var trelloColors = map[string]bool{
	"green": true, "yellow": true, "orange": true, "red": true, "purple": true, "blue": true,
	"sky": true, "lime": true, "pink": true, "black": true,
}

func (t *Trello) postLabel(w http.ResponseWriter, r *http.Request) {
	p := params(r)
	boardID := asString(p["idBoard"])
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.boards[boardID]
	if b == nil {
		http.Error(w, "invalid value for idBoard", http.StatusBadRequest)
		return
	}
	if !b.member || !b.write {
		http.Error(w, "unauthorized board permission requested", http.StatusUnauthorized)
		return
	}
	if b.labelError != 0 {
		http.Error(w, "injected failure", b.labelError)
		return
	}
	color := asString(p["color"])
	if !trelloColors[color] {
		http.Error(w, "invalid value for color", http.StatusBadRequest)
		return
	}
	id := t.addLabel(b, asString(p["name"]), color)
	trJSON(w, map[string]any{"id": id, "name": asString(p["name"]), "color": color, "idBoard": b.id})
}

// authorize é a página de autorização do Trello: confere o pedido e, como quem clicou em Permitir, devolve a
// pessoa ao return_url com o token no fragmento. DenyAuthorization a faz voltar sem token.
func (t *Trello) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("key") != TrelloKey {
		http.Error(w, "Invalid key", http.StatusBadRequest)
		return
	}
	if q.Get("response_type") != "token" || q.Get("callback_method") != "fragment" || !strings.Contains(q.Get("scope"), "write") {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	back := q.Get("return_url")
	if back == "" {
		http.Error(w, "missing return_url", http.StatusBadRequest)
		return
	}
	t.mu.Lock()
	deny := t.denyAuth
	t.mu.Unlock()
	if deny {
		http.Redirect(w, r, back, http.StatusFound)
		return
	}
	http.Redirect(w, r, back+"#token="+TrelloOAuthToken, http.StatusFound)
}
