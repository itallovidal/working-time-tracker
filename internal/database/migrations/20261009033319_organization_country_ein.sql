-- +goose Up
-- O país da organização era texto livre e passa a ser o código do cadastro de países (BR, US). Antes de o banco exigir o
-- valor, converte o que já existe: os Estados Unidos, em qualquer grafia comum, viram US; todo o resto, inclusive vazio e
-- "Brasil", vira BR, porque até aqui o sistema só tinha CNPJ, BRL e America/Sao_Paulo como padrão. Um país escrito que não
-- é nenhum dos dois (ex.: "Portugal") não se perde: o texto antigo vai para o complemento do endereço. Num UPDATE só, o
-- lado direito de cada atribuição enxerga a linha antiga, então o complemento e o país usam o mesmo valor de partida.
UPDATE "organizations" SET
  "address_line2" = CASE
    WHEN upper(btrim(coalesce("country", ''))) IN (
      '', 'BR', 'BRASIL', 'BRAZIL',
      'US', 'USA', 'U.S.', 'U.S.A.', 'EUA', 'ESTADOS UNIDOS', 'ESTADOS UNIDOS DA AMERICA', 'ESTADOS UNIDOS DA AMÉRICA',
      'UNITED STATES', 'UNITED STATES OF AMERICA'
    ) THEN "address_line2"
    ELSE concat_ws(' · ', nullif("address_line2", ''), btrim("country"))
  END,
  "country" = CASE
    WHEN upper(btrim(coalesce("country", ''))) IN (
      'US', 'USA', 'U.S.', 'U.S.A.', 'EUA', 'ESTADOS UNIDOS', 'ESTADOS UNIDOS DA AMERICA', 'ESTADOS UNIDOS DA AMÉRICA',
      'UNITED STATES', 'UNITED STATES OF AMERICA'
    ) THEN 'US'
    ELSE 'BR'
  END;
-- modify "organizations" table
ALTER TABLE "organizations" ALTER COLUMN "country" SET NOT NULL, ALTER COLUMN "country" SET DEFAULT 'BR', ADD COLUMN "ein" character varying NULL;
-- O CEP e o estado passam a seguir o formato do país. O que já está quase certo é ajustado aqui: CEP de 8 dígitos ganha o
-- hífen, e a sigla do estado vai para maiúsculas. O resto (um estado por extenso, um CEP fora do padrão) fica como está e
-- é escolhido de novo na tela de edição.
UPDATE "organizations"
  SET "postal_code" = left(regexp_replace("postal_code", '[^0-9]', '', 'g'), 5) || '-' || right(regexp_replace("postal_code", '[^0-9]', '', 'g'), 3)
  WHERE "country" = 'BR' AND "postal_code" ~ '^\s*[0-9]{5}[-. ]?[0-9]{3}\s*$';
UPDATE "organizations" SET "state" = upper(btrim("state")) WHERE btrim("state") ~* '^[a-z]{2}$';
