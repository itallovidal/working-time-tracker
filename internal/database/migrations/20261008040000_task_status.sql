-- +goose Up
-- modify "tasks" table
ALTER TABLE "tasks" ADD COLUMN "status" character varying NOT NULL DEFAULT 'backlog';
