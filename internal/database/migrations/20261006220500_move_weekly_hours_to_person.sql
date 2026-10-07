-- +goose Up
-- A jornada semanal deixa de ser do projeto e passa a ser da pessoa: é o que a
-- organização combina com ela, e vale para todos os projetos em que trabalha.
-- Os valores não são copiados. A jornada de um projeto era a expectativa para
-- quem trabalhasse nele, e dela não dá para saber a jornada de cada pessoa: quem
-- está em dois projetos teria duas. Um admin informa a de cada pessoa na aba
-- Colaboradores da organização.
-- modify "persons" table
ALTER TABLE "persons" ADD COLUMN "weekly_hours" bigint NULL;
-- modify "projects" table
ALTER TABLE "projects" DROP COLUMN "weekly_hours";
