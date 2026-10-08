// Package projectinvite junta o convite e o projeto: convidar para a organização uma pessoa que já entra num
// projeto com valor por hora, time e grupo de permissões escolhidos, e fazer isso valer quando ela aceita.
package projectinvite

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/team"
)

// Applier faz a pessoa que aceitou um convite com projeto entrar nele. É o que o auth chama (auth.ProjectApplier).
type Applier struct {
	allocations *allocation.Service
	members     *team.MembershipService
}

var _ auth.ProjectApplier = (*Applier)(nil)

func NewApplier(allocations *allocation.Service, members *team.MembershipService) *Applier {
	return &Applier{allocations: allocations, members: members}
}

// Apply põe a pessoa no projeto com o valor por hora, depois dá o grupo de permissões e a põe no time. O valor
// vem primeiro porque sem ele a pessoa não está no projeto (e um time só aceita quem está). Cada passo que falha
// vai para o erro devolvido, e os seguintes ainda rodam: a pessoa que ficou no projeto sem o time, por exemplo,
// é melhor que a que ficou fora dele.
func (a *Applier) Apply(_ context.Context, personID uuid.UUID, setup auth.ProjectSetup) error {
	project, person := setup.ProjectID.String(), personID.String()
	rate := 0
	if setup.PayRateCents != nil {
		rate = *setup.PayRateCents
	}
	if _, err := a.allocations.Set(project, person, rate); err != nil {
		return err
	}
	var errs []error
	if setup.Preset != "" && setup.Preset != permission.PresetMember {
		if _, err := a.allocations.SetPreset(project, person, setup.Preset); err != nil {
			errs = append(errs, err)
		}
	}
	if setup.TeamID != nil {
		if _, err := a.members.Add(setup.TeamID.String(), person); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
