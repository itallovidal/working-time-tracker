# Os comandos do dia a dia, com nomes curtos. `make` sozinho lista todos; o que cada um roda por baixo está em
# _docs/development/commands.md. Os parâmetros vão depois do nome: make migrate-new name=add_payout_note.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# A porta do Postgres do docker compose: a do ambiente, ou a do .env (que o compose também lê), ou 5432.
POSTGRES_PORT ?= $(or $(shell sed -n 's/^POSTGRES_PORT=//p' .env 2>/dev/null),5432)

# O servidor, o seed e o migrate leem o .env sozinhos; o go test não, então os testes o carregam antes.
LOAD_ENV := set -a; [ ! -f .env ] || . ./.env; set +a;

# O psql de dentro do container do banco, com o usuário do docker-compose.yml.
PSQL := docker compose exec -T db psql -U wtt -v ON_ERROR_STOP=1

DEV_URL := postgres://wtt:wtt@localhost:$(POSTGRES_PORT)/working_time_tracker?sslmode=disable
DEMO_DB := working_time_tracker_demo
DEMO_URL := postgres://wtt:wtt@localhost:$(POSTGRES_PORT)/$(DEMO_DB)?sslmode=disable
TEST_LOG := /tmp/wtt-test.log

# Os parâmetros, com o padrão de cada um.
db ?= working_time_tracker
github_port ?= 8091
trello_port ?= 8092
demo: port ?= 8090
image-run: port ?= 8080

.PHONY: help setup db-up db-down db-psql db-reset run dev seed demo fakes fake-github fake-trello \
	migrate-new migrate-status migrate-up migrate-checksum test test-pkg generate check lint \
	build image image-run backup clean

help: ## Lista os comandos
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } \
		/^[a-z][a-z-]*:.*## / { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo

##@ Ambiente

setup: .env db-up ## Primeira vez: cria o .env com uma chave nova e sobe o banco

# Só roda quando o .env não existe: um .env que já está aí nunca é sobrescrito.
.env:
	cp .env.example .env
	sed -i.bak "s/^INTEGRATION_ENCRYPTION_KEY=.*/INTEGRATION_ENCRYPTION_KEY=$$(openssl rand -hex 32)/" .env && rm .env.bak
	@echo ".env criado, com uma INTEGRATION_ENCRYPTION_KEY nova"

db-up: ## Sobe o Postgres com o banco de dev e o de testes, e espera ficar pronto
	POSTGRES_PORT=$(POSTGRES_PORT) docker compose up -d --wait
	@# O docker/initdb só cria o banco de testes quando o volume nasce; num volume antigo, cria aqui.
	@$(PSQL) -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'working_time_tracker_test'" | grep -q 1 \
		|| $(PSQL) -d postgres -c 'CREATE DATABASE working_time_tracker_test OWNER wtt'

db-down: ## Para o Postgres (os dados ficam no volume)
	docker compose down

db-psql: ## Abre o psql no banco de dev (db=working_time_tracker_test para o de testes)
	docker compose exec db psql -U wtt -d $(db)

db-reset: ## Apaga o banco de dev, recria vazio e roda o seed (pede confirmação)
	@read -r -p "Isso apaga todos os dados do banco working_time_tracker. Continuar? [s/N] " answer; \
		[ "$$answer" = s ] || [ "$$answer" = S ] || { echo "Nada foi apagado."; exit 1; }
	$(PSQL) -d postgres -c 'DROP DATABASE IF EXISTS working_time_tracker WITH (FORCE)' \
		-c 'CREATE DATABASE working_time_tracker OWNER wtt'
	DATABASE_URL='$(DEV_URL)' go run ./cmd/seed

##@ Rodar

run: ## Sobe o servidor na porta do API_PORT (aplica as migrações ao subir)
	go run ./cmd

dev: ## O mesmo, mas reinicia sozinho a cada mudança no código, nos templates ou nos textos (precisa do air)
	@command -v air >/dev/null || { echo "Instale o air: go install github.com/air-verse/air@latest"; exit 1; }
	air --tmp_dir .air \
		--build.cmd "go build -o .air/wtt ./cmd" \
		--build.entrypoint ".air/wtt" \
		--build.include_ext "go,html,css,js,yaml,sql" \
		--build.exclude_dir ".air,.git,.claude,_docs,_test,docker,backups" \
		--build.send_interrupt true --build.kill_delay 2s

seed: ## Cria os dados de demonstração (só num banco sem nenhuma organização)
	go run ./cmd/seed

demo: ## Sobe a demonstração em http://localhost:8090, num banco à parte recriado a cada vez (port=... muda a porta)
	$(PSQL) -d postgres -c 'DROP DATABASE IF EXISTS $(DEMO_DB) WITH (FORCE)' -c 'CREATE DATABASE $(DEMO_DB) OWNER wtt'
	DATABASE_URL='$(DEMO_URL)' go run ./cmd/seed
	DATABASE_URL='$(DEMO_URL)' API_PORT=$(port) PUBLIC_URL=http://localhost:$(port) go run ./cmd

fakes: ## Sobe o GitHub falso (:8091) e o Trello falso (:8092), com o painel em /_fake (github_port=, trello_port=)
	@$(MAKE) --no-print-directory -j2 fake-github fake-trello

fake-github:
	go run ./cmd/fakegithub -addr :$(github_port)

fake-trello:
	go run ./cmd/faketrello -addr :$(trello_port)

##@ Migrações

migrate-new: ## Regera o ent e cria a migração do que mudou no ent/schema (name=<nome>)
	@[ -n "$(name)" ] || { echo "uso: make migrate-new name=<nome>"; exit 1; }
	go generate ./ent
	go run ./cmd/migrate new $(name)

migrate-status: ## Lista as migrações e quais já rodaram no banco do DATABASE_URL
	go run ./cmd/migrate status

migrate-up: ## Aplica as migrações pendentes sem subir o servidor
	go run ./cmd/migrate up

migrate-checksum: ## Recalcula o atlas.sum depois de editar uma migração à mão
	go run ./cmd/migrate checksum

##@ Testes

test: ## Roda a suíte inteira e mostra só o resumo (o log completo fica em /tmp/wtt-test.log)
	@echo "Rodando a suíte (a primeira vez leva alguns minutos; depois, só os pacotes que mudaram rodam de novo)..."
	@# O resumo: o resultado de cada pacote, os testes que falharam e a mensagem de cada falha (arquivo.go:linha:).
	@$(LOAD_ENV) go test -p 1 ./... > $(TEST_LOG) 2>&1; status=$$?; \
		grep -E '^([[:space:]]*--- FAIL|FAIL|ok |panic:|[[:space:]]+[^[:space:]]+\.go:[0-9]+: )' $(TEST_LOG); \
		echo "Log completo: $(TEST_LOG)"; exit $$status

test-pkg: ## Roda um pacote com a saída inteira (pkg=./internal/domain/task, e run=TestNome se quiser)
	@[ -n "$(pkg)" ] || { echo "uso: make test-pkg pkg=./internal/domain/task [run=TestNome]"; exit 1; }
	@$(LOAD_ENV) go test -p 1 $(pkg) $(if $(run),-run '$(run)')

generate: ## Regera o código do ent, o atlas.sum e o _docs/api/errors.md
	go generate ./ent
	go run ./cmd/migrate checksum
	@$(LOAD_ENV) UPDATE_ERROR_DOCS=1 go test ./internal/server -run ErrorCodesDoc -count=1

check: ## Antes de um commit: o gofmt, o go vet, o golangci-lint e a suíte
	@unformatted=$$(gofmt -l .); [ -z "$$unformatted" ] || { echo "Arquivos sem gofmt:"; echo "$$unformatted"; exit 1; }
	go vet ./...
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory test

lint: ## Roda o golangci-lint, com a configuração padrão
	@command -v golangci-lint >/dev/null || { echo "Instale o golangci-lint v2: brew install golangci-lint (ou veja golangci-lint.run)"; exit 1; }
	golangci-lint run ./...

##@ Build e deploy

build: ## Gera o binário ./wtt, com os templates, os estáticos e as migrações embutidos
	go build -o wtt ./cmd

image: ## Constrói a imagem Docker wtt
	docker build -t wtt .

image-run: image ## Constrói e roda a imagem em http://localhost:8080 (port=...), contra o banco de dev do compose
	docker run --rm -p $(port):8080 --env-file .env -e API_PORT= \
		-e DATABASE_URL='postgres://wtt:wtt@host.docker.internal:$(POSTGRES_PORT)/working_time_tracker?sslmode=disable' wtt

backup: ## Guarda um dump do banco de url=... (o do Supabase) em backups/
	@[ -n "$(url)" ] || { echo 'uso: make backup url="postgresql://..."'; exit 1; }
	@mkdir -p backups
	@file=backups/wtt-$$(date +%F-%H%M).dump; pg_dump "$(url)" -Fc -f $$file && echo "Dump em $$file"

clean: ## Apaga o binário e a pasta do air
	rm -rf wtt .air
