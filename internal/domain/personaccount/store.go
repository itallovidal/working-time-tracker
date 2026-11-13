package personaccount

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/personaccount"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// ForPerson devolve as contas da pessoa, na ordem de Platforms.
func (s *Store) ForPerson(personID uuid.UUID) ([]Account, error) {
	rows, err := s.client.PersonAccount.Query().
		Where(personaccount.PersonIDEQ(personID)).
		Order(ent.Asc(personaccount.FieldPlatform)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(rows))
	for i, r := range rows {
		out[i] = *toDomain(r)
	}
	return out, nil
}

// Get devolve a conta da pessoa na plataforma, ou database.ErrNotFound.
func (s *Store) Get(personID uuid.UUID, platform string) (*Account, error) {
	r, err := s.client.PersonAccount.Query().
		Where(personaccount.PersonIDEQ(personID), personaccount.PlatformEQ(personaccount.Platform(platform))).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomain(r), nil
}

// FindByUsername devolve a conta da organização com este login na plataforma, sem olhar maiúsculas (os logins do
// GitHub e do GitLab não as distinguem), ou database.ErrNotFound.
func (s *Store) FindByUsername(orgID uuid.UUID, platform, login string) (*Account, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, database.ErrNotFound
	}
	r, err := s.client.PersonAccount.Query().
		Where(personaccount.OrganizationIDEQ(orgID), personaccount.PlatformEQ(personaccount.Platform(platform)),
			personaccount.UsernameEqualFold(login)).
		First(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomain(r), nil
}

// Link grava a conta a da pessoa, com a precedência entre as origens:
//   - SourceOAuth: a pessoa provou o login. Tira o mesmo login de quem o tinha por outra origem e troca o que ela
//     tinha. ErrLoginTaken se outra pessoa também o autorizou.
//   - SourceAdmin: não tira o login de quem o autorizou (ErrLoginTaken) nem troca a conta autorizada da pessoa por
//     outra (ErrPersonLinked).
//   - SourceIntegration e SourceEmail: só entram onde não há conta (a resposta "não tenho conta" conta como nada), e
//     não tiram o login de ninguém. Fora disso, nada muda e o retorno é nil, nil.
//   - SourceSelf: a pessoa diz que não tem conta, o que só vale enquanto ela não tem um login ligado.
//
// A conta que a pessoa já tem fica travada na transação; duas ligações do mesmo login ao mesmo tempo esbarram no
// índice único e dão ErrLoginTaken.
func (s *Store) Link(a Account) (*Account, error) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	saved, err := link(ctx, tx, a)
	if err != nil {
		_ = tx.Rollback() // o erro que volta é o que motivou desfazer
		if ent.IsConstraintError(err) {
			return nil, ErrLoginTaken.With("login", a.Username)
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrLoginTaken.With("login", a.Username)
		}
		return nil, err
	}
	return saved, nil
}

func link(ctx context.Context, tx *ent.Tx, a Account) (*Account, error) {
	platform := personaccount.Platform(a.Platform)
	mine, err := tx.PersonAccount.Query().
		Where(personaccount.PersonIDEQ(a.PersonID), personaccount.PlatformEQ(platform)).
		ForUpdate().
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if a.Source == SourceSelf {
		if mine != nil && mine.Username != "" {
			return toDomain(mine), nil
		}
		a.Username, a.Email, a.NoAccount = "", "", true
		return save(ctx, tx, mine, a)
	}

	var holder *ent.PersonAccount
	if a.Username != "" {
		holder, err = tx.PersonAccount.Query().
			Where(personaccount.OrganizationIDEQ(a.OrganizationID), personaccount.PlatformEQ(platform),
				personaccount.UsernameEqualFold(a.Username), personaccount.PersonIDNEQ(a.PersonID)).
			First(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, err
		}
	}

	switch a.Source {
	case SourceOAuth:
		if holder != nil {
			if holder.Source == personaccount.SourceOauth {
				return nil, ErrLoginTaken.With("login", a.Username)
			}
			if err := tx.PersonAccount.DeleteOne(holder).Exec(ctx); err != nil {
				return nil, err
			}
		}
	case SourceAdmin:
		if holder != nil && verified(string(holder.Source)) {
			return nil, ErrLoginTaken.With("login", a.Username)
		}
		if mine != nil && mine.Username != "" && verified(string(mine.Source)) &&
			!strings.EqualFold(mine.Username, a.Username) {
			return nil, ErrPersonLinked.With("login", mine.Username)
		}
		if holder != nil {
			if err := tx.PersonAccount.DeleteOne(holder).Exec(ctx); err != nil {
				return nil, err
			}
		}
	default: // SourceIntegration, SourceEmail
		if (mine != nil && mine.Username != "") || holder != nil {
			return nil, nil
		}
	}
	a.NoAccount = false
	return save(ctx, tx, mine, a)
}

// save troca a conta que a pessoa tinha na plataforma (mine) por a, ou a cria.
func save(ctx context.Context, tx *ent.Tx, mine *ent.PersonAccount, a Account) (*Account, error) {
	if mine != nil {
		r, err := tx.PersonAccount.UpdateOne(mine).
			SetUsername(a.Username).
			SetEmail(a.Email).
			SetNoAccount(a.NoAccount).
			SetSource(personaccount.Source(a.Source)).
			Save(ctx)
		if err != nil {
			return nil, err
		}
		return toDomain(r), nil
	}
	r, err := tx.PersonAccount.Create().
		SetPersonID(a.PersonID).
		SetOrganizationID(a.OrganizationID).
		SetPlatform(personaccount.Platform(a.Platform)).
		SetUsername(a.Username).
		SetEmail(a.Email).
		SetNoAccount(a.NoAccount).
		SetSource(personaccount.Source(a.Source)).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return toDomain(r), nil
}

// Delete apaga a conta da pessoa na plataforma (a ligação e também o "não tenho conta"), ou database.ErrNotFound.
func (s *Store) Delete(personID uuid.UUID, platform string) error {
	n, err := s.client.PersonAccount.Delete().
		Where(personaccount.PersonIDEQ(personID), personaccount.PlatformEQ(personaccount.Platform(platform))).
		Exec(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return database.ErrNotFound
	}
	return nil
}

// LinkedPlatforms devolve, para cada pessoa da lista, as plataformas em que ela tem um login ligado. Serve à trava da
// tarefa e ao select de responsável.
func (s *Store) LinkedPlatforms(personIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := map[uuid.UUID][]string{}
	if len(personIDs) == 0 {
		return out, nil
	}
	rows, err := s.client.PersonAccount.Query().
		Where(personaccount.PersonIDIn(personIDs...), personaccount.UsernameNEQ("")).
		Order(ent.Asc(personaccount.FieldPlatform)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PersonID] = append(out[r.PersonID], string(r.Platform))
	}
	return out, nil
}

func toDomain(r *ent.PersonAccount) *Account {
	return &Account{
		PersonID:       r.PersonID,
		OrganizationID: r.OrganizationID,
		Platform:       string(r.Platform),
		Username:       r.Username,
		Email:          r.Email,
		NoAccount:      r.NoAccount,
		Source:         string(r.Source),
		CreatedAt:      r.CreatedAt,
	}
}
