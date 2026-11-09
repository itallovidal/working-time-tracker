package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
)

// As ferramentas do ponto. O ponto é sempre o da pessoa do token, e as sessões são sempre as abertas dela: não há como
// bater o ponto de outra pessoa nem mexer na sessão de alguém. As regras são do work_session: uma sessão aberta por
// pessoa em cada projeto (Decisão 26), valor por hora no projeto, a tarefa de outro projeto não entra na sessão deste,
// a última tarefa ativa não sai. As ferramentas que mexem numa tarefa usam a sessão aberta do projeto dela.

type switchInput struct {
	TaskID string `json:"task_id" jsonschema:"The task id. It goes into the open session of its project."`
	Switch bool   `json:"switch,omitempty" jsonschema:"Stop the other running tasks of the session, so only this one runs (switching tasks)."`
}

// ActiveOut são as sessões abertas, uma por projeto no máximo.
type ActiveOut struct {
	Open     bool         `json:"open"`
	Sessions []SessionOut `json:"sessions"`
}

// clockOutInput escolhe o projeto do ponto que fecha. Só é preciso quando há sessões abertas em mais de um projeto.
type clockOutInput struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"The project whose session to close. Needed only when sessions are open in more than one project."`
}

func (s *server) addSessionTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_active_session",
		Description: "The person's open work sessions, at most one per project, each with its tasks' stretches and whether they are running. open is false when no clock is running.",
		Annotations: readOnly("Get active session"),
	}, s.getActiveSession)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "clock_in",
		Description: "Start the clock on a task (opens a work session in the task's project). Sessions in other projects keep running. The task becomes in_progress and, if it had no assignee, the person's. Fails if a session is already open in this project: use add_task_to_session.",
		Annotations: write("Clock in", false),
	}, s.clockIn)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_task_to_session",
		Description: "Add a task to the open session of its project, starting now. With switch=true the other running tasks of that session stop, so the person switches to this one.",
		Annotations: write("Add task to session", false),
	}, s.addTaskToSession)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "stop_session_task",
		Description: "Stop a running task in the open session of its project now; its time stays. The last running task cannot stop: use clock_out.",
		Annotations: write("Stop task in session", true),
	}, s.stopSessionTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "remove_session_task",
		Description: "Take a task out of the open session of its project, with its stretch (the session's own time stays). The session keeps at least one running task.",
		Annotations: write("Remove task from session", false),
	}, s.removeSessionTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "clock_out",
		Description: "Stop the clock: close the person's open work session in a project; sessions in other projects keep running. project_id is needed only when sessions are open in more than one project. The tasks stay in_progress.",
		Annotations: write("Clock out", false),
	}, s.clockOut)
}

func (s *server) getActiveSession(_ context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, ActiveOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, ActiveOut{}, s.fail(err)
	}
	open, err := s.Sessions.Open(id.PersonID.String())
	if err != nil {
		return nil, ActiveOut{}, s.fail(err)
	}
	out := ActiveOut{Open: len(open) > 0, Sessions: make([]SessionOut, 0, len(open))}
	for i := range open {
		out.Sessions = append(out.Sessions, s.sessionOut(&open[i], id))
	}
	return nil, out, nil
}

func (s *server) clockIn(_ context.Context, req *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	t, err := s.reachTask(id, in.TaskID)
	if err != nil {
		return nil, SessionOut{}, err
	}
	ws, err := s.Sessions.ClockIn(t.ProjectID.String(), t.ID.String(), id.PersonID.String())
	if errors.Is(err, work_session.ErrAlreadyOpen) {
		return nil, SessionOut{}, s.failWith(err, s.openHint(id, t.ProjectID.String()))
	}
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	return nil, s.sessionOut(ws, id), nil
}

// openHint diz ao Claude o que já corre na sessão aberta do projeto e como pôr a tarefa nela.
func (s *server) openHint(id *auth.Identity, projectID string) string {
	open, err := s.Sessions.OpenIn(id.PersonID.String(), projectID)
	if err != nil || open == nil {
		return ""
	}
	names := make([]string, 0, len(open.Tasks))
	for _, st := range open.Tasks {
		if st.UntilAt == nil && st.Task != nil {
			names = append(names, st.Task.Name)
		}
	}
	return fmt.Sprintf("Project %q already has an open session, running: %s. Use add_task_to_session (switch=true to switch tasks).", s.projectName(projectID), strings.Join(names, ", "))
}

func (s *server) addTaskToSession(_ context.Context, req *mcp.CallToolRequest, in switchInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	open, t, err := s.sessionOfTask(id, in.TaskID)
	if err != nil {
		return nil, SessionOut{}, err
	}
	projectID, sessionID := open.ProjectID.String(), open.ID.String()
	ws := open
	if running := runningLink(open, t.ID); running == nil {
		ws, err = s.Sessions.AddTask(projectID, sessionID, t.ID.String(), nil, nil)
		if err != nil {
			return nil, SessionOut{}, s.fail(err)
		}
	} else if !in.Switch {
		return nil, SessionOut{}, s.fail(ErrTaskAlreadyActive)
	}
	if in.Switch {
		for _, st := range ws.Tasks {
			if st.UntilAt == nil && st.TaskID != t.ID {
				if ws, err = s.Sessions.UpdateTask(projectID, sessionID, st.ID.String(), work_session.TaskChange{Stop: true}); err != nil {
					return nil, SessionOut{}, s.fail(err)
				}
			}
		}
	}
	return nil, s.sessionOut(ws, id), nil
}

func (s *server) stopSessionTask(_ context.Context, req *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	open, t, err := s.sessionOfTask(id, in.TaskID)
	if err != nil {
		return nil, SessionOut{}, err
	}
	link := runningLink(open, t.ID)
	if link == nil {
		return nil, SessionOut{}, s.fail(ErrTaskNotInSession)
	}
	ws, err := s.Sessions.UpdateTask(open.ProjectID.String(), open.ID.String(), link.ID.String(), work_session.TaskChange{Stop: true})
	if errors.Is(err, work_session.ErrLastTask) {
		return nil, SessionOut{}, s.failWith(err, "To stop everything, use clock_out.")
	}
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	return nil, s.sessionOut(ws, id), nil
}

func (s *server) removeSessionTask(_ context.Context, req *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	open, t, err := s.sessionOfTask(id, in.TaskID)
	if err != nil {
		return nil, SessionOut{}, err
	}
	// O trecho que sai é o que está rodando; sem ele, o mais recente da tarefa.
	link := runningLink(open, t.ID)
	if link == nil {
		for i := range open.Tasks {
			if open.Tasks[i].TaskID == t.ID {
				link = &open.Tasks[i]
			}
		}
	}
	if link == nil {
		return nil, SessionOut{}, s.fail(ErrTaskNotInSession)
	}
	ws, err := s.Sessions.RemoveTask(open.ProjectID.String(), open.ID.String(), link.ID.String())
	if errors.Is(err, work_session.ErrLastTask) {
		return nil, SessionOut{}, s.failWith(err, "To stop everything, use clock_out.")
	}
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	return nil, s.sessionOut(ws, id), nil
}

func (s *server) clockOut(_ context.Context, req *mcp.CallToolRequest, in clockOutInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	projectID := in.ProjectID
	if projectID == "" {
		open, err := s.Sessions.Open(id.PersonID.String())
		if err != nil {
			return nil, SessionOut{}, s.fail(err)
		}
		switch len(open) {
		case 0:
			return nil, SessionOut{}, s.failWith(work_session.ErrNotOpen, "Use clock_in to start the clock on a task.")
		case 1:
			projectID = open[0].ProjectID.String()
		default:
			names := make([]string, 0, len(open))
			for i := range open {
				names = append(names, fmt.Sprintf("%q (%s)", s.projectName(open[i].ProjectID.String()), open[i].ProjectID))
			}
			return nil, SessionOut{}, s.failWith(ErrProjectRequired, "Sessions are open in: "+strings.Join(names, ", ")+". Call clock_out again with project_id.")
		}
	}
	open, err := s.openSession(id, projectID)
	if err != nil {
		return nil, SessionOut{}, err
	}
	ws, err := s.Sessions.ClockOut(open.ProjectID.String(), id.PersonID.String())
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	return nil, s.sessionOut(ws, id), nil
}

// openSession é a sessão aberta da pessoa no projeto, para as ferramentas que mexem nela, ou work_session.not_open.
// Como as rotas da sessão na API, ela passa pela checagem do projeto: quem saiu do projeto com o ponto aberto recebe
// project.not_found, e só um admin encerra aquela sessão. Ler as sessões (get_active_session, whoami) não passa por
// aqui.
func (s *server) openSession(id *auth.Identity, projectID string) (*work_session.WorkSession, error) {
	if err := s.reachProject(id, projectID); err != nil {
		return nil, err
	}
	open, err := s.Sessions.OpenIn(id.PersonID.String(), projectID)
	if err != nil {
		return nil, s.fail(err)
	}
	if open == nil {
		return nil, s.failWith(work_session.ErrNotOpen, "This project has no open session. Use clock_in to start the clock on a task.")
	}
	return open, nil
}

// sessionOfTask é a sessão aberta da pessoa no projeto da tarefa, com a tarefa, para as ferramentas que mexem numa
// tarefa da sessão. Quando a tarefa já está numa sessão aberta, o projeto dela é checado antes da tarefa: quem saiu do
// projeto com o ponto aberto recebe project.not_found, como em openSession. Senão, a tarefa é checada e a sessão é a
// do projeto dela (work_session.not_open quando não há).
func (s *server) sessionOfTask(id *auth.Identity, taskID string) (*work_session.WorkSession, *task.Task, error) {
	if tid, err := uuid.Parse(taskID); err == nil {
		open, err := s.Sessions.Open(id.PersonID.String())
		if err != nil {
			return nil, nil, s.fail(err)
		}
		for i := range open {
			if open[i].HasTask(tid) {
				if err := s.reachProject(id, open[i].ProjectID.String()); err != nil {
					return nil, nil, err
				}
				break
			}
		}
	}
	t, err := s.reachTask(id, taskID)
	if err != nil {
		return nil, nil, err
	}
	open, err := s.openSession(id, t.ProjectID.String())
	if err != nil {
		return nil, nil, err
	}
	return open, t, nil
}

// runningLink é o trecho sem fim da tarefa na sessão, ou nil.
func runningLink(ws *work_session.WorkSession, taskID uuid.UUID) *work_session.SessionTask {
	for i := range ws.Tasks {
		if ws.Tasks[i].TaskID == taskID && ws.Tasks[i].UntilAt == nil {
			return &ws.Tasks[i]
		}
	}
	return nil
}

func (s *server) sessionOut(ws *work_session.WorkSession, id *auth.Identity) SessionOut {
	return sessionOut(ws, s.projectName(ws.ProjectID.String()), s.location(id), time.Now())
}
