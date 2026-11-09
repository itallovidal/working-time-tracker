-- +goose Up
-- modify "customers" table
ALTER TABLE "customers" ADD COLUMN "estimated_monthly_hours" bigint NULL;
