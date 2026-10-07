package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	entalloc "working-time-tracker/ent/allocation"
	"working-time-tracker/ent/integration"
	"working-time-tracker/ent/task"
	"working-time-tracker/ent/team"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/permission"
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

// ProjectSet devolve o que a pessoa pode fazer no projeto: tudo, para os admins, e o que
// a alocação dela no projeto libera, para os outros.
func (r *Resolver) ProjectSet(me *Identity, projectID uuid.UUID) (permission.Set, error) {
	if me.IsAdmin() {
		return permission.Set{All: true}, nil
	}
	if me == nil {
		return permission.Set{}, nil
	}
	a, err := r.client.Allocation.Query().
		Where(entalloc.ProjectIDEQ(projectID), entalloc.PersonIDEQ(me.PersonID)).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return permission.Set{}, nil
	}
	if err != nil {
		return permission.Set{}, err
	}
	return permission.Set{Keys: permission.Normalize(a.Permissions, permission.ProjectKeys)}, nil
}

// SameOrganization diz se o recurso existe e é da organização da pessoa logada.
func (r *Resolver) SameOrganization(c *echo.Context, kind Kind, id string) bool {
	me := CurrentPerson(c)
	if me == nil {
		return false
	}
	orgID, err := r.OrganizationOf(kind, id)
	return err == nil && orgID == me.OrganizationID
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
			if !m.resolver.SameOrganization(c, kind, c.Param(param)) {
				return deny(c)
			}
			// Num recurso de projeto, as permissões da pessoa nele ficam à mão das rotas e dos
			// handlers que vêm depois. Os admins têm tudo em todos os projetos: para eles
			// nada se consulta.
			if me := CurrentPerson(c); !me.IsAdmin() {
				if projectID, err := m.resolver.ProjectOf(kind, c.Param(param)); err == nil && projectID != uuid.Nil {
					set, err := m.resolver.ProjectSet(me, projectID)
					if err != nil {
						return err
					}
					c.Set(projectSetKey, set)
				}
			}
			return next(c)
		}
	}
}
