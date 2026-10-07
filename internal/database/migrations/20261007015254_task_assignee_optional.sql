-- +goose Up
-- modify "tasks" table
ALTER TABLE "tasks" ALTER COLUMN "assignee_id" DROP NOT NULL;
