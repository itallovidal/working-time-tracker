package person

import (
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/permission"
)

// NormalizeEmail tira espaços e deixa o email em minúsculas, para que
// "Ana@X.com" e "ana@x.com " sejam a mesma conta.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail faz uma checagem mínima de formato; a confirmação real seria por email.
func ValidEmail(email string) bool {
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1 && !strings.ContainsAny(email, " \t\n")
}

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(orgID, name, email string) (*Person, error) {
	name = strings.TrimSpace(name)
	email = NormalizeEmail(email)
	if name == "" {
		return nil, ErrNameRequired
	}
	if email == "" {
		return nil, ErrEmailRequired
	}
	if !ValidEmail(email) {
		return nil, ErrInvalidEmail
	}
	orgUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, ErrInvalidOrganization
	}
	inUse, err := s.store.EmailInUse(email, nil)
	if err != nil {
		return nil, err
	}
	if inUse {
		return nil, ErrEmailInUse
	}
	person := &Person{
		Name:           name,
		Email:          email,
		OrganizationID: orgUID,
	}
	if err := s.store.Create(person); err != nil {
		return nil, err
	}
	return person, nil
}

func (s *Service) ListByOrg(orgID string) ([]Person, error) {
	return s.store.ListByOrg(orgID)
}

func (s *Service) Get(id string) (*Person, error) {
	return s.store.GetByID(id)
}

func (s *Service) Update(id, name, email string) (*Person, error) {
	name = strings.TrimSpace(name)
	email = NormalizeEmail(email)
	if name == "" {
		return nil, ErrNameRequired
	}
	if email == "" {
		return nil, ErrEmailRequired
	}
	if !ValidEmail(email) {
		return nil, ErrInvalidEmail
	}
	person, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	inUse, err := s.store.EmailInUse(email, &person.ID)
	if err != nil {
		return nil, err
	}
	if inUse {
		return nil, ErrEmailInUse
	}
	person.Name = name
	person.Email = email
	if err := s.store.Update(person); err != nil {
		return nil, err
	}
	return person, nil
}

// SetRole muda o papel de uma pessoa, sem deixar a organização sem admin. A
// checagem do último admin fica no store, na mesma transação da mudança.
func (s *Service) SetRole(id, role string) (*Person, error) {
	if role != RoleAdmin && role != RoleMember {
		return nil, ErrInvalidRole
	}
	return s.store.SetRole(id, role)
}

// SetPermissions define as permissões da organização de quem não é admin: admins já
// têm todas, e guardar uma lista para eles confundiria a tela.
func (s *Service) SetPermissions(id string, keys []string) (*Person, error) {
	if !permission.Valid(keys, permission.OrganizationKeys) {
		return nil, ErrInvalidPermission
	}
	p, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if p.Role == RoleAdmin {
		return nil, ErrAdminHasAll
	}
	return s.store.SetPermissions(id, permission.Normalize(keys, permission.OrganizationKeys))
}

// SetWeeklyHours define a jornada semanal da pessoa. nil ou zero apaga.
func (s *Service) SetWeeklyHours(id string, hours *int) (*Person, error) {
	if hours != nil && (*hours < 0 || *hours > 168) {
		return nil, ErrInvalidWeekHours
	}
	if hours != nil && *hours == 0 {
		hours = nil
	}
	return s.store.SetWeeklyHours(id, hours)
}
