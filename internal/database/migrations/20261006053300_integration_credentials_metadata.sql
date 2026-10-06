-- +goose Up
-- rename a column from "config" to "credentials"
ALTER TABLE "integrations" RENAME COLUMN "config" TO "credentials";
-- modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "metadata" jsonb NULL;
