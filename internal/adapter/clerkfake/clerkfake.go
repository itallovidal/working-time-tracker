// Package clerkfake é um Clerk em memória para os testes: implementa adapter.ClerkProvider sem rede nem
// assinatura de JWT. Os testes cadastram usuários, pedem um token por usuário e entregam o token ao servidor
// como o navegador faria.
package clerkfake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"working-time-tracker/internal/adapter"
)

// Invitation é um convite que o servidor pediu ao Clerk.
type Invitation struct {
	ID      string
	Params  adapter.ClerkInviteParams
	Revoked bool
}

// Fake é o Clerk falso. É seguro para uso concorrente.
type Fake struct {
	mu          sync.Mutex
	users       map[string]*adapter.ClerkIdentity
	tokens      map[string]string // token -> usuário
	invitations []*Invitation
	down        bool
	inviteErr   error
	seq         int
}

var _ adapter.ClerkProvider = (*Fake)(nil)

func New() *Fake {
	return &Fake{users: map[string]*adapter.ClerkIdentity{}, tokens: map[string]string{}}
}

// AddUser cadastra um usuário no Clerk. verified diz se o e-mail dele foi verificado.
func (f *Fake) AddUser(id, email, name string, verified bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[id] = &adapter.ClerkIdentity{UserID: id, Email: strings.ToLower(email), EmailVerified: verified, Name: name}
}

// DeleteUser apaga o usuário: os tokens dele deixam de valer e UserExists passa a dizer que não existe.
func (f *Fake) DeleteUser(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.users, id)
}

// Token emite um token de sessão para o usuário, como o clerk-js faria.
func (f *Fake) Token(userID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	tok := fmt.Sprintf("tok_%s_%d", userID, f.seq)
	f.tokens[tok] = userID
	return tok
}

// Expire faz o token deixar de valer.
func (f *Fake) Expire(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tokens, token)
}

// SetDown liga e desliga a indisponibilidade do Clerk: com ele fora do ar, toda chamada falha.
func (f *Fake) SetDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

// FailInvitations faz a criação de convites falhar com esse erro (nil volta ao normal).
func (f *Fake) FailInvitations(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inviteErr = err
}

// Invitations devolve os convites pedidos até agora, do primeiro ao último.
func (f *Fake) Invitations() []Invitation {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Invitation, len(f.invitations))
	for i, inv := range f.invitations {
		out[i] = *inv
	}
	return out
}

func (f *Fake) Verify(_ context.Context, sessionToken string) (*adapter.ClerkIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, fmt.Errorf("%w: fake is down", adapter.ErrClerkUnavailable)
	}
	uid, ok := f.tokens[sessionToken]
	if !ok {
		return nil, fmt.Errorf("%w: unknown or expired token", adapter.ErrClerkTokenInvalid)
	}
	u, ok := f.users[uid]
	if !ok {
		return nil, fmt.Errorf("%w: the user no longer exists", adapter.ErrClerkTokenInvalid)
	}
	cp := *u
	return &cp, nil
}

func (f *Fake) UserExists(_ context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return false, fmt.Errorf("%w: fake is down", adapter.ErrClerkUnavailable)
	}
	_, ok := f.users[userID]
	return ok, nil
}

func (f *Fake) CreateInvitation(_ context.Context, p adapter.ClerkInviteParams) (*adapter.ClerkInvitation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, fmt.Errorf("%w: fake is down", adapter.ErrClerkUnavailable)
	}
	if f.inviteErr != nil {
		return nil, f.inviteErr
	}
	f.seq++
	inv := &Invitation{ID: fmt.Sprintf("inv_%d", f.seq), Params: p}
	f.invitations = append(f.invitations, inv)
	return &adapter.ClerkInvitation{ID: inv.ID, URL: "https://clerk.test/v1/tickets/accept?ticket=" + inv.ID}, nil
}

func (f *Fake) RevokeInvitation(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return fmt.Errorf("%w: fake is down", adapter.ErrClerkUnavailable)
	}
	for _, inv := range f.invitations {
		if inv.ID == id {
			inv.Revoked = true
		}
	}
	return nil
}
