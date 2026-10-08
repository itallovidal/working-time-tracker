-- +goose Up
-- O vínculo da tarefa com um item externo passa a morar só em issue_syncs, e uma tarefa pode ter um por integração (a issue do GitHub e o cartão do Trello ao mesmo tempo). As colunas external_* da tarefa saem.
-- modify "issue_syncs" table: o endereço do item, que ficava na tarefa
ALTER TABLE "issue_syncs" ADD COLUMN "url" character varying NOT NULL DEFAULT '';
-- drop index "issue_syncs_task_id_key" from table: "issue_syncs"
DROP INDEX "issue_syncs_task_id_key";
-- modify "issue_syncs" table: a aresta da tarefa deixa de ser única (o nome da chave estrangeira muda com ela)
ALTER TABLE "issue_syncs" DROP CONSTRAINT "issue_syncs_tasks_issue_sync", ADD CONSTRAINT "issue_syncs_tasks_issue_syncs" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "issuesync_task_id_integration_id" to table: "issue_syncs"
CREATE UNIQUE INDEX "issuesync_task_id_integration_id" ON "issue_syncs" ("task_id", "integration_id");

-- Os vínculos de hoje, com a chave do item como a sincronização a lê: o link curto no Trello (que pode estar guardado como a URL do cartão), o texto sem espaços nos demais.
CREATE TEMPORARY TABLE "legacy_links" ON COMMIT DROP AS
SELECT t."id" AS "task_id", t."external_integration_id" AS "integration_id",
  CASE WHEN i."type" = 'trello'
    THEN COALESCE(substring(t."external_item_id" from 'trello[.]com/c/([A-Za-z0-9]+)'), btrim(t."external_item_id"))
    ELSE btrim(t."external_item_id") END AS "item_id",
  COALESCE(t."external_item_url", '') AS "url",
  t."created_at" AS "created_at"
FROM "tasks" t JOIN "integrations" i ON i."id" = t."external_integration_id"
WHERE btrim(COALESCE(t."external_item_id", '')) <> '';

-- 1) A linha presa a uma tarefa que já não aponta para aquele item (foi desligada, ou ligada a outro) vira item descartado.
UPDATE "issue_syncs" s SET "task_id" = NULL
WHERE s."task_id" IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM "legacy_links" l WHERE l."task_id" = s."task_id" AND l."integration_id" = s."integration_id" AND l."item_id" = s."item_id");

-- 2) O endereço guardado na tarefa vai para a linha que ela já tem.
UPDATE "issue_syncs" s SET "url" = l."url"
FROM "legacy_links" l
WHERE s."task_id" = l."task_id" AND s."integration_id" = l."integration_id" AND s."item_id" = l."item_id";

-- 3) A tarefa ligada à mão, que ainda não tinha linha, ganha uma "pending": a próxima rodada a adota como hoje (a mais antiga vence por item; só um item descartado é religado).
INSERT INTO "issue_syncs" ("id", "integration_id", "task_id", "item_id", "url", "state", "synced_at", "created_at")
SELECT gen_random_uuid(), l."integration_id", l."task_id", l."item_id", l."url", 'pending', now(), now()
FROM (SELECT DISTINCT ON ("integration_id", "item_id") * FROM "legacy_links" ORDER BY "integration_id", "item_id", "created_at", "task_id") l
WHERE NOT EXISTS (SELECT 1 FROM "issue_syncs" s WHERE s."task_id" = l."task_id" AND s."integration_id" = l."integration_id")
ON CONFLICT ("integration_id", "item_id") DO UPDATE SET
  "task_id" = EXCLUDED."task_id", "state" = 'pending', "title" = '', "body" = '', "labels" = NULL, "assignee_logins" = NULL,
  "mapped_login" = '', "mapped_person_id" = NULL, "deadline" = NULL, "stuck_sig" = '', "last_error" = '', "url" = EXCLUDED."url"
WHERE "issue_syncs"."task_id" IS NULL;

-- modify "tasks" table: o vínculo já está em issue_syncs (o de uma integração que foi apagada fica para trás)
ALTER TABLE "tasks" DROP CONSTRAINT "tasks_integrations_tasks", DROP COLUMN "external_item_id", DROP COLUMN "external_item_url", DROP COLUMN "external_integration_id";
