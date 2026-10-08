package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"working-time-tracker/testutil"
)

func trelloSyncFixture(t *testing.T) (*TrelloIntegration, Connection, *testutil.Trello) {
	t.Helper()
	fake := testutil.NewTrello()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	conn := Connection{Token: "trello-token", Metadata: map[string]any{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardID}}
	return &TrelloIntegration{BaseURL: srv.URL}, conn, fake
}

// lastWrite devolve o corpo JSON da última requisição que escreve.
func lastWrite(t *testing.T, fake *testutil.Trello, method string) map[string]any {
	t.Helper()
	reqs := fake.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Method == method {
			var body map[string]any
			if err := json.Unmarshal([]byte(reqs[i].Body), &body); err != nil {
				t.Fatalf("the %s body is not JSON: %q", method, reqs[i].Body)
			}
			return body
		}
	}
	t.Fatalf("no %s request was made", method)
	return nil
}

func TestTrelloSync_AccountAndBoards(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	if got, err := tr.Account(conn); err != nil || got != testutil.TrelloUsername {
		t.Errorf("account = %q, %v, want %q", got, err, testutil.TrelloUsername)
	}
	// Antes de existir um quadro só a chave do app é preciso.
	keyOnly := Connection{Token: conn.Token, Metadata: map[string]any{"api_key": testutil.TrelloKey}}
	boards, err := tr.ListRepositories(keyOnly)
	if err != nil {
		t.Fatalf("boards: %v", err)
	}
	want := []Repository{
		{ID: testutil.TrelloBoardID, FullName: "Acme / App", Private: true},
		{ID: testutil.TrelloOtherBoardID, FullName: "Outro quadro", Private: true},
	}
	if !reflect.DeepEqual(boards, want) {
		t.Errorf("boards = %+v, want %+v", boards, want)
	}
	fake.SetClosed(testutil.TrelloOtherBoardID, true)
	fake.SetVisible(testutil.TrelloBoardID, false)
	if boards, _ = tr.ListRepositories(keyOnly); len(boards) != 0 {
		t.Errorf("a closed board and a board the token cannot see must not be listed: %+v", boards)
	}

	bad := Connection{Token: testutil.InvalidToken, Metadata: keyOnly.Metadata}
	if _, err := tr.Account(bad); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("account with an invalid token: %v", err)
	}
	if _, err := tr.ListRepositories(bad); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("boards with an invalid token: %v", err)
	}
	if _, err := tr.Account(Connection{Token: conn.Token}); !errors.Is(err, ErrFieldRequired) {
		t.Errorf("account without the app key: %v", err)
	}
	if _, err := tr.Account(Connection{Token: conn.Token, Metadata: map[string]any{"api_key": "../x"}}); !errors.Is(err, ErrTrelloInvalidKey) {
		t.Errorf("account with a key that is not one: %v", err)
	}
	if _, err := tr.Account(Connection{Metadata: keyOnly.Metadata}); !errors.Is(err, ErrTokenRequired) {
		t.Errorf("account without a token: %v", err)
	}
	for _, req := range fake.Requests() {
		if strings.Contains(req.Query, "token") || strings.Contains(req.Query, "key=") {
			t.Errorf("credentials went in the URL: %+v", req)
		}
	}
}

func TestTrelloSync_Repo(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	repo, err := tr.Repo(ctx, conn)
	if err != nil || repo.FullName != "App" || !repo.CanPush || repo.Archived {
		t.Fatalf("repo = %+v, %v, want App, writable, open", repo, err)
	}
	fake.SetWrite(testutil.TrelloBoardID, false)
	if repo, _ = tr.Repo(ctx, conn); repo.CanPush {
		t.Error("an observer must not be able to push")
	}
	fake.SetClosed(testutil.TrelloBoardID, true)
	if repo, _ = tr.Repo(ctx, conn); !repo.Archived {
		t.Error("a closed board must say so")
	}

	cases := []struct {
		name string
		conn Connection
		want error
	}{
		{"unknown board", Connection{Token: conn.Token, Metadata: map[string]any{"api_key": testutil.TrelloKey, "board_id": "ZzZ999zz"}}, ErrTrelloBoardMissing},
		{"invalid token", Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}, ErrInvalidToken},
		{"wrong key", Connection{Token: conn.Token, Metadata: map[string]any{"api_key": "outrachave", "board_id": testutil.TrelloBoardID}}, ErrInvalidToken},
	}
	for _, c := range cases {
		if _, err := tr.Repo(ctx, c.conn); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	fake.SetVisible(testutil.TrelloBoardID, false)
	if _, err := tr.Repo(ctx, conn); !errors.Is(err, ErrTrelloNoAccessBoard) {
		t.Errorf("a board the token cannot see: %v", err)
	}
	fake.SetVisible(testutil.TrelloBoardID, true)
	fake.RevokeToken(conn.Token)
	if _, err := tr.Repo(ctx, conn); !errors.Is(err, ErrInvalidToken) || !StopsSync(err) {
		t.Errorf("a revoked token: %v, it must stop the round", err)
	}
}

func TestTrelloSync_ListIssues(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	bug := fake.AddLabel(testutil.TrelloBoardID, "Bug", "red")
	green := fake.AddLabel(testutil.TrelloBoardID, "", "green")
	nothing := fake.AddLabel(testutil.TrelloBoardID, "", "")
	due := time.Date(2026, 11, 3, 17, 30, 0, 0, time.UTC)
	born := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	fake.AddCard(testutil.TrelloCard{
		ShortLink: "Rich0001", Name: "Rico", Desc: "linha 1\nlinha 2", Labels: []string{bug, green, nothing}, Due: due, Created: born,
	})
	fake.AddCard(testutil.TrelloCard{ShortLink: "DueDone1", Name: "Feito sem arquivar", Due: due, DueComplete: true})
	fake.AddCard(testutil.TrelloCard{ShortLink: "NoDueDne", Name: "Sem data, marca solta", DueComplete: true})

	list, err := tr.ListIssues(ctx, conn, ListIssuesOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]Issue{}
	for _, i := range list.Issues {
		byID[i.ID] = i
	}
	if _, ok := byID["ArqUiv4d"]; ok {
		t.Error("the open listing must leave the archived card out")
	}
	if _, ok := byID["OutroQdr"]; ok {
		t.Error("a card from another board must not be listed")
	}
	rich := byID["Rich0001"]
	if rich.Title != "Rico" || rich.Body != "linha 1\nlinha 2" || rich.State != "open" || rich.URL != "https://trello.com/c/Rich0001" ||
		!reflect.DeepEqual(rich.Labels, []string{"Bug", "green"}) || !rich.Deadline.Equal(due) || !rich.CreatedAt.Equal(born) || rich.UpdatedAt.IsZero() {
		t.Errorf("rich card = %+v", rich)
	}
	if len(rich.Assignees) != 0 {
		t.Errorf("a card has no assignees here: %v", rich.Assignees)
	}
	// Cartão com a data de entrega concluída conta como fechado; a marca solta, sem data, não conta.
	if byID["DueDone1"].State != "closed" {
		t.Errorf("a completed due date must close the card, got %q", byID["DueDone1"].State)
	}
	if byID["NoDueDne"].State != "open" {
		t.Errorf("dueComplete with no due date must not close the card, got %q", byID["NoDueDne"].State)
	}
	if byID["H0TZyzbK"].Deadline.IsZero() != true || byID["H0TZyzbK"].ID != "H0TZyzbK" {
		t.Errorf("seed card = %+v", byID["H0TZyzbK"])
	}
	if list.ServerTime.IsZero() || time.Since(list.ServerTime) > time.Minute {
		t.Errorf("server time = %v, want the Date header", list.ServerTime)
	}

	all, err := tr.ListIssues(ctx, conn, ListIssuesOptions{State: "all"})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	found := false
	for _, i := range all.Issues {
		if i.ID == "ArqUiv4d" && i.State == "closed" {
			found = true
		}
	}
	if !found {
		t.Errorf("the full listing must bring the archived card: %+v", all.Issues)
	}
	if n := fake.Count("GET", "/boards/"+testutil.TrelloBoardID+"/cards"); n != 2 {
		t.Errorf("each listing is one request, got %d", n)
	}

	cases := []struct {
		name string
		conn Connection
		want error
	}{
		{"unknown board", Connection{Token: conn.Token, Metadata: map[string]any{"api_key": testutil.TrelloKey, "board_id": "ZzZ999zz"}}, ErrTrelloBoardMissing},
		{"invalid token", Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}, ErrInvalidToken},
	}
	for _, c := range cases {
		if _, err := tr.ListIssues(ctx, c.conn, ListIssuesOptions{}); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	fake.Fail("GET", "/boards/", 500, 1)
	if _, err := tr.ListIssues(ctx, conn, ListIssuesOptions{}); !errors.Is(err, ErrProviderStatus) {
		t.Errorf("a 500 from the listing: %v, want the platform status", err)
	}
	// Um quadro grande demais para uma resposta só é um erro da listagem, e não um quadro que sumiu.
	tooMany := &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"error":"API_TOO_MANY_CARDS_REQUESTED"}`))}
	if err := trelloCheck(tooMany, boardErrs); !errors.Is(err, ErrListTooLong) {
		t.Errorf("too many cards: %v, want a list too long", err)
	}
}

func TestTrelloSync_GetIssue(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	issue, err := tr.GetIssue(ctx, conn, "H0TZyzbK")
	if err != nil || issue.ID != "H0TZyzbK" || issue.Title != "Corrigir login" || issue.State != "open" {
		t.Fatalf("get = %+v, %v", issue, err)
	}
	if archived, _ := tr.GetIssue(ctx, conn, "ArqUiv4d"); archived == nil || archived.State != "closed" {
		t.Errorf("archived card = %+v", archived)
	}

	// O cartão apagado, o de outro quadro, o que foi para um quadro que o token não vê: é tudo "sumiu".
	other := fake.AddCard(testutil.TrelloCard{ShortLink: "Movido01", Name: "Vai mudar"})
	fake.AddCard(testutil.TrelloCard{ShortLink: "Apagado1", Name: "Vai sumir"})
	fake.MoveCard(other, testutil.TrelloOtherBoardID)
	fake.DeleteCard("Apagado1")
	fake.AddCard(testutil.TrelloCard{ShortLink: "Escondid", Name: "Escondido", Board: testutil.TrelloOtherBoardID})
	fake.SetVisible(testutil.TrelloOtherBoardID, false)
	for _, id := range []string{"OutroQdr", "Movido01", "Apagado1", "Escondid", "NaoExist"} {
		if _, err := tr.GetIssue(ctx, conn, id); !errors.Is(err, ErrIssueGone) {
			t.Errorf("get %s: err = %v, want gone", id, err)
		}
	}
	for _, id := range []string{"", "../boards/x", "a b"} {
		if _, err := tr.GetIssue(ctx, conn, id); !errors.Is(err, ErrTrelloInvalidCard) {
			t.Errorf("get %q: err = %v, want an invalid card", id, err)
		}
	}
	if _, err := tr.GetIssue(ctx, Connection{Token: testutil.InvalidToken, Metadata: conn.Metadata}, "H0TZyzbK"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("invalid token: %v", err)
	}
	// O link do cartão serve, depois de normalizado.
	if got := tr.NormalizeItemID(" https://trello.com/c/H0TZyzbK/12-corrigir-login "); got != "H0TZyzbK" {
		t.Errorf("NormalizeItemID(url) = %q", got)
	}
	if got := tr.NormalizeItemID(" H0TZyzbK "); got != "H0TZyzbK" {
		t.Errorf("NormalizeItemID(short link) = %q", got)
	}
}

func TestTrelloSync_UpdateIssue(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	board := testutil.TrelloBoardID
	bug := fake.AddLabel(board, "Bug", "red")
	green := fake.AddLabel(board, "", "green")
	ux := fake.AddLabel(board, "UX", "blue")
	dup := fake.AddLabel(board, "bug", "orange")
	due := time.Date(2026, 11, 3, 17, 30, 0, 0, time.UTC)
	id := fake.AddCard(testutil.TrelloCard{ShortLink: "Alvo0001", Name: "Alvo", Desc: "corpo", Labels: []string{bug, green}, Due: due})
	str := func(s string) *string { return &s }
	names := func(n ...string) *[]string { return &n }

	// Nome e descrição: um PUT só, com só o que mudou.
	fake.Reset()
	got, err := tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Title: str("Novo"), Body: str("outro corpo")})
	if err != nil || got.Title != "Novo" || got.Body != "outro corpo" {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if body := lastWrite(t, fake, "PUT"); !reflect.DeepEqual(body, map[string]any{"name": "Novo", "desc": "outro corpo"}) {
		t.Errorf("PUT body = %v, want only the fields that changed", body)
	}
	if fake.Count("GET", "/cards/") != 0 {
		t.Error("a title change needs no read of the card")
	}

	// Fechar um cartão com data de entrega: arquiva e marca como concluída.
	closed := "closed"
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{State: &closed}); err != nil || got.State != "closed" {
		t.Fatalf("close = %+v, %v", got, err)
	}
	if body := lastWrite(t, fake, "PUT"); !reflect.DeepEqual(body, map[string]any{"closed": true, "dueComplete": true}) {
		t.Errorf("close body = %v", body)
	}
	if c, _ := fake.Card(id); !c.Closed || !c.DueComplete {
		t.Errorf("fake card after closing = %+v", c)
	}
	// Reabrir desarquiva e desmarca a conclusão.
	open := "open"
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{State: &open}); err != nil || got.State != "open" {
		t.Fatalf("reopen = %+v, %v", got, err)
	}
	if c, _ := fake.Card(id); c.Closed || c.DueComplete {
		t.Errorf("fake card after reopening = %+v", c)
	}
	// Sem data de entrega, fechar só arquiva.
	bare := fake.AddCard(testutil.TrelloCard{ShortLink: "SemData1", Name: "Sem data"})
	if _, err = tr.UpdateIssue(ctx, conn, "SemData1", IssuePatch{State: &closed}); err != nil {
		t.Fatalf("close without a due date: %v", err)
	}
	if body := lastWrite(t, fake, "PUT"); !reflect.DeepEqual(body, map[string]any{"closed": true}) {
		t.Errorf("close body without a due date = %v", body)
	}
	if c, _ := fake.Card(bare); !c.Closed || c.DueComplete {
		t.Errorf("fake card = %+v", c)
	}
	// Fechar e dar a data no mesmo empurrão também marca a conclusão.
	when := time.Date(2026, 12, 1, 9, 0, 0, 123_000_000, time.FixedZone("BRT", -3*3600))
	if got, err = tr.UpdateIssue(ctx, conn, "SemData1", IssuePatch{Deadline: &when, State: &closed}); err != nil {
		t.Fatalf("close and date: %v", err)
	}
	if body := lastWrite(t, fake, "PUT"); body["due"] != "2026-12-01T12:00:00.123Z" || body["dueComplete"] != true {
		t.Errorf("close and date body = %v", body)
	}
	if !got.Deadline.Equal(when) || got.State != "closed" {
		t.Errorf("after closing with a date: %+v", got)
	}

	// A data: muda e limpa (o tempo zero).
	next := due.Add(48 * time.Hour)
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Deadline: &next}); err != nil || !got.Deadline.Equal(next) {
		t.Fatalf("date = %+v, %v", got, err)
	}
	var none time.Time
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Deadline: &none}); err != nil || !got.Deadline.IsZero() {
		t.Fatalf("clear date = %+v, %v", got, err)
	}
	if body := lastWrite(t, fake, "PUT"); body["due"] != nil || !hasKey(body, "due") {
		t.Errorf("clearing the date must send null, got %v", body)
	}

	// Etiquetas: as que o cartão já tem e continuam pedidas ficam, a nova vem do quadro (por nome sem
	// diferenciar caixa), a só com cor casa pela cor.
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Labels: names("bug", "GREEN", "ux")}); err != nil {
		t.Fatalf("labels: %v", err)
	}
	if !reflect.DeepEqual(got.Labels, []string{"Bug", "green", "UX"}) {
		t.Errorf("labels after the update = %v", got.Labels)
	}
	if c, _ := fake.Card(id); !reflect.DeepEqual(c.Labels, []string{bug, green, ux}) {
		t.Errorf("label ids = %v, want %v", c.Labels, []string{bug, green, ux})
	}
	// Duas etiquetas de mesmo nome no cartão continuam as duas se o nome continua pedido.
	fake.EditCard("Alvo0001", func(c *testutil.TrelloCard) { c.Labels = []string{bug, dup} })
	if _, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Labels: names("Bug")}); err != nil {
		t.Fatalf("dup labels: %v", err)
	}
	if c, _ := fake.Card(id); !reflect.DeepEqual(c.Labels, []string{bug, dup}) {
		t.Errorf("duplicates must be kept: %v", c.Labels)
	}
	// Uma lista vazia tira todas; um nome que o quadro não tem fica de fora, e o cartão devolvido mostra.
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Labels: names("fantasma")}); err != nil || len(got.Labels) != 0 {
		t.Errorf("an unknown label must be left out: %+v, %v", got, err)
	}
	fake.EditCard("Alvo0001", func(c *testutil.TrelloCard) { c.Labels = []string{bug} })
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Labels: names()}); err != nil || len(got.Labels) != 0 {
		t.Errorf("an empty list must clear the labels: %+v, %v", got, err)
	}

	// Nada a mudar não escreve.
	fake.Reset()
	if _, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{}); err != nil || fake.Writes() != 0 {
		t.Errorf("an empty patch: err = %v, writes = %d", err, fake.Writes())
	}

	// Defeitos: o que o Trello descarta sem avisar aparece no cartão devolvido, o observador é recusado, o
	// cartão apagado some, uma descrição grande demais é recusada.
	fake.Discard(board, "idLabels")
	if got, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Labels: names("UX")}); err != nil || len(got.Labels) != 0 {
		t.Errorf("a discarded label change must show in the returned card: %+v, %v", got, err)
	}
	fake.Discard(board)
	long := strings.Repeat("x", 16385)
	if _, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Body: &long}); !errors.Is(err, ErrProviderStatus) {
		t.Errorf("a description over the limit: %v, want the platform status", err)
	}
	fake.SetWrite(board, false)
	if _, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Title: str("x")}); !errors.Is(err, ErrForbidden) {
		t.Errorf("an observer: %v, want forbidden", err)
	}
	fake.SetWrite(board, true)
	fake.DeleteCard("Alvo0001")
	if _, err = tr.UpdateIssue(ctx, conn, "Alvo0001", IssuePatch{Title: str("x")}); !errors.Is(err, ErrIssueGone) {
		t.Errorf("a deleted card: %v, want gone", err)
	}
	if _, err = tr.UpdateIssue(ctx, conn, "../x", IssuePatch{Title: str("x")}); !errors.Is(err, ErrTrelloInvalidCard) {
		t.Errorf("an invalid id: %v", err)
	}
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }

func TestTrelloSync_CreateIssue(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	board := testutil.TrelloBoardID
	bug := fake.AddLabel(board, "Bug", "red")
	// A primeira lista é a de menor posição, não a primeira que o Trello devolve.
	first := fake.AddList(board, "Entrada", 1)
	due := time.Date(2026, 11, 3, 17, 30, 0, 0, time.UTC)

	created, err := tr.CreateIssue(ctx, conn, NewIssue{Title: "Nova", Body: "corpo", Labels: []string{"bug", "inexistente"}, Deadline: due})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Title != "Nova" || created.Body != "corpo" || !created.Deadline.Equal(due) || created.State != "open" ||
		!reflect.DeepEqual(created.Labels, []string{"Bug"}) || created.URL == "" || created.CreatedAt.IsZero() {
		t.Errorf("created = %+v", created)
	}
	card, ok := fake.Card(created.ID)
	if !ok || card.List != first || !reflect.DeepEqual(card.Labels, []string{bug}) {
		t.Errorf("fake card = %+v, want it on list %s with the label", card, first)
	}
	// Sem data e sem etiquetas, nada disso vai no corpo.
	if _, err = tr.CreateIssue(ctx, conn, NewIssue{Title: "Pelada"}); err != nil {
		t.Fatalf("create bare: %v", err)
	}
	body := lastWrite(t, fake, "POST")
	if hasKey(body, "due") || hasKey(body, "idLabels") || body["name"] != "Pelada" {
		t.Errorf("bare POST body = %v", body)
	}

	fake.SetWrite(board, false)
	if _, err = tr.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("an observer: %v, want forbidden", err)
	}
	fake.SetWrite(board, true)
	fake.ClearLists(board)
	if _, err = tr.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrTrelloNoList) {
		t.Errorf("a board with no list: %v, want trello_no_list", err)
	}
}

func TestTrelloSync_Labels(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	board := testutil.TrelloBoardID
	fake.AddLabel(board, "Bug", "red")
	fake.AddLabel(board, "", "green")
	fake.AddLabel(board, "", "")

	names, err := tr.ListLabels(ctx, conn)
	if err != nil || !reflect.DeepEqual(names, []string{"Bug", "green"}) {
		t.Fatalf("labels = %v, %v, want the name or, with none, the color (and nothing for a label with neither)", names, err)
	}
	if err := tr.CreateLabel(ctx, conn, "Urgente"); err != nil {
		t.Fatalf("create label: %v", err)
	}
	if err := tr.CreateLabel(ctx, conn, "urgente"); err != nil {
		t.Fatalf("create label again: %v", err)
	}
	var made []testutil.TrelloLabel
	for _, l := range fake.Labels(board) {
		if strings.EqualFold(l.Name, "urgente") {
			made = append(made, l)
		}
	}
	if len(made) != 2 || made[0].Color == "" || made[0].Color != made[1].Color {
		t.Errorf("created labels = %+v, want a color, the same for the same name", made)
	}
	// A criação pede o id do quadro, que se descobre quando o metadata tem o link curto.
	short := Connection{Token: conn.Token, Metadata: map[string]any{"api_key": testutil.TrelloKey, "board_id": testutil.TrelloBoardShortLink}}
	if err := tr.CreateLabel(ctx, short, "Pelo link"); err != nil {
		t.Errorf("create label by short link: %v", err)
	}
	if body := lastWrite(t, fake, "POST"); body["idBoard"] != board {
		t.Errorf("label POST body = %v, want the full board id", body)
	}
	fake.LabelCreateStatus(board, http.StatusUnauthorized)
	if err := tr.CreateLabel(ctx, conn, "Recusada"); !errors.Is(err, ErrForbidden) {
		t.Errorf("a label the token cannot create: %v, want forbidden", err)
	}
}

func TestTrelloSync_RateLimitAndRedirects(t *testing.T) {
	tr, conn, fake := trelloSyncFixture(t)
	ctx := context.Background()
	fake.RateLimitedFor(time.Minute)
	_, err := tr.ListIssues(ctx, conn, ListIssuesOptions{})
	until, limited := RateLimitedUntil(err)
	if !errors.Is(err, ErrRateLimited) || !limited || !StopsSync(err) || until.Before(time.Now()) {
		t.Fatalf("rate limited list: err = %v, until %v", err, until)
	}

	// Um redirecionamento não é seguido: o PUT viraria um GET, e o "200" não teria gravado nada.
	var followed atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed.Add(1)
			return
		}
		http.Redirect(w, r, "/elsewhere", http.StatusMovedPermanently)
	}))
	defer srv.Close()
	moved := &TrelloIntegration{BaseURL: srv.URL}
	title := "x"
	if _, err := moved.UpdateIssue(ctx, conn, "H0TZyzbK", IssuePatch{Title: &title}); err == nil {
		t.Error("a redirect must not look like a success")
	}
	if followed.Load() != 0 {
		t.Errorf("the redirect was followed %d times", followed.Load())
	}

	// A plataforma fora do ar acaba a rodada; o contexto cancelado, também.
	srv.Close()
	if _, err := moved.Repo(ctx, conn); !errors.Is(err, ErrProviderUnreachable) || !StopsSync(err) {
		t.Errorf("an unreachable platform: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := tr.ListIssues(canceled, conn, ListIssuesOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("a canceled context: %v", err)
	}
}

func TestTrello_NoEmailsAndHelpers(t *testing.T) {
	tr, conn, _ := trelloSyncFixture(t)
	ctx := context.Background()
	if email, err := tr.UserEmail(ctx, conn, "ana"); err != nil || email != "" {
		t.Errorf("UserEmail = %q, %v, want none", email, err)
	}
	if login, err := tr.FindLoginByEmail(ctx, conn, "ana@test.com"); err != nil || login != "" {
		t.Errorf("FindLoginByEmail = %q, %v, want none", login, err)
	}
	d := tr.Descriptor()
	if !d.Sync || !d.Caps.Deadline || !d.Caps.AutoPublish || d.Caps.Assignee || d.Caps.ServerSince || d.ItemNumeric {
		t.Errorf("trello descriptor = %+v", d)
	}
	born := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if got := trelloCreated("6a96be40" + "0000000000000001"); !got.Equal(born) {
		t.Errorf("trelloCreated = %v, want %v", got, born)
	}
	if !trelloCreated("zz").IsZero() || !trelloCreated("00000001"+"0000000000000001").IsZero() {
		t.Error("an id that is not a timestamp has no creation date")
	}
}
