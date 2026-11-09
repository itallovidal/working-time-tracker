-- +goose Up
-- Os apps conectados pelo OAuth do servidor MCP (o claude.ai, o Claude Code) e os tokens de acesso deles. Como nas
-- sessões, só o sha256 de cada código e token fica no banco, e tudo some com a pessoa.
-- create "oauth_grants" table
CREATE TABLE "oauth_grants" (
  "id" uuid NOT NULL,
  "client_id" character varying NOT NULL,
  "client_name" character varying NOT NULL,
  "redirect_uri" character varying NOT NULL,
  "scope" character varying NOT NULL,
  "resource" character varying NOT NULL,
  "code_hash" character varying NOT NULL,
  "code_challenge" character varying NOT NULL,
  "code_expires_at" timestamptz NOT NULL,
  "code_used_at" timestamptz NULL,
  "refresh_hash" character varying NULL,
  "prev_refresh_hash" character varying NULL,
  "refresh_replay_enc" character varying NULL,
  "rotated_at" timestamptz NULL,
  "refresh_expires_at" timestamptz NULL,
  "last_used_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "oauth_grants_persons_oauth_grants" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON DELETE CASCADE
);
-- create index "oauth_grants_code_hash_key" to table: "oauth_grants"
CREATE UNIQUE INDEX "oauth_grants_code_hash_key" ON "oauth_grants" ("code_hash");
-- create index "oauth_grants_refresh_hash_key" to table: "oauth_grants"
CREATE UNIQUE INDEX "oauth_grants_refresh_hash_key" ON "oauth_grants" ("refresh_hash");
-- create index "oauth_grants_prev_refresh_hash_key" to table: "oauth_grants"
CREATE UNIQUE INDEX "oauth_grants_prev_refresh_hash_key" ON "oauth_grants" ("prev_refresh_hash");
-- create index "oauthgrant_person_id" to table: "oauth_grants"
CREATE INDEX "oauthgrant_person_id" ON "oauth_grants" ("person_id");
-- create "oauth_access_tokens" table
CREATE TABLE "oauth_access_tokens" (
  "id" uuid NOT NULL,
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  "grant_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "oauth_access_tokens_oauth_grants_access_tokens" FOREIGN KEY ("grant_id") REFERENCES "oauth_grants" ("id") ON DELETE CASCADE
);
-- create index "oauth_access_tokens_token_hash_key" to table: "oauth_access_tokens"
CREATE UNIQUE INDEX "oauth_access_tokens_token_hash_key" ON "oauth_access_tokens" ("token_hash");
-- create index "oauthaccesstoken_grant_id_expires_at" to table: "oauth_access_tokens"
CREATE INDEX "oauthaccesstoken_grant_id_expires_at" ON "oauth_access_tokens" ("grant_id", "expires_at");
