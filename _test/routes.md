# Rotas da API — Working Time Tracker

**Base URL:** `http://localhost:8080`

Todas as rotas falam JSON. Erros sempre vêm como `{"error": {"code": "dominio.motivo", "params": {...}}}`: a API **não manda mensagem**, manda um código estável (e `params`, quando o texto precisa de valores), e quem chama mostra o texto no idioma da pessoa. A lista completa de códigos, com o status e os textos em português e inglês, está em [`_docs/error-codes.md`](../_docs/error-codes.md).

## Como a autenticação funciona

- O login e o signup devolvem um cookie `wtt_session` (HttpOnly, SameSite=Lax, válido por 7 dias). Clientes HTTP como o Insomnia guardam o cookie sozinhos: faça o login uma vez e as próximas chamadas já vão autenticadas.
- Tudo fora de `/api/auth/*` e `/healthcheck` exige a sessão. Sem ela, a resposta é **401**.
- Cada pessoa pertence a uma organização. Um recurso de outra organização responde **404**, como se não existisse.
- Rotas marcadas como **admin** respondem **403** para membros. As marcadas com uma permissão (`project.edit`, `billing.view`…) ou **dono** respondem **403** a quem não a tem: admins têm todas, e o dono tem também as que são só dele. A lista está em [`_docs/permissions.md`](../_docs/permissions.md).
- `POST`, `PUT` e `PATCH` precisam de `Content-Type: application/json`. Outro formato responde **415**, o que também protege contra CSRF.
- Signup, login e convites têm limite de tentativas por IP. Acima dele, a resposta é **429**.

## Códigos de status

| Status | Quando |
|---|---|
| 200 / 201 / 204 | Sucesso (204 não tem corpo) |
| 400 | Dado inválido, com o código dizendo o quê (`organization.invalid_cnpj`, `task.invalid_page`...) |
| 401 | Sem sessão, sessão expirada ou email e senha errados |
| 403 | A pessoa não é admin, ou tentou agir por outra pessoa |
| 404 | O recurso não existe ou é de outra organização |
| 409 | Email já em uso |
| 415 | Corpo que não é JSON |
| 429 | Muitas tentativas de login, signup ou convite |

---

## Healthcheck

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/healthcheck` | Verifica se o servidor está rodando |
| GET | `/healthcheck/hello` | Hello world de teste |

---

## Autenticação e conta

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/auth/signup` | público | Cria uma organização com você como admin e abre a sessão |
| POST | `/api/auth/login` | público | Abre uma sessão |
| POST | `/api/auth/logout` | logado | Encerra a sessão atual |
| GET | `/api/auth/me` | logado | Quem está logado, com a organização e o papel |
| POST | `/api/auth/password` | logado | Troca a senha e encerra as suas outras sessões |
| GET | `/api/auth/invites/:token` | público | Mostra para qual organização e papel é o convite |
| POST | `/api/auth/invites/:token/accept` | público | Cria a conta pelo convite e abre a sessão |

### Signup
```http
POST /api/auth/signup
Content-Type: application/json

{
  "organization_name": "Minha Empresa",
  "name": "Ana Souza",
  "email": "ana@empresa.com",
  "password": "pelo-menos-8"
}
```
Resposta `201` (e o cookie `wtt_session`):
```json
{
  "id": "…",
  "name": "Ana Souza",
  "email": "ana@empresa.com",
  "role": "admin",
  "is_owner": true,
  "organization_id": "…",
  "organization_name": "Minha Empresa"
}
```
O email vira minúsculas e é único no sistema todo. A senha precisa ter entre 8 e 72 caracteres.

### Login
```http
POST /api/auth/login
Content-Type: application/json

{ "email": "ana@empresa.com", "password": "pelo-menos-8" }
```
Resposta `200` com o mesmo formato do signup, ou `401` com `"email ou senha incorretos"`.

### Trocar a senha
```http
POST /api/auth/password
Content-Type: application/json

{ "current_password": "senha-atual", "new_password": "senha-nova-123" }
```

### Aceitar convite
```http
POST /api/auth/invites/:token/accept
Content-Type: application/json

{ "name": "Bruno Lima", "email": "bruno@empresa.com", "password": "pelo-menos-8" }
```
Se o convite foi criado com email, só esse email consegue aceitar. Um convite vale por 7 dias e funciona uma vez.

---

## Organização

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/orgs/:orgId` | logado | Detalhes da organização |
| PATCH | `/api/orgs/:orgId` | admin | Altera o nome e o perfil. Veja os campos abaixo |
| DELETE | `/api/orgs/:orgId` | dono | Exclui a organização com as pessoas e os convites. Falha se ainda houver projetos |
| GET | `/api/orgs/:orgId/persons` | logado | Pessoas da organização |
| POST | `/api/orgs/:orgId/projects` | `projects.create` | Cria um projeto |
| GET | `/api/orgs/:orgId/projects` | logado | Projetos da organização |
| POST | `/api/orgs/:orgId/customers` | `customers.manage` | Cria um cliente |
| GET | `/api/orgs/:orgId/customers` | `customers.manage` | Clientes da organização, em ordem alfabética |

A organização é criada pelo signup, e o `organization_id` vem no `/api/auth/me`, junto com `organization_currency`.

### Perfil da organização

O `GET` devolve todos os campos para qualquer membro. No `PATCH`, **campo que não vem no corpo fica como está**; texto vazio (`""`) ou número zero apaga o valor. `timezone` e `currency` são a exceção: nunca ficam sem valor, e vazios voltam para o padrão. `{"name": "…"}` sozinho continua renomeando.

| Campo | Regra |
|---|---|
| `name` | Obrigatório, até 120 caracteres |
| `summary` | Uma linha, até 160 caracteres |
| `description` | Texto puro, até 2000 caracteres |
| `industry` | Segmento de atuação, até 100 caracteres |
| `founded_year` | De 1900 ao ano atual |
| `size` | `1-10`, `11-50`, `51-200`, `201-500` ou `500+` |
| `website`, `linkedin_url`, `instagram_url` | Link `http` ou `https`. Sem esquema, vira `https://…` |
| `contact_email`, `phone` | Email válido; telefone com números, espaços, `+`, parênteses e hífen |
| `legal_name` | Razão social, até 200 caracteres |
| `cnpj` | Com ou sem máscara. Os dígitos verificadores são conferidos, inclusive no formato alfanumérico. A resposta traz sem máscara |
| `address_line1`, `address_line2`, `city`, `state`, `postal_code`, `country` | Texto livre |
| `work_mode` | Regime de trabalho: `remote`, `hybrid` ou `onsite` |
| `timezone` | Nome IANA, por exemplo `America/Sao_Paulo` (o padrão) |
| `currency` | `BRL` (o padrão), `USD` ou `EUR` |

A duração da sprint e a jornada semanal não são da organização: a sprint fica em cada projeto (`sprint_duration_days`) e a jornada, em cada pessoa (`weekly_hours`).

```http
PATCH /api/orgs/:orgId
Content-Type: application/json

{
  "summary": "Entregas no mesmo dia para o comércio de bairro.",
  "website": "acme-delivery.example",
  "cnpj": "11.222.333/0001-81",
  "work_mode": "hybrid"
}
```

### Convites

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/orgs/:orgId/invites` | `people.manage` | Gera um link de convite |
| GET | `/api/orgs/:orgId/invites` | `people.manage` | Convites ainda válidos |
| DELETE | `/api/invites/:inviteId` | `people.manage` | Revoga um convite |

```http
POST /api/orgs/:orgId/invites
Content-Type: application/json

{ "email": "bruno@empresa.com", "role": "member" }
```
`email` é opcional, e `role` é `admin` ou `member` (o padrão é `member`). A resposta traz `token` e `path` (`/invite/<token>`). **O token só aparece nesta resposta**: o banco guarda apenas o hash dele.

---

## Clientes

Quem contrata a organização. Todas as rotas pedem `customers.manage` (admins têm).

| Método | Rota | Descrição |
|---|---|---|
| POST | `/api/orgs/:orgId/customers` | Cria um cliente |
| GET | `/api/orgs/:orgId/customers` | Lista, com `project_count` em cada um |
| GET | `/api/customers/:customerId` | Detalhes |
| PATCH | `/api/customers/:customerId` | Altera. Campo omitido mantém o valor; `""` apaga |
| DELETE | `/api/customers/:customerId` | Exclui. Responde `400` enquanto algum projeto apontar para o cliente |

```http
POST /api/orgs/:orgId/customers
Content-Type: application/json

{
  "name": "Rede Bom Preço",
  "document": "12.ABC.345/01DE-35",
  "contact_name": "Carla Dias",
  "contact_email": "carla@bompreco.example",
  "contact_phone": "+55 (11) 3003-1000"
}
```
Só `name` é obrigatório. `document` é o CNPJ, com ou sem máscara: os dígitos verificadores são conferidos e a resposta traz sem máscara.

---

## Pessoas

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/persons/:personId` | logado | Detalhes da pessoa |
| PATCH | `/api/persons/:personId` | a própria pessoa ou admin | Altera nome e email |
| PATCH | `/api/persons/:personId/role` | dono | Muda o papel: `{"role": "admin"}` ou `{"role": "member"}` |
| PATCH | `/api/persons/:personId/permissions` | dono | Define as permissões da organização de quem não é admin: `{"permissions": ["projects.create", "customers.manage"]}`. Admin recusa (`400 person.admin_has_all_permissions`), e uma chave que não é da organização também (`400 person.invalid_permission`). Repetidas saem, e a ordem é a do catálogo |
| GET | `/api/permissions` | logado | O catálogo: `project` e `organization` (as chaves, na ordem das telas) e `presets` (os grupos do projeto, cada um com `id` e `permissions`) |
| PATCH | `/api/persons/:personId/weekly-hours` | `people.manage` | Define a jornada semanal: `{"weekly_hours": 40}` |
| GET | `/api/persons/:personId/allocations` | a própria pessoa ou admin | Quanto a pessoa recebe por hora em cada projeto |

A pessoa traz `permissions`, as da organização que o dono liberou (vazio nos admins, que têm todas), e `/api/auth/me` traz as mesmas. As permissões estão em [`_docs/permissions.md`](../_docs/permissions.md). A organização nunca fica sem admin: rebaixar o último admin responde `400`. A pessoa traz `is_owner`: o dono da organização é quem a criou (o signup), é um só por organização e é sempre admin, então rebaixá-lo responde `400 person.owner_is_admin`, mesmo com outros admins. Quem entra por convite nunca é o dono. Pessoas entram na organização pelo signup ou por convite.

A pessoa traz `weekly_hours`, a jornada semanal combinada com ela, em horas: vale para a organização toda, e não por projeto. Vai de 1 a 168 e vem `null` enquanto nenhum admin informou. Só um admin altera, e `0` ou `null` apagam; o `PATCH` de nome e email não mexe nela. Todos da organização leem.

---

## Projetos

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId` | logado | Detalhes do projeto |
| PATCH | `/api/projects/:projectId` | `project.edit` | Altera o projeto |
| DELETE | `/api/projects/:projectId` | `project.edit` | Exclui o projeto com times, tarefas, sessões e integrações |
| GET | `/api/projects/:projectId/overview` | `billing.view` | O projeto em números: pessoas, times, horas, custo, receita, tempo de projeto, tarefas e integrações. Veja [Visão geral](#visão-geral) |
| GET | `/api/projects/:projectId/members` | logado | Pessoas que estão no projeto (com valor por hora ou em algum time), sem repetir: são as que podem ser responsáveis por tarefas |

```http
POST /api/orgs/:orgId/projects
Content-Type: application/json

{
  "name": "App de Pedidos",
  "description": "Opcional",
  "sprint_duration_days": 14,
  "daily_time": "09:30",
  "weekly_sync_day": "friday",
  "weekly_sync_time": "14:00",
  "customer_meeting_day": "wednesday",
  "customer_meeting_time": "10:30"
}
```
- `sprint_duration_days` vai de 1 a 90, e o padrão é 14. As telas oferecem 7, 14 e 30 (um mês); um projeto com outra duração continua com ela.
- `daily_time` usa o formato `HH:MM`. Sem ele, o projeto não tem daily.
- `weekly_sync_day` vai de `monday` a `sunday`. Sem ele, o projeto não tem weekly.
- `weekly_sync_time` usa o formato `HH:MM` e só existe com o dia: sem `weekly_sync_day` é `400 project.weekly_time_without_day`. Weeklies antigas podem ter o dia sem o horário.
- `customer_meeting_day` e `customer_meeting_time` são a reunião semanal com o cliente, com as mesmas regras da weekly: o dia vai de `monday` a `sunday` (outro valor é `400 project.invalid_weekday`), o horário usa `HH:MM` (`400 project.invalid_customer_meeting_time`) e só existe com o dia (`400 project.customer_meeting_time_without_day`). Não dependem da weekly do time. Qualquer pessoa do projeto lê os dois campos; só admin altera. Tirar o cliente do projeto (`PUT /api/projects/:projectId/billing` com `customer_id` nulo) apaga a reunião. As telas só oferecem a reunião num projeto com cliente, mas a API não exige o cliente ao gravar o dia.

No `PATCH`, um campo omitido mantém o valor atual, e `""` apaga `daily_time`, `weekly_sync_day`, `weekly_sync_time`, `customer_meeting_day` ou `customer_meeting_time`. Apagar o dia da weekly ou da reunião apaga o horário junto. O projeto não tem jornada semanal: ela é da pessoa (`PATCH /api/persons/:personId/weekly-hours`).

O projeto traz `member_count`, os colaboradores dele (quem tem valor por hora nele, cada pessoa uma vez: a mesma conta de `/collaborators`), e `task_count`, as tarefas dele. Os dois vêm na lista e no detalhe.

O projeto traz `customer` (`{"id", "name"}` ou `null`) para qualquer membro. O valor cobrado nunca vem aqui: ele fica em `/billing`.

### Cliente e valor cobrado

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/billing` | `billing.view` | Cliente e valor que ele paga por hora |
| PUT | `/api/projects/:projectId/billing` | `billing.manage` | Substitui os dois. O que vier `null` é apagado |

```http
PUT /api/projects/:projectId/billing
Content-Type: application/json

{ "customer_id": "<id do cliente>", "bill_rate_cents": 10000 }
```
Os valores são sempre em **centavos**, na moeda da organização: `10000` é 100,00. O cliente precisa ser da mesma organização do projeto. Mandar `{"customer_id": null, "bill_rate_cents": null}` volta o projeto a ser interno.

### Valor pago a cada pessoa

O vínculo de uma pessoa com o projeto e quanto ela recebe por hora nele. Há um valor por pessoa em cada projeto, e é ele que põe a pessoa no projeto: sem valor ela não entra em time nem bate ponto.

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/allocations` | logado | Quem vê o valor dos outros (`rates.view`, `rates.manage` e os admins) recebe todos; um membro recebe só o dele, ou `[]`. Cada vínculo traz `preset` e `permissions` |
| PUT | `/api/projects/:projectId/allocations/:personId` | `collaborators.manage` ou `rates.manage` | Põe a pessoa no projeto com o valor dela, ou troca o valor de quem já está: `{"pay_rate_cents": 2000}` |

`pay_rate_cents` vai de `0` a `100000000` e é obrigatório para quem entra no projeto. Zero vale: é alguém que trabalha no projeto sem receber por hora. A pessoa precisa ser da mesma organização do projeto. Para o dono da organização o valor gravado é sempre `0`.

O corpo aceita também `preset`, o grupo de permissões da pessoa no projeto: `member` (o padrão de quem entra), `manager`, `finance` ou `admin` (outro valor é `400 allocation.invalid_preset`). Pode vir sozinho, para trocar o grupo de quem já está no projeto: `{"preset": "manager"}`. Cada mudança pede a sua permissão: pôr alguém ou trocar o grupo, `collaborators.manage`; trocar o valor, `rates.manage` (`403 auth.permission_required`). Quem dá um grupo precisa ter todas as permissões dele (`403 allocation.preset_above_yours`). Sem valor e sem grupo, `400 allocation.rate_required`.

Não há rota para apagar só o valor: a pessoa ficaria no projeto sem ele. Para tirá-la do projeto, use o `DELETE` de `/collaborators/:personId`, abaixo.

### Colaboradores

Quem está no projeto. Uma pessoa é colaboradora quando tem valor por hora nele; os times vêm depois, e só aceitam quem já é colaborador. Esta rota mostra o valor e os times juntos.

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/collaborators` | logado | Colaboradores do projeto, por nome, com os times e o valor de cada um |
| DELETE | `/api/projects/:projectId/collaborators/:personId` | `collaborators.manage` | Tira a pessoa do projeto: apaga o valor e a tira de todos os times dele |

```json
[
  {
    "person": { "id": "…", "name": "Bruno Lima", "email": "bruno@example.com" },
    "teams": [{ "id": "…", "name": "Web" }],
    "pay_rate_cents": 5500
  },
  {
    "person": { "id": "…", "name": "Elisa Prado", "email": "elisa@example.com" },
    "teams": [],
    "pay_rate_cents": 4000
  }
]
```
- `teams` traz só os times deste projeto, por nome. Vem vazio para quem ainda não entrou em nenhum time; essa pessoa bate ponto, mas não pode ser responsável por tarefa.
- `pay_rate_cents` é o valor da pessoa. Um membro recebe o próprio valor e `null` no dos colegas. Para um admin, `null` só aparece em quem entrou num time antes de o valor ser obrigatório: essa pessoa não bate ponto até receber um valor (o `PUT` acima) ou sair do projeto.

O `DELETE` faz as duas remoções numa transação e responde `204`. As tarefas e as sessões de trabalho da pessoa ficam como estão. Se ela não tinha valor nem time no projeto, a resposta é `404`.

Para **pôr** alguém no projeto, use o `PUT` de `/allocations/:personId` acima e, depois, se quiser, `POST /api/teams/:teamId/members`. Nessa ordem: o time recusa quem ainda não tem valor.

### Visão geral

Tudo o que a aba Visão geral mostra, numa resposta só. É só de admins, porque soma o que o projeto custou e rendeu: um membro recebe `403`.

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/overview` | `billing.view` | Pessoas, times, horas, custo, receita e margem, tempo de projeto, tarefas, integrações e as horas de cada pessoa |

```json
{
  "project": {
    "id": "…",
    "name": "App de Pedidos",
    "created_at": "2025-11-10T06:47:17Z",
    "age": { "days": 330, "weeks": 47, "months": 10 },
    "customer": { "id": "…", "name": "Rede Bom Preço" },
    "bill_rate_cents": 14000
  },
  "people": { "total": 3, "without_team": 0, "without_rate": 0, "working_now": 0 },
  "teams": { "total": 2 },
  "time": {
    "total_seconds": 95400,
    "session_count": 10,
    "last_7_days_seconds": 44100,
    "last_30_days_seconds": 86400,
    "first_session_at": "2026-08-23T12:00:00Z",
    "last_session_at": "2026-10-05T12:10:00Z"
  },
  "money": { "pay_amount_cents": 169083, "bill_amount_cents": 371000, "margin_cents": 201917 },
  "tasks": { "total": 14, "overdue": 2 },
  "integrations": {
    "total": 2,
    "enabled": 1,
    "items": [
      { "id": "…", "type": "gitlab", "display_name": "Espelho no GitLab", "enabled": false, "has_token": false, "created_at": "…" },
      { "id": "…", "type": "github", "display_name": "Repositório do app", "enabled": true, "has_token": false, "created_at": "…" }
    ]
  },
  "by_person": [
    {
      "person": { "id": "…", "name": "Diego Rocha" },
      "in_project": true,
      "working_now": false,
      "total_seconds": 54600,
      "session_count": 5,
      "pay_amount_cents": 91000,
      "bill_amount_cents": 212333
    }
  ],
  "generated_at": "2026-10-06T06:49:02Z"
}
```
- `generated_at` é a hora da conta. As sessões abertas, a idade do projeto e as janelas de 7 e 30 dias foram medidas até ela; a resposta é uma fotografia, e chamar de novo dá números novos.
- `project.created_at` é o início do projeto: o dia em que ele foi cadastrado. `age` é o tempo desde então em três medidas, cada uma arredondada para baixo: dias e semanas corridos, e meses de calendário já completos.
- `project.customer` e `project.bill_rate_cents` são `null` em projeto interno.
- `people.total` é a mesma conta de `/collaborators`: quem tem valor por hora no projeto ou está em algum time dele, cada pessoa uma vez. `without_rate` são os que estão num time sem valor e não batem ponto; `working_now`, quem está com uma sessão aberta no projeto.
- `time` soma todas as sessões do projeto, e a aberta conta até `generated_at`. As duas janelas contam só o trecho de cada sessão que caiu dentro delas. `first_session_at` e `last_session_at` são o início da sessão mais antiga e o da mais recente, ou `null` se ninguém bateu ponto.
- `money` soma o valor de cada sessão, já arredondado, então bate com as linhas de `/work-sessions`. `pay_amount_cents` é o custo e `bill_amount_cents`, a receita; cada um é `null` quando nenhuma sessão tem aquele valor por hora. `margin_cents` é a receita menos o custo, e `null` sem receita.
- `tasks.overdue` conta as tarefas com o prazo vencido. Tarefa sem prazo não entra, e como tarefa não tem estado de concluída, ela só sai da conta quando o prazo muda ou ela é apagada.
- `integrations.items` vem da mais nova para a mais antiga, sem o token e sem o `metadata`. É `[]` quando não há integração.
- `by_person` traz só quem tem sessão no projeto, de quem mais trabalhou para quem menos, e a soma das linhas dá os totais. Quem já saiu do projeto continua na lista, com `in_project: false`, e não entra em `people.total`. É `[]` quando ninguém bateu ponto.

---

## Times

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/teams` | `teams.manage` | Cria um time: `{"name": "Backend"}` |
| GET | `/api/projects/:projectId/teams` | logado | Times do projeto |
| GET | `/api/teams/:teamId` | logado | Detalhes do time |
| PATCH | `/api/teams/:teamId` | `teams.manage` | Renomeia |
| DELETE | `/api/teams/:teamId` | `teams.manage` | Exclui o time e os vínculos dos membros |
| POST | `/api/teams/:teamId/members` | `teams.manage` | Adiciona um membro que já está no projeto: `{"person_id": "…"}` |
| DELETE | `/api/teams/:teamId/members` | `teams.manage` | Remove um membro: `{"person_id": "…"}` |
| GET | `/api/teams/:teamId/members` | logado | Membros do time |

A pessoa precisa ser da mesma organização do projeto e já estar nele, com valor por hora (`PUT /api/projects/:projectId/allocations/:personId`). Sem isso o `POST` de membro responde `400`.

---

## Tarefas

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/tasks` | logado | Cria uma tarefa |
| GET | `/api/projects/:projectId/tasks` | logado | Tarefas do projeto. Filtros: `?q=`, `?assignee_id=`, `?deadline_to=`, `?priority=`, `?status=` e `?label_id=`; com `?page=`, uma página por vez |
| GET | `/api/projects/:projectId/labels` | logado | Etiquetas do projeto, em ordem alfabética |
| POST | `/api/projects/:projectId/labels` | `labels.manage` | Cria uma etiqueta: `{"name": "bug"}` |
| PATCH | `/api/projects/:projectId/labels/:labelId` | `labels.manage` | Renomeia: `{"name": "defeito"}` |
| DELETE | `/api/projects/:projectId/labels/:labelId` | `labels.manage` | Exclui a etiqueta; as tarefas só a perdem |
| GET | `/api/tasks/:taskId` | logado | Detalhes da tarefa |
| PATCH | `/api/tasks/:taskId` | logado | Altera a tarefa |
| DELETE | `/api/tasks/:taskId` | logado | Exclui a tarefa e as sessões dela |
| POST | `/api/tasks/:taskId/link-external-item` | logado | Vincula a uma issue ou a um cartão |
| DELETE | `/api/tasks/:taskId/link-external-item` | logado | Desfaz o vínculo |
| GET | `/api/tasks/:taskId/external-details` | logado | Busca título e estado do item na plataforma |

```http
POST /api/projects/:projectId/tasks
Content-Type: application/json

{
  "name": "Tela de checkout",
  "description": "Opcional",
  "assignee_id": "…",
  "deadline": "2026-10-05T23:59:00-03:00",
  "priority": "high",
  "label_ids": ["…", "…"]
}
```
`priority` é `urgent`, `high`, `medium`, `low` ou `none` (o padrão, sem prioridade). `label_ids` são etiquetas do próprio projeto: uma de outro projeto responde `400 task.label_other_project`, e um id repetido conta uma vez. A tarefa devolve `priority` e `labels` (`[{"id", "name"}]`, sempre uma lista). No `PATCH`, campo omitido mantém o valor, e `label_ids: []` tira todas as etiquetas.

Toda tarefa nasce em `backlog`: o `POST` não lê `status`. O `PATCH` aceita `status` (`backlog`, `in_progress`, `awaiting_closure` ou `closed`; omitido, mantém) e a tarefa o devolve; outro valor responde `400 task.invalid_status`. Qualquer um do projeto muda o status, e a troca é livre entre os quatro. Além do `PATCH`, o `POST .../work-sessions/clock-in` põe a tarefa em `in_progress`, seja qual for o status dela, depois de criar a sessão (um ponto recusado não muda nada); o clock-out não mexe no status.

As etiquetas são do projeto, não da organização, e o nome é único no projeto sem diferenciar maiúsculas (`label.name_taken`), com até 30 caracteres. Só admins criam, renomeiam e excluem; quem está no projeto lê e escolhe.
O `assignee_id` é opcional: sem ele a tarefa fica disponível, e quem bater o ponto nela passa a ser o responsável. Um responsável escolhido precisa estar no projeto, isto é, ter valor por hora nele ou estar em algum time dele (time é opcional). Quem está logado também pode se pôr como responsável (o "atribuir a mim") sem estar no projeto, no `POST` e no `PATCH`. Fora isso, `task.assignee_not_in_team`. Sem `deadline`, o prazo fica em 7 dias a partir de agora.

```http
GET /api/projects/:projectId/tasks?q=frete&assignee_id=…&deadline_to=2026-10-12T02:59:59Z&page=1&per_page=10
```
Todos os parâmetros são opcionais:

- `q` busca no nome, sem diferenciar maiúsculas de minúsculas (acentos contam), com até 100 caracteres.
- `assignee_id` traz só as tarefas daquela pessoa; `none` traz só as sem responsável, e `any`, só as que têm responsável, de qualquer pessoa. É com `none` e `any` que a Lista de tarefas monta as duas listas, e com o id da pessoa logada, sem `page`, que a aba Minhas tarefas traz tudo o que é dela.
- `deadline_to` traz as tarefas com prazo até aquele instante, inclusive. É uma data com hora em RFC 3339; num fuso escrito com `+`, use `%2B` na URL.
- `priority` traz as tarefas com qualquer uma das prioridades, separadas por vírgula: `priority=urgent,high`.
- `status` traz as tarefas com qualquer um dos status, separados por vírgula: `status=in_progress,awaiting_closure`. Um valor fora dos quatro responde `400 task.invalid_status_filter`.
- `label_id` traz as tarefas que têm qualquer uma das etiquetas, separadas por vírgula; uma tarefa com duas delas conta uma vez. Junto de `priority`, valem os dois.
- `page` começa em 1. `per_page` vale 10 por padrão, vai até 100 e só é lido junto de `page`.

Sem `page`, a resposta é o array com todas as tarefas que passam pelos filtros, da mais nova para a mais antiga. Com `page`, vem uma página:

```json
{
  "items": [{ "id": "…", "name": "Endpoint de cálculo de frete", "assignee_id": "…", "deadline": "2026-10-03T23:59:00-03:00" }],
  "total": 1,
  "page": 1,
  "per_page": 10,
  "assignees": [{ "id": "…", "name": "Bruno Lima", "email": "bruno@example.com" }]
}
```
`total` conta tudo o que passa pelos filtros. Uma `page` além do fim devolve a última, e o campo `page` diz qual foi. `assignees` lista quem é responsável por alguma tarefa do projeto, mesmo que já tenha saído dos times. Um parâmetro inválido responde `400`.

```http
POST /api/tasks/:taskId/link-external-item
Content-Type: application/json

{
  "integration_id": "…",
  "external_item_id": "42",
  "external_item_url": "https://github.com/acme/app/issues/42"
}
```
A integração precisa ser do mesmo projeto da tarefa. O `external_item_id` é o número da issue (GitHub e GitLab) ou o cartão do Trello: o link curto, o id ou o endereço dele.

O `external-details` nunca falha por causa da plataforma. Se ela estiver fora, o token estiver errado ou ilegível, faltar um campo do `metadata`, o item não for do repositório ou do quadro configurado, ou a integração estiver desativada, a resposta é `200` com `{"details": null, "error": {"code": "integration.invalid_token", "params": {"provider": "GitHub"}}}`, o mesmo formato dos outros erros. Com sucesso, `details` traz `title`, `state` e `url`; num cartão do Trello, `state` é o nome da lista em que ele está, ou `arquivado`.

Na lista de tarefas e no detalhe, a tarefa vinculada traz `external_integration` com o `id` e o `type` da integração.

---

## Registro de tempo (ponto)

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/work-sessions/clock-in` | logado | Abre uma sessão: `{"task_id": "…"}` |
| POST | `/api/projects/:projectId/work-sessions/clock-out` | logado | Fecha a sessão aberta: `{}` |
| GET | `/api/projects/:projectId/work-sessions` | logado | Sessões do projeto. Filtros: `?task_id=` e `?person_id=`. O membro só recebe as próprias: sem `person_id` vale o dele, e o de outra pessoa dá 403 |
| GET | `/api/projects/:projectId/work-sessions/total` | logado | `{"total_seconds", "pay_amount_cents", "bill_amount_cents"}`. Exige `task_id`, `person_id` ou os dois. Para o membro vale o mesmo recorte das sessões |
| GET | `/api/work-sessions/active` | logado | A sua sessão aberta, com a tarefa e o projeto, ou `null` |

- Sem `person_id`, clock-in e clock-out valem para a pessoa logada. Só admins podem mandar o `person_id` de outra pessoa, e ela precisa ser da organização do projeto.
- O total só soma sessões de tarefas do projeto da rota. Um `task_id` de outro projeto dá `0`.
- Cada pessoa tem no máximo uma sessão aberta. O banco garante isso com o índice único parcial `one_active_session`, então nem duas requisições simultâneas conseguem abrir duas sessões.
- Uma sessão aberta conta no total até o momento da consulta.

### Valores nas sessões

- **Sem valor, sem ponto.** O clock-in responde `400` quando a pessoa não tem valor por hora no projeto (`PUT /api/projects/:projectId/allocations/:personId`). É o caso de quem foi tirado do projeto e ficou com uma tarefa dele. Vale também para um admin batendo o ponto de outra pessoa.
- **O valor é travado no clock-in.** A sessão guarda `pay_rate_cents` (o que a pessoa recebe por hora) e `bill_rate_cents` (o que o cliente paga; `null` em projeto sem valor cobrado). Mudar um valor depois só afeta as sessões seguintes.
- Cada sessão traz também `pay_amount_cents` e `bill_amount_cents`: o tempo da sessão vezes o valor por hora, arredondado para o centavo. O total soma as sessões já arredondadas.
- **Quem vê o quê.** Um admin recebe os quatro campos de todas as sessões. Um membro recebe `pay_rate_cents` e `pay_amount_cents` só nas próprias sessões; nas dos colegas, e sempre nos dois campos de `bill`, vem `null`. No total, um membro só recebe `pay_amount_cents` quando filtra por ele mesmo (`?person_id=` o próprio id).
- Sessões criadas antes dos valores existirem ficam com tudo `null`.
- **O dono não tem valor pago.** O clock-in do dono (`is_owner`) não exige valor por hora: a sessão guarda `pay_rate_cents` `0`, `bill_rate_cents` do projeto e `owner_hours` `true`, e ele entra no projeto (com valor `0`) se ainda não estava. O custo dessas horas é zero e a receita é o valor cobrado, então a margem do projeto soma o que o dono trabalha. `PUT .../allocations/:personId` no dono grava `0` seja qual for o valor enviado, e `POST /api/orgs/:orgId/projects` feito pelo dono já o põe no projeto. As telas mostram o valor cobrado como o que o dono ganhou.

```json
{
  "id": "…",
  "task_id": "…",
  "person_id": "…",
  "start_at": "2026-10-05T09:00:00-03:00",
  "end_at": "2026-10-05T10:30:00-03:00",
  "pay_rate_cents": 2000,
  "pay_amount_cents": 3000,
  "bill_rate_cents": 10000,
  "bill_amount_cents": 15000
}
```

---

## Integrações

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/integrations` | `integrations.manage` | Cria e valida a conexão na plataforma |
| GET | `/api/projects/:projectId/integrations` | logado | Integrações do projeto |
| GET | `/api/integrations/:integrationId` | logado | Detalhes |
| PATCH | `/api/integrations/:integrationId` | `integrations.manage` | Altera nome, token, `metadata` ou `enabled` |
| DELETE | `/api/integrations/:integrationId` | `integrations.manage` | Exclui. As tarefas vinculadas perdem o vínculo |

O corpo é o mesmo para todas as plataformas. O que é comum fica no primeiro nível, e os campos próprios de cada uma vão em `metadata`:

```http
POST /api/projects/:projectId/integrations
Content-Type: application/json

{
  "type": "github",
  "display_name": "Repositório do app",
  "token": "ghp_…",
  "metadata": { "repo": "acme/app" },
  "enabled": true
}
```

| `type` | `token` | `metadata` (todos os campos são obrigatórios) |
|---|---|---|
| `github` | token pessoal com leitura de issues | `repo`: `dono/repositorio` ou o endereço do repositório |
| `gitlab` | token com escopo `read_api` | `project_url`: `grupo/projeto` ou o endereço do projeto no gitlab.com |
| `trello` | token da API do Trello, com leitura | `api_key`: a chave do Power-Up (trello.com/apps/admin); `board_id`: o endereço do quadro, o link curto ou o id |

Resposta (`201`), igual em todas as rotas de integração:

```json
{
  "id": "…",
  "project_id": "…",
  "type": "github",
  "display_name": "Repositório do app",
  "has_token": true,
  "metadata": { "repo": "acme/app" },
  "enabled": true,
  "created_at": "2026-10-06T09:00:00-03:00"
}
```

- Cada tipo confere o próprio `metadata` antes de falar com a plataforma. Faltando um campo, a resposta é `400` com o nome dele, por exemplo `informe o campo "Quadro" do Trello`. Chave que o tipo não declara é descartada, e o valor é guardado normalizado (o endereço do repositório vira `dono/repositorio`; o do quadro, o link curto).
- A conexão é validada na plataforma antes de salvar. O token fica criptografado (AES-GCM) no banco e nunca volta nas respostas: elas mostram só `has_token`. O `metadata` fica em claro e volta, então não é lugar de segredo.
- Sem `enabled` no corpo, a integração nasce ativa.
- O corpo antigo, com `config`, não é mais aceito: a resposta é `400` pedindo o campo do `metadata`.

```http
PATCH /api/integrations/:integrationId
Content-Type: application/json

{
  "display_name": "Repositório principal",
  "metadata": { "repo": "acme/site" },
  "enabled": false
}
```

- Todos os campos são opcionais. `token` ausente ou vazio mantém o guardado; `metadata` presente substitui o atual inteiro. O `type` não muda.
- A plataforma só é consultada de novo quando veio um token ou o `metadata` mudou de fato, e a conexão nova é validada com o token guardado quando nenhum veio. Renomear ou desativar não depende de o token ainda valer.
- Uma integração criada antes do `metadata` aparece com `metadata: {}` e `has_token: true`. Basta um `PATCH` com o `metadata` dela, sem token.

---

## Idioma

A interface fala português do Brasil (padrão) e inglês. Estas duas rotas são do navegador, não da API JSON, e não precisam de sessão.

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/lang/:code?next=/caminho` | Grava o cookie `wtt_lang` (`pt-BR` ou `en`, por um ano) e responde **303** para `next`. Só caminhos do próprio site: qualquer outro vira `/`. Um código desconhecido não grava nada |
| GET | `/i18n/:idioma.js?v=<hash>` | `window.I18N = {lang, messages}`, os textos do idioma para o JavaScript. Com o `v` certo, o cache é de um ano (`immutable`); sem ele, o navegador revalida pelo `ETag`. **404** para idioma desconhecido |

Qual idioma uma página usa: o cookie `wtt_lang`; sem ele, o `Accept-Language`; sem nenhum dos dois, português. As respostas variam por `Cookie` e `Accept-Language`.

---

## Fluxo de teste sugerido

1. **Healthcheck:** `GET /healthcheck`.
2. **Signup:** `POST /api/auth/signup`. O cliente guarda o cookie.
3. **Quem sou eu:** `GET /api/auth/me`. Copie o `organization_id` para `org_id`.
4. **Criar projeto:** `POST /api/orgs/:orgId/projects`. Copie o `id` para `project_id`.
5. **Entrar no projeto:** `PUT /api/projects/:projectId/allocations/:personId` com o seu `id` e `pay_rate_cents`. Sem esse passo você não entra em time nem bate ponto.
6. **Criar time:** `POST /api/projects/:projectId/teams`, e depois `POST /api/teams/:teamId/members` com o seu próprio `id`.
7. **Criar tarefa:** `POST /api/projects/:projectId/tasks` com você como responsável.
8. **Cliente e valor cobrado:** `POST /api/orgs/:orgId/customers` e `PUT /api/projects/:projectId/billing` com o cliente e `bill_rate_cents`.
9. **Bater o ponto:** `clock-in` com `{"task_id": "…"}`, `GET /api/work-sessions/active` e `clock-out`.
10. **Totais:** `GET /api/projects/:projectId/work-sessions/total?task_id=…`.
11. **Convidar alguém:** `POST /api/orgs/:orgId/invites`, depois `POST /api/auth/invites/:token/accept` numa sessão sem cookie (ou após `logout`).

## Variáveis de ambiente (Insomnia)

| Variável | Descrição |
|---|---|
| `base_url` | URL do servidor, por padrão `http://localhost:8080` |
| `org_id` | `organization_id` do `/api/auth/me` |
| `person_id` | `id` do `/api/auth/me` ou de outra pessoa |
| `project_id` | `id` retornado ao criar o projeto |
| `team_id` | `id` retornado ao criar o time |
| `customer_id` | `id` retornado ao criar o cliente |
| `task_id` | `id` retornado ao criar a tarefa |
| `integration_id` | `id` retornado ao criar a integração |
| `invite_id` / `invite_token` | `id` e `token` retornados ao criar o convite |
