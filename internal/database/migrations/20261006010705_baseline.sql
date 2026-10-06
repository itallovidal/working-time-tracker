-- +goose Up
-- create "organizations" table
CREATE TABLE "organizations" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "summary" character varying NULL,
  "description" text NULL,
  "industry" character varying NULL,
  "founded_year" bigint NULL,
  "size" character varying NULL,
  "website" character varying NULL,
  "contact_email" character varying NULL,
  "phone" character varying NULL,
  "linkedin_url" character varying NULL,
  "instagram_url" character varying NULL,
  "legal_name" character varying NULL,
  "cnpj" character varying NULL,
  "address_line1" character varying NULL,
  "address_line2" character varying NULL,
  "city" character varying NULL,
  "state" character varying NULL,
  "postal_code" character varying NULL,
  "country" character varying NULL,
  "work_mode" character varying NULL,
  "timezone" character varying NOT NULL DEFAULT 'America/Sao_Paulo',
  "currency" character varying NOT NULL DEFAULT 'BRL',
  "created_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
-- create "persons" table
CREATE TABLE "persons" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "email" character varying NOT NULL,
  "password_hash" character varying NULL,
  "role" character varying NOT NULL DEFAULT 'member',
  "created_at" timestamptz NOT NULL,
  "organization_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "persons_organizations_persons" FOREIGN KEY ("organization_id") REFERENCES "organizations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "person_email" to table: "persons"
CREATE UNIQUE INDEX "person_email" ON "persons" ("email");
-- create "customers" table
CREATE TABLE "customers" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "document" character varying NULL,
  "contact_name" character varying NULL,
  "contact_email" character varying NULL,
  "contact_phone" character varying NULL,
  "created_at" timestamptz NOT NULL,
  "organization_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "customers_organizations_customers" FOREIGN KEY ("organization_id") REFERENCES "organizations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create "projects" table
CREATE TABLE "projects" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "description" character varying NULL,
  "github_repo_url" character varying NULL,
  "gitlab_repo_url" character varying NULL,
  "sprint_duration_days" bigint NOT NULL DEFAULT 14,
  "weekly_hours" bigint NULL,
  "daily_time" character varying NULL,
  "weekly_sync_day" character varying NULL,
  "bill_rate_cents" bigint NULL,
  "created_at" timestamptz NOT NULL,
  "customer_id" uuid NULL,
  "organization_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "projects_customers_projects" FOREIGN KEY ("customer_id") REFERENCES "customers" ("id") ON UPDATE NO ACTION ON DELETE SET NULL,
  CONSTRAINT "projects_organizations_projects" FOREIGN KEY ("organization_id") REFERENCES "organizations" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
-- create "allocations" table
CREATE TABLE "allocations" (
  "id" uuid NOT NULL,
  "pay_rate_cents" bigint NOT NULL,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  "project_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "allocations_persons_allocations" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "allocations_projects_allocations" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "allocation_project_id_person_id" to table: "allocations"
CREATE UNIQUE INDEX "allocation_project_id_person_id" ON "allocations" ("project_id", "person_id");
-- create "integrations" table
CREATE TABLE "integrations" (
  "id" uuid NOT NULL,
  "type" character varying NOT NULL,
  "display_name" character varying NOT NULL,
  "config" jsonb NULL,
  "enabled" boolean NOT NULL DEFAULT true,
  "created_at" timestamptz NOT NULL,
  "project_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "integrations_projects_integrations" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create "invites" table
CREATE TABLE "invites" (
  "id" uuid NOT NULL,
  "token_hash" character varying NOT NULL,
  "email" character varying NULL,
  "role" character varying NOT NULL DEFAULT 'member',
  "expires_at" timestamptz NOT NULL,
  "accepted_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL,
  "organization_id" uuid NOT NULL,
  "created_by_id" uuid NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "invites_organizations_invites" FOREIGN KEY ("organization_id") REFERENCES "organizations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "invites_persons_created_invites" FOREIGN KEY ("created_by_id") REFERENCES "persons" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- create index "invites_token_hash_key" to table: "invites"
CREATE UNIQUE INDEX "invites_token_hash_key" ON "invites" ("token_hash");
-- create "sessions" table
CREATE TABLE "sessions" (
  "id" uuid NOT NULL,
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "sessions_persons_sessions" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "sessions_token_hash_key" to table: "sessions"
CREATE UNIQUE INDEX "sessions_token_hash_key" ON "sessions" ("token_hash");
-- create "tasks" table
CREATE TABLE "tasks" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "description" character varying NULL,
  "deadline" timestamptz NULL,
  "external_item_id" character varying NULL,
  "external_item_url" character varying NULL,
  "created_at" timestamptz NOT NULL,
  "external_integration_id" uuid NULL,
  "assignee_id" uuid NOT NULL,
  "project_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "tasks_integrations_tasks" FOREIGN KEY ("external_integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE SET NULL,
  CONSTRAINT "tasks_persons_tasks" FOREIGN KEY ("assignee_id") REFERENCES "persons" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "tasks_projects_tasks" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create "teams" table
CREATE TABLE "teams" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "project_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "teams_projects_teams" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create "team_memberships" table
CREATE TABLE "team_memberships" (
  "id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  "team_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "team_memberships_persons_team_memberships" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "team_memberships_teams_memberships" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "teammembership_person_id_team_id" to table: "team_memberships"
CREATE UNIQUE INDEX "teammembership_person_id_team_id" ON "team_memberships" ("person_id", "team_id");
-- create "work_sessions" table
CREATE TABLE "work_sessions" (
  "id" uuid NOT NULL,
  "start_at" timestamptz NOT NULL,
  "end_at" timestamptz NULL,
  "pay_rate_cents" bigint NULL,
  "bill_rate_cents" bigint NULL,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  "task_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "work_sessions_persons_work_sessions" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "work_sessions_tasks_work_sessions" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "one_active_session" to table: "work_sessions"
CREATE UNIQUE INDEX "one_active_session" ON "work_sessions" ("person_id") WHERE (end_at IS NULL);
