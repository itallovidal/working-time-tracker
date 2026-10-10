# Permissões

O que cada pessoa pode fazer além do que todo mundo já faz. A fonte é `internal/domain/permission/catalog.go`; a tela
lê a mesma lista em `GET /api/permissions`.

## Princípios

1. **Dois eixos.** Gerir gente e projeto é uma hierarquia; o dinheiro (custo e cobrança) é outro eixo e não segue o
   cargo: hoje o dono concentra a cobrança, os clientes e os pagamentos, e o **Financeiro** entra depois, junto com o
   custo de infraestrutura.
2. **Time é só organização visual.** Serve para achar gente num projeto grande. Não dá autoridade, e por isso não
   existe "líder de time".
3. **Ninguém trabalha de graça, e todo cliente paga.** O custo e o valor cobrado têm o piso de 10,00 por hora (só o dono
   entra com custo 0), e um projeto ou tem cliente com valor cobrado ou é interno, sem os dois.
4. **Cada cargo só convida e promove quem está abaixo dele** (`allocation.preset_above_yours`).

## Os cargos

| Cargo | Escopo | Em uma frase |
|---|---|---|
| **Dono** (`is_owner`) | Organização | Tudo, inclusive todo o dinheiro. Um só por organização |
| **Admin** (`role: admin`) | Organização | Cria projetos internos, convida pessoas e administra todos os projetos. Sem cliente, cobrança nem pagamentos |
| **Administrador de projeto** (grupo `manager`) | Um projeto | Administra o projeto que lhe deram: convida, põe gente, define custo, times, integrações |
| **Colaborador** | Projetos em que está | Tarefas, ponto e o que é dele |

O admin e o administrador de projeto são cargos de gestão; "admin" continua valendo para *quem vê todos os projetos*
(`Identity.IsAdmin()`), mas não para *quem pode tudo*: o que o cargo dá fica em um lugar só,
`permission.OrgKeysFor` (organização) e `auth.BaseProjectSet` (projeto).

### Tabela

✔ pode · — não pode · (seu) só no projeto que lhe deram

| Capacidade | Dono | Admin | Administrador de projeto | Colaborador |
|---|---|---|---|---|
| Ver as informações gerais da organização | ✔ | ✔ | ✔ | ✔ |
| Editar os dados da organização | ✔ | — | — | — |
| Editar papéis (inclui convidar um Admin) | ✔ | — | — | — |
| Excluir a organização | ✔ | — | — | — |
| Cadastrar e alterar clientes | ✔ | — | — | — |
| Criar projeto interno | ✔ | ✔ | — | — |
| Criar projeto com cliente | ✔ | — | — | — |
| Definir cliente e valor cobrado | ✔ | — | — | — |
| Ver cobrança, receita e margem | ✔ | — | — | — |
| Ver as pessoas: nome, e-mail e papel | ✔ todas | ✔ todas | ✔ todas (para achar quem convidar) | quem está nos projetos dele |
| Ver a lista de permissões de alguém | ✔ | ✔ | — | só a sua |
| Convidar pessoas | ✔ | ✔ | ✔ (seu) | — |
| Pôr e tirar gente do projeto | ✔ | ✔ | ✔ (seu) | — |
| Ver o custo de quem está no projeto | ✔ | ✔ | ✔ (seu) | só o seu |
| Definir o custo (obrigatório para pôr alguém) | ✔ | ✔ | ✔ (seu) | — |
| Editar projeto, integrações, times e etiquetas | ✔ | ✔ | ✔ (seu) | — |
| Excluir projeto | ✔ | ✔ | — | — |
| Ver as horas da equipe | ✔ | ✔ | ✔ (seu) | só as suas |
| Ver a jornada semanal | ✔ | ✔ | ✔ (seu) | só a sua |
| Definir a jornada semanal | ✔ | ✔ | — | — |
| Ver os pagamentos de uma pessoa | ✔ todos | só as horas | — | só os seus |
| Página de pagamentos da equipe | ✔ | — | — | — |
| Definir a regra de pagamento | ✔ | — | — | — |
| Tarefas, ponto, ver o próprio valor | ✔ | ✔ | ✔ | ✔ |

A jornada é da pessoa (vale na organização inteira); o que varia por projeto é o custo. Por isso o administrador de
projeto vê a jornada e não a define: definir vai além do projeto dele.

## Dois escopos

- **Do projeto**: vale num projeto só e fica na alocação da pessoa nele (`allocations.permissions`, com o nome do grupo
  em `allocations.preset`). Ser administrador de um projeto não dá nada no outro. Tirar a pessoa do projeto tira as
  permissões dela nele. O dono tem todas em todo projeto; o admin tem as de `AdminProjectKeys` (tudo, menos
  `billing.*`) em todo projeto, mais as do grupo da alocação dele, se tiver uma (por exemplo o dono pode dar o
  financeiro a um admin num projeto).
- **Da organização**: vale em todos os projetos e **decorre do cargo** (`OrgKeysFor`): nada se grava na pessoa, e não há
  rota para dar uma a uma.

## Quem vê quais projetos

O dono e os admins veem todos os projetos. Um membro **só vê aqueles em que está**: onde tem valor por hora (a mesma conta
dos colaboradores e do número de pessoas do cartão do projeto; `internal/domain/projectaccess`). Estar no projeto é ter o
custo definido: ninguém entra num time sem o valor. Quem convida alguém para a organização sem projeto não lhe dá
projeto nenhum; o convite com projeto a põe nele ao ser aceito.

- A lista `GET /orgs/:id/projects` (e a primeira tela) traz só os projetos da pessoa.
- Toda rota e toda página de um projeto, de uma tarefa, de um time ou de uma integração responde **404** a quem não está
  no projeto, a mesma resposta de um recurso que não existe (a diferença de 403 é de propósito: não revelar que ele
  existe). Isso vem antes de qualquer permissão do projeto, e vale para quem foi tirado dele na hora.
- Criar projeto é do dono e dos admins (`POST /orgs/:id/projects` responde `403` a um membro). Só o dono é matriculado no
  projeto que cria, com valor zero; o admin que não é dono vê todos os projetos sem entrar em nenhum. Com `customer_id`
  ou `bill_rate_cents`, só o dono (`403 auth.owner_only` para o admin).
- O painel da primeira tela (`/me/overview`, `/me/tasks`) só conta as tarefas de projetos em que a pessoa está; as horas
  dela são sempre todas.
- Tirar a pessoa do projeto encerra o ponto que ela tinha aberto nele, porque fora do projeto ela não alcança mais a
  rota de parar.

### Quem vê quem

A lista `GET /orgs/:id/persons` e `GET /persons/:id` trazem nome, e-mail e papel, e valem para:

| Quem | Vê |
|---|---|
| Dono, admin e quem tem `collaborators.manage` em algum projeto em que está | Todas as pessoas da organização (cuidam de pessoas ou precisam achar quem entra num projeto) |
| Os outros | Só a si mesmos e quem está em algum projeto deles; as demais respondem `404` pelo id e não vêm na lista |

Dentro do que vê, cada um recebe só parte do cadastro de cada pessoa:

| Campo | Quem vê | Os outros recebem |
|---|---|---|
| Papel e e-mail | todos | |
| `weekly_hours` (a jornada) | a própria pessoa, o dono, os admins e quem tem `collaborators.manage` num projeto em que a pessoa está | `null` |
| `payment` (a regra de pagamento) | a própria pessoa e o dono | `null` |

Na lista de valores do projeto (`GET /projects/:id/allocations`), o grupo (`preset`) de cada pessoa vai para quem vê a
lista, mas o array `permissions` só vai para a própria pessoa, o dono e os admins (os outros recebem `[]`). A identidade
(`GET /auth/me`) e o cadastro da pessoa não trazem lista de permissões da organização.

## Permissões do projeto

| Chave | Libera |
|---|---|
| `project.edit` | Alterar o projeto (nome, descrição, sprint, daily, weekly e a reunião com o cliente) e abrir a aba Configurações |
| `project.delete` | Excluir o projeto, com as horas e os valores dele. Não está no grupo administrador de projeto: só o dono, os admins e o grupo `admin` |
| `teams.manage` | Criar, editar e excluir times, e escolher quem está em cada um |
| `collaborators.manage` | Pôr e tirar pessoas do projeto, convidar para ele e escolher o grupo delas |
| `rates.view` | Ver o valor pago aos colegas e as horas de todos |
| `rates.manage` | Definir o valor por hora de uma pessoa no projeto (mínimo 10,00; só o dono entra com 0) |
| `billing.view` | Ver o cliente, o valor cobrado, a receita e a margem na Visão geral |
| `billing.manage` | Trocar o cliente e o valor cobrado (andam juntos: com cliente o valor é obrigatório, mínimo 10,00; sem cliente não há valor) |
| `labels.manage` | Criar, renomear e excluir as etiquetas do projeto |
| `integrations.manage` | Criar, editar e excluir as integrações do projeto |

## Permissões da organização

Decorrem do cargo (`permission.OrgKeysFor`), e a página só mostra o que o cargo abre.

| Chave | Dono | Admin | Libera |
|---|---|---|---|
| `projects.create` | ✔ | ✔ | Criar projetos (o admin, só interno) |
| `people.manage` | ✔ | ✔ | Convidar pessoas (como membro), revogar convites e definir a jornada semanal (a página Colaboradores) |
| `customers.manage` | ✔ | — | Cadastrar, alterar e excluir clientes (a página Clientes) |
| `payments.manage` | ✔ | — | A página Pagamentos da equipe e a regra de pagamento de cada pessoa |

## Grupos do projeto

Ao pôr alguém no projeto escolhe-se um grupo, que dá de uma vez uma lista de permissões. Cada permissão avulsa fica para
depois: ao mexer numa, o grupo passa a `custom`.

| Grupo | `preset` | Permissões |
|---|---|---|
| Colaborador | `member` | nenhuma |
| Administrador de projeto | `manager` | `project.edit`, `teams.manage`, `collaborators.manage`, `rates.view`, `rates.manage`, `labels.manage`, `integrations.manage` |
| Financeiro | `finance` | `rates.view`, `rates.manage`, `billing.view`, `billing.manage` |
| Administração completa | `admin` | todas as do projeto (inclusive `project.delete` e `billing.*`) |

As telas oferecem só dois: Colaborador e Administrador de projeto. O financeiro e a administração completa existem na
API (`preset: "finance"` e `"admin"`) e no seed, mas estão escondidos das telas (`HIDDEN_PRESETS` em `project.js`), até
o Financeiro existir.

Regra contra escalada: quem dá um grupo precisa ter todas as permissões dele (`allocation.preset_above_yours`). O
administrador de projeto põe colaboradores e outros administradores de projeto, mas não dá o financeiro, que tem o
faturamento que ele não vê. O dono dá qualquer um; o admin, só os que cabem nas permissões dele (sem `billing.*`).

## O que cada rota pede

| Rota | Permissão |
|---|---|
| `PATCH /projects/:id` | `project.edit` |
| `DELETE /projects/:id` | `project.delete` |
| `GET /projects/:id/overview` | qualquer permissão do projeto; o cliente, o valor cobrado, a receita e a margem só vão com `billing.view` |
| `GET`/`PUT /projects/:id/billing` | `billing.view` / `billing.manage` |
| `POST /projects/:id/teams`, `PATCH`/`DELETE /teams/:id`, `POST`/`DELETE /teams/:id/members` | `teams.manage` |
| `PUT /projects/:id/allocations/:personId` | `collaborators.manage` para pôr alguém ou trocar o grupo; `rates.manage` para o valor |
| `DELETE /projects/:id/collaborators/:personId` | `collaborators.manage` |
| `POST /projects/:id/invites`, `GET /projects/:id/invites` | `collaborators.manage` (e `rates.manage` para convidar com valor): o administrador de projeto convida para o projeto dele |
| `GET /projects/:id/allocations`, `GET /projects/:id/collaborators` | todos; o valor dos colegas só com `rates.view` ou `rates.manage` |
| `POST`/`PATCH`/`DELETE /projects/:id/work-sessions/:sessionId/tasks…` | só quem bateu o ponto e os admins: mudar as tarefas não muda o tempo nem os valores da sessão, então não há permissão própria |
| `GET /projects/:id/work-sessions`, `.../total` | todos; as sessões dos colegas só com alguma permissão do projeto, e o valor cobrado só com `billing.view` |
| `POST`/`PATCH`/`DELETE /projects/:id/labels` | `labels.manage` |
| `POST /projects/:id/integrations`, `PATCH`/`DELETE /integrations/:id` | `integrations.manage` |
| `POST /orgs/:id/projects` | admin (o dono ou um admin); com cliente ou valor cobrado, só o dono |
| `*/customers` | `customers.manage` (só o dono) |
| `POST`/`GET /orgs/:id/invites`, `DELETE /invites/:id`, `PATCH /persons/:id/weekly-hours` | `people.manage` (convidar um admin é só do dono) |
| `PATCH /persons/:id/payment` | `payments.manage` (só o dono) |
| `PATCH /persons/:id/role`, `PATCH /orgs/:id`, `DELETE /orgs/:id` | só o dono |
| `GET /orgs/:id/me/overview`, `GET /orgs/:id/me/tasks` | qualquer membro da organização (cada um lê só as próprias horas e tarefas, nunca as de outra pessoa) |
| `GET /persons/:id/payments` | a própria pessoa e o dono; o admin recebe só as horas e os projetos de outra pessoa, sem regra, períodos nem valores; `403` para os outros |
| `GET /orgs/:id/payments` | `payments.manage` (só o dono): o que pagar e as horas de cada pessoa |
| Página `/orgs/:id/payments` | só o dono (`404` para os outros) |
| Página `/orgs/:id/settings` | só o dono (`404` para os outros); a página `/orgs/:id/about` é de todos |
| Página `/orgs/:id/customers` | só o dono |
| Página `/orgs/:id/people` | `people.manage` (o dono e os admins) |
| Página `/orgs/:id/people/:personId` (o perfil de um colaborador) | admin (`404` para os outros); sem pagamentos nem receita para quem não é o dono |
| Página `/orgs/:id/projects` | qualquer membro (cada um vê só os projetos dele; o Novo projeto pede admin, e só o dono vê o cliente e o valor) |
| `GET /orgs/:id/overview` | admin; a receita e a margem só vão para o dono |
| `GET /orgs/:id/working-now` | admin (só diz quem está com o ponto aberto e em que tarefas, sem tempo nem dinheiro) |

Sem a permissão a API responde `403 auth.permission_required` (ou `auth.owner_only`, ou `auth.admin_only`), e a página, `404`.
O teste `TestRoles_Matrix` confere cada célula dessa tabela com os quatro cargos.

## Onde se escolhe

- **Grupo do projeto**: no passo 2 de Adicionar pessoa e no modal Editar colaborador (aba Colaboradores da Gestão).
- **Papel (admin ou membro)**: pelo botão da linha na página Colaboradores, só para o dono.
- **Jornada e regra de pagamento**: no modal Editar colaborador (abre pelo lápis da página Colaboradores ou pelo Editar do
  perfil da pessoa): a jornada para o dono e os admins, a regra de pagamento só para o dono.
- **Valor por hora em cada projeto**: no mesmo modal, na aba Projetos e valores, para os admins. Ela usa as rotas do
  projeto (`PUT /projects/:id/allocations/:personId` e `DELETE /projects/:id/collaborators/:personId`) e
  `GET /persons/:id/allocations`, que continuam conferindo a permissão.

## Ainda não existe

- O **Financeiro**: custo de infraestrutura, contas a receber e o dinheiro da organização inteira, sem gerir gente nem
  projeto. Hoje o dono concentra o dinheiro, e os grupos `finance` e `admin` do projeto ficam guardados, escondidos.
- Editar as permissões avulsas de alguém já no projeto (os grupos mudam todas de uma vez).
- Editar as horas já batidas (o começo e o fim da sessão): não há rota, e a permissão entra junto com ela. As tarefas da
  sessão já se editam, porque só dividem o tempo.
- Trocar o dono.
