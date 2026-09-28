package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/person"
)

const (
	SessionTTL = 7 * 24 * time.Hour
	InviteTTL  = 7 * 24 * time.Hour
)

var (
	ErrInvalidCredentials  = errors.New("email ou senha incorretos")
	ErrUnauthenticated     = errors.New("faça login para continuar")
	ErrInviteInvalid       = errors.New("este convite não é válido: ele expirou, foi revogado ou já foi usado")
	ErrInviteEmailMismatch = errors.New("este convite foi feito para outro email")
	ErrWrongPassword       = errors.New("a senha atual está incorreta")
	ErrEmailInUse          = person.ErrEmailInUse
	ErrAccountExists       = errors.New("já existe uma conta com este email")
	ErrNameRequired        = errors.New("informe o seu nome")
	ErrOrgNameRequired     = errors.New("informe o nome da organização")
)

type Service struct {
	store *Store
	now   func() time.Time
}

func NewService(store *Store) *Service {
	return &Service{store: store, now: time.Now}
}

type SignupInput struct {
	OrganizationName string `json:"organization_name"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
}

// Signup cria uma organização nova com a pessoa como admin e já abre a sessão.
func (s *Service) Signup(in SignupInput) (*Identity, string, error) {
	orgName := strings.TrimSpace(in.OrganizationName)
	if orgName == "" {
		return nil, "", ErrOrgNameRequired
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
		Name:             name,
		Email:            email,
		PasswordHash:     hash,
		Role:             person.RoleAdmin,
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

// CreateInvite gera um link de convite para a organização de quem convida. O
// token em claro só existe no retorno desta chamada.
func (s *Service) CreateInvite(actor *Identity, email, role string) (*Invite, string, error) {
	if role == "" {
		role = person.RoleMember
	}
	if role != person.RoleAdmin && role != person.RoleMember {
		return nil, "", person.ErrInvalidRole
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
	inv, err := s.store.CreateInvite(actor.OrganizationID, actor.PersonID, emailPtr, role, tokenHash, s.now().Add(InviteTTL))
	if err != nil {
		return nil, "", err
	}
	inv.CreatedByName = actor.Name
	return inv, token, nil
}

func (s *Service) ListInvites(orgID uuid.UUID) ([]Invite, error) {
	return s.store.PendingInvites(orgID, s.now())
}

func (s *Service) RevokeInvite(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return database.ErrNotFound
	}
	return s.store.DeleteInvite(uid)
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
		PasswordHash:   hash,
		Role:           string(inv.Role),
		SessionHash:    sessionHash,
		SessionExpires: now.Add(SessionTTL),
		InviteID:       &inv.ID,
		Now:            now,
	})
	if err != nil {
		return nil, "", err
	}
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
