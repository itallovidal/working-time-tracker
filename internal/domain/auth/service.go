package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/country"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/person"
)

const (
	SessionTTL = 7 * 24 * time.Hour
	InviteTTL  = 7 * 24 * time.Hour
)

var (
	ErrEmailInUse = person.ErrEmailInUse
)

type Service struct {
	store *Store
	now   func() time.Time
	// clerk é o login pelo Clerk; nulo, ele está desligado (ver SetClerk).
	clerk         adapter.ClerkProvider
	clerkSettings ClerkSettings
	// projects faz a pessoa que aceita um convite com projeto entrar nele (ver SetProjectApplier).
	projects   ProjectApplier
	projectLog func(msg string, args ...any)
}

func NewService(store *Store) *Service {
	return &Service{store: store, now: time.Now}
}

type SignupInput struct {
	OrganizationName string `json:"organization_name"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	// Country é o código do país da organização (BR, US). Vazio é o país padrão, como era antes de o campo existir.
	Country string `json:"country"`
}

// signupCountry confere o país de quem cria a organização. Vazio é o país padrão: quem chama a API sem o campo continua
// recebendo uma organização brasileira.
func signupCountry(raw string) (*country.Country, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return country.MustGet(country.Default), nil
	}
	code, ok := country.Parse(raw)
	if !ok {
		return nil, ErrInvalidCountry
	}
	return country.MustGet(code), nil
}

// Signup cria uma organização nova com a pessoa como admin e já abre a sessão.
func (s *Service) Signup(in SignupInput) (*Identity, string, error) {
	orgName := strings.TrimSpace(in.OrganizationName)
	if orgName == "" {
		return nil, "", ErrOrgNameRequired
	}
	profile, err := signupCountry(in.Country)
	if err != nil {
		return nil, "", err
	}
	name, email, hash, err := s.validateAccount(in.Name, in.Email, in.Password)
	if err != nil {
		return nil, "", err
	}
	token, tokenHash, err := NewToken()
	if err != nil {
		return nil, "", err
	}
	now := s.now()
	id, err := s.store.CreateAccount(NewAccount{
		OrganizationName: orgName,
		Country:          profile.Code,
		Currency:         profile.Currency,
		Timezone:         profile.Timezone,
		Name:             name,
		Email:            email,
		PasswordHash:     &hash,
		Role:             person.RoleAdmin,
		IsOwner:          true,
		SessionHash:      tokenHash,
		SessionExpires:   now.Add(SessionTTL),
		Now:              now,
	})
	if err != nil {
		return nil, "", err
	}
	return id, token, nil
}

func (s *Service) Login(email, password string) (*Identity, string, error) {
	email = person.NormalizeEmail(email)
	p, err := s.store.PersonForLogin(email)
	if errors.Is(err, database.ErrNotFound) || (err == nil && p.PasswordHash == nil) {
		burnPasswordCheck(password)
		return nil, "", ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", err
	}
	if !CheckPassword(*p.PasswordHash, password) {
		return nil, "", ErrInvalidCredentials
	}
	return s.openSession(p)
}

// openSession abre uma sessão para a pessoa e devolve a identidade dela com o token do cookie. É o que o
// login por senha e o login pelo Clerk fazem depois de provar quem a pessoa é.
func (s *Service) openSession(p *ent.Person) (*Identity, string, error) {
	token, tokenHash, err := NewToken()
	if err != nil {
		return nil, "", err
	}
	now := s.now()
	if err := s.store.CreateSession(p.ID, tokenHash, now.Add(SessionTTL)); err != nil {
		return nil, "", err
	}
	// Aproveita o login para limpar sessões vencidas de todo mundo.
	_ = s.store.DeleteExpiredSessions(now)
	return identityOf(p, p.Edges.Organization), token, nil
}

func (s *Service) Logout(token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(HashToken(token))
}

// Authenticate resolve o token do cookie para a pessoa logada.
func (s *Service) Authenticate(token string) (*Identity, error) {
	if token == "" {
		return nil, ErrUnauthenticated
	}
	id, err := s.store.SessionIdentity(HashToken(token), s.now())
	if errors.Is(err, database.ErrNotFound) {
		return nil, ErrUnauthenticated
	}
	return id, err
}

// ChangePassword troca a senha e encerra as outras sessões da pessoa, mantendo a atual.
func (s *Service) ChangePassword(actor *Identity, currentToken, current, next string) error {
	hash, err := s.store.PasswordHash(actor.PersonID)
	if err != nil {
		return err
	}
	if hash == "" || !CheckPassword(hash, current) {
		return ErrWrongPassword
	}
	newHash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.store.SetPasswordHash(actor.PersonID, newHash); err != nil {
		return err
	}
	return s.store.DeleteOtherSessions(actor.PersonID, HashToken(currentToken))
}

// CompleteOnboarding dá baixa nas boas-vindas do primeiro acesso de quem está logado, seja porque terminou, seja porque
// as dispensou. É idempotente e só mexe na própria pessoa.
func (s *Service) CompleteOnboarding(actor *Identity) error {
	return s.store.MarkOnboarded(actor.PersonID, s.now())
}

// CreateInvite gera o convite para a organização de quem convida. O token em claro só existe no retorno desta
// chamada. Com o Clerk ligado e um e-mail, o convite também é criado no Clerk, que manda o e-mail com o link (no
// modo terminal ele não manda, e o link vai para o log do servidor), e só é gravado se o Clerk o aceitou; um novo
// convite para o mesmo e-mail na organização substitui o anterior. Sem e-mail, ou sem o Clerk, é só o link.
func (s *Service) CreateInvite(ctx context.Context, actor *Identity, email, role string) (*Invite, string, error) {
	return s.createInvite(ctx, actor, email, role, nil)
}

// CreateProjectInvite é o convite que também leva a pessoa a um projeto: o e-mail é obrigatório, o papel é o de
// membro, e quando a pessoa aceita entra no projeto com o valor, o time e o grupo de permissões do setup. Quem
// chama já conferiu o que o setup pede (o handler do projeto, que conhece as permissões dele).
func (s *Service) CreateProjectInvite(ctx context.Context, actor *Identity, email string, setup ProjectSetup) (*Invite, string, error) {
	if person.NormalizeEmail(email) == "" {
		return nil, "", person.ErrInvalidEmail
	}
	return s.createInvite(ctx, actor, email, person.RoleMember, &setup)
}

func (s *Service) createInvite(ctx context.Context, actor *Identity, email, role string, setup *ProjectSetup) (*Invite, string, error) {
	if role == "" {
		role = person.RoleMember
	}
	if role != person.RoleAdmin && role != person.RoleMember {
		return nil, "", person.ErrInvalidRole
	}
	// Quem convida para admin dá poder sobre a organização inteira: é do dono.
	if role == person.RoleAdmin && !actor.IsOwner {
		return nil, "", ErrOwnerOnly
	}
	var emailPtr *string
	if e := person.NormalizeEmail(email); e != "" {
		if !person.ValidEmail(e) {
			return nil, "", person.ErrInvalidEmail
		}
		inUse, err := s.store.EmailInUse(e)
		if err != nil {
			return nil, "", err
		}
		if inUse {
			return nil, "", ErrAccountExists
		}
		emailPtr = &e
	}
	token, tokenHash, err := NewToken()
	if err != nil {
		return nil, "", err
	}

	delivery := DeliveryLink
	var clerkInv *adapter.ClerkInvitation
	var clerkID *string
	if emailPtr != nil && s.clerk != nil {
		notify := s.clerkSettings.InviteDelivery != DeliveryTerminal
		clerkInv, err = s.clerk.CreateInvitation(ctx, adapter.ClerkInviteParams{
			Email:         *emailPtr,
			RedirectURL:   s.clerkSettings.PublicURL + "/invite/" + token,
			Notify:        notify,
			ExpiresInDays: int(InviteTTL / (24 * time.Hour)),
		})
		if err != nil {
			if errors.Is(err, adapter.ErrClerkRejected) {
				return nil, "", ErrClerkInviteFailed.Wrap(err)
			}
			return nil, "", ErrClerkUnavailable.Wrap(err)
		}
		clerkID = &clerkInv.ID
		delivery = DeliveryEmail
		if !notify {
			delivery = DeliveryTerminal
		}
	}

	inv, err := s.store.CreateInvite(actor.OrganizationID, actor.PersonID, emailPtr, role, setup, clerkID, tokenHash, s.now().Add(InviteTTL))
	if err != nil {
		if clerkInv != nil {
			s.revokeAtClerk(ctx, clerkInv.ID)
		}
		return nil, "", err
	}
	if clerkInv != nil {
		s.supersedeInvites(ctx, actor.OrganizationID, *emailPtr, inv.ID)
		if delivery == DeliveryTerminal {
			link := clerkInv.URL
			if link == "" {
				link = s.clerkSettings.PublicURL + "/invite/" + token
			}
			s.log("invite not emailed (INVITE_DELIVERY=terminal)", "email", *emailPtr, "role", role, "organization", actor.OrganizationName, "link", link)
		}
	}
	inv.CreatedByName = actor.Name
	inv.Delivery = delivery
	return inv, token, nil
}

// supersedeInvites apaga, e cancela no Clerk, os convites pendentes da organização para o mesmo e-mail: o novo
// convite é o que vale (é o "reenviar"). Uma falha aqui não desfaz o convite novo.
func (s *Service) supersedeInvites(ctx context.Context, orgID uuid.UUID, email string, keep uuid.UUID) {
	old, err := s.store.PendingInvitesOfEmail(orgID, email, s.now(), keep)
	if err != nil {
		s.log("could not list the invites to replace", "email", email, "error", err)
		return
	}
	for _, o := range old {
		if _, err := s.store.DeleteInvite(o.ID); err != nil {
			s.log("could not replace an invite", "email", email, "error", err)
			continue
		}
		if o.ClerkInvitationID != nil {
			s.revokeAtClerk(ctx, *o.ClerkInvitationID)
		}
	}
}

// revokeAtClerk cancela um convite no Clerk. É o melhor esforço: o convite daqui já não existe, então um que
// sobrou no Clerk só cria uma conta lá e não dá acesso a organização nenhuma.
func (s *Service) revokeAtClerk(ctx context.Context, clerkInvitationID string) {
	if s.clerk == nil {
		return
	}
	if err := s.clerk.RevokeInvitation(ctx, clerkInvitationID); err != nil {
		s.log("could not cancel the invite at Clerk", "clerk_invitation_id", clerkInvitationID, "error", err)
	}
}

func (s *Service) ListInvites(orgID uuid.UUID) ([]Invite, error) {
	return s.store.PendingInvites(orgID, s.now())
}

// ProjectInvites lista os convites pendentes que levam a pessoa a esse projeto.
func (s *Service) ProjectInvites(projectID string) ([]Invite, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	return s.store.PendingInvitesOfProject(id, s.now())
}

// RevokeInvite apaga o convite e, se ele foi criado no Clerk, o cancela lá.
func (s *Service) RevokeInvite(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return database.ErrNotFound
	}
	clerkID, err := s.store.DeleteInvite(uid)
	if err != nil {
		return err
	}
	if clerkID != nil {
		s.revokeAtClerk(ctx, *clerkID)
	}
	return nil
}

func (s *Service) InviteInfo(token string) (*InviteInfo, error) {
	inv, err := s.store.ValidInvite(HashToken(token), s.now())
	if err != nil {
		return nil, err
	}
	return &InviteInfo{
		OrganizationName: inv.Edges.Organization.Name,
		Email:            inv.Email,
		Role:             string(inv.Role),
		ExpiresAt:        inv.ExpiresAt,
	}, nil
}

type AcceptInviteInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AcceptInvite cria a conta da pessoa convidada na organização do convite e abre a sessão.
func (s *Service) AcceptInvite(token string, in AcceptInviteInput) (*Identity, string, error) {
	now := s.now()
	inv, err := s.store.ValidInvite(HashToken(token), now)
	if err != nil {
		return nil, "", err
	}
	name, email, hash, err := s.validateAccount(in.Name, in.Email, in.Password)
	if err != nil {
		return nil, "", err
	}
	if inv.Email != nil && *inv.Email != email {
		return nil, "", ErrInviteEmailMismatch
	}
	sessionToken, sessionHash, err := NewToken()
	if err != nil {
		return nil, "", err
	}
	orgID := inv.OrganizationID
	id, err := s.store.CreateAccount(NewAccount{
		OrganizationID: &orgID,
		Name:           name,
		Email:          email,
		PasswordHash:   &hash,
		Role:           string(inv.Role),
		SessionHash:    sessionHash,
		SessionExpires: now.Add(SessionTTL),
		InviteID:       &inv.ID,
		Now:            now,
	})
	if err != nil {
		return nil, "", err
	}
	s.applyProject(context.Background(), inv, id.PersonID)
	return id, sessionToken, nil
}

// validateAccount confere nome, email e senha de uma conta nova e devolve os
// valores normalizados junto com o hash da senha.
func (s *Service) validateAccount(name, email, password string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", ErrNameRequired
	}
	email = person.NormalizeEmail(email)
	if !person.ValidEmail(email) {
		return "", "", "", person.ErrInvalidEmail
	}
	inUse, err := s.store.EmailInUse(email)
	if err != nil {
		return "", "", "", err
	}
	if inUse {
		return "", "", "", ErrEmailInUse
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", "", "", err
	}
	return name, email, hash, nil
}
