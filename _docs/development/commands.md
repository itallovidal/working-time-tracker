# Comandos

O [`Makefile`](../../Makefile) da raiz junta os comandos do dia a dia com nomes curtos. `make` sozinho lista todos,
com uma linha sobre cada um. Os parâmetros vão depois do nome, sem traço: `make migrate-new name=add_payout_note`.

O `make` já vem no macOS e no Linux; no Windows, use o WSL. Cada comando abaixo diz o que roda por baixo, para quem
preferir digitar à mão ou precisar mudar algum detalhe.

## Primeira vez

```bash
make setup    # cria o .env com uma chave nova e sobe o banco
make seed     # opcional: dados de demonstração
make run      # http://localhost:1243
```

## O que rodar para cada coisa

| Quero | Rode |
|---|---|
| Subir o sistema | `make run`, ou `make dev` para reiniciar sozinho a cada mudança |
| Ver as telas cheias, sem mexer no meu banco | `make demo` |
| Mudar o banco | Edite o `ent/schema` e rode `make migrate-new name=<nome>` |
| Conferir antes de um commit | `make check` |
| Rodar um teste só | `make test-pkg pkg=./internal/domain/task run=TestNome` |
| Voltar o banco de dev para a demonstração | `make db-reset` |
| Abrir o banco no terminal | `make db-psql` |
| Testar a imagem de produção | `make image-run` |
| Guardar o banco de produção | `make backup url="postgresql://..."` |

## Ambiente

| Comando | O que roda | Detalhes |
|---|---|---|
| `make setup` | `cp .env.example .env`, a chave com `openssl rand -hex 32` e o `make db-up` | O `.env` só é criado quando não existe: um `.env` que já está aí nunca é sobrescrito |
| `make db-up` | `docker compose up -d --wait` | Espera o Postgres ficar pronto e cria o `working_time_tracker_test` se ele faltar (num volume criado antes do `docker/initdb`) |
| `make db-down` | `docker compose down` | Os dados ficam no volume. Para apagar tudo, `docker compose down -v`, que de propósito não tem atalho |
| `make db-psql` | `docker compose exec db psql -U wtt -d working_time_tracker` | `db=working_time_tracker_test` abre o banco dos testes |
| `make db-reset` | `DROP DATABASE`, `CREATE DATABASE` e o seed | Pede confirmação (`s`). Derruba as conexões abertas, a de um servidor rodando também, e o seed vai sempre no banco do `docker compose`, qualquer que seja o `DATABASE_URL` |

A porta do Postgres é a do `POSTGRES_PORT`, do ambiente ou do `.env` (que o `docker compose` também lê), e `5432` sem
nenhum dos dois. Se mudá-la, ajuste o `DATABASE_URL` e o `TEST_DATABASE_URL` no `.env`.

## Rodar

| Comando | O que roda | Detalhes |
|---|---|---|
| `make run` | `go run ./cmd` | Na porta do `API_PORT` do `.env` (1243). Aplica as migrações pendentes ao subir |
| `make dev` | O `air`, que recompila e reinicia o servidor | Ver abaixo |
| `make seed` | `go run ./cmd/seed` | Só num banco sem nenhuma organização. O que ele cria está em [Instalação](../getting-started/installation.md#dados-de-demonstração) |
| `make demo` | Recria o banco `working_time_tracker_demo`, roda o seed nele e sobe o servidor na 8090 | Ver abaixo |
| `make fakes` | `go run ./cmd/fakegithub -addr :8091` e `go run ./cmd/faketrello -addr :8092`, juntos | `github_port=` e `trello_port=` trocam as portas. Um Ctrl+C para os dois. Para um só: `make fake-github` ou `make fake-trello`. Como apontar o sistema para eles está em [GitHub](../integrations/github.md#conferir-sem-o-github-de-verdade) e [Trello](../integrations/trello.md#conferir-sem-o-trello-de-verdade) |

### `make dev`

Os templates, os estáticos e os textos vão embutidos no binário, então o `make run` só mostra uma mudança em `web/`
depois de reiniciar. O `make dev` reinicia sozinho: o [air](https://github.com/air-verse/air) olha os arquivos `.go`,
`.html`, `.css`, `.js`, `.yaml` (os textos) e `.sql` (as migrações), recompila o binário em `.air/` e sobe de novo.
Recarregue a página no navegador. Os testes, a documentação e o `docker/` não disparam nada.

Instale o air uma vez:

```bash
go install github.com/air-verse/air@latest
```

### `make demo`

Sobe a demonstração ao lado do seu servidor, sem tocar no banco de dev: o banco é o `working_time_tracker_demo`, no
mesmo Postgres. Ele é **apagado e recriado a cada `make demo`**, com as datas contadas a partir do dia, e o seed
imprime os logins ao terminar (a senha de todos é `demo12345`).

- `port=9000` troca a porta (o padrão é 8090). O `PUBLIC_URL` passa a ser o da demonstração, para os links dos
  convites apontarem para ela.
- O resto da configuração vem do `.env`. O GitHub e o Trello de verdade não funcionam ali: os apps deles conhecem só o
  endereço do seu `PUBLIC_URL`.

## Migrações

| Comando | O que roda | Detalhes |
|---|---|---|
| `make migrate-new name=<nome>` | `go generate ./ent` e `go run ./cmd/migrate new <nome>` | Gera o código do ent e a migração com a diferença para o `ent/schema`. Diz `nada a gerar` quando não há diferença |
| `make migrate-status` | `go run ./cmd/migrate status` | As migrações e quando cada uma rodou no banco do `DATABASE_URL` |
| `make migrate-up` | `go run ./cmd/migrate up` | Aplica as pendentes sem subir o servidor |
| `make migrate-checksum` | `go run ./cmd/migrate checksum` | Depois de editar uma migração à mão |

Revise sempre o SQL gerado. As regras (rename, coluna `NOT NULL` nova, a ordem dos timestamps) estão em
[Fluxo de trabalho](workflow.md#migrações).

## Testes

| Comando | O que roda | Detalhes |
|---|---|---|
| `make test` | `go test -p 1 ./...`, com o `.env` carregado | Grava a saída em `/tmp/wtt-test.log` e mostra só o resumo |
| `make test-pkg pkg=<pacote>` | `go test -p 1 <pacote>`, com o `.env` carregado | A saída vai inteira para o terminal. `run=TestNome` roda só os testes cujo nome bate (é o `-run` do Go) |
| `make generate` | `go generate ./ent`, `go run ./cmd/migrate checksum` e `UPDATE_ERROR_DOCS=1 go test ./internal/server -run ErrorCodesDoc -count=1` | Os três [arquivos gerados](workflow.md#arquivos-gerados), que vão no commit da mudança que os alterou |
| `make check` | `gofmt -l .`, `go vet ./...`, o `make lint` e o `make test` | Para no primeiro que falhar |
| `make lint` | `golangci-lint run ./...` | A configuração padrão do golangci-lint v2: o projeto não tem `.golangci.yml` |

O resumo do `make test` tem uma linha por pacote (`ok` ou `FAIL`), o nome de cada teste que falhou e a mensagem da
falha (`arquivo_test.go:123: ...`). O resto, como o log das requisições, fica no arquivo.

O Go guarda o resultado de cada pacote, e numa segunda rodada só os pacotes que mudaram rodam de novo: o resumo mostra
`(cached)` nos outros. Para rodar tudo do zero, `go clean -testcache` antes. O `-p 1` e o banco dos testes estão
explicados em [Fluxo de trabalho](workflow.md#testes).

O golangci-lint mostra no máximo 3 avisos iguais e 50 de cada linter, e o total que ele imprime no fim conta só os
que mostrou. Para ver todos de uma vez, como depois de um merge grande:

```bash
golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...
```

## Build e deploy

| Comando | O que roda | Detalhes |
|---|---|---|
| `make build` | `go build -o wtt ./cmd` | O binário só precisa do `.env` (ou das variáveis no ambiente) e do banco |
| `make image` | `docker build -t wtt .` | A imagem de produção, descrita em [Deploy](../getting-started/deploy.md#a-imagem) |
| `make image-run` | `make image` e `docker run --rm -p 8080:8080 --env-file .env -e API_PORT= -e DATABASE_URL=...` | Roda a imagem contra o banco de dev do `docker compose` (pelo `host.docker.internal`). `port=` troca a porta da sua máquina |
| `make backup url=<DATABASE_URL>` | `pg_dump <url> -Fc` | Grava em `backups/wtt-AAAA-MM-DD-HHMM.dump` |
| `make clean` | `rm -rf wtt .air` | Apaga o binário e a pasta do `make dev` |

O `make backup` é para o banco de produção, que no Supabase Free não tem backup. A pasta `backups/` fica fora do git e
da imagem. O `pg_dump` precisa ser da mesma versão do Postgres do servidor ou mais novo (`brew install libpq` no macOS).
Para restaurar um dump:

```bash
pg_restore --clean --if-exists --no-owner -d "$DATABASE_URL" backups/wtt-2026-10-18-1530.dump
```

Um `pg_restore` 17 ou mais novo num Postgres 16 (o do `docker compose`) reclama de `transaction_timeout` e termina com
erro, mas o resto do dump entra: o parâmetro só existe a partir do 17.

## Acrescentar um comando

- Escreva `## descrição` na mesma linha do nome, e o comando aparece no `make help`. Um comando sem o `##` funciona,
  mas fica fora da lista. Os grupos são as linhas `##@ Nome`.
- Acrescente o nome ao `.PHONY`.
- Os parâmetros são em minúsculas, com o padrão no topo do arquivo (`db ?= working_time_tracker`, ou
  `demo: port ?= 8090` para valer só num comando). Quem chama troca com `nome=valor`.
- O macOS traz o GNU Make 3.81: nada de `.ONESHELL`, `!=` ou outras coisas do Make 4.
- Documente o comando aqui.
