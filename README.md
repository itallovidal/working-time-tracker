# Working Time Tracker

Ponto por tarefa para times de engenharia. A pessoa faz **clock-in** numa tarefa, faz **clock-out** quando para, e o sistema soma o tempo por tarefa e por pessoa. Tudo fica organizado em organização, projetos e times, e as tarefas podem apontar para issues do GitHub ou do GitLab.

É um único binário em Go que serve a API JSON e a interface web.

## O que dá para fazer

- **Contas e organizações.** O signup cria uma organização com você como admin. Outras pessoas entram por **link de convite** (uso único, válido por 7 dias, opcionalmente preso a um email).
- **Papéis.** Admins gerenciam a organização, as pessoas, os projetos, os times e as integrações. Membros gerenciam tarefas e batem o próprio ponto.
- **Projetos** com duração da sprint, horário da daily e dia da weekly.
- **Times** dentro de cada projeto. Só quem está em algum time do projeto pode ser responsável por tarefas.
- **Tarefas** com responsável, prazo (7 dias por padrão, com destaque quando está atrasada ou perto de vencer) e vínculo opcional com uma issue.
- **Ponto.** Clock-in e clock-out com cronômetro ao vivo no topo de todas as páginas, sessões filtradas por tarefa e pessoa, totais do filtro e o seu tempo de hoje e da semana. O banco garante uma única sessão aberta por pessoa.
- **Integrações** com GitHub e GitLab. A credencial é validada na plataforma, guardada criptografada e nunca volta nas respostas. Os detalhes da issue (título e estado) são buscados na hora, e se a plataforma não responde a tela mostra o motivo, sem quebrar.

## Como rodar

Pré-requisitos: **Go 1.26+** e **Docker** (ou um PostgreSQL 14+ seu).

```bash
# 1. Configuração
cp .env.example .env            # ajuste INTEGRATION_ENCRYPTION_KEY (openssl rand -hex 32)

# 2. Banco (sobe o Postgres com o banco de dev e o de testes)
docker compose up -d

# 3. Dados de demonstração (opcional, só num banco vazio)
go run ./cmd/seed

# 4. Servidor
go run ./cmd
```

Abra **http://localhost:8080**. Com o seed, entre como:

| Papel | Email | Senha |
|---|---|---|
| Admin | `ana@example.com` | `demo12345` |
| Membro | `bruno@example.com` | `demo12345` |

Sem o seed, clique em **Crie uma organização** na tela de login.

Se a porta 5432 já estiver ocupada, suba o banco com `POSTGRES_PORT=5433 docker compose up -d` e ajuste a porta no `.env`.

Para gerar o binário: `go build -o wtt ./cmd && ./wtt`. Templates e arquivos estáticos vão embutidos nele via `embed.FS`.

### Variáveis de ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `API_PORT` | sim | Porta HTTP, por exemplo `8080` |
| `DATABASE_URL` | sim | Conexão com o PostgreSQL |
| `INTEGRATION_ENCRYPTION_KEY` | sim | Chave da criptografia AES-GCM das credenciais de integração. Trocá-la torna as credenciais salvas ilegíveis |
| `COOKIE_SECURE` | não | `true` marca o cookie de sessão como `Secure`. Use em produção, atrás de HTTPS |
| `TEST_DATABASE_URL` | só nos testes | Banco usado por `go test` |

O servidor aplica as migrações do schema ao iniciar.

## Interface web

A interface segue a Decision 8 de `_docs/design.md`. O servidor renderiza a casca de cada página em HTML (Go `html/template`), já sabendo quem está logado e qual é a organização e o projeto. Os componentes [Alpine.js](https://alpinejs.dev) buscam e alteram os dados pela API JSON em `/api`. **Não há etapa de build**: o CSS e o JavaScript são servidos como estão.

### Rotas do navegador

| Rota | Tela |
|---|---|
| `/login`, `/signup` | Entrar e criar organização |
| `/invite/:token` | Aceitar um convite e criar a conta |
| `/` | Leva para a organização de quem está logado |
| `/orgs/:orgId` | Projetos da organização (admin cria) |
| `/orgs/:orgId/people` | Pessoas e papéis. O admin gera, copia e revoga convites |
| `/orgs/:orgId/settings` | Nome da organização, seu perfil, sua senha e exclusão da organização |
| `/projects/:projectId` | Leva para a aba Tarefas |
| `/projects/:projectId/tasks` | Tarefas, com início de ponto em um clique |
| `/tasks/:taskId` | Edição da tarefa, vínculo com issue e tempo registrado |
| `/projects/:projectId/time-tracking` | Cronômetro, sessões, filtros e totais |
| `/projects/:projectId/teams` | Times e membros |
| `/projects/:projectId/integrations` | Integrações com GitHub e GitLab |
| `/projects/:projectId/settings` | Configurações e exclusão do projeto |

Sem sessão, qualquer página leva ao login, e a pessoa volta para a página pedida depois de entrar. Uma página de outra organização mostra "Página não encontrada".

### Navegação

- A **barra superior** mostra a organização, o menu (Projetos, Pessoas, Configurações), o **indicador do ponto aberto** com cronômetro e botão Parar, quem está logado e o botão Sair.
- As páginas de projeto têm **abas**: Tarefas, Ponto, Times, Integrações e Configurações.
- Ações de admin não aparecem para membros. A API continua sendo quem garante as permissões.

### Onde fica cada coisa

```
web/
  templates/
    layouts/base.gohtml        # HTML base, carrega app.css, app.js, o script da página e o Alpine
    partials/                  # barra superior, cabeçalho do projeto, indicador do ponto, toasts
    pages/*.gohtml             # uma casca por tela; cada uma define o bloco "content"
  static/
    app.css                    # estilos com tema claro e escuro, sem framework
    app.js                     # api() sobre fetch, estado de formulário, toasts, formatadores, cronômetro
    pages/*.js                 # componentes Alpine de cada grupo de telas (auth, org, project)
    alpine.min.js              # Alpine.js 3.14.8 vendorizado (dist/cdn.min.js do pacote npm)
internal/page/                 # handlers das páginas
internal/template/renderer.go  # um conjunto de templates por página, carregado na inicialização
```

Para atualizar o Alpine, troque `web/static/alpine.min.js` e o hash em `web/embed_test.go` juntos. O teste existe porque a cópia vendorizada já esteve corrompida sem ninguém perceber.

## API

A referência completa, com exemplos, está em [`_test/routes.md`](_test/routes.md). A coleção do Insomnia está em `_test/insomnia-collection.json`: importe, rode **Signup** ou **Login** e o Insomnia guarda o cookie de sessão para as outras chamadas.

Resumo dos grupos de rotas:

| Grupo | Rotas |
|---|---|
| Autenticação | `/api/auth/signup`, `login`, `logout`, `me`, `password`, `invites/:token` |
| Organização | `/api/orgs/:orgId` (+ `persons`, `projects`, `invites`) |
| Pessoas | `/api/persons/:personId` (+ `role`) |
| Projetos | `/api/projects/:projectId` (+ `teams`, `tasks`, `members`, `integrations`, `work-sessions`) |
| Times | `/api/teams/:teamId` (+ `members`) |
| Tarefas | `/api/tasks/:taskId` (+ `link-external-item`, `external-details`) |
| Ponto | `/api/projects/:projectId/work-sessions/*`, `/api/work-sessions/active` |
| Integrações | `/api/integrations/:integrationId` |

## Segurança

- **Senhas** com bcrypt. Contas sem senha (criadas antes do login existir) não conseguem entrar.
- **Sessões e convites** usam tokens aleatórios de 32 bytes, e o banco guarda só o sha256 deles. Trocar a senha encerra as outras sessões.
- **Cookie** `wtt_session` HttpOnly e SameSite=Lax, com `Secure` via `COOKIE_SECURE`. Como a API só aceita corpo JSON em `POST`, `PUT` e `PATCH`, um formulário de outro site não consegue agir em nome de quem está logado.
- **Isolamento entre organizações.** Cada rota com ID confere se o recurso é da organização de quem chama e responde 404 caso não seja.
- **Limite de tentativas** por IP em signup, login e convites.
- **Credenciais de integração** criptografadas com AES-GCM (`INTEGRATION_ENCRYPTION_KEY`) e nunca devolvidas pela API.

## Testes

Os testes usam um PostgreSQL de verdade. O `docker compose` já cria o banco `working_time_tracker_test`.

```bash
TEST_DATABASE_URL=postgres://wtt:wtt@localhost:5432/working_time_tracker_test?sslmode=disable \
  go test -p 1 ./...
```

O `-p 1` é necessário porque todos os pacotes limpam e usam o mesmo banco. As chamadas ao GitHub nos testes vão para um servidor fake (`httptest`), então a suíte não depende de rede.

Cobertura:
- **Domínios:** services e handlers.
- **Migração:** regras de FK, índice parcial e idempotência.
- **Router real:** tabela de rotas, autenticação, permissões e isolamento entre organizações.
- **Páginas:** toda página renderiza para admin e membro, redireciona sem sessão e dá 404 entre organizações.
- **Alpine vendorizado:** o hash confere com o pacote oficial.

## Estrutura do projeto

```
cmd/
  main.go                 # servidor: config, banco, migração e server.New
  seed/main.go            # dados de demonstração
ent/
  schema/                 # schema do banco (Ent); o resto de ent/ é gerado: go generate ./ent
internal/
  adapter/                # clientes do GitHub e do GitLab, e a criptografia das credenciais
  config/                 # variáveis de ambiente
  database/               # conexão e migração
  domain/
    auth/                 # signup, login, sessões, convites, middlewares e acesso por organização
    organization/  person/  project/  team/  task/  work_session/  integration/
                          # cada domínio com model, store (Ent), service e handler
  page/                   # páginas HTML
  routes/                 # rotas da API (routes.go) e das páginas (pages.go)
  server/                 # monta o servidor completo; usado pelo main e pelos testes
  template/               # renderer dos templates
testutil/                 # conexão e limpeza do banco de teste
web/                      # templates e arquivos estáticos (embutidos no binário)
docker-compose.yml        # PostgreSQL de dev e de testes
_docs/                    # proposta, design e plano das sprints
_test/                    # referência da API e coleção do Insomnia
```

## Modelo de dados

```
Organization (1) ── (N) Project
Organization (1) ── (N) Person        email único no sistema, senha (bcrypt), papel admin|member
Organization (1) ── (N) Invite        token (hash), email opcional, papel, expira em 7 dias, uso único
Person       (1) ── (N) Session       token (hash), expira em 7 dias
Project      (1) ── (N) Team ── (N) Person   via TeamMembership
Project      (1) ── (N) Task ── (N) WorkSession
Project      (1) ── (N) Integration   config criptografada
Task      (0..1) ── (0..1) Integration  via external_integration_id
```

Excluir um projeto apaga os times, as tarefas, as sessões e as integrações dele. Excluir uma organização só é permitido sem projetos, e apaga as pessoas e os convites. O índice único parcial `one_active_session` em `work_sessions (person_id) WHERE end_at IS NULL` garante uma sessão aberta por pessoa.

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go + Echo v5 |
| Banco | PostgreSQL + [Ent](https://entgo.io) |
| Frontend | HTML renderizado no servidor (`html/template`) + Alpine.js vendorizado, sem build |
| Entrega | Binário único com `embed.FS` |
