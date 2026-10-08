package adapter

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A leitura e a escrita dos cartões de um quadro, para a sincronização com as tarefas (IssueSyncer).
// Cada cartão é um item; a chave dele é o link curto (o que vai na URL do cartão), e as listas ficam de
// fora por ora.

var (
	_ IssueSyncer      = (*TrelloIntegration)(nil)
	_ AccountLookup    = (*TrelloIntegration)(nil)
	_ RepositoryLister = (*TrelloIntegration)(nil)
	_ ItemNormalizer   = (*TrelloIntegration)(nil)
)

const (
	// trelloCardFields são os campos do cartão que a sincronização usa.
	trelloCardFields = "name,desc,closed,due,dueComplete,idList,idBoard,idLabels,labels,shortLink,shortUrl,dateLastActivity"
	// trelloDueLayout é como o Trello escreve (e aceita) uma data: UTC, com milissegundos.
	trelloDueLayout = "2006-01-02T15:04:05.000Z"
	trelloBodyLimit = 16 << 20
	// trelloRetry é quanto esperar quando o Trello recusa por limite e não diz até quando: a janela do limite é de 10 s.
	trelloRetry = 15 * time.Second
)

// trelloLabelColors são as cores de uma etiqueta criada por nós: o Trello pede uma, e a escolhida pelo nome
// faz a mesma etiqueta ter sempre a mesma cor.
var trelloLabelColors = []string{"green", "yellow", "orange", "red", "purple", "blue", "sky", "lime", "pink", "black"}

var trelloFullID = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

type trelloLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// trelloLabelName é como a etiqueta aparece para nós: o nome, ou a cor quando ela não tem nome (o Trello
// permite etiqueta só com cor).
func trelloLabelName(l trelloLabel) string {
	if n := strings.TrimSpace(l.Name); n != "" {
		return n
	}
	return strings.TrimSpace(l.Color)
}

// trelloCard é o cartão como a API o devolve.
type trelloCard struct {
	ID               string        `json:"id"`
	ShortLink        string        `json:"shortLink"`
	Name             string        `json:"name"`
	Desc             string        `json:"desc"`
	Closed           bool          `json:"closed"`
	Due              *string       `json:"due"`
	DueComplete      bool          `json:"dueComplete"`
	IDList           string        `json:"idList"`
	IDBoard          string        `json:"idBoard"`
	Labels           []trelloLabel `json:"labels"`
	ShortURL         string        `json:"shortUrl"`
	DateLastActivity string        `json:"dateLastActivity"`
	Board            *struct {
		ID        string `json:"id"`
		ShortLink string `json:"shortLink"`
	} `json:"board"`
}

// dueTime é a data de entrega do cartão, ou o tempo zero.
func (c trelloCard) dueTime() time.Time {
	if c.Due == nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, *c.Due)
	if err != nil {
		return time.Time{}
	}
	return t
}

// closed diz se o cartão conta como fechado: arquivado, ou com a data de entrega marcada como concluída.
// As duas fecham a tarefa (o cartão "feito" e esquecido sem arquivar também).
func (c trelloCard) closed() bool { return c.Closed || (c.Due != nil && c.DueComplete) }

// trelloCreated é o instante em que o cartão nasceu: os quatro primeiros bytes do id são um timestamp Unix.
func trelloCreated(id string) time.Time {
	if len(id) < 8 {
		return time.Time{}
	}
	n, err := strconv.ParseInt(id[:8], 16, 64)
	if err != nil || n < 946684800 { // antes de 2000 não é um timestamp
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

func (c trelloCard) toIssue() Issue {
	id := c.ShortLink
	if id == "" {
		id = c.ID
	}
	state := "open"
	if c.closed() {
		state = "closed"
	}
	out := Issue{
		ID: id, Title: c.Name, Body: c.Desc, State: state, URL: c.ShortURL,
		Labels: []string{}, Assignees: []string{},
		Deadline: c.dueTime(), CreatedAt: trelloCreated(c.ID),
	}
	if t, err := time.Parse(time.RFC3339, c.DateLastActivity); err == nil {
		out.UpdatedAt = t
	}
	for _, l := range c.Labels {
		if name := trelloLabelName(l); name != "" {
			out.Labels = append(out.Labels, name)
		}
	}
	return out
}

// sameBoard diz se o cartão é do quadro da integração, que pode estar escrito pelo id ou pelo link curto.
func (c trelloCard) sameBoard(board string) bool {
	if c.Board == nil && c.IDBoard == "" {
		return true
	}
	if c.Board != nil && (board == c.Board.ShortLink || strings.EqualFold(board, c.Board.ID)) {
		return true
	}
	return strings.EqualFold(board, c.IDBoard)
}

func trelloKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// trelloErrs diz que erro cada resposta de falha vira, conforme o que se pediu.
type trelloErrs struct {
	NotFound   error // 404
	BadRequest error // 400 (o id que o Trello não entende); nulo deixa o 400 como erro da plataforma
	Denied     error // 401 que não é de credencial: o token não enxerga o que se pediu (ou não escreve)
}

// trelloCheck devolve o erro de uma resposta que não é de sucesso e fecha o corpo; nil deixa a resposta
// aberta para quem chamou ler.
func trelloCheck(resp *http.Response, errs trelloErrs) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.ToLower(string(raw))
	if strings.Contains(message, "too_many_cards") {
		return ErrListTooLong.With("provider", "Trello")
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		// O Trello responde 401 para a credencial errada e também para o que o token não pode ver ou mudar;
		// o texto é o que diferencia.
		if strings.Contains(message, "invalid token") || strings.Contains(message, "invalid key") {
			return ErrInvalidToken.With("provider", "Trello")
		}
		if errs.Denied != nil {
			return errs.Denied
		}
		return ErrForbidden.With("provider", "Trello")
	case http.StatusTooManyRequests:
		until := time.Now().Add(trelloRetry)
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			until = time.Now().Add(time.Duration(s) * time.Second)
		}
		return ErrRateLimited.With("provider", "Trello", "until", until.Unix())
	case http.StatusNotFound:
		if errs.NotFound != nil {
			return errs.NotFound
		}
	case http.StatusBadRequest:
		if errs.BadRequest != nil {
			return errs.BadRequest
		}
	}
	return ErrProviderStatus.With("provider", "Trello", "status", resp.StatusCode)
}

// trelloDecode lê o corpo JSON e o fecha.
func trelloDecode(resp *http.Response, into any) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, trelloBodyLimit)).Decode(into); err != nil {
		return ErrUnexpectedResponse.With("provider", "Trello").Wrap(err)
	}
	return nil
}

// trelloKeyOnly lê só a chave do app do metadata: o que se pergunta sobre quem o token representa e sobre
// os quadros dele vale antes de existir um quadro.
func trelloKeyOnly(raw map[string]any) (string, error) {
	key, _ := raw["api_key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrFieldRequired.With("field", "api_key", "provider", "Trello")
	}
	if !trelloID.MatchString(key) {
		return "", ErrTrelloInvalidKey
	}
	return key, nil
}

// NormalizeItemID tira o link curto do cartão de uma URL colada; o resto fica como está.
func (t *TrelloIntegration) NormalizeItemID(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := trelloCardURL.FindStringSubmatch(raw); m != nil {
		return m[1]
	}
	return raw
}

// Account diz quem o token representa (o nome de usuário no Trello).
func (t *TrelloIntegration) Account(conn Connection) (string, error) {
	key, err := trelloKeyOnly(conn.Metadata)
	if err != nil {
		return "", err
	}
	resp, err := t.do(context.Background(), conn, key, "GET", "/members/me?fields=username", nil)
	if err != nil {
		return "", err
	}
	if err := trelloCheck(resp, trelloErrs{Denied: ErrInvalidToken.With("provider", "Trello")}); err != nil {
		return "", err
	}
	var me struct {
		Username string `json:"username"`
	}
	if err := trelloDecode(resp, &me); err != nil {
		return "", err
	}
	if me.Username == "" {
		return "", ErrUnexpectedResponse.With("provider", "Trello")
	}
	return me.Username, nil
}

// ListRepositories lista os quadros abertos que o token enxerga, com o espaço de trabalho de cada um, para
// a pessoa escolher em vez de colar o id. O ID é o que se guarda no metadata.
func (t *TrelloIntegration) ListRepositories(conn Connection) ([]Repository, error) {
	key, err := trelloKeyOnly(conn.Metadata)
	if err != nil {
		return nil, err
	}
	q := url.Values{"filter": {"open"}, "fields": {"name,shortLink,prefs"}, "organization": {"true"}, "organization_fields": {"displayName"}}
	resp, err := t.do(context.Background(), conn, key, "GET", withQuery("/members/me/boards", q), nil)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, trelloErrs{Denied: ErrInvalidToken.With("provider", "Trello")}); err != nil {
		return nil, err
	}
	var boards []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Prefs struct {
			PermissionLevel string `json:"permissionLevel"`
		} `json:"prefs"`
		Organization *struct {
			DisplayName string `json:"displayName"`
		} `json:"organization"`
	}
	if err := trelloDecode(resp, &boards); err != nil {
		return nil, err
	}
	out := make([]Repository, 0, len(boards))
	for _, b := range boards {
		name := b.Name
		if b.Organization != nil && strings.TrimSpace(b.Organization.DisplayName) != "" {
			name = strings.TrimSpace(b.Organization.DisplayName) + " / " + b.Name
		}
		out = append(out, Repository{ID: b.ID, FullName: name, Private: b.Prefs.PermissionLevel == "private"})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].FullName) < strings.ToLower(out[j].FullName) })
	return out, nil
}

// boardErrs são os erros de quem lê o quadro da integração.
var boardErrs = trelloErrs{NotFound: ErrTrelloBoardMissing, BadRequest: ErrTrelloBoardMissing, Denied: ErrTrelloNoAccessBoard}

// Repo lê o quadro: o nome e se o token escreve nele (é membro do quadro como administrador ou membro
// comum; observador não escreve).
func (t *TrelloIntegration) Repo(ctx context.Context, conn Connection) (*IssueRepo, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	q := url.Values{"fields": {"name,closed"}, "memberships": {"me"}}
	resp, err := t.do(ctx, conn, meta.APIKey, "GET", withQuery("/boards/"+meta.BoardID, q), nil)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, boardErrs); err != nil {
		return nil, err
	}
	var body struct {
		Name        string `json:"name"`
		Closed      bool   `json:"closed"`
		Memberships []struct {
			MemberType  string `json:"memberType"`
			Deactivated bool   `json:"deactivated"`
		} `json:"memberships"`
	}
	if err := trelloDecode(resp, &body); err != nil {
		return nil, err
	}
	repo := &IssueRepo{FullName: body.Name, Archived: body.Closed}
	for _, m := range body.Memberships {
		if !m.Deactivated && (m.MemberType == "admin" || m.MemberType == "normal") {
			repo.CanPush = true
		}
	}
	return repo, nil
}

// ListIssues traz os cartões abertos do quadro numa chamada só. O Trello não filtra por data, então Since
// só descarta, aqui, o que não mexeu depois dela.
func (t *TrelloIntegration) ListIssues(ctx context.Context, conn Connection, opts ListIssuesOptions) (*IssueList, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	filter := "open"
	if opts.State == "all" {
		filter = "all"
	}
	list := &IssueList{ServerTime: time.Now()}
	resp, err := t.do(ctx, conn, meta.APIKey, "GET", withQuery("/boards/"+meta.BoardID+"/cards/"+filter, url.Values{"fields": {trelloCardFields}}), nil)
	if err != nil {
		return list, err
	}
	if err := trelloCheck(resp, boardErrs); err != nil {
		return list, err
	}
	if when, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
		list.ServerTime = when
	}
	var cards []trelloCard
	if err := trelloDecode(resp, &cards); err != nil {
		return list, err
	}
	for _, c := range cards {
		issue := c.toIssue()
		if !opts.Since.IsZero() && !issue.UpdatedAt.IsZero() && issue.UpdatedAt.Before(opts.Since) {
			continue
		}
		list.Issues = append(list.Issues, issue)
	}
	return list, nil
}

// trelloCardID confere o id de um cartão antes de ele entrar na URL da API.
func trelloCardID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if !trelloID.MatchString(id) {
		return "", ErrTrelloInvalidCard
	}
	return id, nil
}

// fetchCard lê um cartão do quadro. O que sumiu, o que o token deixou de ver e o cartão que foi para outro
// quadro são o mesmo: ErrIssueGone.
func (t *TrelloIntegration) fetchCard(ctx context.Context, conn Connection, meta *trelloMetadata, id string) (*trelloCard, error) {
	card, err := trelloCardID(id)
	if err != nil {
		return nil, err
	}
	gone := ErrIssueGone.With("item", card)
	q := url.Values{"fields": {trelloCardFields}, "board": {"true"}, "board_fields": {"shortLink"}}
	resp, err := t.do(ctx, conn, meta.APIKey, "GET", withQuery("/cards/"+card, q), nil)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, trelloErrs{NotFound: gone, BadRequest: gone, Denied: gone}); err != nil {
		return nil, err
	}
	var body trelloCard
	if err := trelloDecode(resp, &body); err != nil {
		return nil, err
	}
	if !body.sameBoard(meta.BoardID) {
		return nil, gone
	}
	return &body, nil
}

func (t *TrelloIntegration) GetIssue(ctx context.Context, conn Connection, id string) (*Issue, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	card, err := t.fetchCard(ctx, conn, meta, id)
	if err != nil {
		return nil, err
	}
	issue := card.toIssue()
	return &issue, nil
}

// boardLabels lê as etiquetas do quadro.
func (t *TrelloIntegration) boardLabels(ctx context.Context, conn Connection, meta *trelloMetadata) ([]trelloLabel, error) {
	q := url.Values{"fields": {"name,color"}, "limit": {"1000"}}
	resp, err := t.do(ctx, conn, meta.APIKey, "GET", withQuery("/boards/"+meta.BoardID+"/labels", q), nil)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, boardErrs); err != nil {
		return nil, err
	}
	var labels []trelloLabel
	if err := trelloDecode(resp, &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// findBoardLabel acha no quadro a etiqueta que um nome pede: o nome igual, o nome sem diferenciar caixa, ou
// a etiqueta só com cor cuja cor é o nome.
func findBoardLabel(board []trelloLabel, name string) string {
	name = strings.TrimSpace(name)
	for _, l := range board {
		if l.Name != "" && l.Name == name {
			return l.ID
		}
	}
	for _, l := range board {
		if l.Name != "" && strings.EqualFold(l.Name, name) {
			return l.ID
		}
	}
	for _, l := range board {
		if l.Name == "" && strings.EqualFold(l.Color, name) {
			return l.ID
		}
	}
	return ""
}

// trelloLabelIDs traduz os nomes das etiquetas que o cartão deve ter nos ids do Trello. As que o cartão já
// tem e continuam pedidas ficam como estão (inclusive duas de mesmo nome); as outras vêm do quadro. Um nome
// que o quadro não tem fica de fora: quem chamou criou as que faltavam, e o que não foi criado aparece no
// cartão devolvido como um descarte.
func trelloLabelIDs(attached, board []trelloLabel, want []string) []string {
	wanted := map[string]bool{}
	for _, w := range want {
		if k := trelloKey(w); k != "" {
			wanted[k] = true
		}
	}
	ids := []string{}
	seen := map[string]bool{}
	covered := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, l := range attached {
		if k := trelloKey(trelloLabelName(l)); wanted[k] {
			add(l.ID)
			covered[k] = true
		}
	}
	for _, w := range want {
		if k := trelloKey(w); k != "" && !covered[k] {
			if id := findBoardLabel(board, w); id != "" {
				add(id)
				covered[k] = true
			}
		}
	}
	return ids
}

// UpdateIssue muda o cartão com um PUT só, com os campos que mudaram. Fechar arquiva o cartão e, se ele tem
// data de entrega, a marca como concluída; reabrir faz o contrário.
func (t *TrelloIntegration) UpdateIssue(ctx context.Context, conn Connection, id string, patch IssuePatch) (*Issue, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	card, err := trelloCardID(id)
	if err != nil {
		return nil, err
	}
	// Fechar precisa saber se há data de entrega, e as etiquetas, quais o cartão já tem.
	var current *trelloCard
	if patch.Labels != nil || patch.State != nil {
		if current, err = t.fetchCard(ctx, conn, meta, card); err != nil {
			return nil, err
		}
	}

	body := map[string]any{}
	if patch.Title != nil {
		body["name"] = *patch.Title
	}
	if patch.Body != nil {
		body["desc"] = *patch.Body
	}
	hasDue := current != nil && current.Due != nil
	if patch.Deadline != nil {
		if patch.Deadline.IsZero() {
			body["due"] = nil
			hasDue = false
		} else {
			body["due"] = patch.Deadline.UTC().Format(trelloDueLayout)
			hasDue = true
		}
	}
	if patch.State != nil {
		if *patch.State == "closed" {
			body["closed"] = true
			if hasDue {
				body["dueComplete"] = true
			}
		} else {
			body["closed"] = false
			body["dueComplete"] = false
		}
	}
	if patch.Labels != nil {
		board, err := t.boardLabels(ctx, conn, meta)
		if err != nil {
			return nil, err
		}
		body["idLabels"] = strings.Join(trelloLabelIDs(current.Labels, board, *patch.Labels), ",")
	}
	if len(body) == 0 {
		return t.GetIssue(ctx, conn, card)
	}

	gone := ErrIssueGone.With("item", card)
	resp, err := t.do(ctx, conn, meta.APIKey, "PUT", "/cards/"+card, body)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, trelloErrs{NotFound: gone, Denied: ErrForbidden.With("provider", "Trello")}); err != nil {
		return nil, err
	}
	var updated trelloCard
	if err := trelloDecode(resp, &updated); err != nil {
		return nil, err
	}
	issue := updated.toIssue()
	return &issue, nil
}

// CreateIssue cria um cartão na primeira lista aberta do quadro (a de menor posição): o Trello só cria
// cartão numa lista, e a escolha da lista fica para quando as listas entrarem na sincronização. O que o
// Trello não grava (uma etiqueta que o quadro não tem) aparece no cartão devolvido.
func (t *TrelloIntegration) CreateIssue(ctx context.Context, conn Connection, in NewIssue) (*Issue, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	resp, err := t.do(ctx, conn, meta.APIKey, "GET", withQuery("/boards/"+meta.BoardID+"/lists", url.Values{"filter": {"open"}, "fields": {"name,pos"}}), nil)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, boardErrs); err != nil {
		return nil, err
	}
	var lists []struct {
		ID  string  `json:"id"`
		Pos float64 `json:"pos"`
	}
	if err := trelloDecode(resp, &lists); err != nil {
		return nil, err
	}
	if len(lists) == 0 {
		return nil, ErrTrelloNoList
	}
	first := lists[0]
	for _, l := range lists[1:] {
		if l.Pos < first.Pos {
			first = l
		}
	}

	body := map[string]any{"idList": first.ID, "name": in.Title, "desc": in.Body}
	if !in.Deadline.IsZero() {
		body["due"] = in.Deadline.UTC().Format(trelloDueLayout)
	}
	if len(in.Labels) > 0 {
		board, err := t.boardLabels(ctx, conn, meta)
		if err != nil {
			return nil, err
		}
		if ids := trelloLabelIDs(nil, board, in.Labels); len(ids) > 0 {
			body["idLabels"] = strings.Join(ids, ",")
		}
	}
	resp, err = t.do(ctx, conn, meta.APIKey, "POST", "/cards", body)
	if err != nil {
		return nil, err
	}
	if err := trelloCheck(resp, trelloErrs{NotFound: ErrTrelloBoardMissing, Denied: ErrForbidden.With("provider", "Trello")}); err != nil {
		return nil, err
	}
	var created trelloCard
	if err := trelloDecode(resp, &created); err != nil {
		return nil, err
	}
	issue := created.toIssue()
	return &issue, nil
}

// ListLabels lista as etiquetas do quadro pelo nome (ou, na que só tem cor, pela cor).
func (t *TrelloIntegration) ListLabels(ctx context.Context, conn Connection) ([]string, error) {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	labels, err := t.boardLabels(ctx, conn, meta)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range labels {
		if name := trelloLabelName(l); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// boardID devolve o id completo do quadro: criar uma etiqueta pede o id, e o metadata pode ter o link curto.
func (t *TrelloIntegration) boardID(ctx context.Context, conn Connection, meta *trelloMetadata) (string, error) {
	if trelloFullID.MatchString(meta.BoardID) {
		return meta.BoardID, nil
	}
	resp, err := t.do(ctx, conn, meta.APIKey, "GET", withQuery("/boards/"+meta.BoardID, url.Values{"fields": {"id"}}), nil)
	if err != nil {
		return "", err
	}
	if err := trelloCheck(resp, boardErrs); err != nil {
		return "", err
	}
	var board struct {
		ID string `json:"id"`
	}
	if err := trelloDecode(resp, &board); err != nil {
		return "", err
	}
	return board.ID, nil
}

// CreateLabel cria a etiqueta no quadro, com uma cor tirada do nome.
func (t *TrelloIntegration) CreateLabel(ctx context.Context, conn Connection, name string) error {
	meta, err := parseTrelloMetadata(conn.Metadata)
	if err != nil {
		return err
	}
	board, err := t.boardID(ctx, conn, meta)
	if err != nil {
		return err
	}
	sum := fnv.New32a()
	sum.Write([]byte(trelloKey(name)))
	color := trelloLabelColors[int(sum.Sum32())%len(trelloLabelColors)]
	resp, err := t.do(ctx, conn, meta.APIKey, "POST", "/labels", map[string]any{"name": name, "color": color, "idBoard": board})
	if err != nil {
		return err
	}
	if err := trelloCheck(resp, trelloErrs{NotFound: ErrTrelloBoardMissing, Denied: ErrForbidden.With("provider", "Trello")}); err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// O Trello não mostra o e-mail dos membros, então o responsável não se liga a ninguém daqui.
func (t *TrelloIntegration) UserEmail(context.Context, Connection, string) (string, error) {
	return "", nil
}

func (t *TrelloIntegration) FindLoginByEmail(context.Context, Connection, string) (string, error) {
	return "", nil
}
