-- +goose Up
-- A weekly passa a ter horário, além do dia. As que já existem ficam só com o
-- dia: o horário é opcional na coluna, e a tela pede quando alguém edita o projeto.
-- modify "projects" table
ALTER TABLE "projects" ADD COLUMN "weekly_sync_time" character varying NULL;
