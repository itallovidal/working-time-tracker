// Command seed popula um banco vazio com dados de demonstração: uma organização
// com uma admin e um membro, um projeto, um time, tarefas e algumas sessões de
// trabalho dos últimos dias.
//
// Uso: go run ./cmd/seed (lê DATABASE_URL do ambiente ou do .env)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

const password = "demo12345"

func main() {
	godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("defina DATABASE_URL (veja o .env.example)")
	}
	db, err := database.Open(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := database.AutoMigrate(db.Client); err != nil {
		log.Fatalf("migration: %v", err)
	}
	ctx := context.Background()
	if n := db.Client.Organization.Query().CountX(ctx); n > 0 {
		log.Fatalf("o banco já tem %d organização(ões); o seed só roda num banco vazio", n)
	}

	authSvc := auth.NewService(auth.NewStore(db.Client))
	projectSvc := project.NewService(project.NewStore(db.Client))
	teamSvc := team.NewService(team.NewStore(db.Client))
	membershipStore := team.NewMembershipStore(db.Client)
	membershipSvc := team.NewMembershipService(membershipStore)
	taskSvc := task.NewService(task.NewStore(db.Client), membershipStore, nil)

	ana, _, err := authSvc.Signup(auth.SignupInput{
		OrganizationName: "Acme Delivery",
		Name:             "Ana Souza",
		Email:            "ana@example.com",
		Password:         password,
	})
	must(err)

	_, inviteToken, err := authSvc.CreateInvite(ana, "bruno@example.com", "member")
	must(err)
	bruno, _, err := authSvc.AcceptInvite(inviteToken, auth.AcceptInviteInput{
		Name:     "Bruno Lima",
		Email:    "bruno@example.com",
		Password: password,
	})
	must(err)

	daily, weekly := "09:30", "friday"
	prj, err := projectSvc.Create(ana.OrganizationID.String(), "App de Pedidos",
		"Aplicativo para clientes pedirem e acompanharem entregas.", 14, &daily, &weekly)
	must(err)

	tm, err := teamSvc.Create(prj.ID.String(), "Produto")
	must(err)
	for _, p := range []string{ana.PersonID.String(), bruno.PersonID.String()} {
		_, err := membershipSvc.Add(tm.ID.String(), p)
		must(err)
	}

	now := time.Now()
	day := 24 * time.Hour
	newTask := func(name, description string, assignee *auth.Identity, deadline time.Duration) *task.Task {
		dl := now.Add(deadline)
		t, err := taskSvc.Create(prj.ID.String(), name, description, assignee.PersonID.String(), &dl)
		must(err)
		return t
	}
	checkout := newTask("Tela de checkout", "Resumo do pedido, endereço e pagamento em uma tela só.", bruno, 5*day)
	gateway := newTask("Integração com o gateway de pagamento", "Cartão de crédito e Pix.", ana, 10*day)
	frete := newTask("Corrigir cálculo de frete", "O frete grátis não aplica acima de R$ 100.", bruno, -2*day)
	textos := newTask("Revisar textos da loja", "", ana, 20*time.Hour)

	// Sessões encerradas nos últimos dias, em horários de trabalho.
	at := func(daysAgo int, hour, minute int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, d.Location())
	}
	sessions := []struct {
		task       *task.Task
		person     *auth.Identity
		start, end time.Time
	}{
		{checkout, bruno, at(3, 9, 0), at(3, 11, 45)},
		{frete, bruno, at(2, 14, 0), at(2, 15, 20)},
		{checkout, bruno, at(1, 9, 10), at(1, 12, 5)},
		{gateway, ana, at(2, 10, 0), at(2, 12, 30)},
		{gateway, ana, at(1, 14, 0), at(1, 17, 15)},
		{textos, ana, at(1, 17, 30), at(1, 18, 10)},
	}
	for _, s := range sessions {
		if s.end.After(now) {
			continue
		}
		db.Client.WorkSession.Create().
			SetTaskID(s.task.ID).
			SetPersonID(s.person.PersonID).
			SetStartAt(s.start).
			SetEndAt(s.end).
			SaveX(ctx)
	}

	fmt.Println("Dados de demonstração criados.")
	fmt.Println()
	fmt.Println("  Organização: Acme Delivery · projeto App de Pedidos")
	fmt.Printf("  Admin:  ana@example.com   / %s\n", password)
	fmt.Printf("  Membro: bruno@example.com / %s\n", password)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
