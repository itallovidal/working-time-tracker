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
	payRate, billRate, hasRate, err := s.rates.Rates(personUID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	if !hasRate {
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
		TaskID:        task.ID,
		PersonID:      personUID,
		StartAt:       time.Now(),
		PayRateCents:  &payRate,
		BillRateCents: billRate,
	}
	if err := s.sessionStore.Create(session); err != nil {
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

// TotalTimeResult soma as sessões de um filtro. Os dois valores ficam nil quando
// nenhuma sessão do filtro tem valor por hora.
type TotalTimeResult struct {
	TotalSeconds    float64 `json:"total_seconds"`
	PayAmountCents  *int    `json:"pay_amount_cents"`
	BillAmountCents *int    `json:"bill_amount_cents"`
}

// TotalTime soma as sessões do projeto filtradas por tarefa, por pessoa ou pelas
// duas. O filtro pelo projeto impede que uma tarefa de outra organização seja
// somada pela rota deste.
func (s *Service) TotalTime(projectID string, taskID, personID *string) (*TotalTimeResult, error) {
	hasTask := taskID != nil && *taskID != ""
	hasPerson := personID != nil && *personID != ""
	if !hasTask && !hasPerson {
		return nil, ErrFilterRequired
	}
	if hasTask {
		if _, err := uuid.Parse(*taskID); err != nil {
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
		total.TotalSeconds += sessions[i].Seconds(now)
		total.PayAmountCents = add(total.PayAmountCents, sessions[i].PayAmountCents)
		total.BillAmountCents = add(total.BillAmountCents, sessions[i].BillAmountCents)
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
