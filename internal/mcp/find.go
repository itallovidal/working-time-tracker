package mcp

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/validate"
)

// A busca de tarefas do MCP. O list_tasks olha um projeto e só o nome; a find_tasks olha todos os projetos que a
// pessoa alcança, pela issue (ou pelo cartão) ligada à tarefa ou por palavras no nome e na descrição. É o que uma
// skill usa para achar a tarefa de "#12" sem depender do título.

type findTasksInput struct {
	Issue     string   `json:"issue,omitempty" jsonschema:"A GitHub or GitLab issue or a Trello card: its URL, or owner/repo#12 (group/project#12 on GitLab). Finds the tasks linked to it."`
	Query     string   `json:"query,omitempty" jsonschema:"Words that must all appear in the task name or description, in any order, case-insensitive."`
	ProjectID string   `json:"project_id,omitempty" jsonschema:"Only this project. Without it, every project the person reaches."`
	Statuses  []string `json:"statuses,omitempty" jsonschema:"Only tasks in one of these statuses. Without it, every status."`
	Assignee  string   `json:"assignee,omitempty" jsonschema:"me (yours), none (no assignee), any (someone's) or others (someone else's). Without it, everyone's."`
	Page      int      `json:"page,omitempty" jsonschema:"Page number, from 1."`
	PerPage   int      `json:"per_page,omitempty" jsonschema:"Tasks per page, up to 100. Defaults to 20."`
}

// FoundTasksOut é uma página do que a find_tasks achou. Cada tarefa traz o projeto.
type FoundTasksOut struct {
	Items   []TaskOut `json:"items"`
	Total   int       `json:"total"`
	Page    int       `json:"page"`
	PerPage int       `json:"per_page"`
}

func (s *server) addFindTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "find_tasks",
		Description: "Find tasks across every project the person reaches: by the GitHub or GitLab issue or Trello card linked to them " +
			"(its URL, or owner/repo#12), or by words in the name or description. Give issue, query or both. " +
			"An issue with no task may not have been imported yet: sync_project brings it in.",
		InputSchema: schemaFor[findTasksInput](map[string][]string{
			"statuses": task.Statuses, "assignee": {"me", "none", "any", "others"},
		}),
		Annotations: readOnly("Find tasks"),
	}, s.findTasks)
}

func (s *server) findTasks(_ context.Context, req *mcp.CallToolRequest, in findTasksInput) (*mcp.CallToolResult, FoundTasksOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, FoundTasksOut{}, s.fail(err)
	}
	issue, query := strings.TrimSpace(in.Issue), strings.TrimSpace(in.Query)
	if issue == "" && query == "" {
		return nil, FoundTasksOut{}, s.fail(ErrFindCriteria)
	}
	if len([]rune(query)) > validate.MaxSearch {
		return nil, FoundTasksOut{}, s.fail(task.ErrQueryTooLong.With("max", validate.MaxSearch, "field", "query"))
	}
	f := task.ListFilter{Words: strings.Fields(query), Statuses: in.Statuses}
	if err := assigneeFilter(&f, in.Assignee, id); err != nil {
		return nil, FoundTasksOut{}, s.fail(err)
	}
	var ref itemRef
	if issue != "" {
		var ok bool
		if ref, ok = parseItemRef(issue); !ok {
			return nil, FoundTasksOut{}, s.fail(ErrInvalidIssue)
		}
		f.ItemID = ref.item
	}
	// Num projeto dado vale a checagem de sempre; sem ele, quem não é admin só vê os projetos em que está.
	var projectID *uuid.UUID
	switch {
	case in.ProjectID != "":
		if err := s.reachProject(id, in.ProjectID); err != nil {
			return nil, FoundTasksOut{}, err
		}
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return nil, FoundTasksOut{}, s.fail(project.ErrNotFound)
		}
		projectID = &pid
	case !id.IsOwner && !id.IsAdmin():
		f.InProjectsOf = &id.PersonID
	}

	page := max(in.Page, 1)
	perPage := in.PerPage
	if perPage <= 0 {
		perPage = defaultPerPage
	}
	perPage = min(perPage, task.MaxPerPage)
	var items []task.Assigned
	var total int
	if issue == "" {
		f.Page, f.PerPage = page, perPage
		if items, total, err = s.Tasks.Search(id.OrganizationID, projectID, f); err != nil {
			return nil, FoundTasksOut{}, s.fail(err)
		}
	} else {
		// O mesmo número de issue existe em vários repositórios: a consulta traz as tarefas com a chave, e a URL do
		// vínculo diz qual é a do repositório pedido. São poucas, então a página sai daqui.
		all, _, err := s.Tasks.Search(id.OrganizationID, projectID, f)
		if err != nil {
			return nil, FoundTasksOut{}, s.fail(err)
		}
		var matched []task.Assigned
		for _, a := range all {
			if linkedTo(a.Links, ref) {
				matched = append(matched, a)
			}
		}
		total = len(matched)
		start := min((page-1)*perPage, total)
		items = matched[start:min(start+perPage, total)]
	}

	loc := s.location(id)
	out := FoundTasksOut{Items: []TaskOut{}, Total: total, Page: page, PerPage: perPage}
	for i := range items {
		t := s.taskOut(&items[i].Task, id, loc, false)
		t.Project = &Ref{ID: items[i].ProjectID.String(), Name: items[i].ProjectName}
		out.Items = append(out.Items, t)
	}
	return nil, out, nil
}

// assigneeFilter põe no filtro o responsável que o Claude pediu: me, none, any ou others; vazio é de todos.
func assigneeFilter(f *task.ListFilter, assignee string, id *auth.Identity) error {
	switch assignee {
	case "":
	case "me":
		f.AssigneeID = &id.PersonID
	case "none":
		f.Unassigned = true
	case "any":
		f.Assigned = true
	case "others":
		f.OthersOf = &id.PersonID
	default:
		return task.ErrInvalidAssigneeFilter.With("field", "assignee")
	}
	return nil
}

// itemRef é um item de plataforma como a pessoa o cita, e também como o vínculo o guarda (a URL dele).
type itemRef struct {
	host string // github.com, gitlab.com, trello.com; vazio na forma curta
	repo string // owner/repo ou group/sub/project, em minúsculas; vazio no Trello
	item string // o número da issue ou o link curto do cartão (este diferencia maiúsculas)
}

// parseItemRef lê a URL de uma issue do GitHub (owner/repo/issues/12) ou do GitLab (group/project/-/issues/12, ou
// work_items), a de um cartão do Trello (/c/<link curto>, com ou sem o nome depois), com ou sem https://, ou a forma
// curta owner/repo#12.
func parseItemRef(raw string) (itemRef, bool) {
	s := strings.TrimSpace(raw)
	if !strings.Contains(s, "://") {
		// Sem um domínio no começo (github.com/…), é a forma curta.
		if repo, num, ok := strings.Cut(s, "#"); ok {
			repo = strings.Trim(repo, "/")
			if first, _, _ := strings.Cut(repo, "/"); !strings.Contains(first, ".") {
				if !strings.Contains(repo, "/") || !isNumber(num) {
					return itemRef{}, false
				}
				return itemRef{repo: strings.ToLower(repo), item: num}, true
			}
		}
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return itemRef{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if host == "trello.com" {
		if len(parts) >= 2 && parts[0] == "c" && parts[1] != "" {
			return itemRef{host: host, item: parts[1]}, true
		}
		return itemRef{}, false
	}
	n := len(parts)
	if n < 4 || (parts[n-2] != "issues" && parts[n-2] != "work_items") || !isNumber(parts[n-1]) {
		return itemRef{}, false
	}
	repo := parts[:n-2]
	if repo[len(repo)-1] == "-" {
		repo = repo[:len(repo)-1]
	}
	if len(repo) < 2 {
		return itemRef{}, false
	}
	return itemRef{host: host, repo: strings.ToLower(strings.Join(repo, "/")), item: parts[n-1]}, true
}

// linkedTo diz se algum vínculo da tarefa é o item pedido: a mesma chave no mesmo repositório (e no mesmo endereço,
// quando a pessoa deu a URL).
func linkedTo(links []task.Link, ref itemRef) bool {
	for _, l := range links {
		got, ok := parseItemRef(l.URL)
		if ok && got.item == ref.item && got.repo == ref.repo && (ref.host == "" || got.host == ref.host) {
			return true
		}
	}
	return false
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
