// Package projectaccess diz quem está num projeto. Estar nele é ter valor por hora nele (a mesma conta dos
// colaboradores e do número de pessoas do cartão do projeto): ninguém entra num time sem o valor. Quem não é admin
// só vê os projetos em que está: o dono e os admins veem todos e não passam por aqui.
package projectaccess

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entalloc "working-time-tracker/ent/allocation"
	entperson "working-time-tracker/ent/person"
	"working-time-tracker/ent/predicate"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/internal/domain/permission"
)

// PersonIn escolhe as pessoas que estão no projeto: as que têm valor por hora nele.
func PersonIn(projectID uuid.UUID) predicate.Person {
	return entperson.HasAllocationsWith(entalloc.ProjectIDEQ(projectID))
}

// ProjectsOf escolhe os projetos em que a pessoa está: aqueles em que tem valor por hora.
func ProjectsOf(personID uuid.UUID) predicate.Project {
	return entproject.HasAllocationsWith(entalloc.PersonIDEQ(personID))
}

// ColleaguesOf escolhe a própria pessoa e quem está em algum dos projetos dela: as pessoas que ela tem motivo
// para ver. Quem não é admin e não cuida de pessoas não vê mais ninguém da organização.
func ColleaguesOf(personID uuid.UUID) predicate.Person {
	mine := ProjectsOf(personID)
	return entperson.Or(
		entperson.IDEQ(personID),
		entperson.HasAllocationsWith(entalloc.HasProjectWith(mine)),
	)
}

// ManagesPeople diz se a pessoa pode pôr gente num projeto em que está (tem collaborators.manage nele). Quem pode
// precisa achar qualquer pessoa da organização, para escolher quem entra.
func ManagesPeople(ctx context.Context, client *ent.Client, personID uuid.UUID) (bool, error) {
	allocs, err := client.Allocation.Query().Where(entalloc.PersonIDEQ(personID)).All(ctx)
	if err != nil {
		return false, err
	}
	for _, a := range allocs {
		if slices.Contains(permission.Normalize(a.Permissions, permission.ProjectKeys), permission.CollaboratorsManage) {
			return true, nil
		}
	}
	return false, nil
}

// PeopleOfManagedProjects devolve as pessoas que estão em algum projeto em que a pessoa tem collaborators.manage (o
// Administrador de projeto de hoje, o grupo manager). É o que ela vê além de si: a jornada de quem trabalha com ela
// nesse projeto. A própria pessoa não entra por aqui; quem chama já a trata à parte.
func PeopleOfManagedProjects(ctx context.Context, client *ent.Client, personID uuid.UUID) (map[uuid.UUID]bool, error) {
	allocs, err := client.Allocation.Query().Where(entalloc.PersonIDEQ(personID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var projects []uuid.UUID
	for _, a := range allocs {
		if slices.Contains(permission.Normalize(a.Permissions, permission.ProjectKeys), permission.CollaboratorsManage) {
			projects = append(projects, a.ProjectID)
		}
	}
	out := map[uuid.UUID]bool{}
	if len(projects) == 0 {
		return out, nil
	}
	people, err := client.Person.Query().Where(
		entperson.HasAllocationsWith(entalloc.ProjectIDIn(projects...)),
	).IDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range people {
		out[id] = true
	}
	return out, nil
}

// Has diz se a pessoa está no projeto.
func Has(ctx context.Context, client *ent.Client, personID, projectID uuid.UUID) (bool, error) {
	return client.Project.Query().Where(entproject.IDEQ(projectID), ProjectsOf(personID)).Exist(ctx)
}
