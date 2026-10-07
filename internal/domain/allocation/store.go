package allocation

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entalloc "working-time-tracker/ent/allocation"
	entorg "working-time-tracker/ent/organization"
	entperson "working-time-tracker/ent/person"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// Set define o valor da pessoa no projeto, criando o vínculo se ele não existir.
func (s *Store) Set(projectID, personID uuid.UUID, payRateCents int) error {
	ctx := context.Background()
	update := func() (int, error) {
		return s.client.Allocation.Update().
			Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(personID)).
			SetPayRateCents(payRateCents).
			Save(ctx)
	}
	n, err := update()
	if err != nil || n > 0 {
		return err
	}
	_, err = s.client.Allocation.Create().
		SetProjectID(projectID).
		SetPersonID(personID).
		SetPayRateCents(payRateCents).
		Save(ctx)
	if ent.IsConstraintError(err) {
		// Outra requisição criou o vínculo entre a tentativa de atualizar e a de
		// criar; o índice único barrou esta, então basta atualizar.
		_, err = update()
	}
	return err
}

func (s *Store) Get(projectID, personID uuid.UUID) (*Allocation, error) {
	a, err := s.client.Allocation.Query().
		Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(personID)).
		WithPerson().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomain(a), nil
}

// ListByProject lista os vínculos do projeto pelo nome da pessoa.
func (s *Store) ListByProject(projectID uuid.UUID) ([]Allocation, error) {
	rows, err := s.client.Allocation.Query().
		Where(entalloc.ProjectIDEQ(projectID)).
		WithPerson().
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := toDomainList(rows)
	sort.Slice(result, func(i, j int) bool { return result[i].Person.Name < result[j].Person.Name })
	return result, nil
}

// ListByPerson lista os vínculos da pessoa pelo nome do projeto.
func (s *Store) ListByPerson(personID uuid.UUID) ([]Allocation, error) {
	rows, err := s.client.Allocation.Query().
		Where(entalloc.PersonIDEQ(personID)).
		WithProject().
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := toDomainList(rows)
	sort.Slice(result, func(i, j int) bool { return result[i].Project.Name < result[j].Project.Name })
	return result, nil
}

// RateSet são os valores por hora de uma pessoa num projeto: o que ela recebe e o que
// o cliente paga (nil em projeto sem valor cobrado). Found é false quando a pessoa não
// tem vínculo com o projeto. Owner marca o dono da organização: para ele o valor pago é
// zero, porque o que ele tira do projeto é a margem, e Found vale sempre.
type RateSet struct {
	PayRateCents  int
	BillRateCents *int
	Found         bool
	Owner         bool
}

// Rates devolve os valores por hora da pessoa no projeto. O dono entra no projeto aqui,
// com valor zero, se ainda não estava: quem trabalha num projeto é colaborador dele.
func (s *Store) Rates(personID, projectID uuid.UUID) (RateSet, error) {
	ctx := context.Background()
	owner, err := s.IsOwner(personID)
	if err != nil {
		return RateSet{}, err
	}
	if owner {
		if err := s.Set(projectID, personID, 0); err != nil {
			return RateSet{}, err
		}
	}
	a, err := s.client.Allocation.Query().
		Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(personID)).
		WithProject().
		Only(ctx)
	if ent.IsNotFound(err) {
		return RateSet{}, nil
	}
	if err != nil {
		return RateSet{}, err
	}
	return RateSet{PayRateCents: a.PayRateCents, BillRateCents: a.Edges.Project.BillRateCents, Found: true, Owner: owner}, nil
}

// IsOwner diz se a pessoa é o dono da organização.
func (s *Store) IsOwner(personID uuid.UUID) (bool, error) {
	return s.client.Person.Query().
		Where(entperson.IDEQ(personID), entperson.IsOwner(true)).
		Exist(context.Background())
}

// PersonInProjectOrganization diz se a pessoa existe e é da organização do projeto.
func (s *Store) PersonInProjectOrganization(personID, projectID uuid.UUID) (bool, error) {
	return s.client.Person.Query().
		Where(
			entperson.IDEQ(personID),
			entperson.HasOrganizationWith(entorg.HasProjectsWith(entproject.IDEQ(projectID))),
		).
		Exist(context.Background())
}

func toDomain(e *ent.Allocation) *Allocation {
	a := &Allocation{
		ProjectID:    e.ProjectID,
		PersonID:     e.PersonID,
		PayRateCents: e.PayRateCents,
		CreatedAt:    e.CreatedAt,
	}
	if p := e.Edges.Person; p != nil {
		a.Person = &Person{ID: p.ID, Name: p.Name, Email: p.Email}
	}
	if p := e.Edges.Project; p != nil {
		a.Project = &Project{ID: p.ID, Name: p.Name}
	}
	return a
}

func toDomainList(es []*ent.Allocation) []Allocation {
	result := make([]Allocation, len(es))
	for i, e := range es {
		result[i] = *toDomain(e)
	}
	return result
}
