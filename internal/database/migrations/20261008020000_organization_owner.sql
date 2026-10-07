-- +goose Up
-- O dono da organização é quem a criou: nas organizações que já existem, o primeiro
-- admin (o mais antigo) passa a ser o dono. Cada organização tem no máximo um.
-- modify "persons" table
ALTER TABLE "persons" ADD COLUMN "is_owner" boolean NOT NULL DEFAULT false;
UPDATE "persons" SET "is_owner" = true
WHERE "id" IN (
  SELECT DISTINCT ON ("organization_id") "id"
  FROM "persons"
  WHERE "role" = 'admin'
  ORDER BY "organization_id", "created_at", "id"
);
-- create index "one_owner_per_organization" to table: "persons"
CREATE UNIQUE INDEX "one_owner_per_organization" ON "persons" ("organization_id") WHERE is_owner;
-- As horas do dono valem o valor cobrado e não têm custo. As sessões que já existem
-- ficam como estão: não eram do dono, para a conta de custo e margem.
-- modify "work_sessions" table
ALTER TABLE "work_sessions" ADD COLUMN "owner_hours" boolean NOT NULL DEFAULT false;
