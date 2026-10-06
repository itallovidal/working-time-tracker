# Rotas da API — Working Time Tracker

**Base URL:** `http://localhost:8080`

Todas as rotas falam JSON. Erros sempre vêm como `{"error": "mensagem"}`, com a mensagem em português, pronta para mostrar a quem usa.

## Como a autenticação funciona

- O login e o signup devolvem um cookie `wtt_session` (HttpOnly, SameSite=Lax, válido por 7 dias). Clientes HTTP como o Insomnia guardam o cookie sozinhos: faça o login uma vez e as próximas chamadas já vão autenticadas.
- Tudo fora de `/api/auth/*` e `/healthcheck` exige a sessão. Sem ela, a resposta é **401**.
- Cada pessoa pertence a uma organização. Um recurso de outra organização responde **404**, como se não existisse.
- Rotas marcadas como **admin** respondem **403** para membros.
- `POST`, `PUT` e `PATCH` precisam de `Content-Type: application/json`. Outro formato responde **415**, o que também protege contra CSRF.
- Signup, login e convites têm limite de tentativas por IP. Acima dele, a resposta é **429**.

## Códigos de status

| Status | Quando |
|---|---|
| 200 / 201 / 204 | Sucesso (204 não tem corpo) |
| 400 | Dado inválido, com a mensagem explicando o quê |
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
| DELETE | `/api/orgs/:orgId` | admin | Exclui a organização com as pessoas e os convites. Falha se ainda houver projetos |
| GET | `/api/orgs/:orgId/persons` | logado | Pessoas da organização |
| POST | `/api/orgs/:orgId/projects` | admin | Cria um projeto |
| GET | `/api/orgs/:orgId/projects` | logado | Projetos da organização |
| POST | `/api/orgs/:orgId/customers` | admin | Cria um cliente |
| GET | `/api/orgs/:orgId/customers` | admin | Clientes da organização, em ordem alfabética |

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

A jornada semanal e a duração da sprint não são da organização: ficam em cada projeto (`weekly_hours` e `sprint_duration_days`).

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
| POST | `/api/orgs/:orgId/invites` | admin | Gera um link de convite |
| GET | `/api/orgs/:orgId/invites` | admin | Convites ainda válidos |
| DELETE | `/api/invites/:inviteId` | admin | Revoga um convite |

```http
POST /api/orgs/:orgId/invites
Content-Type: application/json

{ "email": "bruno@empresa.com", "role": "member" }
```
`email` é opcional, e `role` é `admin` ou `member` (o padrão é `member`). A resposta traz `token` e `path` (`/invite/<token>`). **O token só aparece nesta resposta**: o banco guarda apenas o hash dele.

---

## Clientes

Quem contrata a organização. Todas as rotas são de **admin**.

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
| PATCH | `/api/persons/:personId/role` | admin | Muda o papel: `{"role": "admin"}` ou `{"role": "member"}` |
| GET | `/api/persons/:personId/allocations` | a própria pessoa ou admin | Quanto a pessoa recebe por hora em cada projeto |

A organização nunca fica sem admin: rebaixar o último admin responde `400`. Pessoas entram na organização pelo signup ou por convite.

---

## Projetos

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId` | logado | Detalhes do projeto |
| PATCH | `/api/projects/:projectId` | admin | Altera o projeto |
| DELETE | `/api/projects/:projectId` | admin | Exclui o projeto com times, tarefas, sessões e integrações |
| GET | `/api/projects/:projectId/members` | logado | Pessoas que estão em algum time do projeto, sem repetir |

```http
POST /api/orgs/:orgId/projects
Content-Type: application/json

{
  "name": "App de Pedidos",
  "description": "Opcional",
  "sprint_duration_days": 14,
  "weekly_hours": 40,
  "daily_time": "09:30",
  "weekly_sync_day": "friday"
}
```
- `sprint_duration_days` vai de 1 a 90, e o padrão é 14.
- `weekly_hours` é a jornada semanal esperada no projeto, de 1 a 168. É opcional.
- `daily_time` usa o formato `HH:MM`.
- `weekly_sync_day` vai de `monday` a `sunday`.

No `PATCH`, um campo omitido mantém o valor atual, `""` apaga `daily_time` ou `weekly_sync_day`, e `0` apaga `weekly_hours`.

O projeto traz `member_count`, as pessoas que estão em algum time dele, sem repetir (a mesma conta de `/members`), e `task_count`, as tarefas dele. Os dois vêm na lista e no detalhe.

O projeto traz `customer` (`{"id", "name"}` ou `null`) para qualquer membro. O valor cobrado nunca vem aqui: ele fica em `/billing`.

### Cliente e valor cobrado

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/billing` | admin | Cliente e valor que ele paga por hora |
| PUT | `/api/projects/:projectId/billing` | admin | Substitui os dois. O que vier `null` é apagado |

```http
PUT /api/projects/:projectId/billing
Content-Type: application/json

{ "customer_id": "<id do cliente>", "bill_rate_cents": 10000 }
```
Os valores são sempre em **centavos**, na moeda da organização: `10000` é 100,00. O cliente precisa ser da mesma organização do projeto. Mandar `{"customer_id": null, "bill_rate_cents": null}` volta o projeto a ser interno.

### Valor pago a cada pessoa

O vínculo de uma pessoa com o projeto e quanto ela recebe por hora nele. Há um valor por pessoa em cada projeto.

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/allocations` | logado | Um admin recebe todos os valores; um membro recebe só o dele, ou `[]` |
| PUT | `/api/projects/:projectId/allocations/:personId` | admin | Define o valor da pessoa: `{"pay_rate_cents": 2000}`. Cria o vínculo ou troca o valor |
| DELETE | `/api/projects/:projectId/allocations/:personId` | admin | Tira a pessoa do projeto |

`pay_rate_cents` vai de `0` a `100000000`. Zero vale: é alguém que trabalha no projeto sem receber por hora. A pessoa precisa ser da mesma organização do projeto.

---

## Times

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/teams` | admin | Cria um time: `{"name": "Backend"}` |
| GET | `/api/projects/:projectId/teams` | logado | Times do projeto |
| GET | `/api/teams/:teamId` | logado | Detalhes do time |
| PATCH | `/api/teams/:teamId` | admin | Renomeia |
| DELETE | `/api/teams/:teamId` | admin | Exclui o time e os vínculos dos membros |
| POST | `/api/teams/:teamId/members` | admin | Adiciona um membro: `{"person_id": "…"}` |
| DELETE | `/api/teams/:teamId/members` | admin | Remove um membro: `{"person_id": "…"}` |
| GET | `/api/teams/:teamId/members` | logado | Membros do time |

A pessoa precisa ser da mesma organização do projeto.

---

## Tarefas

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/tasks` | logado | Cria uma tarefa |
| GET | `/api/projects/:projectId/tasks` | logado | Tarefas do projeto |
| GET | `/api/tasks/:taskId` | logado | Detalhes da tarefa |
| PATCH | `/api/tasks/:taskId` | logado | Altera a tarefa |
| DELETE | `/api/tasks/:taskId` | logado | Exclui a tarefa e as sessões dela |
| POST | `/api/tasks/:taskId/link-external-item` | logado | Vincula a uma issue |
| DELETE | `/api/tasks/:taskId/link-external-item` | logado | Desfaz o vínculo |
| GET | `/api/tasks/:taskId/external-details` | logado | Busca título e estado da issue na plataforma |

```http
POST /api/projects/:projectId/tasks
Content-Type: application/json

{
  "name": "Tela de checkout",
  "description": "Opcional",
  "assignee_id": "…",
  "deadline": "2026-10-05T23:59:00-03:00"
}
```
O responsável precisa estar em algum time do projeto. Sem `deadline`, o prazo fica em 7 dias a partir de agora.

```http
POST /api/tasks/:taskId/link-external-item
Content-Type: application/json

{
  "integration_id": "…",
  "external_item_id": "42",
  "external_item_url": "https://github.com/acme/app/issues/42"
}
```
A integração precisa ser do mesmo projeto da tarefa.

O `external-details` nunca falha por causa da plataforma. Se ela estiver fora, o token estiver errado ou a integração estiver desativada, a resposta é `200` com `{"details": null, "error": "motivo"}`.

---

## Registro de tempo (ponto)

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/work-sessions/clock-in` | logado | Abre uma sessão: `{"task_id": "…"}` |
| POST | `/api/projects/:projectId/work-sessions/clock-out` | logado | Fecha a sessão aberta: `{}` |
| GET | `/api/projects/:projectId/work-sessions` | logado | Sessões do projeto. Filtros: `?task_id=` e `?person_id=` |
| GET | `/api/projects/:projectId/work-sessions/total` | logado | `{"total_seconds", "pay_amount_cents", "bill_amount_cents"}`. Exige `task_id`, `person_id` ou os dois |
| GET | `/api/work-sessions/active` | logado | A sua sessão aberta, com a tarefa e o projeto, ou `null` |

- Sem `person_id`, clock-in e clock-out valem para a pessoa logada. Só admins podem mandar o `person_id` de outra pessoa, e ela precisa ser da organização do projeto.
- O total só soma sessões de tarefas do projeto da rota. Um `task_id` de outro projeto dá `0`.
- Cada pessoa tem no máximo uma sessão aberta. O banco garante isso com o índice único parcial `one_active_session`, então nem duas requisições simultâneas conseguem abrir duas sessões.
- Uma sessão aberta conta no total até o momento da consulta.

### Valores nas sessões

- **Sem valor, sem ponto.** O clock-in responde `400` quando a pessoa não tem valor por hora no projeto (`PUT /api/projects/:projectId/allocations/:personId`). Vale também para um admin batendo o ponto de outra pessoa.
- **O valor é travado no clock-in.** A sessão guarda `pay_rate_cents` (o que a pessoa recebe por hora) e `bill_rate_cents` (o que o cliente paga; `null` em projeto sem valor cobrado). Mudar um valor depois só afeta as sessões seguintes.
- Cada sessão traz também `pay_amount_cents` e `bill_amount_cents`: o tempo da sessão vezes o valor por hora, arredondado para o centavo. O total soma as sessões já arredondadas.
- **Quem vê o quê.** Um admin recebe os quatro campos de todas as sessões. Um membro recebe `pay_rate_cents` e `pay_amount_cents` só nas próprias sessões; nas dos colegas, e sempre nos dois campos de `bill`, vem `null`. No total, um membro só recebe `pay_amount_cents` quando filtra por ele mesmo (`?person_id=` o próprio id).
- Sessões criadas antes dos valores existirem ficam com tudo `null`.

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
| POST | `/api/projects/:projectId/integrations` | admin | Cria e valida as credenciais na plataforma |
| GET | `/api/projects/:projectId/integrations` | logado | Integrações do projeto |
| GET | `/api/integrations/:integrationId` | logado | Detalhes |
| PATCH | `/api/integrations/:integrationId` | admin | Altera nome, credencial ou `enabled` |
| DELETE | `/api/integrations/:integrationId` | admin | Exclui. As tarefas vinculadas perdem o vínculo |

```http
POST /api/projects/:projectId/integrations
Content-Type: application/json

{
  "type": "github",
  "display_name": "Repositório do app",
  "config": { "token": "ghp_…", "repo": "acme/app" },
  "enabled": true
}
```
- Para o GitLab, `config` é `{"token": "…", "project_url": "grupo/projeto"}`.
- A credencial é validada na plataforma antes de salvar e fica criptografada (AES-GCM) no banco.
- A credencial nunca volta nas respostas: elas mostram só `has_config`.

---

## Fluxo de teste sugerido

1. **Healthcheck:** `GET /healthcheck`.
2. **Signup:** `POST /api/auth/signup`. O cliente guarda o cookie.
3. **Quem sou eu:** `GET /api/auth/me`. Copie o `organization_id` para `org_id`.
4. **Criar projeto:** `POST /api/orgs/:orgId/projects`. Copie o `id` para `project_id`.
5. **Criar time:** `POST /api/projects/:projectId/teams`, e depois `POST /api/teams/:teamId/members` com o seu próprio `id`.
6. **Criar tarefa:** `POST /api/projects/:projectId/tasks` com você como responsável.
7. **Cliente e valores:** `POST /api/orgs/:orgId/customers`, `PUT /api/projects/:projectId/billing` com o cliente e `bill_rate_cents`, e `PUT /api/projects/:projectId/allocations/:personId` com o seu `id` e `pay_rate_cents`. Sem esse último passo o ponto não abre.
8. **Bater o ponto:** `clock-in` com `{"task_id": "…"}`, `GET /api/work-sessions/active` e `clock-out`.
9. **Totais:** `GET /api/projects/:projectId/work-sessions/total?task_id=…`.
10. **Convidar alguém:** `POST /api/orgs/:orgId/invites`, depois `POST /api/auth/invites/:token/accept` numa sessão sem cookie (ou após `logout`).

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
