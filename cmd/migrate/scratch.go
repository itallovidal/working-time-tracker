package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// createScratch cria um banco vazio no mesmo servidor do DATABASE_URL e devolve a
// conexão com ele e a função que o apaga. É nele que as migrações são aplicadas para
// gerar a seguinte: o banco de verdade nunca é tocado.
func createScratch(ctx context.Context, dsn string) (*sql.DB, func(), error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	admin := stdlib.OpenDB(*cfg)

	name := fmt.Sprintf("wtt_migrate_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" TEMPLATE template0"); err != nil {
		admin.Close()
		return nil, nil, fmt.Errorf("criando o banco temporário %s: %w", name, err)
	}

	scratchCfg := cfg.Copy()
	scratchCfg.Database = name
	scratch := stdlib.OpenDB(*scratchCfg)

	cleanup := func() {
		scratch.Close()
		// Contexto próprio: a limpeza tem de rodar mesmo se o comando foi interrompido.
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			fmt.Printf("aviso: não consegui apagar o banco temporário %s: %v\n", name, err)
		}
		admin.Close()
	}
	return scratch, cleanup, nil
}
