package task

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entlabel "working-time-tracker/ent/label"
	"working-time-tracker/internal/database"
)

// maxLabelLen é o tamanho máximo do nome de uma etiqueta, em caracteres: o mesmo do nome de uma label do
// GitHub, para a sincronização não renomear a label de lá ao empurrar uma etiqueta.
const maxLabelLen = 50

// ListLabels lista as etiquetas do projeto em ordem alfabética.
func (s *Store) ListLabels(projectID uuid.UUID) ([]Label, error) {
	rows, err := s.client.Label.Query().
		Where(entlabel.ProjectIDEQ(projectID)).
		Order(ent.Asc(entlabel.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toLabels(rows), nil
}

// LabelsInProject devolve, sem repetir, as etiquetas com estes ids que são do
// projeto. Quem chama compara o tamanho para saber se alguma não é.
func (s *Store) LabelsInProject(projectID uuid.UUID, ids []uuid.UUID) ([]Label, error) {
	rows, err := s.client.Label.Query().
		Where(entlabel.ProjectIDEQ(projectID), entlabel.IDIn(ids...)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toLabels(rows), nil
}

// labelNameTaken diz se o projeto já tem uma etiqueta com este nome, sem
// diferenciar maiúsculas, fora a de id except.
func (s *Store) labelNameTaken(projectID uuid.UUID, name string, except uuid.UUID) (bool, error) {
	return s.client.Label.Query().
		Where(entlabel.ProjectIDEQ(projectID), entlabel.NameEqualFold(name), entlabel.IDNEQ(except)).
		Exist(context.Background())
}

func (s *Store) createLabel(projectID uuid.UUID, name string) (*Label, error) {
	row, err := s.client.Label.Create().SetProjectID(projectID).SetName(name).Save(context.Background())
	if err != nil {
		return nil, err
	}
	return &Label{ID: row.ID, Name: row.Name}, nil
}

// labelOfProject lê a etiqueta se ela é do projeto; senão, database.ErrNotFound.
func (s *Store) labelOfProject(projectID, labelID uuid.UUID) (*ent.Label, error) {
	row, err := s.client.Label.Query().
		Where(entlabel.IDEQ(labelID), entlabel.ProjectIDEQ(projectID)).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, database.ErrNotFound
	}
	return row, err
}

func toLabels(rows []*ent.Label) []Label {
	out := make([]Label, len(rows))
	for i, r := range rows {
		out[i] = Label{ID: r.ID, Name: r.Name}
	}
	return out
}

// cleanLabelName apara o nome e confere o tamanho.
func cleanLabelName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrLabelNameRequired
	}
	if utf8.RuneCountInString(name) > maxLabelLen {
		return "", ErrLabelNameTooLong.With("max", maxLabelLen)
	}
	return name, nil
}

func parseProjectID(projectID string) (uuid.UUID, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return uuid.Nil, database.ErrNotFound
	}
	return uid, nil
}

// ListLabels lista as etiquetas do projeto.
func (s *Service) ListLabels(projectID string) ([]Label, error) {
	pid, err := parseProjectID(projectID)
	if err != nil {
		return nil, err
	}
	return s.taskStore.ListLabels(pid)
}

// CreateLabel cria uma etiqueta no projeto. O nome é único no projeto sem
// diferenciar maiúsculas.
func (s *Service) CreateLabel(projectID, name string) (*Label, error) {
	pid, err := parseProjectID(projectID)
	if err != nil {
		return nil, err
	}
	if name, err = cleanLabelName(name); err != nil {
		return nil, err
	}
	taken, err := s.taskStore.labelNameTaken(pid, name, uuid.Nil)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrLabelNameTaken
	}
	return s.taskStore.createLabel(pid, name)
}

// RenameLabel troca o nome de uma etiqueta do projeto.
func (s *Service) RenameLabel(projectID, labelID, name string) (*Label, error) {
	pid, lid, err := parseLabelIDs(projectID, labelID)
	if err != nil {
		return nil, err
	}
	if name, err = cleanLabelName(name); err != nil {
		return nil, err
	}
	row, err := s.taskStore.labelOfProject(pid, lid)
	if err != nil {
		return nil, err
	}
	taken, err := s.taskStore.labelNameTaken(pid, name, lid)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrLabelNameTaken
	}
	saved, err := row.Update().SetName(name).Save(context.Background())
	if err != nil {
		return nil, err
	}
	return &Label{ID: saved.ID, Name: saved.Name}, nil
}

// DeleteLabel exclui a etiqueta do projeto; as tarefas só a perdem.
func (s *Service) DeleteLabel(projectID, labelID string) error {
	pid, lid, err := parseLabelIDs(projectID, labelID)
	if err != nil {
		return err
	}
	if _, err := s.taskStore.labelOfProject(pid, lid); err != nil {
		return err
	}
	return s.taskStore.client.Label.DeleteOneID(lid).Exec(context.Background())
}

func parseLabelIDs(projectID, labelID string) (uuid.UUID, uuid.UUID, error) {
	pid, err := parseProjectID(projectID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	lid, err := uuid.Parse(labelID)
	if err != nil {
		return uuid.Nil, uuid.Nil, database.ErrNotFound
	}
	return pid, lid, nil
}

// resolveLabels confere que todas as etiquetas são do projeto e devolve as
// etiquetas sem repetir. Um id que não é do projeto, ou que não existe, é recusado.
func (s *Service) resolveLabels(projectID string, ids []string) ([]Label, error) {
	pid, err := parseProjectID(projectID)
	if err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{}
	var uids []uuid.UUID
	for _, id := range ids {
		uid, err := uuid.Parse(id)
		if err != nil {
			return nil, ErrLabelOtherProject
		}
		if !seen[uid] {
			seen[uid] = true
			uids = append(uids, uid)
		}
	}
	if len(uids) == 0 {
		return []Label{}, nil
	}
	found, err := s.taskStore.LabelsInProject(pid, uids)
	if err != nil {
		return nil, err
	}
	if len(found) != len(uids) {
		return nil, ErrLabelOtherProject
	}
	return found, nil
}
