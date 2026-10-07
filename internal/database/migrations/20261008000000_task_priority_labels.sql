-- +goose Up
-- create "labels" table
CREATE TABLE "labels" (
  "id" uuid NOT NULL,
  "name" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "project_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "labels_projects_labels" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "label_project_id_name" to table: "labels"
CREATE UNIQUE INDEX "label_project_id_name" ON "labels" ("project_id", "name");
-- modify "tasks" table
ALTER TABLE "tasks" ADD COLUMN "priority" character varying NOT NULL DEFAULT 'none';
-- create "task_labels" table
CREATE TABLE "task_labels" (
  "task_id" uuid NOT NULL,
  "label_id" uuid NOT NULL,
  PRIMARY KEY ("task_id", "label_id"),
  CONSTRAINT "task_labels_label_id" FOREIGN KEY ("label_id") REFERENCES "labels" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "task_labels_task_id" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
