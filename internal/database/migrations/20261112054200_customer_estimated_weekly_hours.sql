-- +goose Up
-- A estimativa de horas do contrato mensal passa a ser por semana (Decisão 29); a meta do período sai dela, vezes os
-- dias do período sobre 7. A que já existia, por mês, vira a semanal mais perto: 12 meses em 52 semanas, nunca menos
-- de 1.
ALTER TABLE "customers" RENAME COLUMN "estimated_monthly_hours" TO "estimated_weekly_hours";
UPDATE "customers"
SET "estimated_weekly_hours" = LEAST(168, GREATEST(1, ROUND("estimated_weekly_hours" * 12 / 52.0)))
WHERE "estimated_weekly_hours" IS NOT NULL;
