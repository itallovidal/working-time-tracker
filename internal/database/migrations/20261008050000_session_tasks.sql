-- +goose Up
-- A sessão deixa de ser de uma tarefa: passa a ser de um projeto e a ter tarefas, cada uma
-- com o intervalo em que esteve nela. As sessões que já existem ficam com a tarefa que tinham,
-- do início ao fim da sessão.
-- create "work_session_tasks" table
CREATE TABLE "work_session_tasks" (
  "id" uuid NOT NULL,
  "from_at" timestamptz NOT NULL,
  "until_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL,
  "task_id" uuid NOT NULL,
  "session_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "work_session_tasks_tasks_session_links" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "work_session_tasks_work_sessions_task_links" FOREIGN KEY ("session_id") REFERENCES "work_sessions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "worksessiontask_session_id" to table: "work_session_tasks"
CREATE INDEX "worksessiontask_session_id" ON "work_session_tasks" ("session_id");
-- create index "worksessiontask_task_id" to table: "work_session_tasks"
CREATE INDEX "worksessiontask_task_id" ON "work_session_tasks" ("task_id");
INSERT INTO "work_session_tasks" ("id", "session_id", "task_id", "from_at", "until_at", "created_at")
SELECT gen_random_uuid(), "id", "task_id", "start_at", "end_at", "created_at" FROM "work_sessions";
-- modify "work_sessions" table
ALTER TABLE "work_sessions" ADD COLUMN "project_id" uuid NULL;
UPDATE "work_sessions" ws SET "project_id" = t."project_id" FROM "tasks" t WHERE t."id" = ws."task_id";
ALTER TABLE "work_sessions" ALTER COLUMN "project_id" SET NOT NULL, DROP COLUMN "task_id", ADD
CONSTRAINT "work_sessions_projects_work_sessions" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
