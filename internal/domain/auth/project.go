package auth

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
)

// ProjectApplier faz a pessoa que aceitou um convite com projeto entrar nesse projeto: o valor por hora, o grupo
// de permissões e o time do ProjectSetup. Quem implementa conhece o projeto e os times; o auth só o chama.
type ProjectApplier interface {
	Apply(ctx context.Context, personID uuid.UUID, setup ProjectSetup) error
}

// SetProjectApplier liga o que o aceite de um convite com projeto faz depois de criar a conta. Sem isso, a pessoa
// entra na organização e o projeto fica por conta de quem convidou. log recebe o que não deu certo no projeto.
func (s *Service) SetProjectApplier(a ProjectApplier, log func(msg string, args ...any)) {
	s.projects = a
	s.projectLog = log
}

// applyProject põe a pessoa recém-criada no projeto do convite, se ele levava um. Uma falha não desfaz a conta,
// que já existe e já abriu a sessão: vai para o log, e quem convidou a adiciona ao projeto à mão.
func (s *Service) applyProject(ctx context.Context, inv *ent.Invite, personID uuid.UUID) {
	setup := projectSetupOf(inv)
	if setup == nil || s.projects == nil {
		return
	}
	if err := s.projects.Apply(ctx, personID, *setup); err != nil && s.projectLog != nil {
		s.projectLog("could not add the person who accepted an invite to its project",
			"person", personID.String(), "project", setup.ProjectID.String(), "error", err)
	}
}
