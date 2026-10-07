package collaborator

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entalloc "working-time-tracker/ent/allocation"
	entperson "working-time-tracker/ent/person"
	entteam "working-time-tracker/ent/team"
	enttm "working-time-tracker/ent/teammembership"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// ListByProject lista, por nome, quem tem valor por hora no projeto, com os
// times e o valor de cada pessoa. Quem está num time sem valor, de antes de o
// valor ser obrigatório, também vem, para um admin poder resolver.
func (s *Store) ListByProject(projectID uuid.UUID) ([]Collaborator, error) {
	hasRate := entalloc.ProjectIDEQ(projectID)
	inTeam := enttm.HasTeamWith(entteam.ProjectIDEQ(projectID))
	people, err := s.client.Person.Query().
		Where(entperson.Or(
			entperson.HasAllocationsWith(hasRate),
			entperson.HasTeamMembershipsWith(inTeam),
		)).
		WithAllocations(func(q *ent.AllocationQuery) { q.Where(hasRate) }).
		WithTeamMemberships(func(q *ent.TeamMembershipQuery) { q.Where(inTeam).WithTeam() }).
		Order(ent.Asc(entperson.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	result := make([]Collaborator, len(people))
	for i, p := range people {
		c := Collaborator{
			Person: Person{ID: p.ID, Name: p.Name, Email: p.Email},
			Teams:  make([]Team, 0, len(p.Edges.TeamMemberships)),
		}
		for _, a := range p.Edges.Allocations {
			rate := a.PayRateCents
			c.PayRateCents = &rate
		}
		for _, m := range p.Edges.TeamMemberships {
			c.Teams = append(c.Teams, Team{ID: m.Edges.Team.ID, Name: m.Edges.Team.Name})
		}
		sort.Slice(c.Teams, func(a, b int) bool { return c.Teams[a].Name < c.Teams[b].Name })
		result[i] = c
	}
	return result, nil
}

// Remove tira a pessoa do projeto: apaga o valor por hora dela e a tira de
// todos os times do projeto, numa transação só. As tarefas e as sessões de
// trabalho dela ficam. Devolve database.ErrNotFound quando ela não tinha
// nenhum dos dois vínculos.
func (s *Store) Remove(projectID, personID uuid.UUID) error {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		tx.Rollback()
		return err
	}

	rates, err := tx.Allocation.Delete().
		Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(personID)).
		Exec(ctx)
	if err != nil {
		return rollback(err)
	}
	memberships, err := tx.TeamMembership.Delete().
		Where(enttm.PersonIDEQ(personID), enttm.HasTeamWith(entteam.ProjectIDEQ(projectID))).
		Exec(ctx)
	if err != nil {
		return rollback(err)
	}
	if rates+memberships == 0 {
		return rollback(database.ErrNotFound)
	}
	return tx.Commit()
}
