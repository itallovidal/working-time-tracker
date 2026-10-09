# Permissões

O que cada pessoa pode fazer além do que todo mundo já faz. A fonte é
`internal/domain/permission/catalog.go`; a tela lê a mesma lista em `GET /api/permissions`.

## Quem tem o quê

| Quem | O que pode |
|---|---|
| **Dono** (`is_owner`) | Tudo, e o que é só dele: mudar papéis, convidar admins, definir as permissões da organização e excluir a organização |
| **Admin** (`role: admin`) | Tudo, em todos os projetos, menos o que é só do dono |
| **Membro** | Ver e editar tarefas, bater o próprio ponto, ver o que o projeto mostra a todos, e **só o que foi liberado a ele** |

O que se libera tem dois escopos.

- **Do projeto**: vale num projeto só e fica na alocação da pessoa nele (`allocations.permissions`, com o nome do grupo em
  `allocations.preset`). Ser gerente de um projeto não dá nada no outro. Tirar a pessoa do projeto tira as permissões dela nele.
- **Da organização**: vale em todos os projetos e fica na pessoa (`persons.permissions`). Só o dono libera, e só a quem não é admin.

## Permissões do projeto

| Chave | Libera |
|---|---|
| `project.edit` | Alterar e excluir o projeto (nome, descrição, sprint, daily, weekly e a reunião com o cliente) e abrir a aba Configurações |
| `teams.manage` | Criar, editar e excluir times, e escolher quem está em cada um |
| `collaborators.manage` | Pôr e tirar pessoas do projeto e escolher o grupo delas |
| `rates.view` | Ver o valor pago aos colegas e as horas de todos |
| `rates.manage` | Definir o valor por hora de uma pessoa no projeto |
| `billing.view` | Ver o cliente, o valor cobrado e a Visão geral (receita, custo e margem) |
| `billing.manage` | Trocar o cliente e o valor cobrado |
| `labels.manage` | Criar, renomear e excluir as etiquetas do projeto |
| `integrations.manage` | Criar, editar e excluir as integrações do projeto |

## Permissões da organização

| Chave | Libera |
|---|---|
| `projects.create` | Criar projetos e abrir a aba Projetos da organização |
| `customers.manage` | Cadastrar, alterar e excluir clientes |
| `people.manage` | Convidar pessoas (como membro), revogar convites e definir a jornada semanal |

## Grupos do projeto

Ao pôr alguém no projeto escolhe-se um grupo, que dá de uma vez uma lista de permissões. Cada permissão avulsa
fica para depois: ao mexer numa, o grupo passa a `custom`.

| Grupo | `preset` | Permissões |
|---|---|---|
| Colaborador | `member` | nenhuma |
| Gerente de projeto | `manager` | `project.edit`, `teams.manage`, `collaborators.manage`, `rates.view`, `rates.manage`, `labels.manage`, `integrations.manage` |
| Financeiro | `finance` | `rates.view`, `rates.manage`, `billing.view`, `billing.manage` |
| Administrador do projeto | `admin` | todas as do projeto |

As telas oferecem só três: colaborador, gerente de projeto e administrador do projeto. O financeiro existe na API (`preset: "finance"`) e nos testes, mas está escondido das telas (`HIDDEN_PRESETS` em `project.js`). O administrador do projeto aparece como "tudo o que o gerente de projeto faz, e mais:" seguido só do que ele acrescenta (ver e definir o faturamento).

Regra contra escalada: quem dá um grupo precisa ter todas as permissões dele (`allocation.preset_above_yours`). O gerente põe
colaboradores e outros gerentes, mas não dá o grupo financeiro, que tem o faturamento que ele não vê. Admins podem dar qualquer um.

## O que cada rota pede

| Rota | Permissão |
|---|---|
| `PATCH`/`DELETE /projects/:id` | `project.edit` |
| `GET /projects/:id/overview` | `billing.view` |
| `GET`/`PUT /projects/:id/billing` | `billing.view` / `billing.manage` |
| `POST /projects/:id/teams`, `PATCH`/`DELETE /teams/:id`, `POST`/`DELETE /teams/:id/members` | `teams.manage` |
| `PUT /projects/:id/allocations/:personId` | `collaborators.manage` para pôr alguém ou trocar o grupo; `rates.manage` para o valor |
| `DELETE /projects/:id/collaborators/:personId` | `collaborators.manage` |
| `GET /projects/:id/allocations`, `GET /projects/:id/collaborators` | todos; o valor dos colegas só com `rates.view` ou `rates.manage` |
| `POST`/`PATCH`/`DELETE /projects/:id/work-sessions/:sessionId/tasks…` | só quem bateu o ponto e os admins: mudar as tarefas não muda o tempo nem os valores da sessão, então não há permissão própria |
| `GET /projects/:id/work-sessions`, `.../total` | todos; as sessões dos colegas só com alguma permissão do projeto, e o valor cobrado só com `billing.view` |
| `POST`/`PATCH`/`DELETE /projects/:id/labels` | `labels.manage` |
| `POST /projects/:id/integrations`, `PATCH`/`DELETE /integrations/:id` | `integrations.manage` |
| `POST /orgs/:id/projects` | `projects.create` |
| `*/customers` | `customers.manage` |
| `POST`/`GET /orgs/:id/invites`, `DELETE /invites/:id`, `PATCH /persons/:id/weekly-hours` | `people.manage` (convidar um admin é só do dono) |
| `PATCH /persons/:id/role`, `PATCH /persons/:id/permissions`, `DELETE /orgs/:id` | só o dono |
| `PATCH /orgs/:id` | admin |
| `GET /orgs/:id/me/overview`, `GET /orgs/:id/me/tasks` | qualquer membro da organização (cada um lê só as próprias horas e tarefas, nunca as de outra pessoa) |
| `GET /orgs/:id/overview`, `GET /orgs/:id/working-now` | admin (a segunda só diz quem está com o ponto aberto e em que tarefas, sem tempo nem dinheiro) |

Sem a permissão a API responde `403 auth.permission_required` (ou `auth.owner_only`, ou `auth.admin_only`), e a página, `404`.

## Onde se escolhe

- **Grupo do projeto**: no passo 2 de Adicionar pessoa e no modal Editar colaborador (aba Colaboradores da Gestão).
- **Permissões da organização**: no modal Jornada e permissões, na aba Colaboradores da organização, só para o dono e só para quem não é admin.

## Ainda não existe

- Editar as permissões avulsas de alguém já no projeto (os grupos mudam todas de uma vez).
- Editar as horas já batidas (o começo e o fim da sessão): não há rota, e a permissão entra junto com ela. As tarefas da sessão já se editam, porque só dividem o tempo.
- Trocar o dono.
