# Rotas da API — Working Time Tracker

**Base URL:** `http://localhost:1243`

Todas as rotas falam JSON. Erros sempre vêm como `{"error": {"code": "dominio.motivo", "params": {...}}}`: a API **não manda mensagem**, manda um código estável (e `params`, quando o texto precisa de valores), e quem chama mostra o texto no idioma da pessoa. A lista completa de códigos, com o status e os textos em português e inglês, está em [`errors.md`](errors.md).

## Como a autenticação funciona

- O login e o signup devolvem um cookie `wtt_session` (HttpOnly, SameSite=Lax, válido por 7 dias). Clientes HTTP como o Insomnia guardam o cookie sozinhos: faça o login uma vez e as próximas chamadas já vão autenticadas.
- Tudo fora de `/api/auth/*` e `/healthcheck` exige a sessão. Sem ela, a resposta é **401**.
- Cada pessoa pertence a uma organização. Um recurso de outra organização responde **404**, como se não existisse.
- Quem não é admin só alcança os projetos em que está (os em que tem valor por hora): a rota de um projeto, de uma tarefa, de um time ou de uma integração de um projeto em que a pessoa não está também responde **404**, antes de qualquer permissão. Os admins alcançam todos. Veja [`permissions.md`](../product/permissions.md).
- Rotas marcadas como **admin** respondem **403** para membros. As marcadas com uma permissão (`project.edit`, `billing.view`…) ou **dono** respondem **403** a quem não a tem. Os cargos são quatro (dono, admin, administrador de projeto e colaborador): o dono tem todas, o admin tem as de projeto menos a cobrança (`billing.*`) e as da organização de criar projeto e cuidar de pessoas, e as do administrador de projeto vêm do grupo dele no projeto. A lista está em [`permissions.md`](../product/permissions.md).
- `POST`, `PUT` e `PATCH` precisam de `Content-Type: application/json`. Outro formato responde **415**, o que também protege contra CSRF.
- Signup, login e convites têm limite de tentativas por IP. Acima dele, a resposta é **429**.
- O servidor MCP (`/mcp`, para o Claude) fica fora da API e entra com um token pessoal em `Authorization: Bearer`, não
  com o cookie. Ver [`mcp.md`](mcp.md).

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


### Validação e erros de campo

Toda validação de formulário devolve o campo que errou: `{"error": {"code": "request.field_too_long", "params": {"field": "name", "max": 120}}}`, em que `field` é a chave JSON do corpo (a descrição de organização e de projeto chega como `long_description`, nome legado). Os códigos genéricos são `request.field_required`, `request.field_too_long` e `request.field_invalid`; os específicos antigos mantêm o significado e ganham `field`. O corpo é lido de forma estrita: chave desconhecida é `400 request.invalid_body`, tipo errado devolve o mesmo código com o `field`, e mais de 1 MiB é `413 request.body_too_large`. Nos `PATCH`, campo omitido mantém o valor (`description` de projeto e de tarefa) e `null` apaga onde faz sentido (`deadline` da tarefa). Tamanhos máximos: nome 120, nome de tarefa 255, e-mail 255, site e LinkedIn 255, resumo 160, descrição de organização e de projeto 2.000, descrição de tarefa 65.536, link do item 2.048, token 512, valor em dinheiro até 1.000.000,00. A lista completa dos códigos está em [`errors.md`](errors.md).

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
| GET | `/api/auth/me` | logado | Quem está logado, com a organização e o papel. `needs_onboarding` é verdadeiro para o dono que ainda não viu as boas-vindas |
| POST | `/api/auth/password` | logado | Troca a senha, encerra as suas outras sessões e revoga os seus tokens do Claude |
| POST | `/api/auth/onboarding/complete` | logado | Dá baixa nas boas-vindas do primeiro acesso de quem chama (`204`). Repetir não muda nada |
| GET | `/api/auth/invites/:token` | público | Mostra para qual organização e papel é o convite |
| POST | `/api/auth/invites/:token/accept` | público | Cria a conta pelo convite e abre a sessão |
| POST | `/api/auth/clerk/login` | público, token do Clerk | Entra pelo Clerk (ver abaixo) |
| POST | `/api/auth/clerk/signup` | público, token do Clerk | Cria uma organização para quem entrou pelo Clerk |
| POST | `/api/auth/clerk/join` | público, token do Clerk | Aceita um convite pendente feito para o email do Clerk |
| GET | `/api/auth/tokens` | logado | Os seus tokens do Claude (ver abaixo), do mais novo para o mais antigo, sem o token em si |
| POST | `/api/auth/tokens` | logado | Gera um token do Claude e o devolve em claro, uma vez só |
| DELETE | `/api/auth/tokens/:tokenId` | logado | Revoga um token seu (`204`). O de outra pessoa responde `404` |

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
POST /api/auth/clerk/signup          { "organization_name": "Minha Empresa", "country": "BR", "name": "Ana Souza" }   // name e country são opcionais
POST /api/auth/clerk/join            { "invite_id": "<uuid de um convite de no_account.invites>", "name": "…" }
```

`signup` e `join` respondem **201** com o mesmo corpo e o cookie. `signup`: 400 `auth.org_name_required`, 409 `auth.account_exists` (já há conta para esse usuário ou email). `join`: o convite precisa ter email e ser o email verificado do Clerk (convite sem email só entra pelo `invite_token` do `login`).

O `signup` do Clerk também aceita `"solo": true`, que cria a organização no modo solo, como o `signup` por senha (ver [Signup](#signup)).

### Signup
```http
POST /api/auth/signup
Content-Type: application/json

{
  "organization_name": "Minha Empresa",
  "country": "BR",
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
O email vira minúsculas e é único no sistema todo. A senha precisa ter entre 8 e 72 caracteres. `country` é o país da organização (`BR` ou `US`) e é opcional: vazio é `BR`. Ele define a moeda e o fuso de partida (`BRL` e `America/Sao_Paulo`, ou `USD` e `America/New_York`); um país desconhecido responde 400 `auth.invalid_country`. O mesmo vale para o `clerk/signup`.

`solo` (`true` ou `false`, opcional; o padrão é `false`) cria a organização já no **modo solo**, a de uma pessoa só (ver [Modo solo](#modo-solo)). Como a organização nasce com exatamente uma pessoa e nenhum convite, a regra de ligar sempre cabe. O signup, o login e o `GET /api/auth/me` devolvem `organization_solo` (booleano) junto com `organization_currency`; é o que as telas leem para esconder o que é de equipe.

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

### Boas-vindas do primeiro acesso
```http
POST /api/auth/onboarding/complete
```
Responde `204`, sem corpo. Só o dono vê as boas-vindas (o modal da página inicial); o `/api/auth/me` traz `needs_onboarding: true` enquanto ele não as concluiu ou dispensou, e a primeira chamada grava a data, as outras não mudam nada. Quem entrou por convite nunca as vê, e quem já existia antes do recurso não as vê.

### Tokens do Claude
```http
POST /api/auth/tokens
Content-Type: application/json

{ "name": "Claude Code no notebook", "days": 90 }
```
O token é o que o Claude Code manda em `Authorization: Bearer` para o servidor MCP em `/mcp`
([mcp.md](mcp.md)); ele **não abre a API**, que continua só com o cookie. `days` é 30, 90 (o padrão, quando não vem)
ou 365. Responde `201`:

```json
{
  "id": "…", "name": "Claude Code no notebook", "prefix": "wtt_Mt-Mne",
  "expires_at": "2027-02-05T15:59:00Z", "last_used_at": null, "created_at": "2026-11-07T15:59:00Z",
  "token": "wtt_…"
}
```

`token` só aparece aqui; a lista traz o resto, e `prefix` é o começo do token, para reconhecê-lo. Cada pessoa tem até
10 tokens válidos (`409` `auth.token_limit`; os vencidos não contam). `last_used_at` é gravado no máximo uma vez por
minuto. Trocar a senha revoga todos.

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
| PATCH | `/api/orgs/:orgId` | dono | Altera o nome e o perfil. Veja os campos abaixo |
| DELETE | `/api/orgs/:orgId` | dono | Exclui a organização com as pessoas, os clientes, os recebíveis e os convites. Falha se ainda houver projetos |
| GET | `/api/orgs/:orgId/persons` | logado | Pessoas da organização que quem pede pode ver: todas, para o dono, os admins e quem tem `collaborators.manage` em algum projeto em que está (precisa achar quem entra); para os outros, só ele mesmo e quem está em algum projeto dele |
| GET | `/api/persons/:personId/payments` | a própria pessoa, o dono ou admin (o admin recebe de outra pessoa só as horas: `rule: null`, sem períodos nem valores) | Pagamentos calculados da regra: `{generated_at, timezone, owner, rule, current, upcoming[], history[], totals, lifetime}`. Os períodos vão como dias de calendário `YYYY-MM-DD` (`start`, `pay_date`, inclusivos) no fuso da organização. `?history=N` (padrão 12, no máximo 60). Sem regra: `rule: null`, `current: null` e as listas vazias. `lifetime` vem sempre, com ou sem regra: `{seconds, amount_cents, first_day, projects[]}`, tudo o que a pessoa registrou desde a primeira sessão (cada sessão inteira, arredondada uma vez), com o dia da primeira (`first_day`, vazio para quem nunca bateu ponto) e o detalhe por projeto (nunca nulo). O dono não tem `amount_cents`. O corrente e cada período de `history[]` trazem `paid` (`{id, amount_cents, bonus_cents, paid_at}`) quando tiveram baixa; com a baixa do restante, os valores são a soma e a data e o `id` são os da última. É o que o perfil da pessoa mostra (`/profile` e, para admins, `/orgs/:orgId/people/:personId`) |
| GET | `/api/orgs/:orgId/payments` | `payments.manage` (dono) | A visão da equipe, sem o dono (que não é pago): `due[]`, os períodos sem baixa (o corrente de cada pessoa, `running`, e os fechados com data de pagamento nos últimos 60 dias e alguma hora, `overdue`) e o restante dos que já tiveram baixa (`rest`, com `seconds` e `amount_cents` só do que veio depois da última baixa e `paid_cents`, o que as baixas já pagaram pelas horas), com `person`, `rule`, `start`, `pay_date`, `days`, `days_left` (negativo no atraso), `seconds`, `amount_cents` e `last_closed` (o último fechado da pessoa, com `paid`), ordenados pela data do pagamento e pelo nome; `paid[]`, as baixas dos últimos 30 dias, da mais nova para a mais antiga, com `id`, `person`, `start`, `pay_date`, `paid_at`, `seconds` e `calculated_cents` (o que a baixa cobriu), `amount_cents` (o pago pelas horas), `bonus_cents`, `seq` (0 na primeira baixa do período, mais nas do restante) e `later_seconds`/`later_cents` (na última baixa do período, o restante que ele ganhou depois dela e que está em `due[]`); `without_rule[]` (sem o dono) e `totals` (`to_pay_cents` e `overdue_cents`, `closing_7_cents` e `closing_30_cents` com os atrasados, `seconds`, `owner_seconds`, `without_rule`) |
| POST | `/api/orgs/:orgId/payouts` | `payments.manage` (dono) | Dá baixa num período, ou no restante de um que já teve baixa: `{person_id, start, pay_date, amount_cents, bonus_cents}`. `bonus_cents` é opcional (sem ele, `0`) e não pode ser negativo (`400 payment.invalid_bonus`). O período tem de ser um da regra da pessoa (`400 payment.not_a_period`) e já ter começado (`400 payment.period_not_started`); o corrente vale, antes da data. `amount_cents` é o valor pago, inteiro e zero ou mais (`400 payment.invalid_amount`); as horas e o valor calculado são do servidor, no momento, e numa baixa do restante são só o que veio depois da última. `400 payment.owner_not_paid` para o dono, `400 payment.no_rule` para quem não tem regra, `404 person.not_found` para um `person_id` malformado ou de alguém de outra organização e `409 payment.already_paid` quando o período já tem baixa e nada foi registrado depois dela. `201` com a baixa |
| DELETE | `/api/orgs/:orgId/payouts/:payoutId` | `payments.manage` (dono) | Desfaz a baixa: o que ela cobria volta para `due[]`. `204`; `404 payment.payout_not_found` |
| GET | `/api/orgs/:orgId/overview` | admin | Visão geral da organização: o tempo e o custo de todos os projetos em três janelas, e as horas de cada pessoa; a receita e a margem só vão para o dono (`null` para o admin). Veja abaixo |
| GET | `/api/orgs/:orgId/solo/overview` | `receivables.manage` (dono), só no modo solo | A tela inicial do modo solo: horas, Receita, Receita projetada e o valor por hora efetivo, no total e por cliente, numa janela de calendário. `404` fora do modo solo. Veja abaixo |
| GET | `/api/orgs/:orgId/working-now` | admin | Quem está com o ponto aberto na organização e em que tarefas, sem tempo nem dinheiro. Veja abaixo |
| GET | `/api/orgs/:orgId/me/overview` | logado | O painel de quem pede: as horas dele (hoje, a semana, dia a dia e por projeto) e as tarefas dele, em todos os projetos. Veja abaixo |
| GET | `/api/orgs/:orgId/me/tasks` | logado | As tarefas de quem pede, em todos os projetos, abertas ou concluídas, em páginas. Veja abaixo |
| POST | `/api/orgs/:orgId/projects` | admin | Cria um projeto, já com o cliente e o acréscimo quando há cliente |
| GET | `/api/orgs/:orgId/projects` | logado | Projetos da organização, do mais novo para o mais antigo (para quem não é admin, só os em que está). Sem `page`, todos num array; com `page`, uma página. Veja abaixo |
| POST | `/api/orgs/:orgId/customers` | `customers.manage` (dono) | Cria um cliente |
| GET | `/api/orgs/:orgId/customers` | `customers.manage` (dono) | Clientes da organização, em ordem alfabética |
| GET, POST | `/api/orgs/:orgId/receivables` (+ `/:id`, `/:id/late-charges`, `/:id/receive`) | `receivables.manage` (dono), só no modo solo | As contas a receber: ver [Contas a receber](#contas-a-receber-modo-solo) |

A organização é criada pelo signup, e o `organization_id` vem no `/api/auth/me`, junto com `organization_currency` e `organization_solo`.

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

### Visão geral do modo solo

`GET /api/orgs/:orgId/solo/overview?window=month|last_month|year` é a tela inicial do [modo solo](../product/overview.md#modo-solo). É só do dono (`403 auth.permission_required` para os outros, antes do `404`) e só de organização solo (`404 request.not_found` fora dela, mesmo para o dono), como as rotas de [recebíveis](#contas-a-receber-modo-solo). `window` é `month` (padrão), `last_month` ou `year`; outro valor é `400 overview.invalid_window` (`params.field: window`).

```json
{
  "window": "month", "from": "2026-11-01", "to": "2026-11-30",
  "timezone": "America/Sao_Paulo", "currency": "BRL",
  "generated_at": "2026-11-18T12:00:00-03:00",
  "totals": {
    "seconds": 43200, "received_cents": 40000, "projected_cents": 565000,
    "effective_rate_cents": 47083, "below_min_rate": false
  },
  "customers": [
    { "customer": { "id": "…", "name": "Alfa" }, "kind": "hourly", "seconds": 23400,
      "received_cents": 40000, "projected_cents": 65000, "effective_rate_cents": 10000, "below_min_rate": false },
    { "customer": { "id": "…", "name": "Beta" }, "kind": "monthly", "seconds": 14400,
      "received_cents": 0, "projected_cents": 500000, "effective_rate_cents": 125000, "below_min_rate": false }
  ],
  "internal_seconds": 5400
}
```

- **Janelas.** São de calendário, no fuso da organização (`timezone`): `month` é o mês corrente, `last_month` o anterior e
  `year` vai de 1º de janeiro ao último dia do mês corrente (assim contém sempre o `month`). `from` e `to` são os dias que a
  janela cobre, `YYYY-MM-DD`, os dois inclusivos. Os valores são em centavos da moeda da organização (`currency`).
- **`totals.seconds`** é o tempo de todas as sessões na janela (as dos projetos internos também), cada uma cortada na borda;
  a aberta conta até `generated_at`. **`totals.received_cents`** é a **Receita**: a soma de `received_cents` dos recebíveis
  com `received_on` dentro da janela, pela data do recebimento. **`totals.projected_cents`** é a **Receita projetada**: dos
  períodos que **terminam** na janela (`period_end`), o `amount_cents` do recebível quando existe e a proposta quando não
  (no por hora, o período em andamento com as horas até agora; no mensal, a mensalidade). Receita e projetada têm datas
  diferentes e não se misturam: veja [money.md](../product/money.md#receita-e-receita-projetada-modo-solo).
- **`effective_rate_cents`** é `projected_cents` dividido pelas horas, ao centavo, e `null` sem horas. `below_min_rate` avisa
  que ficou abaixo de 10,00 por hora (`validate.MinRateCents`); é um aviso, nunca um erro.
- **`customers`** traz os clientes que têm horas, recebimento ou valor projetado na janela, **em ordem de nome** (sem caixa
  nem acento, o id desempata) e nunca por horas ou por valor. `kind` é o modo de cobrança de hoje (`hourly` ou `monthly`) e
  `seconds` são as horas das sessões dos projetos do cliente, também no mensal. Nunca é `null`: `[]` sem nada. As linhas, com
  `internal_seconds`, somam os `totals`.
- **`internal_seconds`** são as horas dos projetos sem cliente (internos) na janela, só tempo. `0` sem elas.

### Quem está trabalhando agora

`GET /api/orgs/:orgId/working-now` é só de admins (`403` para um membro, `404` para outra organização ou um id malformado, `401` sem sessão). Lê só as sessões abertas, então é leve o bastante para a tela repetir de 30 em 30 segundos: a bolinha da Lista de tarefas e a coluna Agora dos colaboradores leem dela. Devolve um array, vazio (nunca `null`) quando ninguém trabalha, com **uma linha por pessoa com o ponto aberto**, da sessão que começou primeiro para a última; quem não aparece não está trabalhando.

```json
[
  { "person_id": "…",
    "working_on": [
      { "task": { "id": "…", "name": "Relatório de repasses" }, "project": { "id": "…", "name": "Painel do Lojista" } }
    ] }
]
```

- `working_on` são as tarefas que a pessoa tem na sessão neste instante (os intervalos sem fim), na ordem em que entraram, cada uma com o projeto. Fica `[]` (nunca `null`) numa sessão que já ficou sem tarefa: a pessoa continua "trabalhando agora".
- É a mesma regra do `working_on` da visão geral da organização, sem somar o tempo de ninguém. Não há tempo de espera: quem esqueceu de bater a saída continua aparecendo até a sessão ser encerrada.

### O painel de quem está logado

As duas rotas são de qualquer membro da organização (`404` para outra organização ou um id malformado, `401` sem sessão) e respondem sempre sobre **quem pede**: não há parâmetro de pessoa, e nada de dinheiro nem de colegas no corpo.

`GET /api/orgs/:orgId/me/overview?tz=America/Sao_Paulo` — `tz` é o fuso (nome IANA) de quem olha, que decide onde o dia e a semana começam; vazio, `Local` ou desconhecido valem `UTC`, e `timezone` diz o que foi usado. A semana vai de segunda a domingo.

```json
{
  "generated_at": "2026-10-07T15:00:00Z",
  "timezone": "America/Sao_Paulo",
  "week_start": "2026-10-05T03:00:00Z",
  "weekly_hours": 40,
  "hours": {
    "today_seconds": 7200,
    "week_seconds": 19800,
    "days": [ { "date": "2026-10-05", "seconds": 7200 }, { "date": "2026-10-06", "seconds": 5400 }, "… até 2026-10-11" ],
    "projects": [ { "project": { "id": "…", "name": "App de Pedidos" }, "seconds": 14400 } ]
  },
  "tasks": { "total": 8, "open": 6, "backlog": 4, "in_progress": 1, "awaiting_closure": 1, "closed": 2, "overdue": 2, "due_this_week": 2 },
  "working_now": true,
  "working_on": [ { "task": { "id": "…", "name": "Login com telefone" }, "project": { "id": "…", "name": "App de Pedidos" } } ]
}
```

- `weekly_hours` é a jornada combinada, `null` quando ninguém informou. `days` tem sempre os sete dias (os que ainda não chegaram vêm com `0`), e `projects` só os projetos com tempo na semana, do que mais teve para o que menos (`[]`, nunca `null`). A sessão aberta conta até `generated_at`, e a que atravessa a meia-noite conta em cada dia o seu pedaço.
- `open` é toda tarefa que não está fechada; `overdue`, a aberta com prazo já vencido; `due_this_week`, a aberta que ainda vence até o domingo. A tarefa não guarda quando foi fechada, então `closed` é o total.
- `working_on` vem `[]` (nunca `null`) sem ponto aberto ou com a sessão sem tarefa.

`GET /api/orgs/:orgId/me/tasks?state=open&page=1&per_page=5` — `state` é `open` (padrão) ou `closed` (`400 overview.invalid_task_state` para outro valor). `page` e `per_page` seguem a lista de projetos: a partir de 1, o padrão é 10 por página, o teto 100 e uma página além da última devolve a última (`400 project.invalid_page` e `project.invalid_per_page` para valor inválido).

```json
{
  "items": [
    { "id": "…", "name": "Endpoint de cálculo de frete", "status": "in_progress", "priority": "urgent",
      "deadline": "2026-10-06T15:00:00Z", "project": { "id": "…", "name": "App de Pedidos" } }
  ],
  "total": 7, "page": 1, "per_page": 5
}
```

- As abertas saem com o prazo mais perto primeiro (as atrasadas à frente, as sem prazo por último), depois a prioridade mais alta e as mais novas; as concluídas, da mais nova para a mais antiga. `deadline` é `null` na tarefa sem prazo.

### Perfil da organização

O `GET` devolve todos os campos para qualquer membro. No `PATCH`, **campo que não vem no corpo fica como está**; texto vazio (`""`) ou número zero apaga o valor. `timezone` e `currency` são a exceção: nunca ficam sem valor, e vazios voltam para o padrão. `{"name": "…"}` sozinho continua renomeando.

| Campo | Regra |
|---|---|
| `name` | Obrigatório, até 120 caracteres |
| `summary` | Uma linha, até 160 caracteres |
| `description` | Texto puro, até 2000 caracteres |
| `website`, `linkedin_url` | Link `http` ou `https`. Sem esquema, vira `https://…` |
| `contact_email` | Email válido |
| `legal_name` | Razão social, até 200 caracteres |
| `country` | O código do país: `BR` (o padrão) ou `US`. Vazio volta para `BR`. Aceita ainda as grafias antigas ("Brasil", "EUA"), e o que fica guardado é o código. Decide o documento fiscal, os textos do endereço, e o fuso e a moeda padrão |
| `cnpj` | Documento do Brasil. Com ou sem máscara; os dígitos verificadores são conferidos, inclusive no formato alfanumérico. A resposta traz sem máscara |
| `ein` | Documento dos EUA, `XX-XXXXXXX`: 9 dígitos, e o prefixo `00` não existe. A resposta traz os 9 dígitos, sem máscara |
| `address_line1`, `address_line2` | Texto livre, até 200 caracteres |
| `work_mode` | Regime de trabalho: `remote`, `hybrid` ou `onsite` |
| `timezone` | Nome IANA, por exemplo `America/Sao_Paulo`. Vazio volta para o padrão do país (`America/Sao_Paulo` no BR, `America/New_York` nos US) |
| `currency` | `BRL`, `USD` ou `EUR`. Vazio volta para o padrão do país (`BRL` no BR, `USD` nos US) |
| `solo` | Booleano: liga ou desliga o modo solo (ver [Modo solo](#modo-solo)). Ligar pede exatamente uma pessoa e nenhum convite pendente (409 `organization.solo_needs_single_person`); desligar sempre vale. Mandar o valor que a organização já tem não muda nada. Tipo errado é 400 `request.invalid_body` |

Cada documento fiscal é conferido pela regra do país dono dele, qualquer que seja o país da organização, então o pedido `{"country":"US","ein":"…"}` não depende da ordem dos campos. **Trocar o país não apaga o documento do outro** (o CNPJ de uma organização que virou `US` continua guardado, e a tela o esconde) e também não muda a moeda nem o fuso. Os erros do país e dos documentos são `organization.invalid_country`, `invalid_cnpj` e `invalid_ein`.

### Modo solo

`solo` é o modo de uma pessoa só (freelancer, PJ). O `GET /api/orgs/:orgId` o devolve (`"solo": false` por padrão), o cadastro o cria ligado (`solo: true` no `signup`) e o `PATCH /api/orgs/:orgId`, que é do dono, o liga e desliga.

| Pedido | Resposta |
|---|---|
| `{"solo": true}` com exatamente uma pessoa e nenhum convite pendente | 200, com `solo: true` |
| `{"solo": true}` com mais de uma pessoa, ou com um convite pendente (um aceito ou vencido não conta) | **409** `organization.solo_needs_single_person`, com `params.field = "solo"`. A organização fica como estava |
| `{"solo": false}` | 200, sempre. Esconde, não apaga: os clientes mensais e os recebíveis ficam guardados |
| Qualquer `PATCH` por quem não é o dono | 403 `auth.owner_only` |

Com `solo = true`, convidar responde **409** `organization.solo_no_invites` (ver [Convites](#convites)), e as páginas de equipe respondem 404 (ver [Modo solo](../product/overview.md#modo-solo)). As rotas da API de pessoas e de pagamentos não mudam: só as telas e os convites.

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

Uma empresa dos EUA:
```http
PATCH /api/orgs/:orgId
Content-Type: application/json

{ "country": "US", "ein": "12-3456789", "address_line1": "500 Congress Ave", "address_line2": "Suite 200" }
```

### Convites

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| POST | `/api/orgs/:orgId/invites` | `people.manage` | Cria um convite: com email e o Clerk ligado, o Clerk manda o email; senão, só o link |
| GET | `/api/orgs/:orgId/invites` | `people.manage` | Convites ainda válidos |
| DELETE | `/api/invites/:inviteId` | `people.manage` | Revoga um convite |
| POST | `/api/projects/:projectId/invites` | `collaborators.manage` + `rates.manage` (projeto) | Convida para a organização alguém que já entra neste projeto, com o valor, o time e o grupo escolhidos |
| GET | `/api/projects/:projectId/invites` | `collaborators.manage` (projeto) | Convites pendentes que levam a pessoa a este projeto |

```http
POST /api/orgs/:orgId/invites
Content-Type: application/json

{ "email": "bruno@empresa.com", "role": "member" }
```
`email` é opcional, e `role` é `admin` ou `member` (o padrão é `member`). A resposta (201) traz `token` e `path` (`/invite/<token>`) e `delivery`, que diz como o convite chega: `email` (o Clerk manda o email), `terminal` (`INVITE_DELIVERY=terminal`: não mandou, o link está no log do servidor) ou `link` (sem email, ou sem o Clerk: só o link). **O token só aparece nesta resposta**: o banco guarda apenas o hash dele; `delivery` também só vem aqui.

Com o Clerk ligado e um email, o convite é criado no Clerk primeiro (com validade de 7 dias e `redirect_url` no `/invite/<token>`) e só então gravado: se o Clerk recusar, a resposta é **502** `auth.clerk_invite_failed`; se não responder, **502** `auth.clerk_unavailable`, e nada fica gravado. Um novo convite para o mesmo email na organização **substitui** o anterior (apagado aqui e cancelado no Clerk). `DELETE /api/invites/:inviteId` também cancela o convite no Clerk (um problema lá não impede de revogar).

**Modo solo.** Numa organização no modo solo (ver [Modo solo](#modo-solo)) os dois `POST` de convite, o da organização e o do projeto, respondem **409** `organization.solo_no_invites`, com qualquer corpo: a recusa vem antes de o corpo ser lido, e depois da checagem de quem pode convidar (sem a permissão continua o 403). Nada é gravado, nem no Clerk. `GET` da lista e `DELETE` continuam valendo (a lista de uma organização solo está sempre vazia). Desligado o modo, convidar volta a funcionar.

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
| DELETE | `/api/customers/:customerId` | Exclui. Responde `409` enquanto algum projeto apontar para o cliente (`customer.has_projects`) ou o cliente tiver recebíveis (`customer.has_receivables`) |

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
Só `name` é obrigatório. `country` é o país do cliente, o código ISO 3166-1 (`BR`, `US`, `DE`…): sem ele (ou vazio, na criação), é o país da organização, e na edição nunca fica vazio (`customer.invalid_country`). O `document` é o documento fiscal e se confere pela regra do país **do cliente**: `BR`, o CNPJ (com ou sem máscara, com os dígitos verificadores conferidos, inclusive no formato alfanumérico); `US`, o EIN (`XX-XXXXXXX`); qualquer outro país, texto livre de até 32 caracteres (letras, números, espaço, `.`, `-`, `/` e `_`), sem conferência. A resposta traz o documento sem máscara. Trocar o país de um cliente confere o documento que já está guardado pelo país novo: se não serve, o pedido é recusado (`customer.invalid_document`), e mandar o `document` (ou `""`) no mesmo pedido resolve.

```http
POST /api/orgs/:orgId/customers
Content-Type: application/json

{ "name": "Acme Inc", "country": "US", "document": "12-3456789" }
```

### Termos comerciais do cliente

Todo cliente traz o **modo de cobrança** e os termos dele, que o `POST` e o `PATCH` aceitam. A cobrança mora no cliente e vale para todos os projetos dele (Decisão 21): o projeto só soma um acréscimo. O cliente só com o valor por hora é por hora, fecha no dia 31 e vence no fechamento. Os valores são em **centavos**, na moeda da organização.

| Campo | Tipo | Regra |
|---|---|---|
| `billing_mode` | `"hourly"` (padrão) ou `"monthly"` | Um ou outro, nunca os dois. Outro valor, ou vazio, é `400 customer.invalid_billing_mode` |
| `hourly_rate_cents` | int | Só `hourly`, **obrigatório** nele (`400 request.field_required`, também com `null`): o valor por hora do cliente, o mesmo em todos os projetos dele. De 1000 (10,00) a 100000000: `400 customer.invalid_hourly_rate`. Mudar o valor vale para as sessões novas de todos os projetos do cliente. Só um cliente antigo, de antes da Decisão 21 e sem projetos, pode vir com `null` |
| `monthly_fee_cents` | int ou `null` | Só `monthly`, obrigatório nele (`400 request.field_required`) e de 1 a 100000000 (`400 customer.invalid_monthly_fee`) |
| `contract_start` | `YYYY-MM-DD` | Só `monthly`, obrigatório nele. Uma data que não existe é `400 request.field_invalid` |
| `contract_end` | `YYYY-MM-DD` ou `""` | Só `monthly`, opcional (sem fim, o contrato segue). Tem de ser **depois** do início: `400 customer.invalid_contract_end` |
| `closing_day` | int, padrão 31 | De 1 a 31: o dia em que o período fecha. 31 é o último dia do mês, e o mês curto fecha no último dia dele (`400 customer.invalid_closing_day`). Com recebíveis no cliente o dia não muda: outro valor é `409 customer.closing_day_locked` (o mesmo dia que ele já tem passa); desfeitos os recebíveis, muda |
| `payment_days` | int, padrão 0 | De 0 a 365: dias entre o fechamento e o vencimento (`400 customer.invalid_payment_days`) |
| `late_fee_bp` | int, padrão 0 | Só `monthly`. A multa em pontos-base (200 = 2%), de 0 a 10000 (`400 customer.invalid_late_fee`) |
| `late_interest_bp_month` | int, padrão 0 | Só `monthly`. Os juros ao mês em pontos-base, de 0 a 10000 (`400 customer.invalid_late_interest`) |
| `grace_days` | int, padrão 0 | Só `monthly`. Dias de tolerância depois do vencimento, de 0 a 365 (`400 customer.invalid_grace_days`) |

- **Um modo ou o outro.** Um campo do outro modo, com valor, é recusado com o campo em `params.field`: `400 customer.field_hourly_only` (`hourly_rate_cents` num cliente mensal) ou `400 customer.field_monthly_only` (a mensalidade, o contrato, a multa, os juros ou a tolerância num cliente por hora). `null`, `""` e `0` são o padrão do campo, não valor. Trocar o modo apaga os campos do modo que saiu.
- **`monthly` só no modo solo.** Numa organização que não é solo, `billing_mode: "monthly"` é `400 customer.monthly_needs_solo` (`params.field` = `billing_mode`), na criação e na troca. Desligar o modo solo esconde as telas, e o cliente mensal segue guardado e editável.
- **O modo trava com projetos.** Trocar o `billing_mode` de um cliente que já tem projetos é `409 customer.mode_locked` (`params.field` = `billing_mode`). Mandar o modo que ele já tem, e mexer nos outros termos, vale.
- **`PATCH`.** Campo omitido mantém o valor. `null` em `hourly_rate_cents` ou `monthly_fee_cents` é recusado no modo que os exige (`400 request.field_required`); em `contract_start` e `contract_end`, `""` apaga (o início de um cliente mensal não pode ficar vazio).
- Todo erro de termo leva `params.field` com a chave do corpo, para a tela marcar o campo. Chave desconhecida, ou do tipo errado (`"closing_day": "5"`), é `400 request.invalid_body`.

```http
PATCH /api/customers/:customerId
Content-Type: application/json

{
  "billing_mode": "monthly",
  "monthly_fee_cents": 630000,
  "contract_start": "2026-11-01",
  "contract_end": "",
  "closing_day": 5,
  "payment_days": 10,
  "late_fee_bp": 200,
  "late_interest_bp_month": 100,
  "grace_days": 3
}
```

---

## Pessoas

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/persons/:personId` | logado | Detalhes da pessoa, para quem pode vê-la (a mesma regra da lista); outra pessoa dá `404` |
| PATCH | `/api/persons/:personId` | a própria pessoa ou admin | Altera nome e email |
| PATCH | `/api/persons/:personId/role` | dono | Muda o papel: `{"role": "admin"}` ou `{"role": "member"}` |
| GET | `/api/permissions` | logado | O catálogo: `project` e `organization` (as chaves, na ordem das telas) e `presets` (os grupos do projeto, cada um com `id` e `permissions`) |
| PATCH | `/api/persons/:personId/weekly-hours` | `people.manage` | Define a jornada semanal: `{"weekly_hours": 40}` |
| PATCH | `/api/persons/:personId/payment` | `payments.manage` (dono) | Define a regra de pagamento: `{"frequency":"monthly","day":5}`, `{"frequency":"biweekly","start":"2026-10-01"}` ou `{"frequency":""}` para apagar. A regra só vai para a própria pessoa e para o dono (`payment: null` para os outros) |
| GET | `/api/persons/:personId/allocations` | a própria pessoa ou admin | Quanto a pessoa recebe por hora em cada projeto |

A pessoa e `/api/auth/me` não trazem lista de permissões da organização: elas decorrem do cargo (dono: todas; admin: criar projeto e cuidar de pessoas; membro: nenhuma), e a tela lê o que pode em `BOOT.can`. As permissões estão em [`permissions.md`](../product/permissions.md). A organização nunca fica sem admin: rebaixar o último admin responde `400`. A pessoa traz `is_owner`: o dono da organização é quem a criou (o signup), é um só por organização e é sempre admin, então rebaixá-lo responde `400 person.owner_is_admin`, mesmo com outros admins. Quem entra por convite nunca é o dono. Pessoas entram na organização pelo signup ou por convite.

A pessoa traz `weekly_hours`, a jornada semanal combinada com ela, em horas: vale para a organização toda, e não por projeto. Vai de 0 a 168 e vem `null` enquanto nenhum admin informou. Só um admin altera, e `0` ou `null` apagam a jornada (fora da faixa é `400 person.invalid_week_hours`); o `PATCH` de nome e email não mexe nela. Quem a vê: a própria pessoa, o dono, os admins, quem tem `people.manage` e quem tem `collaborators.manage` num projeto em que a pessoa está (o administrador de projeto); os colegas recebem `null`, na lista e no detalhe. A jornada própria vem em `/me/overview` e não muda. Na lista de valores do projeto (`GET /api/projects/:projectId/allocations`) o `permissions` de cada vínculo só vai para a própria pessoa e para os admins (os outros recebem `[]`), e o `preset` continua à vista.

---

## Projetos

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId` | logado | Detalhes do projeto |
| PATCH | `/api/projects/:projectId` | `project.edit` | Altera o projeto |
| DELETE | `/api/projects/:projectId` | `project.delete` | Exclui o projeto com times, tarefas, sessões e integrações |
| GET | `/api/projects/:projectId/overview` | qualquer permissão do projeto | O projeto em números: pessoas, times, horas, custo, receita, tempo de projeto, tarefas e integrações; o cliente, o valor cobrado, a receita e a margem só vêm com `billing.view`. Veja [Visão geral](#visão-geral) |
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
  "customer_meeting_time": "10:30",
  "customer_id": "<id do cliente, opcional>",
  "bill_extra_cents": 2000
}
```
- `customer_id` e `bill_extra_cents` andam juntos: um projeto **ou tem cliente ou é interno**. A cobrança é a do cliente (o valor por hora ou o contrato mensal, Decisão 21), e `bill_extra_cents` é o **acréscimo** opcional do projeto sobre ela: por hora com cliente por hora (o valor cobrado é o do cliente mais ele), por mês com cliente mensal (soma à mensalidade). Ausente é zero. Vai de 0 a 100000000 (`400 project.invalid_bill_extra`); sem cliente só vale zero (`400 project.bill_extra_needs_customer`). Esses erros levam `params.field` = `bill_extra_cents`. O `customer_id` de outra organização é `400 project.customer_not_found`, e um cliente por hora ainda sem valor por hora é `400 project.customer_without_rate`, os dois com `params.field` = `customer_id`. Mandar `customer_id` ou `bill_extra_cents` é só do dono (`403 auth.owner_only` para o admin). Só o dono e os admins criam projeto (os outros recebem `403`), e só o dono é matriculado no projeto que cria, com custo 0.
- `sprint_duration_days` vai de 1 a 90, e o padrão é 14. As telas oferecem 7, 14 e 30 (um mês); um projeto com outra duração continua com ela.
- `daily_time` usa o formato `HH:MM`. Sem ele, o projeto não tem daily.
- `weekly_sync_day` vai de `monday` a `sunday`. Sem ele, o projeto não tem weekly.
- `weekly_sync_time` usa o formato `HH:MM` e anda junto com o dia: horário sem `weekly_sync_day` é `400 project.weekly_time_without_day`, e dia sem horário é `400 request.field_required` com `params.field` = `weekly_sync_time`. A recusa do dia sem horário só vale quando o corpo mexe na weekly (manda o dia ou o horário): weeklies antigas, que ficaram só com o dia, seguem lendo e aceitam um `PATCH` que muda outro campo, e para consertar uma delas basta mandar o horário.
- `customer_meeting_day` e `customer_meeting_time` são a reunião semanal com o cliente, com as mesmas regras da weekly: o dia vai de `monday` a `sunday` (outro valor é `400 project.invalid_weekday`), o horário usa `HH:MM` (`400 project.invalid_customer_meeting_time`) e andam juntos: horário sem o dia é `400 project.customer_meeting_time_without_day` e dia sem horário é `400 request.field_required` com `params.field` = `customer_meeting_time` (com a mesma guarda para linhas antigas). Não dependem da weekly do time. Qualquer pessoa do projeto lê os dois campos; só admin altera. Tirar o cliente do projeto (`PUT /api/projects/:projectId/billing` com `customer_id` nulo) apaga a reunião. As telas só oferecem a reunião num projeto com cliente, e a API recusa o dia num projeto sem cliente (`400 project.meeting_needs_customer`).

No `PATCH`, um campo omitido mantém o valor atual, e `""` apaga `daily_time`, `weekly_sync_day`, `weekly_sync_time`, `customer_meeting_day` ou `customer_meeting_time`. Apagar o dia da weekly ou da reunião apaga o horário junto. O `name` também mantém quando não vem (ausente ou `null`), como na organização e no cliente; vazio ou só espaços é `400 project.name_required`. O projeto não tem jornada semanal: ela é da pessoa (`PATCH /api/persons/:personId/weekly-hours`).

O projeto traz `member_count`, os colaboradores dele (quem tem valor por hora nele, cada pessoa uma vez: a mesma conta de `/collaborators`), e `task_count`, as tarefas dele. Os dois vêm na lista e no detalhe.

O projeto traz `customer` (`{"id", "name"}` ou `null`) para qualquer membro. O valor cobrado nunca vem aqui: ele fica em `/billing`.

### Cliente e acréscimo

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/billing` | `billing.view` | O cliente, o valor cobrado por hora calculado (`bill_rate_cents`: o do cliente mais o acréscimo; `null` no interno e com cliente mensal), o acréscimo (`bill_extra_cents`), o modo de cobrança do cliente (`customer_billing_mode`: `hourly`, `monthly` ou `null` no projeto interno) e a cobrança dele (`customer_hourly_rate_cents`, `customer_monthly_fee_cents`) |
| PUT | `/api/projects/:projectId/billing` | `billing.manage` | Substitui o cliente e o acréscimo, que andam juntos |

```http
PUT /api/projects/:projectId/billing
Content-Type: application/json

{ "customer_id": "<id do cliente>", "bill_extra_cents": 2000 }
```
```json
{
  "customer": { "id": "…", "name": "Rede Bom Preço" },
  "bill_rate_cents": 14000,
  "bill_extra_cents": 2000,
  "customer_billing_mode": "hourly",
  "customer_hourly_rate_cents": 12000,
  "customer_monthly_fee_cents": null
}
```
Os valores são sempre em **centavos**, na moeda da organização: `10000` é 100,00. O valor por hora e o contrato são do cliente (Decisão 21): reajustá-los muda o valor cobrado de todos os projetos dele, nas sessões novas. O cliente precisa ser da mesma organização do projeto (`400 project.customer_not_found`), e o cliente por hora precisa ter valor por hora (`400 project.customer_without_rate`). O acréscimo, ausente ou `null`, é zero; vale no modo do cliente **novo**, também ao trocar o cliente do projeto (por hora ou por mês); vai de 0 a 100000000 (`400 project.invalid_bill_extra`); sem cliente só vale zero (`400 project.bill_extra_needs_customer`). Mandar `{"customer_id": null}` volta o projeto a ser interno.

Um projeto **com sessões** não sai de um cliente que já tem recebíveis, nem para outro cliente nem para interno: `409 project.customer_locked` (`params.field` é `customer_id`). A sessão não guarda o cliente, vale o do projeto de hoje, e trocar levaria as horas já cobradas para as propostas do cliente novo. Mandar o mesmo cliente, só com outro acréscimo, passa; o projeto sem sessões, ou de um cliente sem recebíveis, troca à vontade; desfeitos os recebíveis, o projeto sai. A checagem vem depois das de cliente e de acréscimo (`400`).

### Valor pago a cada pessoa

O vínculo de uma pessoa com o projeto e quanto ela recebe por hora nele. Há um valor por pessoa em cada projeto, e é ele que põe a pessoa no projeto: sem valor ela não entra em time nem bate ponto.

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/allocations` | logado | Quem vê o valor dos outros (`rates.view`, `rates.manage` e os admins) recebe todos; um membro recebe só o dele, ou `[]`. Cada vínculo traz `preset` e `permissions` |
| PUT | `/api/projects/:projectId/allocations/:personId` | `collaborators.manage` ou `rates.manage` | Põe a pessoa no projeto com o valor dela, ou troca o valor de quem já está: `{"pay_rate_cents": 2000}` |

`pay_rate_cents` vai de `1000` (10,00) a `100000000` e é obrigatório para quem entra no projeto: zero não vale, ninguém trabalha de graça (`400 allocation.invalid_rate`, com a mesma faixa no convite de projeto). A pessoa precisa ser da mesma organização do projeto. A exceção é o dono da organização: a checagem dele vem antes da do valor, e o valor gravado é sempre `0`, seja qual for o que vier.

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

O `DELETE` faz as duas remoções numa transação e responde `204`. As tarefas e as sessões de trabalho da pessoa ficam como estão, só que a sessão que ela tinha **aberta neste projeto** é encerrada na hora (fora dele ela já não o alcança, nem para parar o ponto); uma aberta em outro projeto não é tocada. Se ela não tinha valor nem time no projeto, a resposta é `404`.

Para **pôr** alguém no projeto, use o `PUT` de `/allocations/:personId` acima e, depois, se quiser, `POST /api/teams/:teamId/members`. Nessa ordem: o time recusa quem ainda não tem valor.

### Visão geral

Tudo o que a aba Visão geral mostra, numa resposta só. É só de admins, porque soma o que o projeto custou e rendeu: um membro recebe `403`.

| Método | Rota | Acesso | Descrição |
|---|---|---|---|
| GET | `/api/projects/:projectId/overview` | qualquer permissão do projeto (a cobrança, com `billing.view`) | Pessoas, times, horas, custo, receita e margem, tempo de projeto, tarefas, integrações e as horas de cada pessoa |

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
| POST | `/api/tasks/:taskId/link-external-item` | logado | Vincula a uma issue ou a um cartão, à mão. Uma tarefa tem **no máximo um item por integração** (a issue do GitHub e o cartão do Trello ao mesmo tempo): `409` `task.already_linked` (com `params.field` = `integration_id`) se ela já tem item nesta integração e `task.item_taken` (com `params.field` = `external_item_id`) se o item é de outra tarefa. O vínculo nasce pendente (`pending`) e a primeira rodada em que o item está aberto o adota |
| DELETE | `/api/tasks/:taskId/link-external-item?integration_id=` | logado | Desfaz o vínculo com aquela integração (obrigatório com mais de um item; com um só, pode faltar). O item que a sincronização já adotou vira item descartado (não volta como tarefa); o que nunca foi adotado some |
| GET | `/api/tasks/:taskId/external-details?integration_id=` | logado | Busca título e estado do item na plataforma (de uma integração; sem o parâmetro, o único item; `400` `task.link_fields_required` com mais de um) |
| POST | `/api/tasks/:taskId/sync` | logado | O botão **Sincronizar** da tela da tarefa: relê o item dela na plataforma e põe os dois em acordo, na hora. Com `?integration_id=` sincroniza só aquele item (é o que o botão de cada cartão faz); sem ele, todos os itens da tarefa, um depois do outro. Mesma resposta da rodada (`created`, `updated`, `closed`, `pushed`, `unmapped`, `errors`, `partial`) mais `problem` (o código do aviso, se o GitHub deixou algo de fora); `409` `integration.sync_running` se há uma rodada na integração; `400` `integration.sync_off` com a sincronização desligada e `task.no_external_item` sem item externo; `404` `integration.issue_gone` se a issue sumiu; uma tarefa ligada à mão a uma issue aberta passa a ser sincronizada |
| POST | `/api/tasks/:taskId/publish` | logado | O passo **Integrações** do modal Nova tarefa: posta a tarefa como uma issue nova do repositório da integração (`{"integration_id"}`) e a liga a ela. O nome é o título, a descrição é o corpo, as etiquetas são criadas no repositório se faltarem e o responsável só vai se o e-mail público de um usuário do GitHub for o dele; prazo e prioridade não vão. No **Trello** a mesma rota cria um cartão na primeira lista aberta do quadro (nome, descrição, etiquetas e prazo como data de entrega; `400` `integration.trello_no_list` se o quadro não tem lista aberta); a tela não a chama para o Trello, porque **toda tarefa criada** (`POST /api/projects/:projectId/tasks`) num projeto com o Trello ligado e a sincronização ligada é postada sozinha, em segundo plano, poucos segundos depois, a menos que a pessoa a tenha desmarcado (`skip_publish`) ou a tarefa já tenha um item desta integração (ligado à mão); a que o modal posta no GitHub também vira cartão. A rota serve para quem quer postar à mão uma tarefa que a postagem sozinha deixou de fora (o servidor reiniciou, o Trello estava fora do ar). `200` `{"task", "problem"}`; `409` `task.already_linked` se a tarefa já tem item **nesta integração** (nas outras ela pode ser postada: é como a mesma tarefa vai para o GitHub e para o Trello), `integration.sync_off` (a integração precisa estar ativa e com a sincronização ligada), `task.integration_other_project`, `issue_sync.publish_read_only` (token sem escrita) ou `integration.issues_disabled`; `404` entre organizações |

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

- `q` busca no nome, sem diferenciar maiúsculas de minúsculas (acentos contam), com até 100 caracteres: acima disso é `400 task.query_too_long` com `params.field` = `q`.
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
Os três campos são obrigatórios: o que falta é `400 task.link_fields_required` com `params.field` dizendo qual (`integration_id`, `external_item_id` ou `external_item_url`). A integração precisa ser do mesmo projeto da tarefa. O `external_item_id` é o número da issue (GitHub e GitLab) ou o cartão do Trello: o link curto ou o endereço dele (o endereço vira o link curto). A resposta é a tarefa, com `links`.

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
- **O valor é travado no clock-in.** A sessão guarda `pay_rate_cents` (o que a pessoa recebe por hora) e `bill_rate_cents` (o que o cliente paga: o valor por hora do cliente mais o acréscimo do projeto naquele momento; `null` em projeto sem valor cobrado: o interno e o de cliente mensal, onde o ponto funciona do mesmo jeito e as horas e o custo contam). Mudar um valor depois, inclusive reajustar o cliente, só afeta as sessões seguintes.
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

## Contas a receber (modo solo)

O A receber é do **modo solo** (`organizations.solo`) e do **dono** (`receivables.manage`). Em uma organização que não
trabalha sozinha as sete rotas respondem `404 request.not_found`, mesmo para o dono (a permissão vem antes: quem não é
o dono recebe `403 auth.permission_required`). Desligar o modo esconde as rotas e guarda os recebíveis. Os valores são
em centavos e os dias, `YYYY-MM-DD` no fuso da organização, os dois lados do período inclusivos. As regras estão em
[../product/money.md](../product/money.md#contas-a-receber-modo-solo).

| Método | Rota | Descrição |
|---|---|---|
| GET | `/api/orgs/:orgId/receivables` | A página inteira: `totals`, `to_confirm[]`, `open[]` e `received[]`. Veja abaixo |
| POST | `/api/orgs/:orgId/receivables` | Confirma um período de um cliente: `{customer_id, period_start, period_end, amount_cents, due_date, invoice_number, note}`. Só `amount_cents` (maior que zero, até 1.000.000,00) e os três primeiros são obrigatórios; sem `due_date` vale o fim do período mais os `payment_days` do cliente. `201` com o recebível; `404 customer.not_found` (cliente malformado, inexistente ou de outra organização); `400 receivable.not_a_period` (as datas não são um período do ciclo do cliente, ou, no mensal, ficam fora do contrato ou depois do período corrente); `409 receivable.period_open` (por hora, período que ainda não fechou); `409 receivable.already_confirmed`; `400 receivable.invalid_amount` (`field: amount_cents`); `request.field_required`, `request.field_invalid` e `request.field_too_long` com `field` (`period_start`, `period_end`, `due_date`, `invoice_number` até 60, `note` até 500) |
| PATCH | `/api/orgs/:orgId/receivables/:id` | Altera `amount_cents`, `due_date`, `invoice_number` e `note` (campo ausente mantém; texto vazio apaga a nota e o número da nota). `200` com o recebível; `409 receivable.already_received` se já foi recebido; `404 receivable.not_found`; os mesmos erros de campo da confirmação |
| DELETE | `/api/orgs/:orgId/receivables/:id` | Desfaz a confirmação: o período volta a ser uma proposta. `204`; `409 receivable.already_received` (desfaça o recebimento antes); `404 receivable.not_found` |
| GET | `/api/orgs/:orgId/receivables/:id/late-charges` | A sugestão de multa e juros para receber na data `?on=YYYY-MM-DD` (hoje, sem ela): `{on, due_date, grace_days, amount_cents, days_late, charged, late_fee_cents, interest_cents, total_cents}`. Só o contrato mensal tem multa e juros; no recebível por hora tudo vem zerado. `400 request.field_invalid` (`field: on`) para uma data que não existe |
| POST | `/api/orgs/:orgId/receivables/:id/receive` | Registra o recebimento: `{received_on, received_cents, late_fee_cents, interest_cents}`, tudo opcional. Sem `received_on` vale hoje; depois de hoje é `400 receivable.received_in_future`. **Por hora:** o valor recebido é o confirmado (outro valor é `400 receivable.received_must_match`) e multa e juros diferentes de zero são `400 receivable.charges_not_allowed`. **Mensal:** `received_cents` é livre (maior que zero, `400 receivable.invalid_amount`; sem ele vale o confirmado mais a multa e os juros), `late_fee_cents` e `interest_cents` valem zero ou mais (`400 receivable.invalid_charge`) e somados não passam de `received_cents` (`400 receivable.charges_exceed_received`). `200` com o recebível; `409 receivable.already_received`; `409 receivable.changed` se o valor confirmado foi editado depois de o recebimento lê-lo (duas abas): nada é gravado, e quem recebe confere o valor e tenta de novo |
| DELETE | `/api/orgs/:orgId/receivables/:id/receive` | Desfaz o recebimento: o recebível volta para `open[]` e a multa e os juros informados se vão com ele. `204`; `409 receivable.not_received`; `404 receivable.not_found` |

Os erros de campo levam `params.field` com a chave do corpo, para a tela mostrar o erro embaixo do campo. O corpo é JSON
estrito (`400 request.invalid_body` para chave desconhecida).

**O recebível** (`201`, `200` e os itens de `open[]` e `received[]`): `id`, `customer` (`{id, name}`), `kind`
(`hourly` ou `monthly`, o modo do cliente na confirmação), `period_start`, `period_end`, `seconds` e `calculated_cents`
(o que o sistema calculou na confirmação), `amount_cents` (o confirmado), `currency`, `due_date`, `invoice_number`,
`note`, `confirmed_at`, `received_on`, `received_cents`, `late_fee_cents`, `interest_cents` (os três do recebimento,
nulos ou zero antes dele) e os derivados `late` (confirmado, sem recebimento e com hoje depois do vencimento),
`days_late`, `effective_rate_cents` (o confirmado dividido pelas horas; nulo sem horas) e `below_min_rate` (abaixo de
10,00 por hora: um aviso, não um erro).

**`GET /receivables`** responde:

- `generated_at`, `timezone`, `currency` e `today` (o dia de hoje no fuso da organização);
- `totals`: `to_confirm_count` e `to_confirm_cents` (as propostas), `open_count` e `open_cents` (tudo o que foi
  confirmado e não entrou, atrasado incluído), `late_count` e `late_cents` (a parte vencida) e `received_month_count` e
  `received_month_cents` (o que entrou no mês corrente do calendário, a soma de `received_cents`);
- `to_confirm[]`: as propostas, do período que fecha primeiro para o que fecha depois (e por nome do cliente). Cada uma
  tem `customer`, `kind`, `period_start`, `period_end`, `seconds`, `calculated_cents`, `extra_cents` (no mensal, a parte
  de `calculated_cents` que vem dos acréscimos mensais dos projetos do cliente; zero no por hora), `due_date` (o padrão),
  `open` (o período ainda não fechou; só acontece no mensal), `effective_rate_cents` e `below_min_rate`;
- `open[]`: os recebíveis sem recebimento, do vencimento mais antigo para o mais novo;
- `received[]`: os recebidos, o mais recente primeiro, até 50.

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
8. **Cliente e acréscimo:** `POST /api/orgs/:orgId/customers` com o `hourly_rate_cents`, e `PUT /api/projects/:projectId/billing` com o cliente e, se quiser, `bill_extra_cents`.
9. **Bater o ponto:** `clock-in` com `{"task_id": "…"}`, `GET /api/work-sessions/active`, `POST .../work-sessions/:sessionId/tasks` com uma segunda tarefa (copie o `id` da sessão para `session_id`) e `clock-out`.
10. **Totais:** `GET /api/projects/:projectId/work-sessions/total?task_id=…`.
11. **Convidar alguém:** `POST /api/orgs/:orgId/invites`, depois `POST /api/auth/invites/:token/accept` numa sessão sem cookie (ou após `logout`).
12. **Modo solo:** numa organização de uma pessoa só (`signup` com `"solo": true`, ou `PATCH /api/orgs/:orgId` com `{"solo": true}`), crie um cliente mensal (`billing_mode: "monthly"`), `GET /api/orgs/:orgId/receivables` para ver as propostas, `POST` para confirmar um período e `POST .../receivables/:id/receive` para receber (copie o `id` do recebível para `receivable_id`).

## Variáveis de ambiente (Insomnia)

| Variável | Descrição |
|---|---|
| `base_url` | URL do servidor, por padrão `http://localhost:1243` |
| `org_id` | `organization_id` do `/api/auth/me` |
| `person_id` | `id` do `/api/auth/me` ou de outra pessoa |
| `session_id` | `id` de uma sessão, da resposta do clock-in |
| `session_task_id` | `id` de uma tarefa da sessão (`tasks[].id`) |
| `project_id` | `id` retornado ao criar o projeto |
| `team_id` | `id` retornado ao criar o time |
| `customer_id` | `id` retornado ao criar o cliente |
| `receivable_id` | `id` retornado ao confirmar um recebível |
| `task_id` | `id` retornado ao criar a tarefa |
| `integration_id` | `id` retornado ao criar a integração |
| `invite_id` / `invite_token` | `id` e `token` retornados ao criar o convite |
