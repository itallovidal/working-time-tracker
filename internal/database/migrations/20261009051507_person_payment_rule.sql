-- +goose Up
-- modify "persons" table
ALTER TABLE "persons" ADD COLUMN "payment_frequency" character varying NULL, ADD COLUMN "payment_day" bigint NULL, ADD COLUMN "payment_start" character varying NULL;
