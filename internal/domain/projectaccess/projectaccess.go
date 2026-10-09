// Package projectaccess diz quem está num projeto. Estar nele é ter valor por hora ou estar em algum time
// dele (a mesma conta dos colaboradores e do número de pessoas do cartão do projeto). Quem não é admin só vê
// os projetos em que está: o dono e os admins veem todos e não passam por aqui.
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
	entteam "working-time-tracker/ent/team"
	enttm "working-time-tracker/ent/teammembership"
	"working-time-tracker/internal/domain/permission"
)

// PersonIn escolhe as pessoas que estão no projeto: as que têm valor por hora nele ou estão em algum time dele.
func PersonIn(projectID uuid.UUID) predicate.Person {
	return entperson.Or(
		entperson.HasAllocationsWith(entalloc.ProjectIDEQ(projectID)),
		entperson.HasTeamMembershipsWith(enttm.HasTeamWith(entteam.ProjectIDEQ(projectID))),
	)
}

// ProjectsOf escolhe os projetos em que a pessoa está: aqueles em que tem valor por hora ou em cujo time está.
func ProjectsOf(personID uuid.UUID) predicate.Project {
	return entproject.Or(
		entproject.HasAllocationsWith(entalloc.PersonIDEQ(personID)),
		entproject.HasTeamsWith(entteam.HasMembershipsWith(enttm.PersonIDEQ(personID))),
	)
}

// ColleaguesOf escolhe a própria pessoa e quem está em algum dos projetos dela: as pessoas que ela tem motivo
// para ver. Quem não é admin e não cuida de pessoas não vê mais ninguém da organização.
func ColleaguesOf(personID uuid.UUID) predicate.Person {
	mine := ProjectsOf(personID)
	return entperson.Or(
		entperson.IDEQ(personID),
		entperson.HasAllocationsWith(entalloc.HasProjectWith(mine)),
		entperson.HasTeamMembershipsWith(enttm.HasTeamWith(entteam.HasProjectWith(mine))),
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

// Has diz se a pessoa está no projeto.
func Has(ctx context.Context, client *ent.Client, personID, projectID uuid.UUID) (bool, error) {
	return client.Project.Query().Where(entproject.IDEQ(projectID), ProjectsOf(personID)).Exist(ctx)
}
