package auth

import (
	"context"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	entalloc "working-time-tracker/ent/allocation"
	"working-time-tracker/ent/integration"
	"working-time-tracker/ent/task"
	"working-time-tracker/ent/team"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/projectaccess"
)

// Kind é o tipo de recurso de um parâmetro de rota, para descobrir a organização dona.
type Kind int

const (
	KindOrganization Kind = iota
	KindProject
	KindTeam
	KindTask
	KindIntegration
	KindPerson
	KindInvite
	KindCustomer
)

// Resolver descobre a qual organização um recurso pertence.
type Resolver struct {
	client *ent.Client
}

func NewResolver(client *ent.Client) *Resolver {
	return &Resolver{client: client}
}

func (r *Resolver) OrganizationOf(kind Kind, id string) (uuid.UUID, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}
	ctx := context.Background()
	switch kind {
	case KindOrganization:
		return uid, nil
	case KindProject:
		p, err := r.client.Project.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return p.OrganizationID, nil
	case KindTeam:
		p, err := r.client.Team.Query().Where(team.IDEQ(uid)).QueryProject().Only(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		return p.OrganizationID, nil
	case KindTask:
		p, err := r.client.Task.Query().Where(task.IDEQ(uid)).QueryProject().Only(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		return p.OrganizationID, nil
	case KindIntegration:
		p, err := r.client.Integration.Query().Where(integration.IDEQ(uid)).QueryProject().Only(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		return p.OrganizationID, nil
	case KindPerson:
		p, err := r.client.Person.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return p.OrganizationID, nil
	case KindInvite:
		inv, err := r.client.Invite.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return inv.OrganizationID, nil
	case KindCustomer:
		cust, err := r.client.Customer.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return cust.OrganizationID, nil
	}
	return uuid.Nil, &ent.NotFoundError{}
}

// ProjectOf devolve o projeto a que o recurso pertence: ele mesmo, ou o projeto do time,
// da tarefa ou da integração. Para os outros tipos devolve uuid.Nil.
func (r *Resolver) ProjectOf(kind Kind, id string) (uuid.UUID, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}
	ctx := context.Background()
	switch kind {
	case KindProject:
		return uid, nil
	case KindTeam:
		t, err := r.client.Team.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return t.ProjectID, nil
	case KindTask:
		t, err := r.client.Task.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return t.ProjectID, nil
	case KindIntegration:
		i, err := r.client.Integration.Get(ctx, uid)
		if err != nil {
			return uuid.Nil, err
		}
		return i.ProjectID, nil
	}
	return uuid.Nil, nil
}

// BaseProjectSet é o que o cargo dá em qualquer projeto, antes da alocação: tudo, para o dono; o que o admin tem em
// todo projeto (permission.AdminProjectKeys, sem a cobrança), para o admin; nada, para os outros. É o único lugar
// que decide isso: ProjectSet, ProjectAccess e o ProjectPermissions de uma rota sem projeto partem dele.
func BaseProjectSet(me *Identity) permission.Set {
	switch {
	case me == nil:
		return permission.Set{}
	case me.IsOwner:
		return permission.Set{All: true}
	case me.IsAdmin():
		return permission.Set{Keys: append([]string{}, permission.AdminProjectKeys...)}
	}
	return permission.Set{}
}

// withGroup soma ao conjunto do cargo as permissões do grupo da alocação, que o dono pode ter dado a um admin
// (por exemplo o financeiro, para um projeto).
func withGroup(base permission.Set, groupKeys []string) permission.Set {
	if base.All {
		return base
	}
	keys := append([]string{}, base.Keys...)
	for _, k := range permission.Normalize(groupKeys, permission.ProjectKeys) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return permission.Set{Keys: permission.Normalize(keys, permission.ProjectKeys)}
}

// ProjectSet devolve o que a pessoa pode fazer no projeto: o que o cargo dá (BaseProjectSet) mais o que a alocação
// dela no projeto libera.
func (r *Resolver) ProjectSet(me *Identity, projectID uuid.UUID) (permission.Set, error) {
	base := BaseProjectSet(me)
	if me == nil || base.All {
		return base, nil
	}
	a, err := r.client.Allocation.Query().
		Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(me.PersonID)).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return base, nil
	}
	if err != nil {
		return permission.Set{}, err
	}
	return withGroup(base, a.Permissions), nil
}

// ProjectAccess diz se a pessoa está no projeto e o que ela pode nele. Os admins estão em todos, com o que o cargo
// dá (BaseProjectSet) mais o grupo da alocação, se tiverem uma. Quem não é admin só está nos projetos em que tem
// valor por hora (projectaccess), e as permissões vêm do que a alocação dele libera.
func (r *Resolver) ProjectAccess(me *Identity, projectID uuid.UUID) (set permission.Set, in bool, err error) {
	if me == nil {
		return permission.Set{}, false, nil
	}
	if me.IsAdmin() {
		set, err = r.ProjectSet(me, projectID)
		return set, true, err
	}
	ctx := context.Background()
	a, err := r.client.Allocation.Query().
		Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(me.PersonID)).
		Only(ctx)
	if err == nil {
		return permission.Set{Keys: permission.Normalize(a.Permissions, permission.ProjectKeys)}, true, nil
	}
	if !ent.IsNotFound(err) {
		return permission.Set{}, false, err
	}
	in, err = projectaccess.Has(ctx, r.client, me.PersonID, projectID)
	return permission.Set{}, in, err
}

// PeopleScope diz até onde a pessoa logada vê as outras da organização: nil, todas (admins, quem cuida de pessoas
// e quem pode pôr gente em algum projeto em que está, que precisa achar quem entra); o id dela, só ela mesma e
// quem está em algum projeto dela.
func (m *Middleware) PeopleScope(c *echo.Context) (*uuid.UUID, error) {
	me := CurrentPerson(c)
	if me == nil {
		return nil, ErrUnauthenticated
	}
	if me.IsAdmin() || me.Can(permission.PeopleManage) {
		return nil, nil
	}
	manages, err := projectaccess.ManagesPeople(c.Request().Context(), m.resolver.client, me.PersonID)
	if err != nil {
		return nil, err
	}
	if manages {
		return nil, nil
	}
	id := me.PersonID
	return &id, nil
}

// SameOrganization diz se o recurso existe e é da organização da pessoa logada.
func (r *Resolver) SameOrganization(c *echo.Context, kind Kind, id string) bool {
	return r.sameOrganization(CurrentPerson(c), kind, id)
}

func (r *Resolver) sameOrganization(me *Identity, kind Kind, id string) bool {
	if me == nil {
		return false
	}
	orgID, err := r.OrganizationOf(kind, id)
	return err == nil && orgID == me.OrganizationID
}

// Reach é a checagem do RequireOrg sem o echo, para quem age em nome da pessoa fora das rotas da API (o servidor MCP):
// o recurso existe e é da organização dela e, num recurso de projeto, quem não é dono está no projeto. Quem não
// alcança deve receber a mesma resposta de um recurso que não existe.
func (r *Resolver) Reach(me *Identity, kind Kind, id string) (bool, error) {
	_, _, ok, err := r.reach(me, kind, id)
	return ok, err
}

// reach faz a checagem. scoped diz que o conjunto veio do projeto do recurso; sem ele, quem chama usa o do cargo.
func (r *Resolver) reach(me *Identity, kind Kind, id string) (set permission.Set, scoped, ok bool, err error) {
	if !r.sameOrganization(me, kind, id) {
		return permission.Set{}, false, false, nil
	}
	// Num recurso de projeto, quem não é admin só passa se estiver no projeto. Quem não está recebe a mesma resposta
	// de um projeto que não existe, para não revelar que ele existe. Os admins estão em todos os projetos, mas com o
	// que o cargo dá a eles (sem a cobrança), então também têm o conjunto montado aqui. Só o dono tem tudo em todos:
	// para ele nada se consulta.
	if me.IsOwner {
		return permission.Set{}, false, true, nil
	}
	// Uma falha ao ler o projeto do recurso não deixa passar sem a checagem: o recurso que sumiu no meio é como um que
	// não existe, e o erro do banco volta para quem chama.
	projectID, err := r.ProjectOf(kind, id)
	if ent.IsNotFound(err) {
		return permission.Set{}, false, false, nil
	}
	if err != nil {
		return permission.Set{}, false, false, err
	}
	if projectID == uuid.Nil {
		return permission.Set{}, false, true, nil
	}
	set, in, err := r.ProjectAccess(me, projectID)
	if err != nil {
		return permission.Set{}, false, false, err
	}
	return set, true, in, nil
}

// RequireOrg responde 404 quando o recurso do parâmetro não existe ou é de outra
// organização. É 404 e não 403 para não revelar que o recurso existe.
func (m *Middleware) RequireOrg(kind Kind, param string) echo.MiddlewareFunc {
	return m.requireOrg(kind, param, func(c *echo.Context) error {
		return apperr.Respond(c, http.StatusNotFound, apperr.ErrNotFound)
	})
}

// RequireOrgPage faz a mesma checagem para páginas, com a resposta de 404 dada.
func (m *Middleware) RequireOrgPage(kind Kind, param string, notFound echo.HandlerFunc) echo.MiddlewareFunc {
	return m.requireOrg(kind, param, notFound)
}

func (m *Middleware) requireOrg(kind Kind, param string, deny echo.HandlerFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// A regra está no reach, que o MCP também usa. Num recurso de projeto, as permissões da pessoa nele
			// ficam à mão das rotas e dos handlers que vêm depois; num recurso sem projeto o conjunto não muda (uma
			// rota com dois parâmetros, como o projeto e a pessoa, guarda o do projeto).
			set, scoped, ok, err := m.resolver.reach(CurrentPerson(c), kind, c.Param(param))
			if err != nil {
				return err
			}
			if !ok {
				return deny(c)
			}
			if scoped {
				c.Set(projectSetKey, set)
			}
			return next(c)
		}
	}
}
