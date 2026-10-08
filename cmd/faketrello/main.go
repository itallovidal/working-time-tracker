// Command faketrello é o Trello falso dos testes (testutil.Trello) como servidor de desenvolvimento, para
// ver a conexão e a sincronização dos cartões numa instância do sistema sem tocar no Trello de verdade.
// Aponte o servidor para ele com TRELLO_URL (o site) e TRELLO_API_URL (a API, com o /1 no fim), e use em
// TRELLO_API_KEY a chave de testutil (0123456789abcdef0123456789abcdef). Atende o site (a tela de
// autorização, que já volta autorizada) e a API no mesmo endereço, e tem um painel de controle em /_fake
// para mexer nos cartões como se fosse outra pessoa no site do Trello.
//
//	go run ./cmd/faketrello -addr :8092
//
// Painel (JSON), sobre o quadro "App" (os cartões se acham pelo id ou pelo link curto):
//
//	GET  /_fake/cards                      os cartões do quadro, os arquivados também
//	POST /_fake/cards                      {"name","desc","labels","due","dueComplete","closed"}: cria um cartão
//	POST /_fake/cards/{id}                 os mesmos campos: edita; só o que vier
//	POST /_fake/cards/{id}/delete          apaga o cartão
//	POST /_fake/cards/{id}/move            {"board":"<id>"}: leva o cartão para outro quadro
//	POST /_fake/write                      {"write":false}: tira ou devolve a permissão de escrita no quadro
//	POST /_fake/revoke                     {"token":"..."}: revoga um token
//	POST /_fake/deny                       {"deny":true}: a próxima autorização volta sem token
//	GET  /_fake/requests                   o que o servidor pediu
//
// "due" é uma data RFC 3339 ("2026-11-03T17:30:00Z") ou vazia, para tirar a data; "labels" são nomes (o
// painel cria no quadro as que faltam).
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"working-time-tracker/testutil"
)

const board = testutil.TrelloBoardID

func main() {
	addr := flag.String("addr", ":8092", "endereço do servidor")
	flag.Parse()

	fake := testutil.NewTrello()
	seed(fake)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_fake/cards", func(w http.ResponseWriter, r *http.Request) { write(w, fake.Cards(board)) })
	mux.HandleFunc("POST /_fake/cards", func(w http.ResponseWriter, r *http.Request) {
		var in patch
		json.NewDecoder(r.Body).Decode(&in)
		c := testutil.TrelloCard{}
		in.apply(fake, &c)
		write(w, map[string]string{"id": fake.AddCard(c), "note": "use o link curto no painel"})
	})
	mux.HandleFunc("POST /_fake/cards/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := fake.Card(id); !ok {
			http.Error(w, "no such card", http.StatusNotFound)
			return
		}
		var in patch
		json.NewDecoder(r.Body).Decode(&in)
		fake.EditCard(id, func(c *testutil.TrelloCard) { in.apply(fake, c) })
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/cards/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		fake.DeleteCard(r.PathValue("id"))
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/cards/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Board string }
		json.NewDecoder(r.Body).Decode(&in)
		if in.Board == "" {
			in.Board = testutil.TrelloOtherBoardID
		}
		if _, ok := fake.Card(r.PathValue("id")); !ok {
			http.Error(w, "no such card", http.StatusNotFound)
			return
		}
		fake.MoveCard(r.PathValue("id"), in.Board)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/write", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Write bool }
		json.NewDecoder(r.Body).Decode(&in)
		fake.SetWrite(board, in.Write)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/revoke", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Token string }
		json.NewDecoder(r.Body).Decode(&in)
		fake.RevokeToken(in.Token)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/deny", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Deny bool }
		json.NewDecoder(r.Body).Decode(&in)
		fake.DenyAuthorization(in.Deny)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /_fake/requests", func(w http.ResponseWriter, r *http.Request) { write(w, fake.Requests()) })
	mux.Handle("/", fake)

	log.Printf("fake Trello on %s (board %s, API key %s, token %s)", *addr, testutil.TrelloBoardShortLink, testutil.TrelloKey, testutil.TrelloOAuthToken)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// patch é o que o painel aceita num cartão; o que não vem fica como está.
type patch struct {
	Name        *string   `json:"name"`
	Desc        *string   `json:"desc"`
	Closed      *bool     `json:"closed"`
	Due         *string   `json:"due"`
	DueComplete *bool     `json:"dueComplete"`
	Labels      *[]string `json:"labels"`
}

func (p patch) apply(fake *testutil.Trello, c *testutil.TrelloCard) {
	if p.Name != nil {
		c.Name = *p.Name
	}
	if p.Desc != nil {
		c.Desc = *p.Desc
	}
	if p.Closed != nil {
		c.Closed = *p.Closed
	}
	if p.DueComplete != nil {
		c.DueComplete = *p.DueComplete
	}
	if p.Due != nil {
		c.Due = time.Time{}
		if t, err := time.Parse(time.RFC3339, *p.Due); err == nil {
			c.Due = t.UTC()
		}
	}
	if p.Labels != nil {
		c.Labels = nil
		for _, name := range *p.Labels {
			c.Labels = append(c.Labels, labelID(fake, name))
		}
	}
}

// labelID acha a etiqueta do quadro pelo nome (ou, na que só tem cor, pela cor) e cria a que falta.
func labelID(fake *testutil.Trello, name string) string {
	for _, l := range fake.Labels(board) {
		if strings.EqualFold(l.Name, name) || (l.Name == "" && strings.EqualFold(l.Color, name)) {
			return l.ID
		}
	}
	return fake.AddLabel(board, name, "sky")
}

func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// seed deixa o quadro com cartões de vários jeitos: com e sem data, com Markdown, com etiqueta só de cor,
// atrasado, com a data concluída sem estar arquivado, e arquivado. As datas de criação são de semanas atrás.
func seed(fake *testutil.Trello) {
	fake.DeleteCard("H0TZyzbK")
	fake.DeleteCard("ArqUiv4d")
	fake.AddList(board, "A fazer", 1)
	bug := fake.AddLabel(board, "bug", "red")
	feature := fake.AddLabel(board, "feature", "green")
	ux := fake.AddLabel(board, "ux", "blue")
	soColor := fake.AddLabel(board, "", "purple")

	now := time.Now().UTC().Truncate(time.Hour)
	ago := func(days int) time.Time { return now.AddDate(0, 0, -days) }
	ahead := func(days int) time.Time { return now.AddDate(0, 0, days) }
	add := func(c testutil.TrelloCard) { fake.AddCard(c) }
	add(testutil.TrelloCard{Name: "Corrigir o login com Google", Desc: "## O que acontece\n\nO botão **Entrar com Google** volta para a tela inicial.\n\n- reproduzir no Chrome\n- conferir o redirect", Labels: []string{bug}, Due: ahead(4), Created: ago(21)})
	add(testutil.TrelloCard{Name: "Relatório mensal em PDF", Desc: "Exportar o relatório do mês em PDF.", Labels: []string{feature, ux}, Due: ahead(15), Created: ago(14)})
	add(testutil.TrelloCard{Name: "Revisar textos do onboarding", Labels: []string{ux, soColor}, Created: ago(9)})
	add(testutil.TrelloCard{Name: "Cache da lista de projetos", Desc: "A lista demora com muitos projetos.", Due: ago(3), Created: ago(30)})
	add(testutil.TrelloCard{Name: "Convite de pessoa de fora", Created: ago(2)})
	add(testutil.TrelloCard{Name: "Atualizar dependências", Due: ago(1), DueComplete: true, Created: ago(12)})
	add(testutil.TrelloCard{Name: "Migrar para o novo provedor de e-mail", Closed: true, Labels: []string{feature}, Created: ago(40)})
}
