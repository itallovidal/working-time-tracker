package testutil

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Setup abre conexão com o banco de testes usando TEST_DATABASE_URL
// (fallback: postgres://localhost:5432/working_time_tracker_test?sslmode=disable).
// Não roda AutoMigrate — cada package de teste externo (_test) deve fazê-lo
// para evitar ciclos de import (database importa os domains, que não podem
// importar database de volta).
func Setup() *gorm.DB {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/working_time_tracker_test?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("failed to connect to test database: %v", err)
	}

	return db
}
