-- +goose Up
-- drop index "one_active_session" from table: "work_sessions"
DROP INDEX "one_active_session";
-- create index "one_open_session_per_project" to table: "work_sessions"
CREATE UNIQUE INDEX "one_open_session_per_project" ON "work_sessions" ("person_id", "project_id") WHERE end_at IS NULL;
