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
	"working-time-tracker/internal/domain/work_session"
)

// As ferramentas do ponto. O ponto é sempre o da pessoa do token, e a sessão é sempre a aberta dela: não há como bater
// o ponto de outra pessoa nem mexer na sessão de alguém. As regras são do work_session: uma sessão aberta por pessoa,
// valor por hora no projeto, a tarefa de outro projeto não entra, a última tarefa ativa não sai.

type switchInput struct {
	TaskID string `json:"task_id" jsonschema:"The task id. It must be in the project of the open session."`
	Switch bool   `json:"switch,omitempty" jsonschema:"Stop the other running tasks of the session, so only this one runs (switching tasks)."`
}

// ActiveOut é a sessão aberta, ou nada.
type ActiveOut struct {
	Open    bool        `json:"open"`
	Session *SessionOut `json:"session"`
}

func (s *server) addSessionTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_active_session",
		Description: "The person's open work session, with each task's stretch in it and whether it is running. open is false when the clock is not running.",
		Annotations: readOnly("Get active session"),
	}, s.getActiveSession)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "clock_in",
		Description: "Start the clock on a task (opens a work session in the task's project). The task becomes in_progress and, if it had no assignee, the person's. Fails if a session is already open.",
		Annotations: write("Clock in", false),
	}, s.clockIn)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_task_to_session",
		Description: "Add another task of the same project to the open session, starting now. With switch=true the other running tasks stop, so the person switches to this one.",
		Annotations: write("Add task to session", false),
	}, s.addTaskToSession)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "stop_session_task",
		Description: "Stop a running task in the open session now; its time stays. The last running task cannot stop: use clock_out.",
		Annotations: write("Stop task in session", true),
	}, s.stopSessionTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "remove_session_task",
		Description: "Take a task out of the open session, with its stretch (the session's own time stays). The session keeps at least one running task.",
		Annotations: write("Remove task from session", false),
	}, s.removeSessionTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "clock_out",
		Description: "Stop the clock: close the person's open work session. The tasks stay in_progress.",
		Annotations: write("Clock out", false),
	}, s.clockOut)
}

func (s *server) getActiveSession(_ context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, ActiveOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, ActiveOut{}, s.fail(err)
	}
	open, err := s.Sessions.Active(id.PersonID.String())
	if err != nil {
		return nil, ActiveOut{}, s.fail(err)
	}
	if open == nil {
		return nil, ActiveOut{}, nil
	}
	out := s.sessionOut(open, id)
	return nil, ActiveOut{Open: true, Session: &out}, nil
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

// openHint diz ao Claude onde está a sessão aberta e o que fazer: pôr a tarefa nela, se é do mesmo projeto, ou
// encerrar o ponto antes.
func (s *server) openHint(id *auth.Identity, projectID string) string {
	open, err := s.Sessions.Active(id.PersonID.String())
	if err != nil || open == nil {
		return ""
	}
	names := make([]string, 0, len(open.Tasks))
	for _, st := range open.Tasks {
		if st.UntilAt == nil && st.Task != nil {
			names = append(names, st.Task.Name)
		}
	}
	where := fmt.Sprintf("The open session is in project %q, running: %s.", s.projectName(open.ProjectID.String()), strings.Join(names, ", "))
	if open.ProjectID.String() == projectID {
		return where + " Use add_task_to_session (switch=true to switch tasks)."
	}
	return where + " That is another project: clock_out first, then clock_in."
}

func (s *server) addTaskToSession(_ context.Context, req *mcp.CallToolRequest, in switchInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	open, err := s.openSession(id)
	if err != nil {
		return nil, SessionOut{}, err
	}
	t, err := s.reachTask(id, in.TaskID)
	if err != nil {
		return nil, SessionOut{}, err
	}
	projectID, sessionID := open.ProjectID.String(), open.ID.String()
	ws := open
	if running := runningLink(open, t.ID); running == nil {
		ws, err = s.Sessions.AddTask(projectID, sessionID, t.ID.String(), nil, nil)
		if errors.Is(err, work_session.ErrTaskOtherProject) {
			return nil, SessionOut{}, s.failWith(err, "The open session is in another project: clock_out first, then clock_in on this task.")
		}
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
	open, err := s.openSession(id)
	if err != nil {
		return nil, SessionOut{}, err
	}
	t, err := s.reachTask(id, in.TaskID)
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
	open, err := s.openSession(id)
	if err != nil {
		return nil, SessionOut{}, err
	}
	t, err := s.reachTask(id, in.TaskID)
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

func (s *server) clockOut(_ context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, SessionOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	open, err := s.openSession(id)
	if err != nil {
		return nil, SessionOut{}, err
	}
	ws, err := s.Sessions.ClockOut(open.ProjectID.String(), id.PersonID.String())
	if err != nil {
		return nil, SessionOut{}, s.fail(err)
	}
	return nil, s.sessionOut(ws, id), nil
}

// openSession é a sessão aberta da pessoa, para as ferramentas que mexem nela, ou work_session.not_open. Como as rotas
// da sessão na API, ela passa pela checagem do projeto: quem saiu do projeto com o ponto aberto recebe
// project.not_found, e só um admin encerra aquela sessão. Ler a sessão (get_active_session, whoami) não passa por aqui.
func (s *server) openSession(id *auth.Identity) (*work_session.WorkSession, error) {
	open, err := s.Sessions.Active(id.PersonID.String())
	if err != nil {
		return nil, s.fail(err)
	}
	if open == nil {
		return nil, s.failWith(work_session.ErrNotOpen, "Use clock_in to start the clock on a task.")
	}
	if err := s.reachProject(id, open.ProjectID.String()); err != nil {
		return nil, err
	}
	return open, nil
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
