-- +goose Up
-- modify "persons" table
ALTER TABLE "persons" DROP COLUMN "permissions";
