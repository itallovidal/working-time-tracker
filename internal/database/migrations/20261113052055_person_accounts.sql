-- +goose Up
-- modify "tasks" table
ALTER TABLE "tasks" ADD COLUMN "required_accounts" jsonb NULL;
-- create "person_accounts" table
CREATE TABLE "person_accounts" (
  "id" uuid NOT NULL,
  "platform" character varying NOT NULL,
  "username" character varying NOT NULL DEFAULT '',
  "email" character varying NOT NULL DEFAULT '',
  "no_account" boolean NOT NULL DEFAULT false,
  "source" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "organization_id" uuid NOT NULL,
  "person_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "person_accounts_organizations_person_accounts" FOREIGN KEY ("organization_id") REFERENCES "organizations" ("id") ON DELETE CASCADE,
  CONSTRAINT "person_accounts_persons_accounts" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON DELETE CASCADE
);
-- create index "personaccount_person_id_platform" to table: "person_accounts"
CREATE UNIQUE INDEX "personaccount_person_id_platform" ON "person_accounts" ("person_id", "platform");
-- create index "one_person_per_platform_login" to table: "person_accounts"
CREATE UNIQUE INDEX "one_person_per_platform_login" ON "person_accounts" ("organization_id", "platform", "username") WHERE ((username)::text <> ''::text);
