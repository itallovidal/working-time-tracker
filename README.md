# Working Time Tracker

Ponto por tarefa para equipes que trabalham por projeto. Foi pensado principalmente para empresas de desenvolvimento, como software houses e consultorias, que atendem vários clientes e alocam as pessoas em um ou mais projetos, mas nada nele depende disso. A pessoa faz **clock-in** numa tarefa, faz **clock-out** quando para, e o sistema soma o tempo por tarefa e por pessoa. Tudo fica organizado em organização, projetos e times, e as tarefas podem apontar para issues do GitHub ou do GitLab.

É um único binário em Go que serve a API JSON e a interface web.

## O que dá para fazer

- **Contas e organizações.** O signup cria uma organização com você como admin. Outras pessoas entram por **link de convite** (uso único, válido por 7 dias, opcionalmente preso a um email).
- **Perfil da organização.** Resumo, descrição, segmento, contato, dados jurídicos (razão social, CNPJ, endereço) e como ela trabalha (regime remoto, híbrido ou presencial, fuso e moeda). O admin edita; todos os membros leem na aba **Sobre**, onde campo sem valor aparece como "Não informado".
- **Papéis.** Admins gerenciam a organização, as pessoas, os clientes, os projetos, os times, os valores e as integrações. Membros gerenciam tarefas e batem o próprio ponto.
- **Clientes e valores por hora.** Cada projeto pode ter um cliente e o **valor cobrado** dele por hora. Cada pessoa tem um **valor pago** por hora em cada projeto, então a mesma pessoa pode receber 20 num projeto e 25 em outro. O admin vê e altera tudo, com a margem por hora; o membro vê só o que ele mesmo recebe.
- **Projetos** com duração da sprint, jornada semanal, horário da daily e dia da weekly. Esses valores são de cada projeto, não da organização, porque projetos diferentes podem trabalhar de formas diferentes.
- **Colaboradores e times** de cada projeto. A aba Colaboradores mostra quem trabalha no projeto, em que times está e, para o admin, quanto recebe por hora e a margem. O admin adiciona pessoas, muda valores, monta os times e tira alguém do projeto de uma vez. Só quem está em algum time do projeto pode ser responsável por tarefas.
- **Tarefas** com responsável, prazo (7 dias por padrão, com destaque quando está atrasada ou perto de vencer) e vínculo opcional com uma issue. A lista tem busca por nome, filtro por responsável, filtro por prazo (atrasadas, até hoje, até o fim desta semana ou da próxima, ou até uma data) e 10 tarefas por página. A caixa "Só as minhas tarefas", abaixo dos filtros, mostra as suas e desliga a busca e o filtro de responsável: só o de prazo continua valendo. A tarefa nova é criada num modal.
- **Ponto.** Clock-in e clock-out com cronômetro ao vivo no topo de todas as páginas, sessões filtradas por tarefa e pessoa, totais do filtro e o seu tempo de hoje e da semana. O banco garante uma única sessão aberta por pessoa.
- **Horas em dinheiro.** Cada sessão guarda os valores por hora de quando o ponto abriu, então **mudar um valor só vale dali em diante**. Quem não tem valor definido no projeto **não bate ponto**. Na tela de ponto, o membro vê quanto ganhou; o admin vê custo, receita e margem.
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

Abra **http://localhost:8080**. O seed cria a **Jatobá Software**, uma software house com três clientes e seis projetos (um deles interno), em que cada pessoa tem um valor por hora diferente conforme o projeto. Entre como:

| Papel | Email | Senha |
|---|---|---|
| Admin | `ana@example.com` | `demo12345` |
| Membro | `bruno@example.com`, `carla@example.com`, `diego@example.com` ou `elisa@example.com` | `demo12345` |

O Bruno trabalha em quatro projetos e recebe mais na API de Cobranças; a Elisa trabalha em um só. O Diego está no time do Portal do Paciente ainda sem valor definido, para mostrar o aviso da aba Colaboradores e o ponto bloqueado. No Painel do Lojista, a Elisa já tem valor e ainda não entrou em nenhum time. O App de Pedidos tem mais de uma página de tarefas.

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
| `TEST_DATABASE_URL` | só nos testes | Banco usado por `go test`. O nome precisa terminar em `_test` |

O servidor e o seed aplicam as migrações pendentes ao iniciar. São arquivos SQL versionados, e só eles mudam o schema: veja [Migrações do banco](#migrações-do-banco).

## Interface web

A interface segue a Decision 8 de `_docs/design.md`. O servidor renderiza a casca de cada página em HTML (Go `html/template`), já sabendo quem está logado e qual é a organização e o projeto. Os componentes [Alpine.js](https://alpinejs.dev) buscam e alteram os dados pela API JSON em `/api`. **Não há etapa de build**: o CSS e o JavaScript são servidos como estão. A única coisa que vem de fora são os ícones ([Font Awesome Free](https://fontawesome.com)), carregados do cdnjs com SRI; sem acesso à CDN a interface continua funcionando, só sem os ícones.

### Rotas do navegador

| Rota | Tela |
|---|---|
| `/login`, `/signup` | Entrar e criar organização |
| `/invite/:token` | Aceitar um convite e criar a conta |
| `/` | Leva para a organização de quem está logado |
| `/orgs/:orgId` | Projetos da organização em cartões, para quem entra para trabalhar num deles. O admin cria projeto num modal, já com o cliente e, quando há cliente, o valor cobrado por hora |
| `/orgs/:orgId/about` | Organização, aba Sobre: o perfil da organização, para todos os membros |
| `/orgs/:orgId/settings` | Organização, tela de edição aberta pelo botão Editar da aba Sobre: perfil, regime, fuso, moeda e exclusão da organização (só admins) |
| `/orgs/:orgId/people` | Organização, aba Colaboradores: integrantes e papéis, convites pendentes e o botão Adicionar colaborador, que gera o link de convite num modal (só admins) |
| `/orgs/:orgId/customers` | Organização, aba Clientes: quem contrata os projetos, com cadastro e edição num modal (só admins) |
| `/orgs/:orgId/projects` | Organização, aba Projetos: tabela de gestão com o cliente, os colaboradores e as tarefas de cada projeto; a linha abre o projeto (só admins) |
| `/profile` | Seu nome, seu email, sua senha e quanto você recebe por hora em cada projeto |
| `/projects/:projectId` | Leva para a aba Tarefas |
| `/projects/:projectId/tasks` | Tarefas, com busca, filtros, paginação e início de ponto em um clique |
| `/tasks/:taskId` | Edição da tarefa, vínculo com issue e tempo registrado |
| `/projects/:projectId/time-tracking` | Cronômetro, sessões, filtros e totais em tempo e em dinheiro |
| `/projects/:projectId/teams` | Colaboradores: as pessoas do projeto, com busca, e os times. Para admins, também o valor por hora de cada pessoa, a margem e as ações de adicionar, tirar e montar times |
| `/projects/:projectId/integrations` | Integrações com GitHub e GitLab |
| `/projects/:projectId/settings` | Configurações e exclusão do projeto; para admins, também o cliente e o valor cobrado |

Sem sessão, qualquer página leva ao login, e a pessoa volta para a página pedida depois de entrar. Uma página de outra organização mostra "Página não encontrada".

### Navegação

- A **barra superior** mostra a organização, o menu (Projetos e Organização), o **indicador do ponto aberto** com cronômetro e botão Parar, quem está logado (o nome leva ao **perfil**) e o botão Sair.
- As páginas de projeto têm **abas**: Tarefas, Ponto, Colaboradores, Integrações e Configurações. A aba Colaboradores juntou as antigas Times e Valores; o endereço dela continua `/teams`. O valor cobrado do cliente fica em Configurações.
- A página **Organização** abre na aba Sobre, que todos os membros leem. Nela o admin tem o botão **Editar**, que leva à tela de edição; salvar volta para a Sobre. As abas Colaboradores, Clientes e Projetos são só de admins.
- Ações de admin não aparecem para membros. A API continua sendo quem garante as permissões.
- A lista de **tarefas** guarda a busca, os filtros e a página na URL (`?q=`, `?assignee=`, `?due=`, `?page=`, e `?mine=1` para "Só as minhas tarefas"). Recarregar mantém o que estava na tela, o link pode ser compartilhado, e o botão Voltar de uma tarefa leva de volta ao mesmo ponto da lista.

### Onde fica cada coisa

```
web/
  templates/
    layouts/base.gohtml        # HTML base, carrega os ícones (cdnjs), app.css, app.js, o script da página e o Alpine
    partials/                  # barra superior, cabeçalhos da organização e do projeto, indicador do ponto, toasts, modal e ícone
    pages/*.gohtml             # uma casca por tela; cada uma define o bloco "content"
  static/
    app.css                    # estilos com tema claro e escuro, sem framework
    app.js                     # api() sobre fetch, estado de formulário, toasts, modal, formatadores, cronômetro
    pages/*.js                 # componentes Alpine de cada grupo de telas (auth, org, project)
    alpine.min.js              # Alpine.js 3.14.8 vendorizado (dist/cdn.min.js do pacote npm)
internal/page/                 # handlers das páginas
internal/template/renderer.go  # um conjunto de templates por página, carregado na inicialização
```

Para atualizar o Alpine, troque `web/static/alpine.min.js` e o hash em `web/embed_test.go` juntos. O teste existe porque a cópia vendorizada já esteve corrompida sem ninguém perceber.

**Ícones.** `{{template "icon" "pen"}}` emite o ícone do Font Awesome com esse nome (sem o prefixo `fa-`). O ícone é decorativo: o botão mantém o texto. Um botão só de ícone, como os de renomear, excluir e remover da aba Colaboradores, leva a classe `btn-icon` e o nome da ação em `aria-label` e em `title`. Num botão com texto dinâmico, ponha o texto num `<span x-text>`, porque um `x-text` no botão apagaria o ícone. Para trocar a versão, mude a URL e o `integrity` em `base.gohtml` juntos; o hash vem de `https://api.cdnjs.com/libraries/font-awesome/<versão>?fields=sri`.

**Modal.** Há um só, no layout base (`partials/modal.gohtml`), controlado por `Alpine.store('modal')`. A página entrega o conteúdo e continua dona dele:

```html
<button type="button" class="btn" @click="$store.modal.open('customer', 'Novo cliente')">Novo cliente</button>

<template x-teleport="#modal-root">
  <form x-show="$store.modal.name === 'customer'" @submit.prevent="save()">
    <input x-model="draft.name" data-autofocus>
    ...
  </form>
</template>
```

O conteúdo aparece dentro do modal, mas segue no escopo do componente da página (`x-model`, `save()`, `errors`). O `<template>` precisa de um único elemento raiz. `open(nome, título, guarda)` abre e foca o campo com `data-autofocus`; `close()` fecha; `dismiss()` é o fechamento pedido pela pessoa (Esc, clique no fundo, X, Cancelar) e respeita a guarda, uma função que devolve `false` enquanto o modal não pode fechar, por exemplo durante um salvamento.

Hoje usam o modal o cadastro de cliente, o Adicionar colaborador, o Novo projeto, a Nova tarefa e, na aba Colaboradores do projeto, o Adicionar pessoa e o Novo time. Para focar um campo que acabou de aparecer dentro do modal (como o link do convite depois de gerado), dê ao bloco um `x-transition`: sem transição, o `x-show` só mostra o elemento no ciclo seguinte e o `$nextTick` chega antes de ele aceitar foco.

## API

A referência completa, com exemplos, está em [`_test/routes.md`](_test/routes.md). A coleção do Insomnia está em `_test/insomnia-collection.json`: importe, rode **Signup** ou **Login** e o Insomnia guarda o cookie de sessão para as outras chamadas.

Resumo dos grupos de rotas:

| Grupo | Rotas |
|---|---|
| Autenticação | `/api/auth/signup`, `login`, `logout`, `me`, `password`, `invites/:token` |
| Organização | `/api/orgs/:orgId` (+ `persons`, `projects`, `customers`, `invites`) |
| Clientes | `/api/customers/:customerId` |
| Pessoas | `/api/persons/:personId` (+ `role`, `allocations`) |
| Projetos | `/api/projects/:projectId` (+ `teams`, `tasks`, `members`, `collaborators`, `integrations`, `work-sessions`) |
| Valores | `/api/projects/:projectId/billing`, `/api/projects/:projectId/allocations` (+ `/:personId`) |
| Times | `/api/teams/:teamId` (+ `members`) |
| Tarefas | `/api/tasks/:taskId` (+ `link-external-item`, `external-details`) |
| Ponto | `/api/projects/:projectId/work-sessions/*`, `/api/work-sessions/active` |
| Integrações | `/api/integrations/:integrationId` |

## Segurança

- **Senhas** com bcrypt. Contas sem senha (criadas antes do login existir) não conseguem entrar.
- **Sessões e convites** usam tokens aleatórios de 32 bytes, e o banco guarda só o sha256 deles. Trocar a senha encerra as outras sessões.
- **Cookie** `wtt_session` HttpOnly e SameSite=Lax, com `Secure` via `COOKIE_SECURE`. Como a API só aceita corpo JSON em `POST`, `PUT` e `PATCH`, um formulário de outro site não consegue agir em nome de quem está logado.
- **Isolamento entre organizações.** Cada rota com ID confere se o recurso é da organização de quem chama e responde 404 caso não seja.
- **Valores.** O valor cobrado do cliente só existe em rotas de admin: ele não entra no JSON do projeto. Na lista de valores de um projeto, um membro recebe só a própria linha, e na de colaboradores, só o próprio valor. Nas sessões de ponto, a API apaga o valor pago das sessões de outras pessoas e todo valor cobrado antes de responder a quem não é admin.
- **Limite de tentativas** por IP em signup, login e convites.
- **Credenciais de integração** criptografadas com AES-GCM (`INTEGRATION_ENCRYPTION_KEY`) e nunca devolvidas pela API.

## Testes

Os testes usam um PostgreSQL de verdade. O `docker compose` já cria o banco `working_time_tracker_test`.

```bash
TEST_DATABASE_URL=postgres://wtt:wtt@localhost:5432/working_time_tracker_test?sslmode=disable \
  go test -p 1 ./...
```

Cada pacote de teste apaga o schema desse banco e aplica as migrações de novo antes de rodar, então ele não precisa de preparo e os testes sempre veem o que os arquivos de migração produzem hoje. Por segurança, isso só acontece num banco cujo nome termina em `_test`.

O `-p 1` é necessário porque todos os pacotes recriam e usam o mesmo banco. As chamadas ao GitHub nos testes vão para um servidor fake (`httptest`), então a suíte não depende de rede.

Cobertura:
- **Domínios:** services e handlers.
- **Migrações:** o banco que elas produzem bate com o `ent/schema`, regras de FK, índice parcial, recusa de banco sem histórico e checksum dos arquivos.
- **Router real:** tabela de rotas, autenticação, permissões e isolamento entre organizações.
- **Páginas:** toda página renderiza para admin e membro, redireciona sem sessão e dá 404 entre organizações; cada uma tem o modal uma única vez e carrega os ícones com SRI.
- **Alpine vendorizado:** o hash confere com o pacote oficial.

## Estrutura do projeto

```
cmd/
  main.go                 # servidor: config, banco, migrações e server.New
  seed/main.go            # dados de demonstração
  migrate/                # gera, confere e aplica as migrações do banco
ent/
  schema/                 # schema do banco (Ent); o resto de ent/ é gerado: go generate ./ent
internal/
  adapter/                # clientes do GitHub e do GitLab, e a criptografia das credenciais
  config/                 # variáveis de ambiente
  database/               # conexão, arquivos de migração (migrations/) e quem os aplica
  domain/
    auth/                 # signup, login, sessões, convites, middlewares e acesso por organização
    organization/  customer/  person/  project/  team/  allocation/  collaborator/  task/  work_session/  integration/
                          # cada domínio com model, store (Ent), service e handler
  page/                   # páginas HTML
  routes/                 # rotas da API (routes.go) e das páginas (pages.go)
  server/                 # monta o servidor completo; usado pelo main e pelos testes
  template/               # renderer dos templates
  validate/               # validações de formato usadas por mais de um domínio (CNPJ, links)
testutil/                 # conexão, schema e limpeza do banco de teste
web/                      # templates e arquivos estáticos (embutidos no binário)
docker-compose.yml        # PostgreSQL de dev e de testes
_docs/                    # proposta, design e plano das sprints
_test/                    # referência da API e coleção do Insomnia
```

## Modelo de dados

```
Organization                          nome, perfil (resumo, contato, dados jurídicos) e como trabalha (regime, fuso, moeda)
Organization (1) ── (N) Customer      cliente: nome, CNPJ e contato
Customer  (0..1) ── (N) Project       projeto interno fica sem cliente; o projeto guarda o valor cobrado por hora
Organization (1) ── (N) Project       sprint, jornada semanal, daily e weekly são do projeto
Organization (1) ── (N) Person        email único no sistema, senha (bcrypt), papel admin|member
Organization (1) ── (N) Invite        token (hash), email opcional, papel, expira em 7 dias, uso único
Person       (1) ── (N) Session       token (hash), expira em 7 dias
Project      (1) ── (N) Team ── (N) Person   via TeamMembership
Project      (1) ── (N) Allocation ── (1) Person   valor pago por hora, um por pessoa em cada projeto
Project      (1) ── (N) Task ── (N) WorkSession   a sessão guarda o valor pago e o cobrado do clock-in
Project      (1) ── (N) Integration   config criptografada
Task      (0..1) ── (0..1) Integration  via external_integration_id
```

Uma pessoa se liga a um projeto de duas formas independentes: pelo **valor por hora** (`Allocation`), que libera o ponto, e por um **time** (`TeamMembership`), que permite ser responsável por tarefas. Colaborador do projeto é quem tem pelo menos uma das duas; não há uma tabela só para isso. Tirar alguém do projeto apaga as duas e mantém as tarefas e as sessões da pessoa.

Excluir um projeto apaga os times, os valores, as tarefas, as sessões e as integrações dele. Excluir um cliente só é permitido quando nenhum projeto aponta para ele. Excluir uma organização só é permitido sem projetos, e apaga as pessoas, os clientes e os convites. O índice único parcial `one_active_session` em `work_sessions (person_id) WHERE end_at IS NULL` garante uma sessão aberta por pessoa.

## Migrações do banco

O schema do banco só muda por arquivos SQL versionados em `internal/database/migrations/`. O servidor e o seed aplicam os pendentes ao iniciar, em ordem, cada arquivo numa transação, e registram o que já rodou na tabela `goose_db_version`. Nada é calculado na hora: o que não está num arquivo commitado não roda.

O `ent/schema` continua sendo a fonte do modelo, e os arquivos são gerados a partir dele:

```bash
# 1. Edite ent/schema e regenere o código
go generate ./ent

# 2. Gere a migração com a diferença
go run ./cmd/migrate new add_invoice_number

# 3. Revise o arquivo criado. Se editar à mão, recalcule o checksum
go run ./cmd/migrate checksum

# 4. Rode os testes e commite o ent/schema, o código gerado e a migração juntos
```

| Comando | O que faz |
|---|---|
| `go run ./cmd/migrate new <nome>` | Gera um arquivo com a diferença entre o que as migrações produzem e o `ent/schema`. Usa um banco temporário no servidor do `DATABASE_URL` e o apaga ao terminar, sem tocar no seu banco |
| `go run ./cmd/migrate checksum` | Recalcula o `atlas.sum` depois de uma edição manual |
| `go run ./cmd/migrate status` | Lista as migrações e quais já foram aplicadas no banco do `DATABASE_URL` |
| `go run ./cmd/migrate up` | Aplica as pendentes sem subir o servidor |

**Revise sempre o arquivo gerado.** O gerador compara schemas e não sabe o que há nas tabelas. Ele avisa quando a mudança mexe em dados existentes, mas quem corrige é você:

- **Rename de campo** sai como `DROP COLUMN` + `ADD COLUMN`, o que apagaria os dados. Troque por `ALTER TABLE ... RENAME COLUMN ...`.
- **Coluna `NOT NULL` nova sem default** falha numa tabela com linhas. Crie nullable, preencha com um `UPDATE` e só então `SET NOT NULL`, no mesmo arquivo.
- **Dado que muda de lugar** (uma coluna que sai de uma tabela e vai para outra) precisa de um `UPDATE` escrito à mão antes do `DROP`.
- **Entidade removida** do `ent/schema` não gera `DROP TABLE`. Se a tabela deve sair, escreva o `DROP TABLE`.

Regras:

- As migrações andam **só para frente**. Para desfazer uma mudança, escreva uma migração nova.
- **Não edite uma migração que já está na `main`.** Os bancos que já a aplicaram não rodam o arquivo de novo.
- Ao trazer a `main` para uma branch que tem migração nova, a sua precisa ficar depois das que chegaram. Se o timestamp dela for menor, apague o arquivo, rode `checksum` e gere de novo.
- Um comando com `;` no meio (função, bloco `DO`) fica entre `-- +goose StatementBegin` e `-- +goose StatementEnd`.
- Índice ou constraint criado à mão numa migração também precisa estar declarado no `ent/schema`. O teste `TestMigrate_NoSchemaDrift` falha quando os dois divergem.

### Banco criado antes das migrações versionadas

Um banco criado pelo auto migrate antigo tem as tabelas, mas não tem histórico. O servidor se recusa a iniciar nele (`database has tables but no migration history`), em vez de adivinhar em que estado ele está. O mesmo vale para um banco com uma migração que o binário não conhece. Recrie o banco e rode o seed:

```bash
docker compose exec db psql -U wtt -d postgres \
  -c 'DROP DATABASE working_time_tracker' \
  -c 'CREATE DATABASE working_time_tracker OWNER wtt'
go run ./cmd/seed
```

Isso apaga os dados desse banco. Para mantê-los, guarde-os antes com `pg_dump --data-only`, recrie o banco, rode `go run ./cmd/migrate up` no lugar do seed e restaure o dump. Só funciona se o banco antigo estava com o schema da versão que introduziu as migrações.

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go + Echo v5 |
| Banco | PostgreSQL + [Ent](https://entgo.io), com migrações versionadas aplicadas pelo [goose](https://github.com/pressly/goose) |
| Frontend | HTML renderizado no servidor (`html/template`) + Alpine.js vendorizado, sem build; ícones do Font Awesome Free pelo cdnjs |
| Entrega | Binário único com `embed.FS` |
