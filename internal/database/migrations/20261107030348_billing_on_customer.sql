-- +goose Up
-- A cobrança passa a morar num lugar só, o cliente (Decisão 21). O cliente por hora tem o valor por hora, que vale
-- para todos os projetos dele, e o projeto só guarda um acréscimo opcional sobre a cobrança do cliente (por hora no
-- cliente por hora, por mês no mensal). O valor por hora de cada projeto sai do projeto.
--
-- Os números de hoje ficam iguais: o cliente por hora com projetos recebe o menor valor cobrado entre os projetos
-- dele, e cada projeto fica com a diferença como acréscimo (zero ou mais). O cliente sem projetos guarda o valor
-- padrão que tinha, ou fica sem valor até alguém informar em Clientes. As sessões não mudam: cada uma guarda o valor
-- cobrado do clock-in.
-- modify "customers" table
ALTER TABLE "customers" RENAME COLUMN "default_rate_cents" TO "hourly_rate_cents";
-- modify "projects" table
ALTER TABLE "projects" ADD COLUMN "bill_extra_cents" bigint NOT NULL DEFAULT 0;
UPDATE "customers" AS c
SET "hourly_rate_cents" = m.min_rate
FROM (
  SELECT "customer_id", MIN("bill_rate_cents") AS min_rate
  FROM "projects"
  WHERE "customer_id" IS NOT NULL AND "bill_rate_cents" IS NOT NULL
  GROUP BY "customer_id"
) AS m
WHERE c."id" = m."customer_id" AND c."billing_mode" = 'hourly';
UPDATE "projects" AS p
SET "bill_extra_cents" = p."bill_rate_cents" - c."hourly_rate_cents"
FROM "customers" AS c
WHERE p."customer_id" = c."id" AND c."billing_mode" = 'hourly'
  AND p."bill_rate_cents" IS NOT NULL AND c."hourly_rate_cents" IS NOT NULL;
ALTER TABLE "projects" DROP COLUMN "bill_rate_cents";
