-- +goose Up
-- Quem entra pelo Clerk guarda o usuário de lá. É único; as contas que já existem ficam sem (entram por senha
-- e, no primeiro login pelo Clerk, são ligadas pelo e-mail).
-- modify "persons" table
ALTER TABLE "persons" ADD COLUMN "clerk_user_id" character varying NULL;
-- create index "persons_clerk_user_id_key" to table: "persons"
CREATE UNIQUE INDEX "persons_clerk_user_id_key" ON "persons" ("clerk_user_id");
