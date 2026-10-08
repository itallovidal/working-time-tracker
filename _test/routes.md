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
| POST | `/api/auth/clerk/login` | público, token do Clerk | Entra pelo Clerk (ver abaixo) |
| POST | `/api/auth/clerk/signup` | público, token do Clerk | Cria uma organização para quem entrou pelo Clerk |
| POST | `/api/auth/clerk/join` | público, token do Clerk | Aceita um convite pendente feito para o email do Clerk |

### Entrar pelo Clerk

Só existem com `CLERK_SECRET_KEY` e `CLERK_PUBLISHABLE_KEY` no servidor; sem eles respondem **404** `auth.clerk_disabled`. Quem chama é o navegador, depois de entrar no Clerk (`/auth/clerk/continue`): o token de sessão do Clerk vai em `Authorization: Bearer <jwt>` (vale 60 segundos), e o corpo é JSON. A resposta abre a sessão do sistema (cookie `wtt_session`) quando há `Token`; os estados que não abrem sessão voltam em **200** sem cookie.

```http
POST /api/auth/clerk/login
Authorization: Bearer eyJhbGciOi...
Content-Type: application/json

{ "invite_token": "…", "password": "…" }   // os dois são opcionais
```

| Resposta | Quando |
|---|---|
| `200 {"status":"ok","identity":{…}}` + cookie | Entrou: o usuário do Clerk já estava ligado, ou a conta com o mesmo email verificado foi ligada agora |
| `201 {"status":"ok","identity":{…}}` + cookie | O `invite_token` criou a conta na organização do convite |
| `200 {"status":"needs_password","email":"…"}` | Há uma conta com esse email e senha; repita com `password` para ligá-la. Senha errada: 401 `auth.invalid_credentials` |
| `200 {"status":"no_account","email":"…","name":"…","invites":[{"id","organization_name","role"}]}` | Não há conta. `invites` são os convites pendentes para o email; nada é aceito sozinho |
| 401 `auth.clerk_token_invalid` | Assinatura, prazo ou origem (`azp`) não conferem, ou o usuário não existe mais |
| 403 `auth.clerk_email_unverified` | O email primário do Clerk não foi verificado |
| 404 `auth.invite_invalid` / 400 `auth.invite_email_mismatch` | O `invite_token` não vale, ou o convite é para outro email |
| 409 `auth.account_exists` / `auth.clerk_account_linked` | O email já é de uma conta em outra organização (convite de outra), ou a conta está ligada a outro usuário do Clerk que ainda existe |
| 502 `auth.clerk_unavailable` | O Clerk não respondeu; tente de novo |

```http
POST /api/auth/clerk/signup          { "organization_name": "Minha Empresa", "name": "Ana Souza" }   // name é opcional
POST /api/auth/clerk/join            { "invite_id": "<uuid de um convite de no_account.invites>", "name": "…" }
```

`signup` e `join` respondem **201** com o mesmo corpo e o cookie. `signup`: 400 `auth.org_name_required`, 409 `auth.account_exists` (já há conta para esse usuário ou email). `join`: o convite precisa ter email e ser o email verificado do Clerk (convite sem email só entra pelo `invite_token` do `login`).

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
| GET | `/api/orgs/:orgId/overview` | admin | Visão geral da organização: o tempo e o dinheiro de todos os projetos em três janelas, e as horas de cada pessoa. Veja abaixo |
| POST | `/api/orgs/:orgId/projects` | `projects.create` | Cria um projeto |
| GET | `/api/orgs/:orgId/projects` | logado | Projetos da organização, do mais novo para o mais antigo. Sem `page`, todos num array; com `page`, uma página. Veja abaixo |
| POST | `/api/orgs/:orgId/customers` | `customers.manage` | Cria um cliente |
| GET | `/api/orgs/:orgId/customers` | `customers.manage` | Clientes da organização, em ordem alfabética |

A organização é criada pelo signup, e o `organization_id` vem no `/api/auth/me`, junto com `organization_currency`.

### Lista de projetos paginada

`GET /api/orgs/:orgId/projects` sem `page` devolve o array de sempre. Com `page` (a partir de 1), devolve uma página:

```json
{
  "items": [ { "id": "…", "name": "App de Pedidos", "customer": { "id": "…", "name": "Clínica Vida Plena" }, "member_count": 5, "task_count": 6, "…": "…" } ],
  "total": 8,
  "page": 1,
  "per_page": 6
}
```

- `per_page` só vale junto de `page`: o padrão é 10 e o teto, 100 (um valor maior vale 100).
- Os projetos vêm do mais novo para o mais antigo, e o id desempata, então duas páginas seguidas não repetem nem pulam um projeto.
- Uma página além da última volta como a última (`page` na resposta é a que valeu), e uma organização sem projetos volta como a página 1, vazia.
- `page` ou `per_page` que não seja um número a partir de 1 é `400 project.invalid_page` ou `400 project.invalid_per_page`.
- Qualquer membro da organização lista; o cliente de cada projeto vem só com o nome.

### Visão geral da organização

`GET /api/orgs/:orgId/overview` é só de admins (`403` para um membro, `404` para outra organização, `401` sem sessão). Soma as sessões de todos os projetos da organização e lê as pessoas dela, e responde com **as três janelas de uma vez**:

```json
{
  "people": { "total": 12, "working_now": 2 },
  "projects": { "total": 8 },
  "periods": {
    "last_7_days": {
      "seconds": 856080, "my_seconds": 91200, "session_count": 121,
      "money": { "pay_amount_cents": 1938200, "bill_amount_cents": 3683361, "margin_cents": 1745161 }
    },
    "last_30_days": { "…": "…" },
    "all_time": { "…": "…" }
  },
  "by_person": [
    { "person": { "id": "…", "name": "Helena Costa" }, "working_now": true,
      "working_on": [
        { "task": { "id": "…", "name": "Relatório de repasses" }, "project": { "id": "…", "name": "Painel do Lojista" } }
      ],
      "last_7_days_seconds": 123000, "last_30_days_seconds": 552840, "total_seconds": 1322400 }
  ],
  "generated_at": "2026-10-07T16:33:00-03:00"
}
```

- **Janelas.** `last_7_days` e `last_30_days` contam só o trecho de cada sessão que caiu dentro delas, e `all_time`, tudo. Uma sessão toda dentro da janela vale os mesmos valores dela; a que atravessa a borda vale o tempo de dentro vezes os valores por hora dela, arredondado ao centavo. A sessão aberta conta até `generated_at`, o mesmo instante das janelas.
- `seconds` é o tempo de todos e `my_seconds` o de quem pediu (o dono vê o dele); `session_count` são as sessões com algum tempo na janela.
- `money` segue a visão geral do projeto: `pay_amount_cents` (custo) e `bill_amount_cents` (receita) são `null` quando nenhuma sessão da janela tem aquele valor por hora, e `margin_cents` é a receita menos o custo, `null` sem receita. As horas do dono têm custo `0`, então entram inteiras na margem.
- `by_person` traz **todas** as pessoas da organização, também as que ainda não bateram ponto (com zeros), da que mais trabalhou no total para a que menos; a soma de cada janela é a das linhas. `working_now` marca quem tem uma sessão aberta em qualquer projeto, e `people.working_now` conta essas pessoas. `working_on` são as tarefas que a pessoa tem na sessão aberta neste instante (os intervalos que ainda não terminaram), na ordem em que entraram, cada uma com o projeto, com id e nome para a tela levar até elas; vem `[]`, nunca `null`, para quem não trabalha agora e para quem abriu uma sessão que ficou sem tarefa. A tela da página inicial usa isso e não ordena nem compara as pessoas pelas horas.
- O total de `all_time` é a soma das visões gerais dos projetos (`GET /api/projects/:projectId/overview`).

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
| POST | `/api/orgs/:orgId/invites` | `people.manage` | Cria um convite: com email e o Clerk ligado, o Clerk manda o email; senão, só o link |
| GET | `/api/orgs/:orgId/invites` | `people.manage` | Convites ainda válidos |
| DELETE | `/api/invites/:inviteId` | `people.manage` | Revoga um convite |
| POST | `/api/projects/:projectId/invites` | `people.manage` (organização) e `collaborators.manage` + `rates.manage` (projeto) | Convida para a organização alguém que já entra neste projeto, com o valor, o time e o grupo escolhidos |
| GET | `/api/projects/:projectId/invites` | `collaborators.manage` (projeto) | Convites pendentes que levam a pessoa a este projeto |

```http
POST /api/orgs/:orgId/invites
Content-Type: application/json

{ "email": "bruno@empresa.com", "role": "member" }
```
`email` é opcional, e `role` é `admin` ou `member` (o padrão é `member`). A resposta (201) traz `token` e `path` (`/invite/<token>`) e `delivery`, que diz como o convite chega: `email` (o Clerk manda o email), `terminal` (`INVITE_DELIVERY=terminal`: não mandou, o link está no log do servidor) ou `link` (sem email, ou sem o Clerk: só o link). **O token só aparece nesta resposta**: o banco guarda apenas o hash dele; `delivery` também só vem aqui.

Com o Clerk ligado e um email, o convite é criado no Clerk primeiro (com validade de 7 dias e `redirect_url` no `/invite/<token>`) e só então gravado: se o Clerk recusar, a resposta é **502** `auth.clerk_invite_failed`; se não responder, **502** `auth.clerk_unavailable`, e nada fica gravado. Um novo convite para o mesmo email na organização **substitui** o anterior (apagado aqui e cancelado no Clerk). `DELETE /api/invites/:inviteId` também cancela o convite no Clerk (um problema lá não impede de revogar).

**Convite que já leva ao projeto.** `POST /api/projects/:projectId/invites` é o convite da tela Adicionar pessoa ao projeto para um e-mail que ainda não é de ninguém da organização:

```http
POST /api/projects/:projectId/invites
Content-Type: application/json

{ "email": "leo@empresa.com", "pay_rate_cents": 25000, "team_id": "<uuid, opcional>", "preset": "manager" }
```
`email` e `pay_rate_cents` são obrigatórios; `preset` é o grupo de permissões (omitido ou `member` é o básico); `team_id` é opcional e tem de ser de um time deste projeto. O papel é sempre `member`. A resposta (201) é a mesma do convite da organização (`token`, `path`, `delivery`) mais `project` (`project_id`, `pay_rate_cents`, `team_id`, `preset`). Quando a pessoa aceita o convite (por senha ou pelo Clerk), entra no projeto com o valor, o grupo e o time; se algum passo falhar, o que deu certo antes fica (o valor, por exemplo), a conta existe do mesmo jeito e a falha vai para o log do servidor. Cada coisa pede a sua permissão, como em `PUT /api/projects/:projectId/allocations/:personId` e `POST /api/teams/:teamId/members`, mais a permissão de pessoas da organização: 403 sem elas, e 403 `allocation.preset_above_yours` para um grupo com permissão que quem convida não tem. Erros: 400 `allocation.rate_required` / `allocation.invalid_rate` / `allocation.invalid_preset`, 400 `projectinvite.team_not_in_project`, 400 `person.invalid_email`, 409 `auth.account_exists`. Se o projeto for excluído antes do aceite, o convite segue valendo só para a organização.

`GET /api/projects/:projectId/invites` devolve os pendentes, do mais novo ao mais antigo, com `project.project_name`, `project.team_name` e, só para quem vê os valores do projeto (`rates.view` ou `rates.manage`), `project.pay_rate_cents`. A lista da organização (`GET /api/orgs/:orgId/invites`) também traz o `project`, sem o valor para quem não é admin.

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
| POST | `/api/tasks/:taskId/claim` | logado | Pega a tarefa para quem está logado, sem bater o ponto e sem mexer no status: só vale se ela não tem responsável (se já é sua, volta como está; se é de outra pessoa, 409 `task.already_assigned`) |
| PATCH | `/api/tasks/:taskId/attributes` | logado | Edição dos detalhes: `priority`, `status`, `label_ids`, `assignee_id` (vazio tira o responsável) e `deadline`, cada um opcional; não toca no nome nem na descrição |
| DELETE | `/api/tasks/:taskId?remove_in=` | logado | Exclui a tarefa (as sessões e as horas ficam). `remove_in`, repetido (até 20), lista as integrações em que o item dela também deve sair: a issue do **GitHub** é apagada (pelo GraphQL, que só deixa quem é admin do repositório; senão a issue é só **fechada** como não planejada, com o aviso `issue_sync.remove_only_closed`) e o cartão do **Trello** é **arquivado**. Uma integração a que a tarefa não está ligada é ignorada; um valor que não é um UUID dá `400` `task.integration_not_found` e a tarefa fica. Responde `200` `{"remote":[{"integration_id","provider","outcome","problem"}]}`, com a lista vazia quando não há `remove_in`: `outcome` é `deleted`, `archived`, `closed` ou `gone` (o item já não existia), e `problem` (`{code, params}`) é o motivo de o item não ter sido tratado (`issue_sync.remove_read_only`, `integration.sync_off`, `integration.invalid_token`, `integration.forbidden`...) ou o aviso de ele ter sido só fechado. O que falha lá não desfaz a exclusão: a tarefa sai de qualquer jeito |
| POST | `/api/tasks/:taskId/link-external-item` | logado | Vincula a uma issue ou a um cartão, à mão. Uma tarefa tem **no máximo um item por integração** (a issue do GitHub e o cartão do Trello ao mesmo tempo): `400` `task.already_linked` se ela já tem item nesta integração e `task.item_taken` se o item é de outra tarefa. O vínculo nasce pendente (`pending`) e a primeira rodada em que o item está aberto o adota |
| DELETE | `/api/tasks/:taskId/link-external-item?integration_id=` | logado | Desfaz o vínculo com aquela integração (obrigatório com mais de um item; com um só, pode faltar). O item que a sincronização já adotou vira item descartado (não volta como tarefa); o que nunca foi adotado some |
| GET | `/api/tasks/:taskId/external-details?integration_id=` | logado | Busca título e estado do item na plataforma (de uma integração; sem o parâmetro, o único item; `400` `task.link_fields_required` com mais de um) |
| POST | `/api/tasks/:taskId/sync` | logado | O botão **Sincronizar** da tela da tarefa: relê o item dela na plataforma e põe os dois em acordo, na hora. Com `?integration_id=` sincroniza só aquele item (é o que o botão de cada cartão faz); sem ele, todos os itens da tarefa, um depois do outro. Mesma resposta da rodada (`created`, `updated`, `closed`, `pushed`, `unmapped`, `errors`, `partial`) mais `problem` (o código do aviso, se o GitHub deixou algo de fora); `409` `integration.sync_running` se há uma rodada na integração; `400` `integration.sync_off` com a sincronização desligada e `task.no_external_item` sem item externo; `404` `integration.issue_gone` se a issue sumiu; uma tarefa ligada à mão a uma issue aberta passa a ser sincronizada |
| POST | `/api/tasks/:taskId/publish` | logado | O passo **Integrações** do modal Nova tarefa: posta a tarefa como uma issue nova do repositório da integração (`{"integration_id"}`) e a liga a ela. O nome é o título, a descrição é o corpo, as etiquetas são criadas no repositório se faltarem e o responsável só vai se o e-mail público de um usuário do GitHub for o dele; prazo e prioridade não vão. No **Trello** a mesma rota cria um cartão na primeira lista aberta do quadro (nome, descrição, etiquetas e prazo como data de entrega; `400` `integration.trello_no_list` se o quadro não tem lista aberta); a tela não a chama para o Trello, porque **toda tarefa criada** (`POST /api/projects/:projectId/tasks`) num projeto com o Trello ligado e a sincronização ligada é postada sozinha, em segundo plano, poucos segundos depois, a menos que a pessoa a tenha desmarcado (`skip_publish`) ou a tarefa já tenha um item desta integração (ligado à mão); a que o modal posta no GitHub também vira cartão. A rota serve para quem quer postar à mão uma tarefa que a postagem sozinha deixou de fora (o servidor reiniciou, o Trello estava fora do ar). `200` `{"task", "problem"}`; `400` `task.already_linked` se a tarefa já tem item **nesta integração** (nas outras ela pode ser postada: é como a mesma tarefa vai para o GitHub e para o Trello), `integration.sync_off` (a integração precisa estar ativa e com a sincronização ligada), `task.integration_other_project`, `issue_sync.publish_read_only` (token sem escrita) ou `integration.issues_disabled`; `404` entre organizações |

```http
POST /api/projects/:projectId/tasks
Content-Type: application/json

{
  "name": "Tela de checkout",
  "description": "Opcional",
  "assignee_id": "…",
  "deadline": "2026-10-05T23:59:00-03:00",
  "priority": "high",
  "label_ids": ["…", "…"],
  "skip_publish": ["<id da integração do Trello>"]
}
```
`skip_publish` (opcional, só na criação, não é guardado) são as integrações em que a tarefa nova **não** deve ser postada sozinha: é o que o modal manda quando a pessoa desmarca o Trello. Sem ele, num projeto com o Trello ligado e a sincronização ligada, o servidor posta o cartão poucos segundos depois (por integração: uma tarefa que o modal já postou no GitHub também vira cartão). Um id que não é de nenhuma integração do projeto não casa com nada; um que não é um UUID, ou mais de 20 ids, responde `400 task.integration_not_found`. `priority` é `urgent`, `high`, `medium`, `low` ou `none` (o padrão, sem prioridade). `label_ids` são etiquetas do próprio projeto: uma de outro projeto responde `400 task.label_other_project`, e um id repetido conta uma vez. A tarefa devolve `priority` e `labels` (`[{"id", "name"}]`, sempre uma lista). No `PATCH`, campo omitido mantém o valor, e `label_ids: []` tira todas as etiquetas.

Toda tarefa nasce em `backlog`: o `POST` não lê `status`. O `PATCH` aceita `status` (`backlog`, `in_progress`, `awaiting_closure` ou `closed`; omitido, mantém) e a tarefa o devolve; outro valor responde `400 task.invalid_status`. Qualquer um do projeto muda o status, e a troca é livre entre os quatro. Além do `PATCH`, o `POST .../work-sessions/clock-in` põe a tarefa em `in_progress`, seja qual for o status dela, depois de criar a sessão (um ponto recusado não muda nada); o clock-out não mexe no status.

As etiquetas são do projeto, não da organização, e o nome é único no projeto sem diferenciar maiúsculas (`label.name_taken`), com até 50 caracteres. Só admins criam, renomeiam e excluem; quem está no projeto lê e escolhe.
O `assignee_id` é opcional: sem ele a tarefa fica disponível, e quem bater o ponto nela passa a ser o responsável. Um responsável escolhido precisa estar no projeto, isto é, ter valor por hora nele ou estar em algum time dele (time é opcional). Quem está logado também pode se pôr como responsável (o "atribuir a mim") sem estar no projeto, no `POST` e no `PATCH`. Fora isso, `task.assignee_not_in_team`. Sem `deadline`, o prazo fica em 7 dias a partir de agora.

```http
GET /api/projects/:projectId/tasks?q=frete&assignee_id=…&deadline_to=2026-10-12T02:59:59Z&page=1&per_page=10
```
Todos os parâmetros são opcionais:

- `q` busca no nome, sem diferenciar maiúsculas de minúsculas (acentos contam), com até 100 caracteres.
- `assignee_id` traz só as tarefas daquela pessoa; `none` traz só as sem responsável, `any`, só as que têm responsável, de qualquer pessoa, e `others`, só as que têm responsável e não é quem pede (o resultado muda com quem pergunta, e o total e as páginas já vêm sem as dela). É com `none` e `others` que a Lista de tarefas monta as duas listas (as sem responsável e as de outras pessoas; as de quem está logado ficam na aba Minhas tarefas), e com o id da pessoa logada, sem `page`, que a aba Minhas tarefas traz tudo o que é dela.
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
POST /api/tasks/:taskId/claim

PATCH /api/tasks/:taskId/attributes
Content-Type: application/json

{ "status": "in_progress", "priority": "high", "label_ids": ["…"], "assignee_id": "…", "deadline": "2026-11-20T23:59:00Z" }
```
O `claim` não tem corpo e não abre sessão: é só ficar com a tarefa para fazer depois. De dois pedidos juntos, um só leva, e o outro recebe 409. O `attributes` troca as etiquetas pela lista enviada (`[]` tira todas), e um valor que falta (ou `null`) fica como está; `assignee_id` vazio tira o responsável, e outra pessoa só entra se estiver em algum time do projeto (a própria pessoa e o responsável atual sempre podem); prioridade, status, etiqueta ou responsável inválidos dão 400 (`task.invalid_priority`, `task.invalid_status`, `task.label_other_project`, `task.assignee_not_in_team`, `task.invalid_assignee`). Ao contrário do `PATCH /api/tasks/:taskId`, que exige o nome e troca a descrição, ele é seguro para uma edição parcial.

```http
POST /api/tasks/:taskId/link-external-item
Content-Type: application/json

{
  "integration_id": "…",
  "external_item_id": "42",
  "external_item_url": "https://github.com/acme/app/issues/42"
}
```
A integração precisa ser do mesmo projeto da tarefa. O `external_item_id` é o número da issue (GitHub e GitLab) ou o cartão do Trello: o link curto ou o endereço dele (o endereço vira o link curto). A resposta é a tarefa, com `links`.

O `external-details` nunca falha por causa da plataforma. Se ela estiver fora, o token estiver errado ou ilegível, faltar um campo do `metadata`, o item não for do repositório ou do quadro configurado, ou a integração estiver desativada, a resposta é `200` com `{"details": null, "error": {"code": "integration.invalid_token", "params": {"provider": "GitHub"}}}`, o mesmo formato dos outros erros. Com sucesso, `details` traz `title`, `state` e `url`; num cartão do Trello, `state` é o nome da lista em que ele está, ou `arquivado`.

Na lista de tarefas e no detalhe, a tarefa traz `links`, um por item externo a que está ligada (do mais antigo para o mais novo; `[]` sem nenhum):

```json
"links": [
  {"integration_id": "…", "integration": {"id": "…", "type": "github", "name": "Repositório do app"}, "item_id": "42", "url": "https://github.com/acme/app/issues/42"},
  {"integration_id": "…", "integration": {"id": "…", "type": "trello", "name": "Quadro"}, "item_id": "H0TZyzbK", "url": "https://trello.com/c/H0TZyzbK", "last_error": "issue_sync.push_rejected"}
]
```
`last_error` (omitido quando vazio) é o aviso que a última sincronização deixou naquele item.

---

## Registro de tempo (ponto)

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/work-sessions/clock-in` | logado | Abre uma sessão: `{"task_id": "…"}` |
| POST | `/api/projects/:projectId/work-sessions/clock-out` | logado | Fecha a sessão aberta: `{}` |
| GET | `/api/projects/:projectId/work-sessions` | logado | Sessões do projeto. Filtros: `?task_id=` e `?person_id=`. O membro só recebe as próprias: sem `person_id` vale o dele, e o de outra pessoa dá 403 |
| GET | `/api/projects/:projectId/work-sessions/total` | logado | `{"total_seconds", "pay_amount_cents", "bill_amount_cents"}`. Exige `task_id`, `person_id` ou os dois. Para o membro vale o mesmo recorte das sessões |
| GET | `/api/work-sessions/active` | logado | A sua sessão aberta, com as tarefas e o projeto, ou `null` |
| POST | `/api/projects/:projectId/work-sessions/:sessionId/tasks` | quem bateu o ponto, ou admin | Põe uma tarefa do projeto na sessão, aberta ou encerrada: `{"task_id": "…"}`, e opcionalmente `from_at` e `until_at`. Responde `201` com a sessão |
| PATCH | `/api/projects/:projectId/work-sessions/:sessionId/tasks/:linkId` | quem bateu o ponto, ou admin | Muda o intervalo de uma tarefa da sessão: `from_at`, `until_at` (`null` é "até o fim da sessão") ou `{"stop": true}` para encerrar agora. Responde com a sessão |
| DELETE | `/api/projects/:projectId/work-sessions/:sessionId/tasks/:linkId` | quem bateu o ponto, ou admin | Tira a tarefa da sessão. Responde `200` com a sessão |

- Sem `person_id`, clock-in e clock-out valem para a pessoa logada. Só admins podem mandar o `person_id` de outra pessoa, e ela precisa ser da organização do projeto.
- O total só soma sessões de tarefas do projeto da rota. Um `task_id` de outro projeto dá `0`.
- Cada pessoa tem no máximo uma sessão aberta. O banco garante isso com o índice único parcial `one_active_session`, então nem duas requisições simultâneas conseguem abrir duas sessões.
- Uma sessão aberta conta no total até o momento da consulta.

### Tarefas da sessão

- **A sessão é de um projeto e tem tarefas.** O clock-in cria a sessão com a tarefa dele; as outras entram depois, com `POST .../tasks`. Todas precisam ser do projeto da rota (`work_session.task_other_project`).
- **Cada tarefa tem um intervalo** (`from_at`, `until_at`) dentro da sessão. `until_at` nulo é "até o fim da sessão", ou até agora se ela está aberta. Numa sessão aberta a tarefa entra agora (e fica em `in_progress`; sem responsável, passa a ser de quem bateu o ponto, como no clock-in); numa encerrada entra do início ao fim, e o status não muda. A mesma tarefa pode voltar, mas não sobreposta a si mesma (`work_session.task_overlap`). Fora da sessão, `work_session.invalid_interval`.
- **A sessão guarda ao menos uma tarefa** e, aberta, ao menos uma em andamento (sem `until_at`): quem quer parar de vez dá o clock-out (`work_session.last_task`).
- **O tempo.** `seconds` de cada tarefa é o do intervalo dentro da sessão. Tarefas em paralelo contam o tempo cheio cada uma, e a pessoa, o projeto e os valores contam a sessão uma vez só. `?task_id=` lista as sessões em que a tarefa esteve, cada uma com todas as tarefas, e o `total` por tarefa soma o tempo dela.
- **Quem mexe.** Quem bateu o ponto e os admins; outra pessoa recebe `403 work_session.not_yours`. Os valores de cada tarefa (`pay_amount_cents`, `bill_amount_cents`) seguem as regras dos valores da sessão.
- Excluir uma tarefa a tira das sessões, mas as sessões e as horas ficam.

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
  "project_id": "…",
  "person_id": "…",
  "start_at": "2026-10-05T09:00:00-03:00",
  "end_at": "2026-10-05T10:30:00-03:00",
  "pay_rate_cents": 2000,
  "pay_amount_cents": 3000,
  "bill_rate_cents": 10000,
  "bill_amount_cents": 15000,
  "tasks": [
    {
      "id": "…",
      "task_id": "…",
      "task": { "id": "…", "name": "Tela de checkout", "project_id": "…" },
      "from_at": "2026-10-05T09:00:00-03:00",
      "until_at": null,
      "seconds": 5400,
      "pay_amount_cents": 3000,
      "bill_amount_cents": 15000
    }
  ]
}
```

---

## Integrações

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/projects/:projectId/integrations` | `integrations.manage` | Cria e valida a conexão na plataforma |
| GET | `/api/projects/:projectId/integrations` | logado | Integrações do projeto |
| POST | `/api/projects/:projectId/sync` | logado | O botão **Sincronizar** das listas de tarefas e de Minhas tarefas: uma rodada completa em cada integração do projeto com a sincronização ligada, uma depois da outra. Mesma resposta da rodada, somada (`created`, `updated`, `closed`, `pushed`, `unmapped`, `errors`, `partial`; `partial` também quando uma integração não pôde rodar); `409` `integration.sync_running` se todas já estão numa rodada; `400` `integration.sync_off` se nenhuma tem a sincronização ligada, ou o erro da primeira quando nenhuma rodou; `404` entre organizações |
| GET | `/api/integrations/:integrationId` | logado | Detalhes |
| PATCH | `/api/integrations/:integrationId` | `integrations.manage` | Altera nome, token, `metadata`, `enabled` ou `sync_issues` |
| DELETE | `/api/integrations/:integrationId` | `integrations.manage` | Exclui. As tarefas vinculadas perdem o vínculo |
| GET | `/api/integrations/:integrationId/repositories` | `integrations.manage` | Lista o que o token guardado enxerga: `[{"full_name","private"}]` no GitHub (repositórios) e `[{"id","full_name","private"}]` no Trello (os quadros abertos, com o espaço de trabalho em `full_name`, e o `id` que vai em `metadata.board_id`). `400` `integration.no_repositories` nos tipos que não listam |
| POST | `/api/integrations/:integrationId/sync` | `integrations.manage` | Uma rodada completa da sincronização das issues com as tarefas, na hora. `200` `{"created","updated","closed","pushed","unmapped","errors","partial"}`; `409` `integration.sync_running` se já há uma rodada nesta integração; `400` `integration.sync_off` se a sincronização está desligada ou a integração desativada; `404` entre organizações |

**Conectar com o GitHub e com o Trello** são rotas de página (GET e redirecionamentos, fora do `/api`), que o navegador percorre; não há como chamá-las pela API:

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/projects/:projectId/management/integrations/github/connect` | `integrations.manage` | Grava o cookie `wtt_oauth` e responde `302` para o GitHub. `?integration=<id>` reconecta uma integração GitHub do mesmo projeto |
| GET | `/integrations/github/callback?code&state` | logado | O endereço cadastrado no app do GitHub. Confere cookie, `state`, pessoa e permissão; guarda a integração (desativada e sem repositório) e responde `303` para `/projects/:projectId/management/integrations?github=<id>`. Em erro, `?github_error=<código>` (`integration.github_oauth_state`, `_denied`, `_exchange`, `_not_configured`, `integration.not_found`); sem conexão em andamento, `303` para `/` |
| GET | `/projects/:projectId/management/integrations/trello/connect` | `integrations.manage` | Grava o cookie `wtt_oauth` (preso a `/integrations/trello`) e responde `302` para `{TRELLO_URL}/1/authorize` com `key`, `name`, `scope=read,write`, `expiration=never`, `response_type=token`, `callback_method=fragment` e o `return_url` (`PUBLIC_URL/integrations/trello/callback?state=<state>`). `?integration=<id>` reconecta uma integração Trello do mesmo projeto. Sem a chave do app, `303` para a aba com `?trello_error=integration.trello_oauth_not_configured` |
| GET | `/integrations/trello/callback?state` | logado | Onde o Trello devolve a pessoa, com o token no **fragmento** (`#token=…`), que o servidor não recebe. Responde a página de retorno (`Referrer-Policy: no-referrer`, `Cache-Control: no-store`), cujo script lê o token, apaga o endereço do histórico e o entrega à rota abaixo |
| POST | `/integrations/trello/token` | logado, `Content-Type: application/json` | Corpo `{"state","token"}`. Confere o cookie (preso à plataforma), o `state`, a pessoa e a permissão; guarda a integração (desativada e sem quadro, com a chave do app no `metadata`) ou, com `?integration` na ida, troca o token da que existe (validando com o quadro dela). Responde sempre `200` `{"redirect":"…"}`: a aba com `?trello=<id>` ou `?trello_error=<código>` (`integration.trello_oauth_state`, `_denied`, `invalid_token`, `trello_no_access_board`…), ou `/` quando não há conexão em andamento |

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
| `trello` | na tela, vem da autorização no Trello; a API também aceita o token da API do Trello, com leitura e escrita | `api_key`: a chave do app (`TRELLO_API_KEY`; a tela a guarda na conexão e uma edição não a troca); `board_id`: o endereço do quadro, o link curto ou o id; `board_name` (opcional): o nome do quadro, que a tela guarda ao escolher e que o cartão da integração mostra (trocar o quadro sem mandar outro nome o apaga) |

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
  "sync_issues": false,
  "last_synced_at": null,
  "last_sync_error": "",
  "sync_unmatched": 0,
  "created_at": "2026-10-06T09:00:00-03:00"
}
```

- O `gitlab` está "em breve": o `POST` dele responde `400` com `integration.type_coming_soon` (`provider` no `params`). O `PATCH` e o `GET` das que já existem seguem como antes.
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

**Sincronização das issues (GitHub) e dos cartões (Trello).** `sync_issues` liga e desliga pelo `PATCH`, e nasce `false`:

```http
PATCH /api/integrations/:integrationId
Content-Type: application/json

{ "sync_issues": true }
```

- Só liga no GitHub e no Trello (`400` `integration.sync_unsupported` nos outros), com o repositório (ou o quadro) já escolhido (`integration.sync_needs_repo`), a integração ativa (`integration.sync_needs_enabled`, vale para a mudança de desligada para ligada, e dá para mandar `enabled` e `sync_issues` juntos) e com a credencial guardada.
- Ligada, o campo que identifica a conexão (`metadata.repo` no GitHub, `metadata.board_id` no Trello) **não troca**: `400` `integration.sync_repo_locked`. Desligar e trocar na mesma chamada vale, e trocar o repositório com ela desligada esquece o vínculo das issues do antigo (e os vínculos feitos à mão). Trocar só o token com ela ligada vale.
- As respostas trazem `sync_issues`, `last_synced_at` (o fim da última rodada, nulo antes da primeira), `last_sync_error` (o código do último aviso ou erro: `issue_sync.read_only`, `issue_sync.push_discarded`, `issue_sync.push_rejected`, `issue_sync.no_login`, `issue_sync.label_refused`, `integration.rate_limited`, `integration.invalid_token`...; vazio se a última rodada não achou nada) e `sync_unmatched` (quantas issues abertas têm responsável no GitHub sem correspondência aqui; só é calculado com a sincronização ligada).
- `POST .../sync` leva a rodada à hora, sem esperar o intervalo de `SYNC_INTERVAL`, e responde o que ela fez. `partial: true` é uma rodada que parou antes de olhar tudo (limite de requisições do GitHub, falha de rede): o que fez fica feito, e o resto fica para a seguinte.
- As tarefas importadas aparecem na lista como qualquer outra, com um item em `links` (`item_id` é o número da issue, ou o link curto do cartão no Trello, e `url` o endereço); a tarefa criada por uma issue nasce em `backlog`, sem prazo. A criada por um cartão nasce em `backlog` com o prazo da data de entrega do cartão (ou sem prazo) e `created_at` igual à data de criação do cartão.

---

## Idioma

A interface fala português do Brasil (padrão) e inglês. Estas duas rotas são do navegador, não da API JSON, e não precisam de sessão.

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/lang/:code?next=/caminho` | Grava o cookie `wtt_lang` (`pt-BR` ou `en`, por um ano) e responde **303** para `next`. Só caminhos do próprio site: qualquer outro vira `/`. Um código desconhecido não grava nada |
| GET | `/i18n/:idioma.js?v=<hash>` | `window.I18N = {lang, messages}`, os textos do idioma para o JavaScript. Com o `v` certo, o cache é de um ano (`immutable`); sem ele, o navegador revalida pelo `ETag`. **404** para idioma desconhecido |

Qual idioma uma página usa: o cookie `wtt_lang`; sem ele, o `Accept-Language`; sem nenhum dos dois, português. As respostas variam por `Cookie` e `Accept-Language`.

---

## Ajuda

A ajuda é uma página do navegador, não da API JSON, e não precisa de sessão: serve para mandar o link a quem ainda vai se cadastrar.

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/help` | A página de ajuda (HTML), no idioma da requisição. **200** com ou sem sessão; com sessão, traz a barra superior com o item Ajuda |
| GET | `/ajuda` | Responde **303** para `/help` |

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
9. **Bater o ponto:** `clock-in` com `{"task_id": "…"}`, `GET /api/work-sessions/active`, `POST .../work-sessions/:sessionId/tasks` com uma segunda tarefa (copie o `id` da sessão para `session_id`) e `clock-out`.
10. **Totais:** `GET /api/projects/:projectId/work-sessions/total?task_id=…`.
11. **Convidar alguém:** `POST /api/orgs/:orgId/invites`, depois `POST /api/auth/invites/:token/accept` numa sessão sem cookie (ou após `logout`).

## Variáveis de ambiente (Insomnia)

| Variável | Descrição |
|---|---|
| `base_url` | URL do servidor, por padrão `http://localhost:8080` |
| `org_id` | `organization_id` do `/api/auth/me` |
| `person_id` | `id` do `/api/auth/me` ou de outra pessoa |
| `session_id` | `id` de uma sessão, da resposta do clock-in |
| `session_task_id` | `id` de uma tarefa da sessão (`tasks[].id`) |
| `project_id` | `id` retornado ao criar o projeto |
| `team_id` | `id` retornado ao criar o time |
| `customer_id` | `id` retornado ao criar o cliente |
| `task_id` | `id` retornado ao criar a tarefa |
| `integration_id` | `id` retornado ao criar a integração |
| `invite_id` / `invite_token` | `id` e `token` retornados ao criar o convite |
