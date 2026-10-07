package work_session

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	taskdom "working-time-tracker/internal/domain/task"
)

type Service struct {
	sessionStore *Store
	taskStore    *taskdom.Store
	rates        *allocation.Store
}

func NewService(sessionStore *Store, taskStore *taskdom.Store, rates *allocation.Store) *Service {
	return &Service{sessionStore: sessionStore, taskStore: taskStore, rates: rates}
}

func (s *Service) ClockIn(projectID, taskID, personID string) (*WorkSession, error) {
	task, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if task.ProjectID.String() != projectID {
		return nil, ErrTaskOtherProject
	}

	personUID, err := uuid.Parse(personID)
	if err != nil {
		return nil, ErrPersonNotFound
	}
	inOrg, err := s.sessionStore.PersonInProjectOrganization(personID, projectID)
	if err != nil {
		return nil, err
	}
	if !inOrg {
		return nil, ErrPersonNotInOrg
	}

	// O valor por hora é conferido aqui e copiado para a sessão: quem não tem
	// valor no projeto não bate ponto, e uma mudança de valor depois não altera
	// esta sessão.
	rates, err := s.rates.Rates(personUID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	if !rates.Found {
		return nil, ErrNoRate
	}

	_, err = s.sessionStore.GetActiveByPerson(personID)
	if err == nil {
		return nil, ErrAlreadyOpen
	}
	if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	session := &WorkSession{
		ProjectID:     task.ProjectID,
		PersonID:      personUID,
		StartAt:       time.Now(),
		OwnerHours:    rates.Owner,
		PayRateCents:  &rates.PayRateCents,
		BillRateCents: rates.BillRateCents,
	}
	if err := s.sessionStore.Create(session, task.ID); err != nil {
		// Duas requisições simultâneas passam pela checagem acima; o índice
		// one_active_session barra a segunda aqui.
		if ent.IsConstraintError(err) {
			return nil, ErrAlreadyOpen
		}
		return nil, err
	}
	// Bater o ponto numa tarefa sem responsável a torna de quem bateu. Fica depois
	// da sessão criada para um ponto recusado não levar a tarefa.
	if task.AssigneeID == nil {
		if err := s.taskStore.ClaimIfUnassigned(task.ID, personUID); err != nil {
			return nil, err
		}
	}
	// E a tarefa em que se bateu o ponto passa a estar em progresso, de qualquer status. O
	// clock-out não a tira de lá: parar ou pausar não é terminar.
	if err := s.taskStore.StartProgress(task.ID); err != nil {
		return nil, err
	}
	session.Tasks[0].Task = &Task{ID: task.ID, Name: task.Name, ProjectID: task.ProjectID}
	session.fillAmounts(session.StartAt)
	return session, nil
}

// Active devolve a sessão aberta da pessoa, ou nil quando ela não está com o ponto aberto.
func (s *Service) Active(personID string) (*WorkSession, error) {
	active, err := s.sessionStore.GetActiveByPerson(personID)
	if errors.Is(err, database.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	active.fillAmounts(time.Now())
	return active, nil
}

// ClockOut fecha a sessão aberta da pessoa. Como o person_id vem do corpo, a
// pessoa precisa ser da organização do projeto da rota, como no ClockIn.
func (s *Service) ClockOut(projectID, personID string) (*WorkSession, error) {
	inOrg, err := s.sessionStore.PersonInProjectOrganization(personID, projectID)
	if err != nil {
		return nil, err
	}
	if !inOrg {
		return nil, ErrPersonNotInOrg
	}

	active, err := s.sessionStore.GetActiveByPerson(personID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotOpen
		}
		return nil, err
	}

	now := time.Now()
	active.EndAt = &now
	if err := s.sessionStore.Update(active); err != nil {
		return nil, err
	}
	active.fillAmounts(now)
	return active, nil
}

func (s *Service) ListByProject(projectID string, taskID, personID *string) ([]WorkSession, error) {
	sessions, err := s.sessionStore.ListByProject(projectID, taskID, personID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range sessions {
		sessions[i].fillAmounts(now)
	}
	return sessions, nil
}

// ListByOrganization devolve as sessões de todos os projetos da organização, sem as tarefas e
// sem os valores calculados: quem soma usa Within, que corta cada sessão na janela que quer.
func (s *Service) ListByOrganization(orgID string) ([]WorkSession, error) {
	return s.sessionStore.ListByOrganization(orgID)
}

// ListOpenByOrganization devolve as sessões abertas da organização com as tarefas de cada uma.
func (s *Service) ListOpenByOrganization(orgID string) ([]WorkSession, error) {
	return s.sessionStore.ListOpenByOrganization(orgID)
}

// TotalTimeResult soma as sessões de um filtro. Os dois valores ficam nil quando
// nenhuma sessão do filtro tem valor por hora.
type TotalTimeResult struct {
	TotalSeconds    float64 `json:"total_seconds"`
	PayAmountCents  *int    `json:"pay_amount_cents"`
	BillAmountCents *int    `json:"bill_amount_cents"`
}

// TotalTime soma as sessões do projeto filtradas por tarefa, por pessoa ou pelas
// duas. O filtro pelo projeto impede que uma tarefa de outra organização seja
// somada pela rota deste. Com tarefa, soma o tempo que ela teve nas sessões (as tarefas
// em paralelo contam o tempo cheio, cada uma); só com pessoa, o tempo das sessões dela.
func (s *Service) TotalTime(projectID string, taskID, personID *string) (*TotalTimeResult, error) {
	hasTask := taskID != nil && *taskID != ""
	hasPerson := personID != nil && *personID != ""
	if !hasTask && !hasPerson {
		return nil, ErrFilterRequired
	}
	var taskUID uuid.UUID
	if hasTask {
		var err error
		if taskUID, err = uuid.Parse(*taskID); err != nil {
			return nil, ErrInvalidTaskFilter
		}
	}
	if hasPerson {
		if _, err := uuid.Parse(*personID); err != nil {
			return nil, ErrInvalidPersonFilter
		}
	}
	sessions, err := s.ListByProject(projectID, taskID, personID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	total := &TotalTimeResult{}
	for i := range sessions {
		if !hasTask {
			total.TotalSeconds += sessions[i].Seconds(now)
			total.PayAmountCents = add(total.PayAmountCents, sessions[i].PayAmountCents)
			total.BillAmountCents = add(total.BillAmountCents, sessions[i].BillAmountCents)
			continue
		}
		for _, l := range sessions[i].Tasks {
			if l.TaskID != taskUID {
				continue
			}
			total.TotalSeconds += l.Seconds
			total.PayAmountCents = add(total.PayAmountCents, l.PayAmountCents)
			total.BillAmountCents = add(total.BillAmountCents, l.BillAmountCents)
		}
	}
	return total, nil
}

// add soma dois valores opcionais: nil só quando os dois são nil.
func add(sum, v *int) *int {
	if v == nil {
		return sum
	}
	if sum == nil {
		return v
	}
	total := *sum + *v
	return &total
}

// Get devolve a sessão do projeto, com as tarefas e os valores calculados até agora, ou
// ErrSessionNotFound quando ela não existe ou é de outro projeto.
func (s *Service) Get(projectID, sessionID string) (*WorkSession, error) {
	session, err := s.sessionStore.GetByID(sessionID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	if session.ProjectID.String() != projectID {
		return nil, ErrSessionNotFound
	}
	session.fillAmounts(time.Now())
	return session, nil
}

// AddTask põe uma tarefa do projeto na sessão. Numa sessão aberta ela entra agora, passa a
// estar em progresso e, se não tinha responsável, vira de quem está com o ponto aberto, como
// no clock-in; numa sessão já encerrada entra do início ao fim, e o status da tarefa não
// muda. from e until, quando vêm, escolhem o intervalo.
func (s *Service) AddTask(projectID, sessionID, taskID string, from, until *time.Time) (*WorkSession, error) {
	session, err := s.Get(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	t, err := s.taskStore.GetByID(taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if t.ProjectID != session.ProjectID {
		return nil, ErrTaskOtherProject
	}

	now := time.Now()
	start := session.StartAt
	if session.EndAt == nil {
		start = now
	}
	if from != nil {
		start = *from
	}
	if err := validateInterval(session, start, until, now); err != nil {
		return nil, err
	}
	if overlaps(session, t.ID, uuid.Nil, start, until) {
		return nil, ErrTaskOverlap
	}
	if _, err := s.sessionStore.AddTask(session.ID, t.ID, start, until); err != nil {
		return nil, err
	}
	if session.EndAt == nil {
		if t.AssigneeID == nil {
			if err := s.taskStore.ClaimIfUnassigned(t.ID, session.PersonID); err != nil {
				return nil, err
			}
		}
		if err := s.taskStore.StartProgress(t.ID); err != nil {
			return nil, err
		}
	}
	return s.Get(projectID, sessionID)
}

// TaskChange é o que se muda no intervalo de uma tarefa da sessão. Cada campo só vale
// quando vem: From muda o começo; Until muda o fim; ClearUntil volta ao "até o fim da
// sessão"; Stop encerra o intervalo agora (numa sessão encerrada, no fim dela).
type TaskChange struct {
	From       *time.Time
	Until      *time.Time
	ClearUntil bool
	Stop       bool
}

// UpdateTask muda o intervalo de uma tarefa da sessão. Numa sessão aberta ao menos uma tarefa
// continua sem fim: quem quer parar de vez encerra o ponto.
func (s *Service) UpdateTask(projectID, sessionID, linkID string, change TaskChange) (*WorkSession, error) {
	session, err := s.Get(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	link := findLink(session, linkID)
	if link == nil {
		return nil, ErrTaskLinkNotFound
	}

	now := time.Now()
	from, until := link.FromAt, link.UntilAt
	if change.From != nil {
		from = *change.From
	}
	switch {
	case change.Stop:
		end := now
		if session.EndAt != nil {
			end = *session.EndAt
		}
		until = &end
	case change.ClearUntil:
		until = nil
	case change.Until != nil:
		until = change.Until
	}
	if err := validateInterval(session, from, until, now); err != nil {
		return nil, err
	}
	if overlaps(session, link.TaskID, link.ID, from, until) {
		return nil, ErrTaskOverlap
	}
	if session.EndAt == nil && until != nil && !hasOtherActive(session, link.ID) {
		return nil, ErrLastTask
	}
	if err := s.sessionStore.UpdateTask(link.ID, from, until); err != nil {
		return nil, err
	}
	return s.Get(projectID, sessionID)
}

// RemoveTask tira uma tarefa da sessão; a sessão e as horas ficam. A sessão guarda sempre
// ao menos uma tarefa e, enquanto aberta, ao menos uma sem fim.
func (s *Service) RemoveTask(projectID, sessionID, linkID string) (*WorkSession, error) {
	session, err := s.Get(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	link := findLink(session, linkID)
	if link == nil {
		return nil, ErrTaskLinkNotFound
	}
	if len(session.Tasks) == 1 || (session.EndAt == nil && !hasOtherActive(session, link.ID)) {
		return nil, ErrLastTask
	}
	if err := s.sessionStore.RemoveTask(link.ID); err != nil {
		return nil, err
	}
	return s.Get(projectID, sessionID)
}

func findLink(session *WorkSession, linkID string) *SessionTask {
	for i := range session.Tasks {
		if session.Tasks[i].ID.String() == linkID {
			return &session.Tasks[i]
		}
	}
	return nil
}

// hasOtherActive diz se a sessão tem alguma tarefa sem fim além da de id skip.
func hasOtherActive(session *WorkSession, skip uuid.UUID) bool {
	for i := range session.Tasks {
		if session.Tasks[i].ID != skip && session.Tasks[i].UntilAt == nil {
			return true
		}
	}
	return false
}

// validateInterval confere que o intervalo cabe na sessão: começa depois do início dela,
// termina depois de começar, e nenhum dos dois passa do fim dela (ou de agora, se aberta).
func validateInterval(session *WorkSession, from time.Time, until *time.Time, now time.Time) error {
	limit := now
	if session.EndAt != nil {
		limit = *session.EndAt
	}
	if from.Before(session.StartAt) || from.After(limit) || (session.EndAt != nil && !from.Before(limit)) {
		return ErrInvalidInterval
	}
	if until != nil && (!until.After(from) || until.After(limit)) {
		return ErrInvalidInterval
	}
	return nil
}

// overlaps diz se o intervalo se sobrepõe a outro da mesma tarefa na sessão (fora o de id
// skip, que é o que está sendo mudado). O mesmo intervalo sem fim vai até o fim da sessão,
// ou sem limite se ela está aberta; tarefas diferentes podem se sobrepor à vontade.
func overlaps(session *WorkSession, taskID, skip uuid.UUID, from time.Time, until *time.Time) bool {
	end := func(u *time.Time) time.Time {
		switch {
		case u != nil:
			return *u
		case session.EndAt != nil:
			return *session.EndAt
		default:
			return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		}
	}
	newEnd := end(until)
	for i := range session.Tasks {
		l := &session.Tasks[i]
		if l.TaskID != taskID || l.ID == skip {
			continue
		}
		if from.Before(end(l.UntilAt)) && l.FromAt.Before(newEnd) {
			return true
		}
	}
	return false
}
