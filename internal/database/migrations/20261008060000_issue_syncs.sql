-- +goose Up
-- modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "sync_issues" boolean NOT NULL DEFAULT false, ADD COLUMN "sync_cursor" timestamptz NULL, ADD COLUMN "last_synced_at" timestamptz NULL, ADD COLUMN "last_sync_error" character varying NOT NULL DEFAULT '';
-- create "issue_syncs" table
CREATE TABLE "issue_syncs" (
  "id" uuid NOT NULL,
  "issue_number" bigint NOT NULL,
  "state" character varying NOT NULL DEFAULT 'open',
  "title" character varying NOT NULL DEFAULT '',
  "body" character varying NOT NULL DEFAULT '',
  "labels" jsonb NULL,
  "assignee_logins" jsonb NULL,
  "mapped_login" character varying NOT NULL DEFAULT '',
  "mapped_person_id" uuid NULL,
  "stuck_sig" character varying NOT NULL DEFAULT '',
  "last_error" character varying NOT NULL DEFAULT '',
  "synced_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  "integration_id" uuid NOT NULL,
  "task_id" uuid NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "issue_syncs_integrations_issue_syncs" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "issue_syncs_tasks_issue_sync" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- create index "issue_syncs_task_id_key" to table: "issue_syncs"
CREATE UNIQUE INDEX "issue_syncs_task_id_key" ON "issue_syncs" ("task_id");
-- create index "issuesync_integration_id_issue_number" to table: "issue_syncs"
CREATE UNIQUE INDEX "issuesync_integration_id_issue_number" ON "issue_syncs" ("integration_id", "issue_number");
