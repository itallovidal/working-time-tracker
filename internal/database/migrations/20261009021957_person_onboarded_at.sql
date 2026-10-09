-- +goose Up
-- As boas-vindas do primeiro acesso são do dono que acabou de criar a organização. Quem já existe quando esta migração
-- roda não é "primeiro acesso": fica como se já as tivesse visto, senão todo dono antigo levaria o modal no próximo login.
-- modify "persons" table
ALTER TABLE "persons" ADD COLUMN "onboarded_at" timestamptz NULL;
UPDATE "persons" SET "onboarded_at" = now();
