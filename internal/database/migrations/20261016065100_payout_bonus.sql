-- +goose Up
-- modify "payouts" table
ALTER TABLE "payouts" ADD COLUMN "bonus_cents" bigint NOT NULL DEFAULT 0;
