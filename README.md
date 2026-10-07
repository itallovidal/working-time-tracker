# Working Time Tracker

Ponto por tarefa para equipes que trabalham por projeto. Foi pensado principalmente para empresas de desenvolvimento, como software houses e consultorias, que atendem vários clientes e alocam as pessoas em um ou mais projetos, mas nada nele depende disso. A pessoa faz **clock-in** numa tarefa, faz **clock-out** quando para, e o sistema soma o tempo por tarefa e por pessoa. Tudo fica organizado em organização, projetos e times, e as tarefas podem apontar para issues do GitHub ou do GitLab e para cartões do Trello.

É um único binário em Go que serve a API JSON e a interface web.

## O que dá para fazer

- **Contas e organizações.** O signup cria uma organização com você como admin e **dono** dela. Outras pessoas entram por **link de convite** (uso único, válido por 7 dias, opcionalmente preso a um email).
- **Perfil da organização.** Resumo, descrição, segmento, contato, dados jurídicos (razão social, CNPJ, endereço) e como ela trabalha (regime remoto, híbrido ou presencial, fuso e moeda). O admin edita; todos os membros leem na aba **Sobre**, onde campo sem valor aparece como "Não informado".
- **Papéis.** Admins gerenciam a organização, as pessoas, os clientes, os projetos, os times, os valores e as integrações. Membros gerenciam tarefas e batem o próprio ponto, e fazem mais só se receberem **permissões**: num projeto, por **grupo** (colaborador, gerente de projeto, financeiro ou administrador do projeto, escolhido ao pôr a pessoa nele), ou na organização (criar projetos, cuidar dos clientes e das pessoas, liberadas pelo dono). A lista está em [`_docs/permissions.md`](_docs/permissions.md). O **dono** é o admin que criou a organização: um só por organização, sempre admin, e as horas dele valem o valor cobrado, sem custo, em vez de um valor pago (o que ele tira do projeto é a margem). Ele entra sozinho nos projetos que cria e nos que passa a trabalhar.
- **Jornada semanal** de cada pessoa. É o que a organização combinou com ela, em horas por semana, e vale para todos os projetos em que trabalha. O admin informa na aba Colaboradores da organização; a pessoa lê no perfil.
- **Clientes e valores por hora.** Cada projeto pode ter um cliente e o **valor cobrado** dele por hora. Cada pessoa tem um **valor pago** por hora em cada projeto, então a mesma pessoa pode receber 20 num projeto e 25 em outro. O admin vê e altera tudo, com a margem por hora; o membro vê só o que ele mesmo recebe.
- **Projetos** com duração da sprint (7 dias, 14 dias ou 1 mês), daily (com horário) e weekly (com dia e horário). O projeto pode ter só uma das duas, as duas ou nenhuma: no formulário, "O projeto tem daily" e "O projeto tem weekly" são marcas, e sem elas o projeto fica sem a reunião. Num projeto com cliente há ainda a reunião semanal com o cliente (dia e horário), no grupo "Cliente e cobrança": "Possui reunião semanal com o cliente" é outra marca, e tirar o cliente do projeto apaga a reunião. Esses valores são de cada projeto, não da organização, porque projetos diferentes podem trabalhar de formas diferentes. A aba Configurações só mostra o projeto; o admin altera tudo no modal Editar projeto.
- **Colaboradores e times** de cada projeto. A aba Colaboradores abre com um resumo (colaboradores, quantos estão sem time e times; para o admin, também a margem por hora somada, com os valores de hoje, e o valor das horas registradas, que soma as sessões de ponto já fechadas, cada um com um botão de dica que explica a conta) e mostra quem trabalha no projeto em duas visões, escolhidas logo abaixo dele: **Pessoas**, a lista de todos, com busca, os times de cada um e, para o admin, quanto recebe por hora e a margem; e **Times**, um cartão por time com os integrantes. Nenhuma lista mostra mais de cinco pessoas por vez: a tabela e cada cartão têm páginas. A pessoa entra no projeto com o valor por hora dela, que é obrigatório, e só quem já está no projeto entra num time. Adicionar pessoa tem dois passos: o primeiro escolhe a pessoa, o valor e o time, e o segundo, o **grupo de permissões** dela no projeto (colaborador, gerente de projeto, financeiro ou administrador do projeto; o dono e os admins já têm tudo e pulam o segundo passo). O modal do colaborador mostra o grupo atual e deixa trocar. Para o admin, clicar numa pessoa, na tabela ou no cartão de um time, abre o modal do colaborador, onde ele muda o valor por hora, marca os times e tira a pessoa do projeto. O lápis de cada cartão abre o modal Editar time, onde o admin troca o nome, marca quem faz parte, entre as pessoas do projeto, e exclui o time. Só quem está em algum time do projeto pode ser responsável por tarefas.
- **Lista de tarefas e Minhas tarefas.** Cada tarefa tem prazo (7 dias por padrão, com destaque quando está atrasada ou perto de vencer) e vínculo opcional com uma issue, e o **responsável é opcional**: uma tarefa criada sem responsável fica disponível na lista, e quem bater o ponto nela passa a ser o responsável, mesmo sem estar num time. A **Lista de tarefas** abre dizendo o que é (onde o time acha o que ninguém pegou) e tem **duas listas**, cada uma com a sua paginação de 10: **Sem responsável**, para o time pegar, e **Com responsável**. A aba **Minhas tarefas** reúne só as tarefas de que você é responsável, uma lista por status, na ordem em que o trabalho anda (Backlog, Em progresso, Aguardando fechamento e Fechada, esta recolhida), cada uma com o botão Iniciar. A busca por nome, o filtro por responsável (escolher uma pessoa esconde a lista das sem responsável), o filtro por prazo (atrasadas, até hoje, até o fim desta semana ou da próxima, ou até uma data) e 10 tarefas por página. A caixa "Só as minhas tarefas", abaixo dos filtros, mostra as suas e desliga a busca e o filtro de responsável: só o de prazo continua valendo. A tarefa nova é criada num modal.
- **Descrição em Markdown e modais em etapas.** A descrição da tarefa aceita Markdown (títulos, listas, negrito, código, citações, tabelas e links; sem imagens): a página da tarefa a mostra renderizada, e o modal tem o alternar Escrever / Pré-visualizar. O marked gera o HTML e o DOMPurify o sanitiza (só as tags permitidas, links só para `http`, `https` e `mailto`, abrindo em outra aba), tudo no navegador, com as duas bibliotecas vendorizadas e com hash conferido. A descrição vai até 10.000 caracteres. Os modais **Nova tarefa** e **Editar tarefa** são um passo a passo: a etapa 1 é o nome e a descrição, e a etapa 2, o prazo, a prioridade, as etiquetas e o responsável.
- **Status da tarefa.** Toda tarefa tem um **status**: backlog, em progresso, aguardando fechamento ou fechada. A tarefa nova nasce sempre em backlog, e o status muda ao **editar** a tarefa, em qualquer ordem e por qualquer pessoa do projeto, ou sozinho: **bater o ponto** numa tarefa a põe **em progresso**, seja qual for o status dela (inclusive aguardando fechamento ou fechada), e parar ou pausar não a tira de lá. O quadro tem uma coluna de status e filtra por **um ou mais status** (a tarefa passa se tem qualquer um dos marcados), com o filtro na URL como os outros, e a página da tarefa mostra o status.
- **Prioridade e etiquetas.** Toda tarefa tem uma **prioridade** (urgente, alta, média, baixa ou sem prioridade, que é o padrão) e pode ter várias **etiquetas**. As etiquetas são de cada projeto e só têm nome: admins criam, renomeiam e excluem, inclusive na hora, pelo campo "Nova etiqueta" do modal Nova tarefa, e quem não é admin escolhe entre as que existem. O quadro filtra por **uma ou mais prioridades** e por **uma ou mais etiquetas** (a tarefa passa se tem qualquer uma das marcadas), com os filtros na URL como os outros; a lista mostra a prioridade em uma coluna e as etiquetas sob o nome, e a página da tarefa e o modal Editar tarefa trazem os dois campos.
- **Ponto e Visão geral pessoal.** Clock-in e clock-out com cronômetro ao vivo no topo de todas as páginas. **Uma sessão pode ter várias tarefas:** o ponto começa por uma, e outras entram depois, pela pílula do topo (que mostra a primeira tarefa e "+N" das outras e abre a sessão), pelo botão Adicionar à sessão no Início e na página da tarefa, ou pelo modal da sessão, que mostra cada tarefa com o intervalo em que esteve nela, numa barra de tempo. O tempo vale para a sessão e para cada tarefa (tarefas em paralelo contam o tempo cheio cada uma; a pessoa, o projeto e os valores contam a sessão uma vez só), e as tarefas se mexem também depois de a sessão terminar (quem bateu o ponto e os admins). O **Início**, a primeira aba e a mesma tela para admin e membro, tem o relógio, que mostra também quanto a sessão aberta já rendeu, ao lado do seu tempo de hoje e da semana; as **suas tarefas** (as de que você é o responsável), em cartões com o nome, o Ver detalhes e o Iniciar (para pegar uma tarefa disponível, use o quadro de tarefas); e as **suas sessões**, cada uma com as suas tarefas e um botão que abre o modal dela, com filtros de data e tarefa (com uma tarefa no filtro, o tempo e o valor da linha e dos totais são os dela na sessão), os totais do filtro num cartão à parte e dez sessões por página. O membro só enxerga as próprias sessões, e isso vale no servidor; o admin vê as de todos na Gestão. O banco garante uma única sessão aberta por pessoa.
- **Horas em dinheiro.** Cada sessão guarda os valores por hora de quando o ponto abriu, então **mudar um valor só vale dali em diante**. Quem não tem valor definido no projeto **não bate ponto**. Na tela de ponto, o membro vê quanto ganhou; o admin vê custo, receita e margem.
- **Gestão do projeto**, só para admins. Um botão **Gestão**, no fim da barra de abas, leva a uma área com abas próprias (Visão geral, Colaboradores, Integrações e Configurações) e a um link para voltar ao projeto; o membro não vê o botão e recebe "Página não encontrada" nos endereços da Gestão. A **Visão geral** da Gestão mostra, numa tela só: quantas pessoas e quantos times trabalham no projeto, as horas registradas, a receita, o custo e a margem; há quanto tempo o projeto existe, em dias, semanas e meses, contados do dia em que foi cadastrado; as integrações configuradas e se estão ativas; a atividade dos últimos 7 e 30 dias e quem está com o ponto aberto; as tarefas atrasadas; e as horas, o custo e a receita de cada pessoa. É uma fotografia da hora em que foi lida, com um botão para atualizar.
- **Integrações** com GitHub, GitLab e Trello. Todas usam a mesma estrutura: nome, token e, em `metadata`, os campos próprios da plataforma (o repositório, o projeto, a chave e o quadro), que cada integração confere antes de falar com ela. O token é validado na plataforma, guardado criptografado e nunca volta nas respostas. O admin cria e edita num modal só, que desenha os campos da plataforma escolhida; o cartão de cada integração é só de leitura, e desativar ou excluir também ficam no modal. Os detalhes do item (o título e o estado da issue, ou a lista em que o cartão está) são buscados na hora, e se a plataforma não responde a tela mostra o motivo, sem quebrar.

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

Abra **http://localhost:8080**. O seed cria a **Jatobá Software**, uma software house com doze pessoas, quatro clientes e oito projetos (um deles interno), em que cada pessoa tem um valor por hora diferente conforme o projeto. Entre como:

| Papel | Email | Senha |
|---|---|---|
| Admin | `ana@example.com` ou `helena@example.com` | `demo12345` |
| Membro | `bruno`, `carla`, `diego`, `elisa`, `fabio`, `gabriela`, `henrique`, `isabela`, `joao` ou `larissa`, seguidos de `@example.com` | `demo12345` |

O que o seed põe nas telas, para elas aparecerem cheias:

- **Pessoas e times.** O App de Pedidos tem nove pessoas em quatro times, então a aba Colaboradores passa de uma página. O Painel do Lojista tem a Elisa com valor e sem time, e o João está sem time em três projetos.
- **Tarefas.** 62, com prioridade (as atrasadas são urgentes), status nos quatro valores (as sem responsável ficam no backlog) e etiquetas por assunto (backend, frontend, design, qualidade, infra), com prazos de ontem a um mês: algumas atrasadas, algumas para hoje e onze **sem responsável**, disponíveis no Quadro de tarefas. Vinte e quatro estão vinculadas a itens do GitHub, do GitLab e do Trello.
- **Dinheiro.** Cada projeto tem o seu valor cobrado, e cada pessoa, o seu valor pago. A Migração do ERP cobra menos do que a maioria recebe, e a margem fica negativa; o projeto interno não tem receita.
- **Sessões.** Cerca de 1.400 sessões fechadas nos últimos 75 dias, geradas por uma sequência fixa e seguindo a jornada de cada um; cerca de três em dez têm uma segunda tarefa em paralelo, que entra no meio da sessão. O mesmo seed dá sempre o mesmo banco. O Bruno e a Carla estão com o ponto aberto, então a Visão geral mostra "Trabalhando agora"; essas sessões ficam abertas até alguém parar.
- **Integrações.** Oito, de demonstração e **sem credencial**: aparecem nas telas, mas só buscam issues depois que você edita e informa um token seu.
- **Jornada.** 40 horas para a maioria, 30 para o Diego e a Gabriela, 20 para a Elisa e o João; a Ana, sócia, fica sem jornada definida.

Sem o seed, clique em **Crie uma organização** na tela de login.

Se a porta 5432 já estiver ocupada, suba o banco com `POSTGRES_PORT=5433 docker compose up -d` e ajuste a porta no `.env`.

Para gerar o binário: `go build -o wtt ./cmd && ./wtt`. Templates e arquivos estáticos vão embutidos nele via `embed.FS`.

### Variáveis de ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `API_PORT` | sim | Porta HTTP, por exemplo `8080` |
| `DATABASE_URL` | sim | Conexão com o PostgreSQL |
| `INTEGRATION_ENCRYPTION_KEY` | sim | Chave da criptografia AES-GCM dos tokens de integração. Trocá-la torna os tokens salvos ilegíveis: cada integração pede o token de novo na edição, e o resto dela (nome e `metadata`) continua |
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
| `/orgs/:orgId/people` | Organização, aba Colaboradores: os integrantes, com o papel (que muda no botão da linha) e a jornada semanal de cada um (que muda num modal, pelo lápis); os convites pendentes e o botão Adicionar colaborador, que gera o link de convite num modal (só admins) |
| `/orgs/:orgId/customers` | Organização, aba Clientes: quem contrata os projetos, com cadastro e edição num modal (só admins) |
| `/orgs/:orgId/projects` | Organização, aba Projetos: tabela de gestão com o cliente, os colaboradores e as tarefas de cada projeto; a linha abre o projeto (só admins) |
| `/profile` | Seu nome, seu email, sua senha, sua jornada semanal (só para ler) e quanto você recebe por hora em cada projeto |
| `/lang/:code` | Troca o idioma (`pt-BR` ou `en`): grava o cookie e volta para `?next=` |
| `/i18n/:idioma.js` | Os textos do idioma para o JavaScript (`window.I18N`) |
| `/projects/:projectId` | Leva todos para a Visão geral |
| `/projects/:projectId/overview` | O Início do projeto, igual para admin e membro: o relógio e as suas tarefas, o seu tempo de hoje e da semana e as suas sessões, com filtros de data e tarefa, totais e páginas |
| `/projects/:projectId/collaborators` | Colaboradores para todos, só para ler: as pessoas (com busca) e os times, cinco por página, sem valor por hora, margem nem ações, nem para o admin. Quem edita é a aba da Gestão |
| `/projects/:projectId/management` | A raiz da Gestão (só admins): leva para a Visão geral dela |
| `/projects/:projectId/management/overview` | Gestão, Visão geral: o cartão Projeto (pessoas, times, tempo de projeto), integrações, atividade recente, tarefas atrasadas e a lista de sessões de todos, com filtros de pessoa, data e tarefa, e dez por página. Horas, custo, receita e margem aparecem uma vez só, nos totais das sessões, que sem filtro são os do projeto Os caminhos antigos `/teams`, `/integrations` e `/settings` redirecionam para a Gestão |
| `/projects/:projectId/tasks` | Lista de tarefas, com duas listas (as sem responsável e as que têm), busca, filtros e uma paginação para cada; cada linha leva à página da tarefa |
| `/projects/:projectId/my-tasks` | Minhas tarefas: as tarefas de que a pessoa é responsável, numa lista por status, com o botão Iniciar |
| `/tasks/:taskId` | A página da tarefa, uma **tela própria**, sem as abas do projeto: um cabeçalho com o botão Voltar à lista, o caminho (Projetos / o projeto), o título e as ações (Iniciar e o lápis). Só para ler, em quatro cartões: a **Descrição**, os **Detalhes** (status, prioridade, responsável com o email, prazo, etiquetas e a data de criação), o tempo registrado e o item externo vinculado. Em tela estreita os cartões ficam numa coluna, nessa ordem. O botão Iniciar e o lápis, que abre o modal Editar tarefa (com o Excluir tarefa), ficam no cartão. O quadro de tarefas leva a ela pelo botão Detalhes |
| `/projects/:projectId/time-tracking` | O endereço do antigo Ponto: redireciona para o Início |
| `/projects/:projectId/management/teams` | Gestão, Colaboradores, em duas visões: Pessoas (a lista, com busca) e Times (os cartões; `?view=teams` abre nela), com cinco pessoas por página. Para admins, também o valor por hora de cada pessoa, a margem e os modais de adicionar pessoa, do colaborador, de novo time e de editar time |
| `/projects/:projectId/management/integrations` | Gestão, Integrações com GitHub, GitLab e Trello: um cartão por integração e, para admins, o modal de criar e editar |
| `/projects/:projectId/management/settings` | Gestão, Configurações: a descrição, a sprint, a daily, a weekly (dia e horário), o cliente e a reunião com o cliente (dia e horário), só para ler. Para admins, também o valor cobrado e o botão Editar, que abre o modal Editar projeto, onde ficam todos os campos e a exclusão |

Sem sessão, qualquer página leva ao login, e a pessoa volta para a página pedida depois de entrar. Uma página de outra organização mostra "Página não encontrada".

### Navegação

- A **barra superior** mostra a organização, o menu (Projetos e Organização), o **indicador do ponto aberto** com cronômetro e botão Parar, o **toggle de idioma** (PT | EN), quem está logado (o nome leva ao **perfil**) e o botão Sair. Sem login, o toggle fica no canto da tela.
- As páginas de projeto têm duas barras de **abas**. Para todos: Início (o relógio, as suas tarefas e as suas sessões), Minhas tarefas, Lista de tarefas e Colaboradores (só para ler), e o projeto abre no Início. Para admins, o botão Gestão abre a outra: Visão geral, Colaboradores, Integrações e Configurações, em `/projects/:id/management/...`. A aba Colaboradores juntou as antigas Times e Valores. O valor cobrado do cliente fica em Configurações.
- A página **Organização** abre na aba Sobre, que todos os membros leem. Nela o admin tem o botão **Editar**, que leva à tela de edição; salvar volta para a Sobre. As abas Colaboradores, Clientes e Projetos são só de admins.
- Ações de admin não aparecem para membros. A API continua sendo quem garante as permissões.
- A lista de **tarefas** guarda a busca, os filtros e a página na URL (`?q=`, `?assignee=` (um id, ou `none` para as sem responsável), `?due=`, `?page=`, e `?mine=1` para "Só as minhas tarefas"). Recarregar mantém o que estava na tela, o link pode ser compartilhado, e o botão Voltar de uma tarefa leva de volta ao mesmo ponto da lista.

### Idiomas

A interface fala **português do Brasil** (o padrão) e **inglês**. Todo texto que a pessoa lê mora em dois arquivos YAML, `internal/i18n/locales/pt-BR.yaml` e `en.yaml`; o código só conhece as chaves.

- **Qual idioma vale:** o cookie `wtt_lang` (gravado pelo toggle, por um ano); sem ele, o `Accept-Language` do navegador; sem nenhum dos dois, português. O toggle é um link para `/lang/<código>?next=<página>`, então funciona sem JavaScript e nas telas sem login. O idioma não fica na conta: é uma escolha do navegador.
- **Servidor:** [go-i18n v2](https://github.com/nicksnyder/go-i18n) lê os YAML (`internal/i18n`). Os templates usam `{{.T "chave"}}` (dentro de `range` ou `with`, `{{$.T "chave"}}`); com valores, `{{.T "chave" "nome" .Nome}}`. Os títulos das páginas em Go usam `TitleKey: "titles.login"`.
- **Navegador:** o servidor entrega o mesmo YAML como `window.I18N` (`/i18n/<idioma>.js`, com o hash na URL para o cache), antes do `app.js`. No JavaScript, `WTT.t('chave', { nome: valor })`; nas expressões do Alpine, `$t('chave', { nome: valor })`.
- **Regra importante:** dentro de uma expressão Alpine (`x-text`, `:title`, `:placeholder`) use sempre `$t`, nunca `{{.T}}`. O `html/template` só escapa HTML, e um apóstrofo de um texto em inglês ("Don't") quebraria a expressão JavaScript.
- **Como escrever as chaves:** uma chave por **frase inteira**, nunca pedaços para juntar no código (a ordem das palavras muda de um idioma para outro); valores entram por placeholder (`"Em andamento: {{.name}}"`); plural com as formas `one` e `other` e o placeholder `{{.count}}`. No português o zero também é `one` (regra do CLDR), então "nenhuma pessoa" é uma chave à parte. Valores de enum (papel, moeda, regime) continuam sendo os códigos do backend; só o rótulo vem do YAML. Não use como nome de chave `id`, `description`, `hash`, `zero`, `one`, `two`, `few`, `many`, `other`, `translation`, `leftdelim` nem `rightdelim`: o go-i18n os lê como partes de uma mensagem e recusa o arquivo (use `description_label`).
- **Adicionar um idioma:** crie `locales/<código>.yaml` com as mesmas chaves, acrescente o código em `supported` (`internal/i18n/i18n.go`) e as chaves `lang.<código>.short` e `lang.<código>.name` em todos os catálogos. O teste confere o resto.
- **Testes:** `internal/i18n` garante que todos os idiomas têm as mesmas chaves e os mesmos placeholders, e que toda chave usada nos templates, no JavaScript e nas páginas em Go existe. Em `internal/server`, o idioma por cookie, por cabeçalho e pelo toggle.
- **Datas, horas e valores** seguem o idioma (`Intl` com `WTT.lang`): "R$ 20,50" em português e "R$20.50" em inglês. A **moeda** não muda com o idioma, é a da organização. Os campos de valor mostram o separador do idioma e leem os dois jeitos de digitar ("20,5", "20.50", "1.234,56" e "1,234.56"); com um separador só e três dígitos depois ("1.234"), vale o separador de milhar do idioma.
- **Erros da API:** a API não manda mensagem, só um código estável (`{"error": {"code": "auth.invalid_credentials", "params": {...}}}`). O navegador traduz pelo código, procurando `errors.<código>` no mesmo YAML (`WTT.errorText`), e `e.message` já chega no idioma da página. Os códigos são declarados em `errors.go` de cada domínio (`apperr.New`) e estão documentados, com o status e os textos, em [`_docs/error-codes.md`](_docs/error-codes.md), um arquivo gerado por teste. Um teste recusa código sem texto, texto sem código e placeholder que o código não declara.
- **Fora da tradução:** os dados de demonstração do seed e o que as pessoas digitam (nomes de projeto, tarefas, clientes) ficam como foram escritos. Logs e comandos de linha de comando seguem em português.

### Onde fica cada coisa

```
web/
  templates/
    layouts/base.gohtml        # HTML base, carrega os ícones (cdnjs), os textos do idioma, app.css, app.js, o script da página e o Alpine
    partials/                  # barra superior, toggle de idioma, cabeçalhos da organização e do projeto, indicador do ponto, toasts, modal e ícone
    pages/*.gohtml             # uma casca por tela; cada uma define o bloco "content"
  static/
    app.css                    # estilos com tema claro e escuro, sem framework
    app.js                     # api() sobre fetch, estado de formulário, toasts, modal, formatadores, cronômetro
    pages/*.js                 # componentes Alpine de cada grupo de telas (auth, org, project)
    alpine.min.js              # Alpine.js 3.14.8 vendorizado (dist/cdn.min.js do pacote npm)
    marked.min.js              # marked 18.1.0 (lib/marked.umd.js): Markdown da descrição da tarefa
    purify.min.js              # DOMPurify 3.4.16 (dist/purify.min.js): sanitiza o HTML do Markdown
internal/page/                 # handlers das páginas
internal/i18n/                 # catálogos YAML (locales/), tradução, escolha do idioma e o script dos textos
internal/template/renderer.go  # um conjunto de templates por página, carregado na inicialização
```

Para atualizar o Alpine, o marked ou o DOMPurify, troque o arquivo em `web/static/` e o hash em `web/embed_test.go` juntos (o `marked.min.js` e o `purify.min.js` são os do pacote npm sem a última linha, o comentário `sourceMappingURL`). O teste existe porque a cópia vendorizada do Alpine já esteve corrompida sem ninguém perceber, e porque o DOMPurify é a única defesa contra XSS na descrição da tarefa.

**Ícones.** `{{template "icon" "pen"}}` emite o ícone do Font Awesome com esse nome (sem o prefixo `fa-`). O ícone é decorativo: o botão mantém o texto. Um botão só de ícone, como os de editar um colaborador e de editar um time na aba Colaboradores, leva a classe `btn-icon` e o nome da ação em `aria-label` e em `title`. Num botão com texto dinâmico, ponha o texto num `<span x-text>`, porque um `x-text` no botão apagaria o ícone. Para trocar a versão, mude a URL e o `integrity` em `base.gohtml` juntos; o hash vem de `https://api.cdnjs.com/libraries/font-awesome/<versão>?fields=sri`.

**Dica.** Para explicar um número sem ocupar a tela, o bloco leva `x-data="tip"` e a classe `has-tip`, um botão `tip-btn` com o ícone `circle-info`, `aria-label`, `aria-controls` e `@click="toggle()"`, e o texto num elemento `tip-pop` com `x-ref="pop"` e `x-show="open"`. O balão abre abaixo do bloco, não passa da borda da fileira nem da tela e fecha com Esc, com um clique fora ou quando a janela muda de tamanho. Hoje usam a dica os dois blocos de dinheiro do resumo da aba Colaboradores.

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

Hoje usam o modal o cadastro de cliente, o Adicionar colaborador, a Jornada e permissões da pessoa (para o dono, também as permissões da organização), o Novo projeto, o Editar projeto, a Nova tarefa e, na aba Colaboradores do projeto, o Adicionar pessoa, o Editar colaborador, o Novo time e o Editar time, e, na aba Integrações, o formulário único de criar e editar integração. Para focar um campo que acabou de aparecer dentro do modal (como o link do convite depois de gerado), dê ao bloco um `x-transition`: sem transição, o `x-show` só mostra o elemento no ciclo seguinte e o `$nextTick` chega antes de ele aceitar foco.

## API

A referência completa, com exemplos, está em [`_test/routes.md`](_test/routes.md), e os códigos de erro em [`_docs/error-codes.md`](_docs/error-codes.md). A coleção do Insomnia está em `_test/insomnia-collection.json`: importe, rode **Signup** ou **Login** e o Insomnia guarda o cookie de sessão para as outras chamadas.

Resumo dos grupos de rotas:

| Grupo | Rotas |
|---|---|
| Autenticação | `/api/auth/signup`, `login`, `logout`, `me`, `password`, `invites/:token` |
| Organização | `/api/orgs/:orgId` (+ `persons`, `projects`, `customers`, `invites`) |
| Clientes | `/api/customers/:customerId` |
| Pessoas | `/api/persons/:personId` (+ `role`, `weekly-hours`, `allocations`) |
| Projetos | `/api/projects/:projectId` (+ `overview`, `teams`, `tasks`, `labels`, `members`, `collaborators`, `integrations`, `work-sessions`) |
| Valores | `/api/projects/:projectId/billing`, `/api/projects/:projectId/allocations` (+ `/:personId`) |
| Times | `/api/teams/:teamId` (+ `members`) |
| Tarefas | `/api/tasks/:taskId` (+ `link-external-item`, `external-details`) |
| Ponto | `/api/projects/:projectId/work-sessions/*`, `/api/work-sessions/active` |
| Integrações | `/api/integrations/:integrationId` |

**Integrações.** O corpo é o mesmo para qualquer plataforma. O que muda de uma para outra vai em `metadata`:

```json
{
  "type": "trello",
  "display_name": "Quadro do app",
  "token": "…",
  "metadata": { "api_key": "…", "board_id": "https://trello.com/b/AbC123xy/app" }
}
```

| `type` | `token` | `metadata` |
|---|---|---|
| `github` | token pessoal com leitura de issues | `repo`: `dono/repositorio` ou o endereço |
| `gitlab` | token com escopo `read_api` | `project_url`: `grupo/projeto` ou o endereço |
| `trello` | token da API | `api_key`: a chave do Power-Up; `board_id`: o endereço do quadro, o link curto ou o id |

Um tipo novo é um arquivo em `internal/adapter` que implementa `Integration` e uma linha no `registry.go`. O `Descriptor` dele diz que campos o `metadata` tem, e é dele que a tela tira o formulário e os rótulos (as páginas de projeto o recebem em `window.BOOT.integration_types`); o `CheckMetadata` confere e normaliza esses campos sem falar com a plataforma; `Validate` e `FetchItemDetails` recebem a `Connection`, com o token e o `metadata`.

## Segurança

- **Senhas** com bcrypt. Contas sem senha (criadas antes do login existir) não conseguem entrar.
- **Sessões e convites** usam tokens aleatórios de 32 bytes, e o banco guarda só o sha256 deles. Trocar a senha encerra as outras sessões.
- **Cookie** `wtt_session` HttpOnly e SameSite=Lax, com `Secure` via `COOKIE_SECURE`. Como a API só aceita corpo JSON em `POST`, `PUT` e `PATCH`, um formulário de outro site não consegue agir em nome de quem está logado.
- **Isolamento entre organizações.** Cada rota com ID confere se o recurso é da organização de quem chama e responde 404 caso não seja.
- **Valores.** O valor cobrado do cliente só existe em rotas de admin: ele não entra no JSON do projeto. Na lista de valores de um projeto, um membro recebe só a própria linha, e na de colaboradores, só o próprio valor. Nas sessões de ponto, a API apaga o valor pago das sessões de outras pessoas e todo valor cobrado antes de responder a quem não é admin. A visão geral do projeto, que soma o custo e a receita de todos, é uma rota só de admins, e a página dela responde "Página não encontrada" a um membro.
- **Limite de tentativas** por IP em signup, login e convites.
- **Tokens de integração** criptografados com AES-GCM (`INTEGRATION_ENCRYPTION_KEY`) e nunca devolvidos pela API. O `metadata` de uma integração fica em claro e volta nas respostas, então não é lugar de segredo: cada tipo só guarda nele os campos que declara. O que vem de quem usa e entra numa URL da plataforma (o repositório, o quadro, o número da issue, o cartão) é conferido antes.

## Testes

Os testes usam um PostgreSQL de verdade. O `docker compose` já cria o banco `working_time_tracker_test`.

```bash
TEST_DATABASE_URL=postgres://wtt:wtt@localhost:5432/working_time_tracker_test?sslmode=disable \
  go test -p 1 ./...
```

Cada pacote de teste apaga o schema desse banco e aplica as migrações de novo antes de rodar, então ele não precisa de preparo e os testes sempre veem o que os arquivos de migração produzem hoje. Por segurança, isso só acontece num banco cujo nome termina em `_test`.

O `-p 1` é necessário porque todos os pacotes recriam e usam o mesmo banco. As chamadas ao GitHub, ao GitLab e ao Trello nos testes vão para servidores fake (`httptest`, em `testutil/platforms.go`), então a suíte não depende de rede.

Cobertura:
- **Domínios:** services e handlers.
- **Migrações:** o banco que elas produzem bate com o `ent/schema`, regras de FK, índice parcial, recusa de banco sem histórico e checksum dos arquivos.
- **Router real:** tabela de rotas, autenticação, permissões e isolamento entre organizações.
- **Páginas:** toda página renderiza para admin e membro, redireciona sem sessão e dá 404 entre organizações; cada uma tem o modal uma única vez e carrega os ícones com SRI. As abas só de admins (as de gestão da organização e a Gestão do projeto) dão 404 ao membro e somem do menu dele.
- **Visão geral:** os totais com valores exatos (o arredondamento por sessão, a sessão aberta, as janelas de 7 e 30 dias, quem saiu do projeto, projeto interno e projeto vazio) e a idade do projeto em dias, semanas e meses.
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
  adapter/                # clientes do GitHub, do GitLab e do Trello, o que cada tipo pede, e a criptografia do token
  config/                 # variáveis de ambiente
  database/               # conexão, arquivos de migração (migrations/) e quem os aplica
  domain/
    auth/                 # signup, login, sessões, convites, middlewares e acesso por organização
    organization/  customer/  person/  project/  team/  allocation/  collaborator/  task/  work_session/  integration/
                          # cada domínio com model, store (Ent), service e handler
    overview/             # visão geral do projeto: só lê os outros domínios, sem tabela nem store
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
Organization (1) ── (N) Project       sprint, daily e weekly são do projeto
Organization (1) ── (N) Person        email único no sistema, senha (bcrypt), papel admin|member, jornada semanal
Organization (1) ── (N) Invite        token (hash), email opcional, papel, expira em 7 dias, uso único
Person       (1) ── (N) Session       token (hash), expira em 7 dias
Project      (1) ── (N) Team ── (N) Person   via TeamMembership
Project      (1) ── (N) Allocation ── (1) Person   valor pago por hora, um por pessoa em cada projeto
Project      (1) ── (N) WorkSession   a sessão é do projeto e guarda o valor pago e o cobrado do clock-in
WorkSession  (1) ── (N) WorkSessionTask ── (1) Task   as tarefas da sessão, cada uma com o intervalo em que esteve nela
Project      (1) ── (N) Integration   token criptografado (credentials) e metadata em claro
Task      (0..1) ── (0..1) Integration  via external_integration_id
```

Uma pessoa entra num projeto pelo **valor por hora** (`Allocation`), que é obrigatório e libera o ponto: colaborador do projeto é quem tem valor nele, e não há outra tabela para isso. Só depois ela pode entrar num **time** (`TeamMembership`), que permite ser responsável por tarefas; a API recusa pôr num time quem não tem valor no projeto. Tirar alguém do projeto apaga o valor e os times de uma vez e mantém as tarefas e as sessões da pessoa. Não há como apagar só o valor.

Num banco de antes dessa regra pode haver alguém num time sem valor. Essa pessoa continua aparecendo na aba Colaboradores, com um aviso, até um admin definir o valor ou tirá-la do projeto.

Uma sessão pertence a um projeto e começa por uma tarefa dele, mas pode ganhar outras, antes ou depois de encerrada; a mesma tarefa pode entrar de novo, desde que não se sobreponha ao intervalo anterior. O tempo vale para a sessão e para cada tarefa dela: tarefas em paralelo contam o tempo cheio cada uma, e a pessoa, o projeto e os valores contam a sessão uma vez só. Excluir uma tarefa a tira das sessões, mas as sessões e as horas ficam. Excluir um projeto apaga os times, os valores, as tarefas, as sessões e as integrações dele. Excluir um cliente só é permitido quando nenhum projeto aponta para ele. Excluir uma organização só é permitido sem projetos, e apaga as pessoas, os clientes e os convites. O índice único parcial `one_active_session` em `work_sessions (person_id) WHERE end_at IS NULL` garante uma sessão aberta por pessoa.

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
