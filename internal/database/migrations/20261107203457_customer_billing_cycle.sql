-- +goose Up
-- modify "customers" table
ALTER TABLE "customers" ADD COLUMN "billing_cycle" character varying NOT NULL DEFAULT 'monthly', ADD COLUMN "cycle_anchor" character varying NULL;
