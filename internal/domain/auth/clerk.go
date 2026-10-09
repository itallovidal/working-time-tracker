package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/person"
)

// ClerkSettings são os ajustes que o convite por e-mail pelo Clerk precisa.
type ClerkSettings struct {
	// PublicURL é o endereço do sistema, sem barra no fim: o Clerk leva a pessoa a PublicURL/invite/<token>.
	PublicURL string
	// InviteDelivery é DeliveryEmail (o Clerk manda o e-mail) ou DeliveryTerminal (não manda: o link vai para o
	// log, para desenvolver). Vazio vale e-mail.
	InviteDelivery string
	// Log recebe o link do convite no modo terminal e os avisos de falhas que não derrubam o pedido (um convite
	// que não deu para cancelar no Clerk). Nulo descarta.
	Log func(msg string, args ...any)
}

// SetClerk liga o login pelo Clerk e o convite por e-mail. Sem isso (o padrão), as rotas do Clerk respondem que
// ele está desligado, o login segue só por e-mail e senha e o convite é só o link.
func (s *Service) SetClerk(p adapter.ClerkProvider, settings ClerkSettings) {
	s.clerk = p
	s.clerkSettings = settings
}

func (s *Service) log(msg string, args ...any) {
	if s.clerkSettings.Log != nil {
		s.clerkSettings.Log(msg, args...)
	}
}

// ClerkEnabled diz se o login pelo Clerk está ligado.
func (s *Service) ClerkEnabled() bool { return s.clerk != nil }

// Os estados de um login pelo Clerk.
const (
	// ClerkOK: a pessoa entrou, e Identity e Token dizem quem e com que sessão.
	ClerkOK = "ok"
	// ClerkNeedsPassword: já existe uma conta com esse e-mail e senha, e ela precisa da senha para ser ligada ao
	// Clerk (uma vez só). O cadastro por senha não verifica o e-mail; sem essa prova, quem cadastrou o e-mail de
	// outra pessoa com uma senha dele ficaria com a conta quando a dona entrasse pelo Clerk.
	ClerkNeedsPassword = "needs_password"
	// ClerkNoAccount: o Clerk sabe quem é, mas não há conta aqui. Invites traz os convites pendentes para o
	// e-mail dela; nada é aceito sozinho, porque entrar numa organização não se desfaz.
	ClerkNoAccount = "no_account"
)

// ClerkInviteOption é um convite pendente que a pessoa pode aceitar.
type ClerkInviteOption struct {
	ID               uuid.UUID `json:"id"`
	OrganizationName string    `json:"organization_name"`
	Role             string    `json:"role"`
}

// ClerkResult é a resposta de um login, cadastro ou aceite pelo Clerk.
type ClerkResult struct {
	Status   string              `json:"status"`
	Identity *Identity           `json:"identity,omitempty"`
	Email    string              `json:"email,omitempty"`
	Name     string              `json:"name,omitempty"`
	Invites  []ClerkInviteOption `json:"invites,omitempty"`

	// Token é o da sessão que abriu (para o cookie); vazio quando não houve sessão. Created diz que a conta
	// nasceu agora.
	Token   string `json:"-"`
	Created bool   `json:"-"`
}

type ClerkLoginInput struct {
	// InviteToken é o do link do convite: a pessoa que não tem conta entra na organização dele.
	InviteToken string `json:"invite_token"`
	// Password é a senha da conta que já existe, para ligá-la ao Clerk.
	Password string `json:"password"`
}

type ClerkSignupInput struct {
	OrganizationName string `json:"organization_name"`
	Name             string `json:"name"`
	// Country é o código do país da organização (BR, US); vazio é o país padrão.
	Country string `json:"country"`
}

type ClerkJoinInput struct {
	InviteID string `json:"invite_id"`
	Name     string `json:"name"`
}

// clerkIdentity confere o token do Clerk e devolve quem ele diz que é e o e-mail normalizado. Só um e-mail
// verificado vale: é ele que prova que a pessoa é dona da caixa.
func (s *Service) clerkIdentity(ctx context.Context, sessionToken string) (*adapter.ClerkIdentity, string, error) {
	if s.clerk == nil {
		return nil, "", ErrClerkDisabled
	}
	ident, err := s.clerk.Verify(ctx, sessionToken)
	switch {
	case err == nil:
	case errors.Is(err, adapter.ErrClerkTokenInvalid):
		return nil, "", ErrClerkTokenInvalid.Wrap(err)
	default:
		return nil, "", ErrClerkUnavailable.Wrap(err)
	}
	if !ident.EmailVerified {
		return nil, "", ErrClerkEmailUnverified
	}
	email := person.NormalizeEmail(ident.Email)
	if !person.ValidEmail(email) {
		return nil, "", person.ErrInvalidEmail
	}
	return ident, email, nil
}

// ClerkLogin entra pelo Clerk: pelo usuário já ligado, ligando uma conta antiga pelo e-mail verificado ou,
// com o token de um convite, criando a conta na organização dele.
func (s *Service) ClerkLogin(ctx context.Context, sessionToken string, in ClerkLoginInput) (*ClerkResult, error) {
	ident, email, err := s.clerkIdentity(ctx, sessionToken)
	if err != nil {
		return nil, err
	}

	p, err := s.store.PersonByClerkID(ident.UserID)
	if err == nil {
		return s.clerkSession(p)
	}
	if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	p, err = s.store.PersonForLogin(email)
	switch {
	case err == nil:
		// Quem já é membro de uma organização não entra em outra com um convite: o e-mail é de uma conta só.
		if in.InviteToken != "" {
			if inv, ierr := s.store.ValidInvite(HashToken(in.InviteToken), s.now()); ierr == nil && inv.OrganizationID != p.OrganizationID {
				return nil, ErrAccountExists
			}
		}
		return s.clerkLinkByEmail(ctx, p, ident, in.Password)
	case !errors.Is(err, database.ErrNotFound):
		return nil, err
	}

	if in.InviteToken != "" {
		inv, err := s.store.ValidInvite(HashToken(in.InviteToken), s.now())
		if err != nil {
			return nil, err
		}
		if inv.Email != nil && *inv.Email != email {
			return nil, ErrInviteEmailMismatch
		}
		return s.clerkJoin(ctx, ident, email, inv, "")
	}
	return s.clerkNoAccount(ident, email)
}

// clerkLinkByEmail liga ao usuário do Clerk a conta que já existe com o e-mail verificado dele.
func (s *Service) clerkLinkByEmail(ctx context.Context, p *ent.Person, ident *adapter.ClerkIdentity, password string) (*ClerkResult, error) {
	// Uma conta ligada a outro usuário do Clerk só é religada se esse outro já não existe lá (foi apagado e a
	// pessoa voltou com o mesmo e-mail); se existe, não se toma a conta dele.
	replacing := ""
	if p.ClerkUserID != nil {
		exists, err := s.clerk.UserExists(ctx, *p.ClerkUserID)
		if err != nil {
			return nil, ErrClerkUnavailable.Wrap(err)
		}
		if exists {
			return nil, ErrClerkAccountLinked
		}
		replacing = *p.ClerkUserID
	}
	if p.PasswordHash != nil {
		if password == "" {
			return &ClerkResult{Status: ClerkNeedsPassword, Email: p.Email}, nil
		}
		if !CheckPassword(*p.PasswordHash, password) {
			return nil, ErrInvalidCredentials
		}
	}
	linked, err := s.store.LinkClerk(p.ID, ident.UserID, replacing)
	if err != nil {
		return nil, err
	}
	if !linked {
		// Outra requisição ligou a conta no meio tempo.
		return nil, ErrClerkAccountLinked
	}
	return s.clerkSession(p)
}

// clerkSession abre a sessão da pessoa que o Clerk identificou.
func (s *Service) clerkSession(p *ent.Person) (*ClerkResult, error) {
	id, token, err := s.openSession(p)
	if err != nil {
		return nil, err
	}
	return &ClerkResult{Status: ClerkOK, Identity: id, Token: token}, nil
}

// clerkNoAccount diz que não há conta, com os convites que a pessoa pode aceitar.
func (s *Service) clerkNoAccount(ident *adapter.ClerkIdentity, email string) (*ClerkResult, error) {
	invites, err := s.store.PendingInvitesForEmail(email, s.now())
	if err != nil {
		return nil, err
	}
	res := &ClerkResult{Status: ClerkNoAccount, Email: email, Name: ident.Name}
	for _, inv := range invites {
		res.Invites = append(res.Invites, ClerkInviteOption{
			ID: inv.ID, OrganizationName: inv.Edges.Organization.Name, Role: string(inv.Role),
		})
	}
	return res, nil
}

// ClerkSignup cria uma organização nova, com a pessoa do Clerk como dona (sem senha), e abre a sessão.
func (s *Service) ClerkSignup(ctx context.Context, sessionToken string, in ClerkSignupInput) (*ClerkResult, error) {
	ident, email, err := s.clerkIdentity(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	orgName := strings.TrimSpace(in.OrganizationName)
	if orgName == "" {
		return nil, ErrOrgNameRequired
	}
	profile, err := signupCountry(in.Country)
	if err != nil {
		return nil, err
	}
	if err := s.clerkNoPerson(ident, email); err != nil {
		return nil, err
	}
	token, tokenHash, err := NewToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	id, err := s.store.CreateAccount(NewAccount{
		OrganizationName: orgName,
		Country:          profile.Code,
		Currency:         profile.Currency,
		Timezone:         profile.Timezone,
		Name:             accountName(in.Name, ident.Name, email),
		Email:            email,
		ClerkUserID:      &ident.UserID,
		Role:             person.RoleAdmin,
		IsOwner:          true,
		SessionHash:      tokenHash,
		SessionExpires:   now.Add(SessionTTL),
		Now:              now,
	})
	if err != nil {
		return nil, err
	}
	return &ClerkResult{Status: ClerkOK, Identity: id, Token: token, Created: true}, nil
}

// ClerkJoin aceita um convite pendente feito para o e-mail verificado da pessoa.
func (s *Service) ClerkJoin(ctx context.Context, sessionToken string, in ClerkJoinInput) (*ClerkResult, error) {
	ident, email, err := s.clerkIdentity(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	invID, err := uuid.Parse(in.InviteID)
	if err != nil {
		return nil, ErrInviteInvalid
	}
	inv, err := s.store.InviteByID(invID, s.now())
	if err != nil {
		return nil, err
	}
	// Um convite sem e-mail é de quem tem o link: entra pelo token, e não por aqui.
	if inv.Email == nil {
		return nil, ErrInviteInvalid
	}
	if *inv.Email != email {
		return nil, ErrInviteEmailMismatch
	}
	if err := s.clerkNoPerson(ident, email); err != nil {
		return nil, err
	}
	return s.clerkJoin(ctx, ident, email, inv, in.Name)
}

// clerkNoPerson confere que não há conta para esse usuário do Clerk nem para o e-mail dele.
func (s *Service) clerkNoPerson(ident *adapter.ClerkIdentity, email string) error {
	if _, err := s.store.PersonByClerkID(ident.UserID); err == nil {
		return ErrAccountExists
	} else if !errors.Is(err, database.ErrNotFound) {
		return err
	}
	inUse, err := s.store.EmailInUse(email)
	if err != nil {
		return err
	}
	if inUse {
		return ErrAccountExists
	}
	return nil
}

// clerkJoin cria a conta da pessoa do Clerk na organização do convite, marca o convite como aceito e abre a sessão.
func (s *Service) clerkJoin(ctx context.Context, ident *adapter.ClerkIdentity, email string, inv *ent.Invite, name string) (*ClerkResult, error) {
	token, tokenHash, err := NewToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	orgID := inv.OrganizationID
	id, err := s.store.CreateAccount(NewAccount{
		OrganizationID: &orgID,
		Name:           accountName(name, ident.Name, email),
		Email:          email,
		ClerkUserID:    &ident.UserID,
		Role:           string(inv.Role),
		SessionHash:    tokenHash,
		SessionExpires: now.Add(SessionTTL),
		InviteID:       &inv.ID,
		Now:            now,
	})
	if err != nil {
		return nil, err
	}
	s.applyProject(ctx, inv, id.PersonID)
	return &ClerkResult{Status: ClerkOK, Identity: id, Token: token, Created: true}, nil
}

// accountName escolhe o nome da conta: o que a pessoa digitou, senão o do Clerk, senão a parte do e-mail antes
// do "@". Nunca fica vazio.
func accountName(typed, fromClerk, email string) string {
	for _, n := range []string{typed, fromClerk} {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}
