package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/adapter/clerkfake"
	"working-time-tracker/internal/domain/person"
)

var bg = context.Background()

func newClerkService(t *testing.T) (*Service, *clerkfake.Fake) {
	t.Helper()
	svc, _ := newService(t)
	fake := clerkfake.New()
	svc.SetClerk(fake, ClerkSettings{PublicURL: "http://localhost:8080"})
	return svc, fake
}

// clerkUser cadastra um usuário no Clerk falso (e-mail verificado) e devolve um token de sessão dele.
func clerkUser(fake *clerkfake.Fake, id, email, name string) string {
	fake.AddUser(id, email, name, true)
	return fake.Token(id)
}

func mustClerkSignup(t *testing.T, svc *Service, token, org string) *ClerkResult {
	t.Helper()
	res, err := svc.ClerkSignup(bg, token, ClerkSignupInput{OrganizationName: org})
	if err != nil {
		t.Fatalf("clerk signup: %v", err)
	}
	return res
}

// Com o Clerk desligado, as rotas dele respondem que não existe, mesmo com um token.
func TestClerk_DisabledByDefault(t *testing.T) {
	svc, _ := newService(t)
	if svc.ClerkEnabled() {
		t.Fatal("the Clerk must be off until SetClerk")
	}
	if _, err := svc.ClerkLogin(bg, "qualquer", ClerkLoginInput{}); !errors.Is(err, ErrClerkDisabled) {
		t.Errorf("login: err = %v, want ErrClerkDisabled", err)
	}
	if _, err := svc.ClerkSignup(bg, "qualquer", ClerkSignupInput{OrganizationName: "Acme"}); !errors.Is(err, ErrClerkDisabled) {
		t.Errorf("signup: err = %v, want ErrClerkDisabled", err)
	}
}

// Quem cria a organização pelo Clerk é o dono, sem senha, e a sessão aberta vale.
func TestClerkSignup_CreatesOwnerWithoutPassword(t *testing.T) {
	svc, fake := newClerkService(t)
	token := clerkUser(fake, "user_1", "Ana@Acme.com", "Ana Souza")

	res := mustClerkSignup(t, svc, token, "  Acme  ")
	if res.Status != ClerkOK || !res.Created || res.Token == "" {
		t.Fatalf("result = %+v, want ok, created, with a session token", res)
	}
	id := res.Identity
	if id.Email != "ana@acme.com" || id.Name != "Ana Souza" || id.OrganizationName != "Acme" || !id.IsOwner || id.Role != person.RoleAdmin {
		t.Errorf("identity = %+v, want the owner Ana Souza of Acme", id)
	}
	if id.HasPassword {
		t.Error("an account created through the Clerk has no password")
	}
	if got, err := svc.Authenticate(res.Token); err != nil || got.PersonID != id.PersonID {
		t.Errorf("the new session must authenticate: %v", err)
	} else if !id.NeedsOnboarding || !got.NeedsOnboarding {
		t.Error("the owner created through the Clerk gets the welcome too")
	}
	if _, _, err := svc.Login("ana@acme.com", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password login of an account without password: err = %v, want ErrInvalidCredentials", err)
	}
}

func TestClerkSignup_CountrySetsTheOrganizationDefaults(t *testing.T) {
	svc, fake := newClerkService(t)
	token := clerkUser(fake, "user_us", "joe@acme.com", "Joe Smith")
	res, err := svc.ClerkSignup(bg, token, ClerkSignupInput{OrganizationName: "Acme Inc", Country: "us"})
	if err != nil {
		t.Fatalf("clerk signup: %v", err)
	}
	org, err := testClient.Organization.Get(bg, res.Identity.OrganizationID)
	if err != nil {
		t.Fatalf("load the organization: %v", err)
	}
	if org.Country != "US" || org.Currency != "USD" || org.Timezone != "America/New_York" {
		t.Errorf("organization = %s %s %s, want US USD America/New_York", org.Country, org.Currency, org.Timezone)
	}

	// Sem país é o padrão, e um país que o cadastro não conhece é recusado antes de criar qualquer coisa.
	svc2, fake2 := newClerkService(t)
	token2 := clerkUser(fake2, "user_br", "ana@acme.com", "Ana")
	if _, err := svc2.ClerkSignup(bg, token2, ClerkSignupInput{OrganizationName: "Acme", Country: "Portugal"}); !errors.Is(err, ErrInvalidCountry) {
		t.Errorf("unknown country: err = %v, want ErrInvalidCountry", err)
	}
	res2 := mustClerkSignup(t, svc2, token2, "Acme")
	if org2, _ := testClient.Organization.Get(bg, res2.Identity.OrganizationID); org2 == nil || org2.Country != "BR" || org2.Currency != "BRL" {
		t.Errorf("without a country the organization must be BR/BRL: %+v", org2)
	}
}

func TestClerkSignup_Validation(t *testing.T) {
	svc, fake := newClerkService(t)
	token := clerkUser(fake, "user_1", "ana@acme.com", "")

	if _, err := svc.ClerkSignup(bg, token, ClerkSignupInput{OrganizationName: "  "}); !errors.Is(err, ErrOrgNameRequired) {
		t.Errorf("no organization name: err = %v, want ErrOrgNameRequired", err)
	}
	// Sem nome no Clerk nem no corpo, usa a parte do e-mail antes do @.
	res := mustClerkSignup(t, svc, token, "Acme")
	if res.Identity.Name != "ana" {
		t.Errorf("name = %q, want the part of the email before the @", res.Identity.Name)
	}
	// Quem já tem conta não cria outra organização.
	if _, err := svc.ClerkSignup(bg, fake.Token("user_1"), ClerkSignupInput{OrganizationName: "Outra"}); !errors.Is(err, ErrAccountExists) {
		t.Errorf("second signup: err = %v, want ErrAccountExists", err)
	}
	// Nem outro usuário do Clerk com o mesmo e-mail.
	other := clerkUser(fake, "user_2", "ANA@acme.com", "Ana")
	if _, err := svc.ClerkSignup(bg, other, ClerkSignupInput{OrganizationName: "Outra"}); !errors.Is(err, ErrAccountExists) {
		t.Errorf("same email: err = %v, want ErrAccountExists", err)
	}
}

// O usuário já ligado entra direto, quantas vezes quiser, cada uma com uma sessão nova.
func TestClerkLogin_LinkedUserEnters(t *testing.T) {
	svc, fake := newClerkService(t)
	first := mustClerkSignup(t, svc, clerkUser(fake, "user_1", "ana@acme.com", "Ana"), "Acme")

	res, err := svc.ClerkLogin(bg, fake.Token("user_1"), ClerkLoginInput{})
	if err != nil || res.Status != ClerkOK || res.Created {
		t.Fatalf("login = %+v, %v; want ok, not created", res, err)
	}
	if res.Identity.PersonID != first.Identity.PersonID || res.Token == "" || res.Token == first.Token {
		t.Errorf("want the same person with a new session, got %+v", res)
	}
}

func TestClerkLogin_RefusesBadTokensAndUnverifiedEmail(t *testing.T) {
	svc, fake := newClerkService(t)

	if _, err := svc.ClerkLogin(bg, "", ClerkLoginInput{}); !errors.Is(err, ErrClerkTokenInvalid) {
		t.Errorf("empty token: err = %v, want ErrClerkTokenInvalid", err)
	}
	token := clerkUser(fake, "user_1", "ana@acme.com", "Ana")
	fake.Expire(token)
	if _, err := svc.ClerkLogin(bg, token, ClerkLoginInput{}); !errors.Is(err, ErrClerkTokenInvalid) {
		t.Errorf("expired token: err = %v, want ErrClerkTokenInvalid", err)
	}

	fake.AddUser("user_2", "bia@acme.com", "Bia", false)
	if _, err := svc.ClerkLogin(bg, fake.Token("user_2"), ClerkLoginInput{}); !errors.Is(err, ErrClerkEmailUnverified) {
		t.Errorf("unverified email: err = %v, want ErrClerkEmailUnverified", err)
	}
	if _, err := svc.ClerkSignup(bg, fake.Token("user_2"), ClerkSignupInput{OrganizationName: "Acme"}); !errors.Is(err, ErrClerkEmailUnverified) {
		t.Errorf("signup with unverified email: err = %v, want ErrClerkEmailUnverified", err)
	}

	fake.SetDown(true)
	if _, err := svc.ClerkLogin(bg, fake.Token("user_1"), ClerkLoginInput{}); !errors.Is(err, ErrClerkUnavailable) {
		t.Errorf("Clerk down: err = %v, want ErrClerkUnavailable", err)
	}
}

// Uma conta sem senha (criada antes de haver login) é ligada pelo e-mail verificado, sem pedir nada.
func TestClerkLogin_LinksAccountWithoutPassword(t *testing.T) {
	svc, fake := newClerkService(t)
	ctx := bg
	org, err := testClient.Organization.Create().SetName("Acme").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := testClient.Person.Create().SetName("Ana").SetEmail("ana@acme.com").SetOrganizationID(org.ID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.ClerkLogin(ctx, clerkUser(fake, "user_1", "ana@acme.com", "Ana"), ClerkLoginInput{})
	if err != nil || res.Status != ClerkOK || res.Identity.PersonID != p.ID {
		t.Fatalf("login = %+v, %v; want ok for the existing person", res, err)
	}
	// Ligada: o próximo login entra pelo usuário, sem olhar o e-mail.
	if got, err := svc.store.PersonByClerkID("user_1"); err != nil || got.ID != p.ID {
		t.Errorf("the person must be linked to user_1: %v", err)
	}
}

// Uma conta com senha só é ligada ao Clerk com a senha, uma vez: o cadastro por senha não verifica o e-mail, e
// ligar sem essa prova entregaria a conta a quem a cadastrou com o e-mail de outra pessoa.
func TestClerkLogin_PasswordAccountNeedsPasswordOnce(t *testing.T) {
	svc, fake := newClerkService(t)
	created, _ := mustSignup(t, svc, "ana@acme.com")
	token := clerkUser(fake, "user_1", "ana@acme.com", "Ana")

	res, err := svc.ClerkLogin(bg, token, ClerkLoginInput{})
	if err != nil || res.Status != ClerkNeedsPassword || res.Token != "" || res.Identity != nil {
		t.Fatalf("without the password = %+v, %v; want needs_password and no session", res, err)
	}
	if _, err := svc.store.PersonByClerkID("user_1"); err == nil {
		t.Fatal("the account must not be linked before the password is confirmed")
	}

	if _, err := svc.ClerkLogin(bg, fake.Token("user_1"), ClerkLoginInput{Password: "senha-errada"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.store.PersonByClerkID("user_1"); err == nil {
		t.Fatal("a wrong password must not link the account")
	}

	res, err = svc.ClerkLogin(bg, fake.Token("user_1"), ClerkLoginInput{Password: "senha-forte-1"})
	if err != nil || res.Status != ClerkOK || res.Identity.PersonID != created.PersonID || !res.Identity.HasPassword {
		t.Fatalf("right password = %+v, %v; want ok for the same person, still with a password", res, err)
	}
	// Ligada, não pede mais.
	if res, err = svc.ClerkLogin(bg, fake.Token("user_1"), ClerkLoginInput{}); err != nil || res.Status != ClerkOK {
		t.Errorf("after linking = %+v, %v; want ok without a password", res, err)
	}
}

// Um usuário do Clerk que não existe aqui vê os convites pendentes para o e-mail dele e escolhe; nada é aceito
// sozinho, nem com dois convites do mesmo e-mail.
func TestClerkLogin_NoAccountListsPendingInvites(t *testing.T) {
	svc, fake := newClerkService(t)
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	other, _ := mustSignup(t, svc, "bia@beta.com")
	inv1, _, err := svc.CreateInvite(bg, owner, "caio@mail.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateInvite(bg, other, "caio@mail.com", "member"); err != nil {
		t.Fatal(err)
	}
	token := clerkUser(fake, "user_c", "caio@mail.com", "Caio")

	res, err := svc.ClerkLogin(bg, token, ClerkLoginInput{})
	if err != nil || res.Status != ClerkNoAccount || res.Token != "" {
		t.Fatalf("login = %+v, %v; want no_account and no session", res, err)
	}
	if res.Email != "caio@mail.com" || res.Name != "Caio" || len(res.Invites) != 2 {
		t.Errorf("result = %+v, want caio's email, name and both invites", res)
	}
	if inUse, _ := svc.store.EmailInUse("caio@mail.com"); inUse {
		t.Fatal("nothing may be accepted on its own")
	}

	// Escolhe um.
	joined, err := svc.ClerkJoin(bg, fake.Token("user_c"), ClerkJoinInput{InviteID: inv1.ID.String()})
	if err != nil || !joined.Created || joined.Identity.OrganizationName != "Acme" || joined.Identity.Role != person.RoleMember || joined.Identity.HasPassword {
		t.Fatalf("join = %+v, %v; want a new member of Acme without a password", joined, err)
	}
	// O outro convite ficou pendente, mas a conta já existe: não entra em duas organizações.
	if _, err := svc.ClerkJoin(bg, fake.Token("user_c"), ClerkJoinInput{InviteID: inv1.ID.String()}); err == nil {
		t.Error("an invite is single use")
	}
}

func TestClerkJoin_OnlyTheInviteOfTheVerifiedEmail(t *testing.T) {
	svc, fake := newClerkService(t)
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	forCaio, _, _ := svc.CreateInvite(bg, owner, "caio@mail.com", "member")
	open, _, _ := svc.CreateInvite(bg, owner, "", "member")
	other := clerkUser(fake, "user_d", "duda@mail.com", "Duda")

	if _, err := svc.ClerkJoin(bg, other, ClerkJoinInput{InviteID: forCaio.ID.String()}); !errors.Is(err, ErrInviteEmailMismatch) {
		t.Errorf("someone else's invite: err = %v, want ErrInviteEmailMismatch", err)
	}
	// O convite sem e-mail é de quem tem o link: entra pelo token, e não por este caminho.
	if _, err := svc.ClerkJoin(bg, fake.Token("user_d"), ClerkJoinInput{InviteID: open.ID.String()}); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("invite without email: err = %v, want ErrInviteInvalid", err)
	}
	if _, err := svc.ClerkJoin(bg, fake.Token("user_d"), ClerkJoinInput{InviteID: "nao-e-uuid"}); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("garbage id: err = %v, want ErrInviteInvalid", err)
	}
}

// O token do convite (o do link do e-mail) cria a conta na organização dele sem passar pela lista.
func TestClerkLogin_InviteTokenJoinsDirectly(t *testing.T) {
	svc, fake := newClerkService(t)
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	_, invToken, err := svc.CreateInvite(bg, owner, "caio@mail.com", "admin")
	if err != nil {
		t.Fatal(err)
	}

	// Com outro e-mail verificado, não.
	wrong := clerkUser(fake, "user_x", "intruso@mail.com", "Intruso")
	if _, err := svc.ClerkLogin(bg, wrong, ClerkLoginInput{InviteToken: invToken}); !errors.Is(err, ErrInviteEmailMismatch) {
		t.Errorf("another email: err = %v, want ErrInviteEmailMismatch", err)
	}
	if _, err := svc.ClerkLogin(bg, fake.Token("user_x"), ClerkLoginInput{InviteToken: "token-que-nao-existe"}); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("unknown token: err = %v, want ErrInviteInvalid", err)
	}

	token := clerkUser(fake, "user_c", "Caio@Mail.com", "")
	res, err := svc.ClerkLogin(bg, token, ClerkLoginInput{InviteToken: invToken})
	if err != nil || res.Status != ClerkOK || !res.Created || res.Token == "" {
		t.Fatalf("login = %+v, %v; want a new account with a session", res, err)
	}
	if id := res.Identity; id.OrganizationName != "Acme" || id.Role != person.RoleAdmin || id.Email != "caio@mail.com" || id.Name != "caio" {
		t.Errorf("identity = %+v, want caio, admin of Acme", id)
	}
	// O convite foi usado.
	if _, err := svc.InviteInfo(invToken); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("used invite: err = %v, want ErrInviteInvalid", err)
	}
}

// Um convite sem e-mail é de quem tem o link: qualquer usuário do Clerk com e-mail verificado entra.
func TestClerkLogin_OpenInviteTokenAcceptsAnyVerifiedEmail(t *testing.T) {
	svc, fake := newClerkService(t)
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	_, invToken, _ := svc.CreateInvite(bg, owner, "", "member")

	res, err := svc.ClerkLogin(bg, clerkUser(fake, "user_e", "eva@mail.com", "Eva"), ClerkLoginInput{InviteToken: invToken})
	if err != nil || !res.Created || res.Identity.OrganizationName != "Acme" {
		t.Fatalf("login = %+v, %v; want Eva as a new member of Acme", res, err)
	}
}

// Quem já é de uma organização não entra em outra com um convite: o e-mail é de uma conta só.
func TestClerkLogin_MemberOfAnotherOrganizationCannotUseAnInvite(t *testing.T) {
	svc, fake := newClerkService(t)
	mustSignup(t, svc, "ana@acme.com")
	other, _ := mustSignup(t, svc, "bia@beta.com")
	_, invToken, _ := svc.CreateInvite(bg, other, "", "member")
	ana := clerkUser(fake, "user_a", "ana@acme.com", "Ana")

	_, err := svc.ClerkLogin(bg, ana, ClerkLoginInput{InviteToken: invToken, Password: "senha-forte-1"})
	if !errors.Is(err, ErrAccountExists) {
		t.Errorf("err = %v, want ErrAccountExists", err)
	}
	// Quem já é da organização do convite só entra.
	_, ownToken, _ := svc.CreateInvite(bg, other, "", "member")
	bia := clerkUser(fake, "user_b", "bia@beta.com", "Bia")
	res, err := svc.ClerkLogin(bg, bia, ClerkLoginInput{InviteToken: ownToken, Password: "senha-forte-1"})
	if err != nil || res.Status != ClerkOK || res.Created {
		t.Errorf("a member using her own organization's invite = %+v, %v; want a plain login", res, err)
	}
}

// Uma conta ligada a outro usuário do Clerk só é religada se esse outro já não existe lá.
func TestClerkLogin_RelinksOnlyWhenTheOldClerkUserIsGone(t *testing.T) {
	svc, fake := newClerkService(t)
	first := mustClerkSignup(t, svc, clerkUser(fake, "user_old", "ana@acme.com", "Ana"), "Acme")

	// O usuário antigo ainda existe: outro usuário com o mesmo e-mail não toma a conta.
	newer := clerkUser(fake, "user_new", "ana@acme.com", "Ana")
	if _, err := svc.ClerkLogin(bg, newer, ClerkLoginInput{}); !errors.Is(err, ErrClerkAccountLinked) {
		t.Fatalf("old user still there: err = %v, want ErrClerkAccountLinked", err)
	}

	// Apagado e recriado com o mesmo e-mail: religa.
	fake.DeleteUser("user_old")
	res, err := svc.ClerkLogin(bg, fake.Token("user_new"), ClerkLoginInput{})
	if err != nil || res.Status != ClerkOK || res.Identity.PersonID != first.Identity.PersonID {
		t.Fatalf("after the old user is gone = %+v, %v; want the same person", res, err)
	}
	if got, err := svc.store.PersonByClerkID("user_new"); err != nil || got.ID != first.Identity.PersonID {
		t.Errorf("the account must now be linked to user_new: %v", err)
	}
}

// A conta que já existe e tem senha segue pedindo a senha também para religar.
func TestClerkLogin_RelinkingAPasswordAccountStillNeedsThePassword(t *testing.T) {
	svc, fake := newClerkService(t)
	mustSignup(t, svc, "ana@acme.com")
	clerkLogin := func(clerkID, password string) (*ClerkResult, error) {
		return svc.ClerkLogin(bg, fake.Token(clerkID), ClerkLoginInput{Password: password})
	}
	clerkUser(fake, "user_old", "ana@acme.com", "Ana")
	if res, err := clerkLogin("user_old", "senha-forte-1"); err != nil || res.Status != ClerkOK {
		t.Fatalf("first link = %+v, %v", res, err)
	}

	fake.DeleteUser("user_old")
	clerkUser(fake, "user_new", "ana@acme.com", "Ana")
	if res, err := clerkLogin("user_new", ""); err != nil || res.Status != ClerkNeedsPassword {
		t.Errorf("relinking without the password = %+v, %v; want needs_password", res, err)
	}
	if res, err := clerkLogin("user_new", "senha-forte-1"); err != nil || res.Status != ClerkOK {
		t.Errorf("relinking with the password = %+v, %v; want ok", res, err)
	}
}

// O erro do adapter vira o código do domínio; o detalhe técnico fica embrulhado, para o log.
func TestClerkErrors_KeepTheCause(t *testing.T) {
	svc, fake := newClerkService(t)
	fake.SetDown(true)
	_, err := svc.ClerkLogin(bg, "x", ClerkLoginInput{})
	if !errors.Is(err, ErrClerkUnavailable) || !errors.Is(err, adapter.ErrClerkUnavailable) {
		t.Errorf("err = %v, want the domain code wrapping the adapter error", err)
	}
}

// ---------- O convite por e-mail pelo Clerk ----------

// logLines guarda o que o serviço manda ao log, como o log do servidor faria.
type logLines struct{ lines []string }

func (l *logLines) log(msg string, args ...any) {
	l.lines = append(l.lines, fmt.Sprint(append([]any{msg}, args...)...))
}

func newInviteService(t *testing.T, delivery string) (*Service, *clerkfake.Fake, *logLines) {
	t.Helper()
	svc, _ := newService(t)
	fake := clerkfake.New()
	logs := &logLines{}
	svc.SetClerk(fake, ClerkSettings{PublicURL: "http://localhost:8080", InviteDelivery: delivery, Log: logs.log})
	return svc, fake, logs
}

// Com o Clerk ligado, o convite com e-mail é criado lá com o link do sistema, a validade do convite e o pedido para
// mandar o e-mail; o id do convite do Clerk fica guardado.
func TestCreateInvite_IsEmailedThroughClerk(t *testing.T) {
	svc, fake, logs := newInviteService(t, "email")
	owner, _ := mustSignup(t, svc, "ana@acme.com")

	inv, token, err := svc.CreateInvite(bg, owner, " Caio@Mail.com ", "member")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Delivery != DeliveryEmail {
		t.Errorf("delivery = %q, want %q", inv.Delivery, DeliveryEmail)
	}
	sent := fake.Invitations()
	if len(sent) != 1 {
		t.Fatalf("the Clerk got %d invitations, want 1", len(sent))
	}
	p := sent[0].Params
	if p.Email != "caio@mail.com" || p.RedirectURL != "http://localhost:8080/invite/"+token || !p.Notify || p.ExpiresInDays != 7 {
		t.Errorf("invitation = %+v, want caio's normalized email, the system's link, notify and 7 days", p)
	}
	stored, err := testClient.Invite.Get(bg, inv.ID)
	if err != nil || stored.ClerkInvitationID == nil || *stored.ClerkInvitationID != sent[0].ID {
		t.Errorf("stored clerk invitation id = %v, %v; want %s", stored.ClerkInvitationID, err, sent[0].ID)
	}
	if len(logs.lines) != 0 {
		t.Errorf("the email mode must not print the link: %v", logs.lines)
	}
}

// No modo terminal o Clerk não manda o e-mail e o link vai para o log: o link do próprio Clerk, ou o do sistema
// quando ele não o devolve.
func TestCreateInvite_TerminalModePrintsTheLink(t *testing.T) {
	svc, fake, logs := newInviteService(t, "terminal")
	owner, _ := mustSignup(t, svc, "ana@acme.com")

	inv, _, err := svc.CreateInvite(bg, owner, "caio@mail.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Delivery != DeliveryTerminal {
		t.Errorf("delivery = %q, want %q", inv.Delivery, DeliveryTerminal)
	}
	sent := fake.Invitations()
	if len(sent) != 1 || sent[0].Params.Notify {
		t.Fatalf("invitations = %+v, want one, with notify off", sent)
	}
	if len(logs.lines) != 1 || !strings.Contains(logs.lines[0], "https://clerk.test/v1/tickets/accept?ticket="+sent[0].ID) || !strings.Contains(logs.lines[0], "caio@mail.com") {
		t.Errorf("log = %v, want caio's email and the Clerk link", logs.lines)
	}

	fake.OmitInvitationURL(true)
	logs.lines = nil
	_, token, err := svc.CreateInvite(bg, owner, "duda@mail.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs.lines) != 1 || !strings.Contains(logs.lines[0], "http://localhost:8080/invite/"+token) {
		t.Errorf("log = %v, want the system's link when the Clerk gives none", logs.lines)
	}
}

// Sem e-mail, ou sem o Clerk, o convite é só o link, como sempre foi, e o Clerk não é chamado.
func TestCreateInvite_LinkOnlyWithoutEmailOrWithoutClerk(t *testing.T) {
	svc, fake, _ := newInviteService(t, "email")
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	inv, token, err := svc.CreateInvite(bg, owner, "", "member")
	if err != nil || inv.Delivery != DeliveryLink || token == "" {
		t.Fatalf("without email = %+v, %v; want a link", inv, err)
	}
	if n := len(fake.Invitations()); n != 0 {
		t.Errorf("the Clerk got %d invitations for an invite without email, want 0", n)
	}

	off, _ := newService(t)
	owner2, _ := mustSignup(t, off, "bia@beta.com")
	inv, _, err = off.CreateInvite(bg, owner2, "caio@mail.com", "member")
	if err != nil || inv.Delivery != DeliveryLink {
		t.Errorf("without the Clerk = %+v, %v; want a link", inv, err)
	}
}

// Um e-mail que já tem conta não vira convite, e o Clerk nem é chamado.
func TestCreateInvite_ExistingAccountDoesNotReachClerk(t *testing.T) {
	svc, fake, _ := newInviteService(t, "email")
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	if _, _, err := svc.CreateInvite(bg, owner, "ana@acme.com", "member"); !errors.Is(err, ErrAccountExists) {
		t.Errorf("err = %v, want ErrAccountExists", err)
	}
	if n := len(fake.Invitations()); n != 0 {
		t.Errorf("the Clerk got %d invitations, want 0", n)
	}
}

// Se o Clerk falha, nada é gravado: um convite sem e-mail e sem como saber disso seria um convite fantasma.
func TestCreateInvite_ClerkFailureStoresNothing(t *testing.T) {
	svc, fake, _ := newInviteService(t, "email")
	owner, _ := mustSignup(t, svc, "ana@acme.com")

	fake.FailInvitations(fmt.Errorf("%w: the email is not accepted", adapter.ErrClerkRejected))
	if _, _, err := svc.CreateInvite(bg, owner, "caio@mail.com", "member"); !errors.Is(err, ErrClerkInviteFailed) {
		t.Errorf("rejected: err = %v, want ErrClerkInviteFailed", err)
	}
	fake.FailInvitations(nil)
	fake.SetDown(true)
	if _, _, err := svc.CreateInvite(bg, owner, "caio@mail.com", "member"); !errors.Is(err, ErrClerkUnavailable) {
		t.Errorf("Clerk down: err = %v, want ErrClerkUnavailable", err)
	}
	pending, err := svc.ListInvites(owner.OrganizationID)
	if err != nil || len(pending) != 0 {
		t.Errorf("pending invites = %d, %v; want none", len(pending), err)
	}
}

// Se o convite não pôde ser gravado depois de criado no Clerk, ele é cancelado lá.
func TestCreateInvite_StoreFailureCancelsAtClerk(t *testing.T) {
	svc, fake, _ := newInviteService(t, "email")
	// Uma organização que não existe: a gravação falha pela chave estrangeira.
	ghost := &Identity{PersonID: uuid.New(), OrganizationID: uuid.New(), IsOwner: true}
	if _, _, err := svc.CreateInvite(bg, ghost, "caio@mail.com", "member"); err == nil {
		t.Fatal("want an error for an organization that does not exist")
	}
	sent := fake.Invitations()
	if len(sent) != 1 || !sent[0].Revoked {
		t.Errorf("invitations = %+v, want the one created to be cancelled", sent)
	}
}

// Convidar o mesmo e-mail de novo cancela o convite anterior (aqui e no Clerk): é o "reenviar". Os convites de
// outro e-mail e de outra organização ficam como estão.
func TestCreateInvite_ResendReplacesThePreviousOne(t *testing.T) {
	svc, fake, _ := newInviteService(t, "email")
	owner, _ := mustSignup(t, svc, "ana@acme.com")
	other, _ := mustSignup(t, svc, "bia@beta.com")

	_, firstToken, _ := svc.CreateInvite(bg, owner, "caio@mail.com", "member")
	_, _, _ = svc.CreateInvite(bg, owner, "duda@mail.com", "member")
	_, otherOrgToken, _ := svc.CreateInvite(bg, other, "caio@mail.com", "member")

	_, secondToken, err := svc.CreateInvite(bg, owner, "caio@mail.com", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InviteInfo(firstToken); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("the first link: err = %v, want it replaced (ErrInviteInvalid)", err)
	}
	if info, err := svc.InviteInfo(secondToken); err != nil || info.Role != "admin" {
		t.Errorf("the second link: %+v, %v; want a valid admin invite", info, err)
	}
	if _, err := svc.InviteInfo(otherOrgToken); err != nil {
		t.Errorf("another organization's invite for the same email must stay: %v", err)
	}

	pending, _ := svc.ListInvites(owner.OrganizationID)
	if len(pending) != 2 {
		t.Errorf("pending invites = %d, want duda's and the new one for caio", len(pending))
	}
	revoked := 0
	for _, inv := range fake.Invitations() {
		if inv.Revoked {
			revoked++
		}
	}
	if revoked != 1 {
		t.Errorf("%d Clerk invitations cancelled, want only the first one for caio", revoked)
	}
}

// Revogar cancela o convite no Clerk também, e uma falha de lá não impede de revogar aqui.
func TestRevokeInvite_CancelsAtClerkAndSurvivesItsFailure(t *testing.T) {
	svc, fake, logs := newInviteService(t, "email")
	owner, _ := mustSignup(t, svc, "ana@acme.com")

	a, tokenA, _ := svc.CreateInvite(bg, owner, "caio@mail.com", "member")
	if err := svc.RevokeInvite(bg, a.ID.String()); err != nil {
		t.Fatal(err)
	}
	if got := fake.Invitations(); len(got) != 1 || !got[0].Revoked {
		t.Errorf("invitations = %+v, want the Clerk's cancelled", got)
	}
	if _, err := svc.InviteInfo(tokenA); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("revoked link: err = %v, want ErrInviteInvalid", err)
	}

	b, tokenB, _ := svc.CreateInvite(bg, owner, "duda@mail.com", "member")
	fake.SetDown(true)
	if err := svc.RevokeInvite(bg, b.ID.String()); err != nil {
		t.Errorf("revoke with the Clerk down: %v, want it to work", err)
	}
	if _, err := svc.InviteInfo(tokenB); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("revoked link: err = %v, want ErrInviteInvalid", err)
	}
	if len(logs.lines) != 1 || !strings.Contains(logs.lines[0], "could not cancel the invite at Clerk") {
		t.Errorf("log = %v, want the failed cancellation noted", logs.lines)
	}

	// Um convite que não passou pelo Clerk (só o link) se revoga sem chamá-lo.
	fake.SetDown(false)
	c, _, _ := svc.CreateInvite(bg, owner, "", "member")
	if err := svc.RevokeInvite(bg, c.ID.String()); err != nil {
		t.Errorf("revoke a link-only invite: %v", err)
	}
}
