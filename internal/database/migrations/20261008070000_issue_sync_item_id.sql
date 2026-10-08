-- +goose Up
-- O número da issue vira a chave do item, em texto (o Trello usa o link curto do cartão); os vínculos que já existem ficam como estão.
ALTER TABLE "issue_syncs" RENAME COLUMN "issue_number" TO "item_id";
ALTER TABLE "issue_syncs" ALTER COLUMN "item_id" TYPE character varying USING "item_id"::text;
ALTER INDEX "issuesync_integration_id_issue_number" RENAME TO "issuesync_integration_id_item_id";
-- O prazo do último acordo; nulo é sem prazo.
ALTER TABLE "issue_syncs" ADD COLUMN "deadline" timestamptz NULL;
