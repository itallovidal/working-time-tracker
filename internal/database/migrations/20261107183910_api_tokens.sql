-- +goose Up
-- Os tokens pessoais do Claude (o servidor MCP em /mcp). Como as sessões, só o sha256 do token fica no banco, e os
-- tokens somem com a pessoa.
-- create "api_tokens" table
CREATE TABLE "api_tokens" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "token_hash" character varying NOT NULL,
  "prefix" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "last_used_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "api_tokens_persons_api_tokens" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON DELETE CASCADE
);
-- create index "api_tokens_token_hash_key" to table: "api_tokens"
CREATE UNIQUE INDEX "api_tokens_token_hash_key" ON "api_tokens" ("token_hash");
-- create index "apitoken_person_id" to table: "api_tokens"
CREATE INDEX "apitoken_person_id" ON "api_tokens" ("person_id");
