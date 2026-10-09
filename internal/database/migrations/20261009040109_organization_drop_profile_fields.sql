-- +goose Up
-- O perfil da organização perdeu o segmento, o porte, o ano de fundação, o telefone, o Instagram, a cidade, o estado e o
-- código postal: ninguém os usava, e o estado e o código postal só existiam para as regras de cada país, que o endereço
-- (as duas linhas e o país) não precisa. As migrações só andam para frente, então o que estava nessas colunas some: quem
-- quiser guardar algum valor antes de aplicar deve exportá-lo.
-- modify "organizations" table
ALTER TABLE "organizations" DROP COLUMN "industry", DROP COLUMN "founded_year", DROP COLUMN "size", DROP COLUMN "phone", DROP COLUMN "instagram_url", DROP COLUMN "city", DROP COLUMN "state", DROP COLUMN "postal_code";
