-- +goose Up
-- A tarefa passa a ter vários responsáveis (Decisão 31): tasks.assignee_id vira a tabela task_assignees, e o
-- responsável de cada tarefa que já existe vai para ela. No vínculo com a issue, o par login/pessoa vira a lista
-- "mapped" (login -> pessoa), com o par que já estava ligado.
-- create "task_assignees" table
CREATE TABLE "task_assignees" (
  "task_id" uuid NOT NULL,
  "person_id" uuid NOT NULL,
  PRIMARY KEY ("task_id", "person_id"),
  CONSTRAINT "task_assignees_task_id" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON DELETE CASCADE,
  CONSTRAINT "task_assignees_person_id" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON DELETE CASCADE
);
INSERT INTO "task_assignees" ("task_id", "person_id")
SELECT "id", "assignee_id" FROM "tasks" WHERE "assignee_id" IS NOT NULL;
-- modify "tasks" table
ALTER TABLE "tasks" DROP COLUMN "assignee_id";
-- modify "issue_syncs" table
ALTER TABLE "issue_syncs" ADD COLUMN "mapped" jsonb NULL;
UPDATE "issue_syncs" SET "mapped" = jsonb_build_object("mapped_login", "mapped_person_id"::text)
WHERE "mapped_login" <> '' AND "mapped_person_id" IS NOT NULL;
ALTER TABLE "issue_syncs" DROP COLUMN "mapped_login", DROP COLUMN "mapped_person_id";
