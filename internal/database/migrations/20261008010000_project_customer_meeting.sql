-- +goose Up
-- A reunião semanal com o cliente, com dia e horário. É opcional: os projetos que já
-- existem ficam sem, e um projeto sem cliente não tem reunião.
-- modify "projects" table
ALTER TABLE "projects" ADD COLUMN "customer_meeting_day" character varying NULL, ADD COLUMN "customer_meeting_time" character varying NULL;
