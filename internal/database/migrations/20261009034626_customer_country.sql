-- +goose Up
-- modify "customers" table
ALTER TABLE "customers" ADD COLUMN "country" character varying NOT NULL DEFAULT 'BR';
-- O documento de um cliente era sempre um CNPJ. Agora ele tem país, e o dos clientes que já existem é o da organização
-- deles (que, depois da migração anterior, é BR ou US): é o país em que a organização atende, e o documento guardado
-- continua valendo para ele.
UPDATE "customers" c SET "country" = o."country" FROM "organizations" o WHERE o."id" = c."organization_id";
