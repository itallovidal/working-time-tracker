-- +goose Up
-- As permissões de quem não é admin: as do projeto ficam na alocação da pessoa, com o nome
-- do grupo que as deu, e as da organização, na própria pessoa. Quem já estava num projeto
-- fica no grupo "member", sem nenhuma permissão a mais.
-- modify "allocations" table
ALTER TABLE "allocations" ADD COLUMN "permissions" jsonb NULL, ADD COLUMN "preset" character varying NOT NULL DEFAULT 'member';
-- modify "persons" table
ALTER TABLE "persons" ADD COLUMN "permissions" jsonb NULL;
