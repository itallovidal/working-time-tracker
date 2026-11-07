-- +goose Up
-- modify "projects" table
ALTER TABLE "projects" ADD COLUMN "customer_meeting_every_weeks" bigint NOT NULL DEFAULT 1;
