package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client é um app que pode entrar: o client_id é a URL do documento de identidade dele (CIMD), e o resto é o que o
// documento diz. A lista embutida (ClaudeClients) também serve de cópia para quando a busca do documento falha.
type Client struct {
	ID           string
	Name         string
	RedirectURIs []string
}

// ClaudeClients são os apps aceitos: o Claude (claude.ai, e por ele o Claude Desktop e o celular) e o Claude Code. O
// conteúdo é o que os documentos publicados diziam em 2026-11-09; o servidor busca a versão atual e usa esta quando a
// busca falha.
func ClaudeClients() []Client {
	return []Client{
		{
			ID:           "https://claude.ai/oauth/mcp-oauth-client-metadata",
			Name:         "Claude",
			RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"},
		},
		{
			ID:           "https://claude.ai/oauth/claude-code-client-metadata",
			Name:         "Claude Code",
			RedirectURIs: []string{"http://localhost/callback", "http://127.0.0.1/callback"},
		},
	}
}

var (
	// errUnknownClient é um client_id fora da lista.
	errUnknownClient = errors.New("oauth: unknown client")
	// errClientDocument é um documento de identidade que não serve (ou que não veio, sem cópia embutida).
	errClientDocument = errors.New("oauth: client metadata document is invalid")
)

const (
	// O documento fica guardado pelo max-age do Cache-Control, dentro destes limites; sem max-age, vale uma hora.
	minClientTTL     = 5 * time.Minute
	maxClientTTL     = 24 * time.Hour
	defaultClientTTL = time.Hour
	// failedClientTTL é quanto se espera para buscar de novo depois de uma falha (vale a cópia anterior).
	failedClientTTL = 5 * time.Minute
	// maxClientDocument é o maior documento aceito.
	maxClientDocument = 64 << 10
)

// Registry resolve um client_id para o app: só os da lista, com o documento buscado e guardado por um tempo.
type Registry struct {
	pinned map[string]Client
	http   *http.Client
	logger *slog.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]cachedClient
}

type cachedClient struct {
	client  Client
	expires time.Time
}

// NewRegistry monta o registro com a lista de apps aceitos. httpClient nulo usa um com prazo de 5 segundos que não
// segue redirecionamentos (o documento mora no endereço do client_id, não em outro).
func NewRegistry(clients []Client, httpClient *http.Client, logger *slog.Logger) *Registry {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	pinned := make(map[string]Client, len(clients))
	for _, c := range clients {
		pinned[c.ID] = c
	}
	return &Registry{pinned: pinned, http: httpClient, logger: logger, now: time.Now, cache: map[string]cachedClient{}}
}

// Lookup devolve o app do client_id: o guardado, se ainda vale; senão o documento buscado agora; se a busca falha, o
// último que veio ou a cópia embutida. Um client_id fora da lista é errUnknownClient.
func (r *Registry) Lookup(ctx context.Context, id string) (Client, error) {
	pinned, ok := r.pinned[id]
	if !ok {
		return Client{}, errUnknownClient
	}
	now := r.now()
	r.mu.Lock()
	cached, hit := r.cache[id]
	r.mu.Unlock()
	if hit && now.Before(cached.expires) {
		return cached.client, nil
	}

	client, ttl, err := r.fetch(ctx, id)
	if err != nil {
		r.logger.Warn("oauth: client metadata document unavailable, using the last known copy", "client_id", id, "error", err)
		client = pinned
		if hit {
			client = cached.client
		}
		ttl = failedClientTTL
	}
	if client.Name == "" {
		client.Name = pinned.Name
	}
	r.mu.Lock()
	r.cache[id] = cachedClient{client: client, expires: now.Add(ttl)}
	r.mu.Unlock()
	return client, nil
}

// clientDocument são os campos do documento de identidade (RFC 7591 e o CIMD) que importam aqui.
type clientDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// fetch busca o documento de identidade e o confere: o client_id dele tem de ser o próprio endereço, o app tem de ser
// público (sem segredo) e os endereços de volta, https ou http no próprio computador.
func (r *Registry) fetch(ctx context.Context, id string) (Client, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return Client{}, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "working-time-tracker (MCP OAuth)")
	res, err := r.http.Do(req)
	if err != nil {
		return Client{}, 0, err
	}
	defer func() { _ = res.Body.Close() }() // só leitura: o erro do Close não muda nada
	if res.StatusCode != http.StatusOK {
		return Client{}, 0, fmt.Errorf("status %d", res.StatusCode)
	}
	if mt, _, err := mime.ParseMediaType(res.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return Client{}, 0, fmt.Errorf("content type %q", res.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxClientDocument+1))
	if err != nil {
		return Client{}, 0, err
	}
	if len(body) > maxClientDocument {
		return Client{}, 0, fmt.Errorf("document larger than %d bytes", maxClientDocument)
	}
	var doc clientDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return Client{}, 0, err
	}
	if err := validDocument(id, doc); err != nil {
		return Client{}, 0, err
	}
	return Client{ID: id, Name: strings.TrimSpace(doc.ClientName), RedirectURIs: doc.RedirectURIs}, cacheTTL(res.Header.Get("Cache-Control")), nil
}

func validDocument(id string, doc clientDocument) error {
	if doc.ClientID != id {
		return fmt.Errorf("%w: client_id %q is not the document URL", errClientDocument, doc.ClientID)
	}
	if doc.TokenEndpointAuthMethod != "none" {
		return fmt.Errorf("%w: token_endpoint_auth_method %q is not none", errClientDocument, doc.TokenEndpointAuthMethod)
	}
	if len(doc.ResponseTypes) > 0 && !slices.Contains(doc.ResponseTypes, "code") {
		return fmt.Errorf("%w: response_types has no code", errClientDocument)
	}
	if len(doc.RedirectURIs) == 0 {
		return fmt.Errorf("%w: no redirect_uris", errClientDocument)
	}
	for _, raw := range doc.RedirectURIs {
		if !validRedirect(raw) {
			return fmt.Errorf("%w: redirect_uri %q", errClientDocument, raw)
		}
	}
	return nil
}

// cacheTTL lê o max-age do Cache-Control, dentro dos limites. no-store e no-cache valem o mínimo.
func cacheTTL(header string) time.Duration {
	ttl := defaultClientTTL
	for part := range strings.SplitSeq(header, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		switch {
		case part == "no-store" || part == "no-cache":
			return minClientTTL
		case strings.HasPrefix(part, "max-age="):
			if n, err := strconv.Atoi(strings.TrimPrefix(part, "max-age=")); err == nil && n >= 0 {
				ttl = time.Duration(n) * time.Second
			}
		}
	}
	return min(max(ttl, minClientTTL), maxClientTTL)
}

// validRedirect diz se o endereço serve de volta: https, ou http no próprio computador, sem fragmento nem usuário.
func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "#") {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname()))
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// MatchRedirect diz se o redirect_uri pedido é um dos do app. A comparação é exata, menos no http do próprio
// computador (o Claude Code abre uma porta qualquer a cada vez): aí a porta não conta, mas o nome, o caminho e a query
// têm de ser os mesmos (RFC 8252, seção 7.3).
func MatchRedirect(registered []string, got string) bool {
	if !validRedirect(got) {
		return false
	}
	if slices.Contains(registered, got) {
		return true
	}
	g, err := url.Parse(got)
	if err != nil || g.Scheme != "http" || !isLoopback(g.Hostname()) {
		return false
	}
	for _, raw := range registered {
		r, err := url.Parse(raw)
		if err != nil || r.Scheme != "http" || !isLoopback(r.Hostname()) {
			continue
		}
		if r.Hostname() == g.Hostname() && r.EscapedPath() == g.EscapedPath() && r.RawQuery == g.RawQuery {
			return true
		}
	}
	return false
}
