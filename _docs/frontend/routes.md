# Rotas do navegador

As páginas da interface, quem pode abrir cada uma e as regras que valem para todas. A fonte é
`internal/routes/pages.go`. As rotas da API estão em [api/reference.md](../api/reference.md), e quem é dono, admin,
administrador de projeto ou colaborador está em [permissions.md](../product/permissions.md).

## Mapa

```mermaid
flowchart LR
    login["/login, /signup, /invite/:token"] --> home["/orgs/:orgId (Início)"]
    home --> projects["/orgs/:orgId/projects"]
    home --> about["/orgs/:orgId/about (Configurações)"]
    home --> people["/orgs/:orgId/people"]
    home --> customers["/orgs/:orgId/customers"]
    home --> payments["/orgs/:orgId/payments"]
    people --> person["/orgs/:orgId/people/:personId"]
    projects --> pov["/projects/:id/overview (Início do projeto)"]
    pov --> mine["/projects/:id/my-tasks"]
    pov --> tasks["/projects/:id/tasks"]
    pov --> collab["/projects/:id/collaborators"]
    tasks --> task["/tasks/:taskId"]
    pov --> mgmt["/projects/:id/management/*"]
    mgmt --> mov["overview"]
    mgmt --> mteams["teams"]
    mgmt --> minteg["integrations"]
    mgmt --> mset["settings"]
```

## Regras gerais

- **Sem sessão**, qualquer página que não seja pública leva a `/login?next=<página>`, e a pessoa volta para ela depois
  de entrar. Quem já está logado e abre `/login`, `/signup`, `/invite/:token` ou `/auth/clerk/continue` vai para `/`.
- **Outra organização** ou recurso que a pessoa não alcança responde "Página não encontrada" (404), nunca 403: a página
  não revela que o recurso existe. O mesmo vale para quem não tem a permissão de uma página restrita.
- **Projeto.** Quem não é dono nem admin só abre as páginas dos projetos em que está, que são os projetos em que tem
  valor por hora. O dono e os admins estão em todos.
- **Públicas:** entrar, criar organização, o convite, a volta do Clerk, a ajuda, a troca de idioma e o script dos
  textos.
- **Caminhos antigos** continuam valendo e redirecionam: `/projects/:id/time-tracking` vai para o Início do projeto,
  e `/projects/:id/teams`, `/integrations` e `/settings` vão para a aba da Gestão, mantendo a query.

## Públicas

| Rota | Tela |
|---|---|
| `/login` | Entrar com o e-mail e a senha do sistema. Com o Clerk ligado, o Google aparece abaixo. `?out=1` é o login depois de sair: com o Clerk, desconecta dele também |
| `/signup` | Criar organização, em duas etapas (a organização e o acesso) |
| `/invite/:token` | Aceitar um convite e criar a conta, com o e-mail travado no do convite. O ticket do Clerk na URL é ignorado |
| `/auth/clerk/continue` | A volta do Clerk: troca o token dele por uma sessão do sistema. Aceita `?next=` e `?invite=`. Sem o Clerk ligado, leva ao login. Ver [clerk.md](../integrations/clerk.md) |
| `/help` | A ajuda, numa página só. Logada, a pessoa a vê com a barra superior. `/ajuda` redireciona para cá |
| `/lang/:code` | Troca o idioma (`pt-BR` ou `en`): grava o cookie `wtt_lang` e volta para `?next=` |
| `/i18n/:idioma.js` | Os textos do idioma para o JavaScript (`window.I18N`). Com `?v=<hash>`, o cache é eterno |

## Conta e organização

| Rota | Tela | Quem abre |
|---|---|---|
| `/` | Leva à organização de quem está logado | logado |
| `/profile` | O seu perfil: jornada, regra de pagamento, horas e valores, histórico e próximos pagamentos. Editar perfil abre um modal | logado |
| `/orgs/:orgId` | O Início. Para todos, o painel da pessoa (horas de hoje e da semana, as suas tarefas, a semana dia a dia). Para os admins, também a visão geral da organização e a equipe; receita e margem só para o dono | membro |
| `/orgs/:orgId/projects` | Projetos em cartões, seis por página (`?page=`). Quem não é admin vê só os projetos em que está. O dono e os admins têm o Novo projeto | membro |
| `/orgs/:orgId/about` | Configurações: o perfil da organização, para ler. O Editar só aparece para o dono | membro |
| `/orgs/:orgId/settings` | A edição da organização e a exclusão dela | dono |
| `/orgs/:orgId/people` | Colaboradores: as pessoas, os papéis, a jornada, os convites pendentes e o Adicionar colaborador. `?add=1` abre já com o modal do convite | `people.manage` (dono e admin) |
| `/orgs/:orgId/people/:personId` | O perfil de um colaborador, com o Editar. Pagamentos e regra só para o dono | dono e admin |
| `/orgs/:orgId/customers` | Clientes, com cadastro e edição num modal | dono |
| `/orgs/:orgId/payments` | Pagamentos da equipe: o que pagar nos períodos em andamento, o que fecha em 7 dias e quem está sem regra. A linha leva ao perfil | dono |

## Projeto

Todas pedem que a pessoa esteja no projeto.

| Rota | Tela |
|---|---|
| `/projects/:id` | Leva ao Início do projeto |
| `/projects/:id/overview` | Início do projeto, igual para todos: o painel do timer, as suas tarefas, o seu tempo de hoje e da semana e as suas sessões, com filtros e páginas |
| `/projects/:id/my-tasks` | Minhas tarefas: o painel do timer e as tarefas de que a pessoa é responsável, uma lista por status |
| `/projects/:id/tasks` | Lista de tarefas: as sem responsável e as de outras pessoas, cada lista com a sua paginação. A linha abre a tarefa |
| `/projects/:id/collaborators` | Colaboradores, só para ler: pessoas e times, sem valores nem ações. Para os admins, a coluna Agora |
| `/tasks/:taskId` | A página da tarefa, sem as abas do projeto: nome e descrição, detalhes, tempo registrado, um cartão por item externo, publicar nas integrações e excluir |

**A lista de tarefas guarda o estado na URL:** `?q=`, `?assignee=` (só para a lista Com responsável), `?due=`,
`?priority=`, `?status=`, `?label=` (os três últimos separados por vírgula) e `?free_page=` e `?taken_page=`.
Recarregar mantém a tela, o link pode ser compartilhado, e o Voltar da tarefa volta ao mesmo ponto da lista (a query fica
também no `sessionStorage`).

## Gestão do projeto

A Gestão é a área de quem cuida do projeto. O botão aparece para quem tem **alguma** permissão do projeto, e cada aba
pede a permissão do que mostra. Quem não tem recebe o 404.

| Rota | Tela | Permissão |
|---|---|---|
| `/projects/:id/management` | Leva à primeira aba que a pessoa pode abrir | alguma do projeto |
| `/projects/:id/management/overview` | Visão geral: pessoas, times, tempo de projeto, integrações, atividade, tarefas atrasadas e as sessões de todos. A cobrança, a receita e a margem só com `billing.view` | alguma do projeto |
| `/projects/:id/management/teams` | Colaboradores em duas visões, Pessoas e Times (`?view=teams`), cinco por página, com os modais de adicionar pessoa, do colaborador e dos times | `collaborators.manage`, `teams.manage`, `rates.view` ou `rates.manage` |
| `/projects/:id/management/integrations` | Integrações: um cartão por integração e o modal de criar e editar. Na volta da conexão recebe `?github=<id>`, `?github_error=<código>`, `?trello=<id>` ou `?trello_error=<código>` | `integrations.manage` |
| `/projects/:id/management/settings` | Configurações do projeto, para ler, com o Editar projeto | `project.edit`, `billing.view` ou `billing.manage` |

## Conexão com as plataformas

Não são telas: são idas e voltas do navegador durante a autorização. O detalhe está em
[github.md](../integrations/github.md) e [trello.md](../integrations/trello.md).

| Rota | O que faz | Quem |
|---|---|---|
| `/projects/:id/management/integrations/github/connect` | Grava o cookie da conexão e leva ao GitHub. `?integration=<id>` reconecta | `integrations.manage` |
| `/integrations/github/callback` | A volta do GitHub: confere o cookie, o `state`, a pessoa e a permissão e guarda a integração | logado |
| `/projects/:id/management/integrations/trello/connect` | Grava o cookie da conexão e leva à autorização do Trello. `?integration=<id>` reconecta | `integrations.manage` |
| `/integrations/trello/callback` | A volta do Trello, com o token no fragmento da URL: o script da página o lê e o entrega à rota abaixo | logado |
| `POST /integrations/trello/token` | Corpo `{state, token}`. Confere tudo de novo e responde `{"redirect": "…"}` | logado, JSON |
