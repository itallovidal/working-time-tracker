package organization_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/organization"
)

// O modo solo (Organization.Solo): uma pessoa só, sem equipe. Só liga com exatamente uma pessoa e nenhum convite
// pendente; desligar sempre vale.

var bg = context.Background()

// soloOrg cria uma organização com a pessoa dada de dono, e devolve o service e a organização.
func soloOrg(t *testing.T) (*organization.Service, *organization.Organization) {
	t.Helper()
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	org, err := svc.Create("Solo Org")
	if err != nil {
		t.Fatal(err)
	}
	addPerson(t, org.ID, "ana@test.com", true)
	return svc, org
}

func addPerson(t *testing.T, orgID uuid.UUID, email string, owner bool) {
	t.Helper()
	if _, err := testClient.Person.Create().SetName(email).SetEmail(email).SetOrganizationID(orgID).SetIsOwner(owner).Save(bg); err != nil {
		t.Fatalf("create person %s: %v", email, err)
	}
}

func addInvite(t *testing.T, orgID uuid.UUID, hash string, expires time.Time, accepted *time.Time) {
	t.Helper()
	if _, err := testClient.Invite.Create().SetOrganizationID(orgID).SetTokenHash(hash).SetExpiresAt(expires).SetNillableAcceptedAt(accepted).Save(bg); err != nil {
		t.Fatalf("create invite %s: %v", hash, err)
	}
}

func setSolo(svc *organization.Service, org *organization.Organization, solo bool) (*organization.Organization, error) {
	return svc.Update(org.ID.String(), organization.UpdateInput{Solo: &solo})
}

// Uma organização nasce fora do modo solo, e o campo vem no GET.
func TestSolo_DefaultsToOff(t *testing.T) {
	svc, org := soloOrg(t)
	if org.Solo {
		t.Error("a new organization must not be solo")
	}
	got, err := svc.Get(org.ID.String())
	if err != nil || got.Solo {
		t.Errorf("get = %+v (%v), want solo false", got, err)
	}
}

// Com exatamente uma pessoa e nenhum convite, liga; e o que foi gravado volta na leitura.
func TestSolo_TurnsOnWithOnePersonAndNoInvites(t *testing.T) {
	svc, org := soloOrg(t)

	updated, err := setSolo(svc, org, true)
	if err != nil {
		t.Fatalf("turn on: %v", err)
	}
	if !updated.Solo {
		t.Error("the update did not return solo = true")
	}
	if got, _ := svc.Get(org.ID.String()); got == nil || !got.Solo {
		t.Errorf("solo was not saved: %+v", got)
	}
}

// Mais de uma pessoa (ou nenhuma) recusa, com o campo no erro, e a organização fica como estava.
func TestSolo_NeedsExactlyOnePerson(t *testing.T) {
	svc, org := soloOrg(t)
	addPerson(t, org.ID, "bia@test.com", false)

	_, err := setSolo(svc, org, true)
	if !errors.Is(err, organization.ErrSoloNeedsSinglePerson) {
		t.Fatalf("two people: err = %v, want ErrSoloNeedsSinglePerson", err)
	}
	var e *apperr.Error
	if !errors.As(err, &e) || e.Params["field"] != "solo" {
		t.Errorf("the error must say the field: %+v", e)
	}
	if got, _ := svc.Get(org.ID.String()); got == nil || got.Solo {
		t.Errorf("a refused update must not turn solo on: %+v", got)
	}

	// Sem ninguém também não é "uma pessoa".
	cleanup(t)
	empty, _ := svc.Create("Vazia")
	if _, err := setSolo(svc, empty, true); !errors.Is(err, organization.ErrSoloNeedsSinglePerson) {
		t.Errorf("no people: err = %v, want ErrSoloNeedsSinglePerson", err)
	}
}

// Um convite pendente recusa. Um convite aceito ou vencido já não é pendente e não atrapalha.
func TestSolo_PendingInviteBlocksButSpentOnesDoNot(t *testing.T) {
	svc, org := soloOrg(t)
	now := time.Now()

	addInvite(t, org.ID, "pending", now.Add(24*time.Hour), nil)
	if _, err := setSolo(svc, org, true); !errors.Is(err, organization.ErrSoloNeedsSinglePerson) {
		t.Fatalf("pending invite: err = %v, want ErrSoloNeedsSinglePerson", err)
	}

	// Apagado o pendente, ficam só um vencido e um aceito.
	testClient.Invite.Delete().ExecX(bg)
	accepted := now.Add(-time.Hour)
	addInvite(t, org.ID, "expired", now.Add(-time.Hour), nil)
	addInvite(t, org.ID, "accepted", now.Add(24*time.Hour), &accepted)
	if _, err := setSolo(svc, org, true); err != nil {
		t.Errorf("expired and accepted invites must not block: %v", err)
	}
}

// O convite de outra organização não conta, nem as pessoas dela.
func TestSolo_OtherOrganizationsDoNotCount(t *testing.T) {
	svc, org := soloOrg(t)
	other, _ := svc.Create("Outra")
	addPerson(t, other.ID, "bia@test.com", true)
	addPerson(t, other.ID, "caio@test.com", false)
	addInvite(t, other.ID, "other", time.Now().Add(time.Hour), nil)

	if _, err := setSolo(svc, org, true); err != nil {
		t.Errorf("another organization's people and invites must not block: %v", err)
	}
}

// Desligar sempre vale, mesmo se a organização já não cabe no modo (chegou mais gente, ou um convite).
func TestSolo_TurnsOffAlways(t *testing.T) {
	svc, org := soloOrg(t)
	if _, err := setSolo(svc, org, true); err != nil {
		t.Fatal(err)
	}
	addPerson(t, org.ID, "bia@test.com", false)
	addInvite(t, org.ID, "pending", time.Now().Add(time.Hour), nil)

	updated, err := setSolo(svc, org, false)
	if err != nil || updated.Solo {
		t.Fatalf("turn off = %+v (%v), want solo false", updated, err)
	}
	if got, _ := svc.Get(org.ID.String()); got == nil || got.Solo {
		t.Errorf("solo was not turned off: %+v", got)
	}
}

// Mandar o valor que a organização já tem não muda nada nem confere nada: a tela de Configurações manda o campo
// sempre, e salvar o resto não pode falhar por isso. Campo ausente (nil) também mantém.
func TestSolo_SameValueAndMissingFieldKeepIt(t *testing.T) {
	svc, org := soloOrg(t)
	if _, err := setSolo(svc, org, true); err != nil {
		t.Fatal(err)
	}
	// Um estado que a regra não deixaria chegar (outra pessoa dentro de uma organização solo).
	addPerson(t, org.ID, "bia@test.com", false)

	if updated, err := setSolo(svc, org, true); err != nil || !updated.Solo {
		t.Errorf("solo = true again = %+v (%v), want it to stay on without a check", updated, err)
	}
	name := "Novo Nome"
	updated, err := svc.Update(org.ID.String(), organization.UpdateInput{Name: &name})
	if err != nil || !updated.Solo || updated.Name != name {
		t.Errorf("a PATCH without solo = %+v (%v), want the name changed and solo kept", updated, err)
	}
}

// Pela API: 200 ao ligar e ao desligar, 409 com o código e o campo quando não cabe, e o corpo estrito de sempre.
func TestHandler_Solo(t *testing.T) {
	svc, org := soloOrg(t)
	e := newTestEcho(svc)
	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PATCH", "/api/orgs/"+org.ID.String(), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := patch(`{"solo":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"solo":true`) {
		t.Fatalf("turn on = %d %s, want 200 with solo true", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/orgs/"+org.ID.String(), nil))
	if !strings.Contains(rec.Body.String(), `"solo":true`) {
		t.Errorf("get after turning on = %s, want solo true", rec.Body.String())
	}
	if rec = patch(`{"solo":false}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"solo":false`) {
		t.Fatalf("turn off = %d %s, want 200 with solo false", rec.Code, rec.Body.String())
	}

	addPerson(t, org.ID, "bia@test.com", false)
	rec = patch(`{"solo":true}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"organization.solo_needs_single_person"`) || !strings.Contains(rec.Body.String(), `"field":"solo"`) {
		t.Errorf("turn on with two people = %d %s, want 409 organization.solo_needs_single_person on the field solo", rec.Code, rec.Body.String())
	}
}
