-- +goose Up
-- O convite por e-mail é criado também no Clerk, que manda o e-mail. Guarda o id de lá para cancelar o convite
-- quando ele é revogado ou substituído por outro para o mesmo email. Os convites que já existem ficam sem.
-- modify "invites" table
ALTER TABLE "invites" ADD COLUMN "clerk_invitation_id" character varying NULL;
