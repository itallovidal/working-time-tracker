package issuesync

import (
	"strings"
	"sync"
	"time"
)

// cache guarda em memória o que a sincronização pergunta ao GitHub e quase não muda: o e-mail público
// de um usuário e o login de um e-mail. A pessoa daqui nunca entra nele — ela é sempre lida do banco —,
// e o que não se acha também é guardado (com um prazo menor), senão cada rodada repetiria a pergunta
// sobre quem não publica e-mail, e a busca de usuários do GitHub aceita só 30 por minuto.
type cache struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[string]cacheItem
}

type cacheItem struct {
	value string
	until time.Time
}

const (
	cacheFound   = 30 * time.Minute
	cacheMissing = 10 * time.Minute
	// cacheRepo é quanto a rodada do gancho confia no que a última viu do repositório (permissão de escrita).
	cacheRepo = 5 * time.Minute
)

func newCache(now func() time.Time) *cache {
	return &cache{now: now, items: map[string]cacheItem{}}
}

// get devolve o valor guardado ou, vencido ou ausente, o que load devolve. Um erro não é guardado.
// Um valor achado vale found; o vazio (não achei) vale missing.
func (c *cache) get(key string, found, missing time.Duration, load func() (string, error)) (string, error) {
	c.mu.Lock()
	item, ok := c.items[key]
	c.mu.Unlock()
	if ok && c.now().Before(item.until) {
		return item.value, nil
	}
	value, err := load()
	if err != nil {
		return "", err
	}
	ttl := missing
	if value != "" {
		ttl = found
	}
	c.put(key, value, ttl)
	return value, nil
}

func (c *cache) put(key, value string, ttl time.Duration) {
	c.mu.Lock()
	c.items[key] = cacheItem{value: value, until: c.now().Add(ttl)}
	c.mu.Unlock()
}

// forget esquece tudo o que é de um prefixo (a integração): a rodada que a pessoa pediu pelo botão
// pergunta tudo de novo, para ela ver o efeito de ter mudado o e-mail público no GitHub.
func (c *cache) forget(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.items {
		if strings.HasPrefix(k, prefix) {
			delete(c.items, k)
		}
	}
}
