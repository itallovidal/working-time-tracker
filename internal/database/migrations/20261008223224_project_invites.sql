-- +goose Up
-- modify "invites" table
ALTER TABLE "invites" ADD COLUMN "pay_rate_cents" bigint NULL, ADD COLUMN "preset" character varying NULL, ADD COLUMN "project_id" uuid NULL, ADD COLUMN "team_id" uuid NULL, ADD
CONSTRAINT "invites_projects_invites" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE SET NULL, ADD
CONSTRAINT "invites_teams_invites" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE SET NULL;
