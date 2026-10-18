# Decisões de arquitetura

Este arquivo registra as decisões que deram forma ao sistema e o porquê de cada uma, com as alternativas que
ficaram de fora. Cada decisão está como foi tomada; quando o código mudou depois, uma nota **Hoje:** diz como está.
O passo a passo de cada mudança está em [history/sprints.md](../history/sprints.md).

## Contexto

O projeto começou do zero: sem código, banco ou API anteriores. O sistema registra o tempo trabalhado por tarefa
dentro de projetos, e as tarefas se ligam a pessoas, times e, opcionalmente, a itens externos (issues, cartões) por
integrações genéricas.

Há várias organizações (empresas), cada uma com vários projetos. Um projeto tem times de pessoas, duração de sprint,
horário da daily e da weekly. As pessoas se cadastram numa organização e entram nos times. Os projetos podem se
integrar a plataformas externas (GitHub, GitLab, Trello) para ligar tarefas a itens de lá.

O público são equipes que trabalham por projeto e querem saber o tempo por tarefa, o andamento do projeto, os prazos
e os itens ligados. O foco são empresas de desenvolvimento (software houses e consultorias) que atendem vários
clientes e alocam pessoas em um ou mais projetos; o seed de demonstração é uma delas. O modelo continua genérico:
nada no perfil da organização exige uma empresa de software.

## Objetivos e não-objetivos

**Objetivos:**
- Uma estrutura organizacional: organização → projetos → times → pessoas.
- Clock-in e clock-out como a peça central do registro de tempo, com uma sessão aberta por pessoa.
- Tarefas dentro dos projetos, com prazo padrão de uma semana e um responsável só.
- Integrações genéricas com plataformas externas, configuradas por projeto, sem prender o sistema a uma plataforma.
- Ligar tarefas a itens externos e buscar os detalhes deles na hora.
- Configuração por projeto: sprint, daily e weekly.
- Modelar como a organização recebe e como paga: o cliente contrata um projeto por um valor por hora, e cada pessoa
  tem um valor pago por hora em cada projeto. Os admins veem e mudam todos os valores; o membro vê só o dele.
  **Hoje:** o dinheiro de cobrança (cliente, valor cobrado, receita, margem) e os pagamentos são do dono
  (Decisão 17).
- Uma API REST mínima para todo o cadastro e o ponto, por organização e projeto.
- Uma interface web leve (HTML no servidor + Alpine.js) servida pelo próprio binário.

**Não-objetivos:**
- Colaboração em tempo real, SSE ou WebSocket.
- Um framework de SPA ou um build de front-end separado.
- Sincronização automática por webhook (os detalhes do item eram buscados na hora). **Hoje:** há sincronização nos
  dois sentidos com GitHub e Trello, por uma rotina de fundo e um gancho nas tarefas, ainda sem webhook (Decisão 5).
- Controle de acesso fino (bastavam os papéis membro e admin). **Hoje:** quatro cargos e permissões por projeto
  (Decisão 17).
- Folha de pagamento ou nota fiscal: o sistema guarda os valores por hora e mostra quanto vale cada sessão. A partir
  da regra de pagamento de cada pessoa ele calcula os períodos, mas não emite notas e não trata impostos (Decisão
  15). **Hoje:** registra a baixa de cada período pago, com o valor e um bônus (Decisão 18).
- Projetos de preço fechado e histórico de valores com data de vigência: a cobrança é só por hora, e mudar um valor
  vale a partir do próximo clock-in.
- Implantação em várias regiões ou com o banco particionado (um PostgreSQL só).

## Decisões

### Decisão 1: Stack — Go (Echo) + Ent + PostgreSQL + Alpine.js

**Escolha:** backend em Go com o Echo, PostgreSQL como banco e Ent como ORM (o projeto começou no GORM e passou
para o Ent). O mesmo servidor entrega o front-end, em templates renderizados no servidor com Alpine.js para a
interatividade, e o front-end consome a API JSON desse servidor.

**Motivo:** o Ent gera um cliente tipado a partir de `ent/schema`, sem SQL repetido e com o modelo em Go. As
mudanças de schema chegam ao banco por migrações versionadas (Decisão 9), não pelo ORM ao subir. O Echo dá
roteador, middlewares, estáticos e templates. O PostgreSQL dá horários confiáveis, restrições únicas e colunas JSON
para a configuração das integrações. O Alpine.js dá reatividade sem etapa de build.

**Alternativas:**
- Node.js + SQLite: mais simples para começar, mas com garantias de concorrência fracas e sem binário único.
- Python + Postgres: viável, mas com um runtime mais pesado.
- `database/sql` sem ORM: mais controle, mas mais lento de desenvolver para este modelo.
- Um SPA separado (React, Vue): mais poderoso, mas traz um build de JavaScript e um segundo artefato para entregar.

### Decisão 2: Modelo de dados — as entidades centrais

**Escolha:** sete entidades (`Organization`, `Project`, `Team`, `Person`, `Task`, `WorkSession`, `Integration`) e a
tabela de junção `TeamMembership`. Vieram depois `Customer` (o cliente; chama-se assim porque o Ent reserva
"Client"), `Allocation` (o valor pago por hora da pessoa em cada projeto), `WorkSessionTask` (Decisão 12),
`IssueSync` (o vínculo com um item externo), `Label`, `Invite` e `Session`.

- O **valor pago** fica na `Allocation`, e não em `TeamMembership`, porque uma pessoa pode estar em dois times do
  mesmo projeto. Há uma alocação por pessoa e projeto.
- O **valor cobrado** fica no projeto (`bill_rate_cents`), junto do cliente, e nunca aparece no JSON do projeto: é
  servido por `/billing`.
- A **sessão copia os dois valores no clock-in**, então mudar um valor depois não reescreve horas passadas.
- A **jornada semanal** é da pessoa, não do projeto: é o que a organização combina com alguém ao contratar,
  qualquer que seja o projeto.
- A **rotina do projeto** (daily, weekly com dia e horário, reunião semanal com o cliente) é opcional, campo a
  campo: nulo é "o projeto não tem". O horário de uma weekly ou de uma reunião não existe sem o dia, e a reunião com o
  cliente só existe num projeto com cliente.

Um índice único parcial garante uma sessão aberta por pessoa (Decisão 3).

**Motivo:** o modelo segue a hierarquia da organização. Os vínculos com itens externos ficam em `issue_syncs` (no
máximo um por tarefa e integração, então uma tarefa pode estar numa issue e num cartão ao mesmo tempo). O JSON de
`Integration.metadata` deixa cada plataforma ter os seus campos sem mudar o schema.

**Desdobramentos:**
- **O valor por hora é obrigatório para entrar no projeto.** Primeiro, a pessoa entrava no projeto de dois jeitos
  independentes (o valor, que libera o ponto, e o time, que deixa ser responsável). Isso deixava alguém num time,
  e portanto no projeto, sem conseguir bater o ponto. Agora a alocação é o que põe a pessoa no projeto: um time só
  aceita quem já tem valor, e sair do projeto apaga os dois numa transação. Não há como apagar só o valor.
- **Tarefa sem responsável.** O responsável é opcional: a tarefa sem ninguém fica disponível, e quem bate o ponto
  nela, ou a **pega** (`POST /api/tasks/:taskId/claim`), passa a ser o responsável. As duas coisas usam o mesmo
  `UPDATE ... WHERE assignee_id IS NULL`, então de dois pedidos simultâneos só um ganha (o outro recebe `409`
  `task.already_assigned`). Para pegar uma tarefa para si não é preciso estar num time.
- **Status da tarefa.** Um enum de quatro valores (`backlog`, `in_progress`, `awaiting_closure`, `closed`). A tarefa
  nasce sempre em backlog, e a troca é livre entre os quatro, sem ordem, porque os times pulam etapas e voltam
  atrás. O status muda sozinho num lugar só: o clock-in põe a tarefa em progresso, de qualquer status (voltar a
  trabalhar reabre a tarefa de propósito). O clock-out nunca mexe no status, e fechar uma tarefa não para a sessão.
- **Prioridade e etiquetas.** A prioridade é um enum (`urgent`, `high`, `medium`, `low`, `none`). As etiquetas são
  do projeto, só com nome (sem cor), únicas no projeto sem diferenciar maiúsculas; só quem tem `labels.manage` cria,
  renomeia e exclui.

**Alternativas:**
- Colunas na tarefa para um item externo: simples, mas a tarefa ficava presa a uma plataforma, e as colunas eram uma
  segunda verdade ao lado das linhas da sincronização.
- Tabelas por tipo de integração: cada tipo novo pediria uma migração.

**Hoje:** o modelo completo, com os cargos e as exclusões, está em [data-model.md](data-model.md).

### Decisão 3: Uma sessão aberta por pessoa — índice único parcial

**Escolha:** a regra "uma sessão aberta por pessoa" é garantida por um índice único parcial do PostgreSQL:
`CREATE UNIQUE INDEX one_active_session ON work_sessions (person_id) WHERE end_at IS NULL`, declarado no schema do
Ent e criado pela migração inicial.

**Motivo:** é uma garantia do banco, que vale mesmo com dois clock-ins ao mesmo tempo. O service confere antes, para
dar um erro amigável, mas o índice é a garantia de verdade. A regra é por pessoa, em toda a organização, qualquer
que seja o projeto.

**Alternativas:**
- Só uma trava ou checagem na aplicação: vulnerável a pedidos simultâneos.
- Uma coluna `currently_active_entry_id` na pessoa: estado duplicado, que pode divergir da verdade.

### Decisão 4: Hierarquia — Organização → Projeto → Time → Pessoa

**Escolha:**
- Uma pessoa pertence a uma organização só.
- Uma organização tem vários projetos; cada projeto é de uma organização só.
- Um projeto tem vários times; cada time é de um projeto só.
- Uma pessoa pode estar em vários times do mesmo projeto.
- As tarefas são do projeto. Uma tarefa pode não ter responsável (ver a Decisão 2). Um responsável escolhido à mão
  precisa estar em algum time do projeto.
- O ponto é do projeto: o clock-in pede o projeto.

**Motivo:** isolamento claro por organização. O projeto é o lugar natural da configuração (sprint, daily, weekly) e
das integrações. Os times agrupam sem duplicar dados.

**Alternativas:**
- Uma lista de projetos sem organizações: sem várias empresas no mesmo sistema.
- Pessoas direto nos projetos, sem times: perde o agrupamento.
- Organizações com filiais (pai e filho): exagero.

**Desdobramentos:**
- **Quem não é admin só vê os projetos em que está.** "Estar no projeto" é ter valor por hora nele, e a regra mora
  num lugar só, `internal/domain/projectaccess`. A checagem fica no `RequireOrg`, por onde já passa toda rota de
  projeto, time, tarefa e integração. Não estar no projeto responde o mesmo `404` de um recurso que não existe,
  antes de qualquer permissão, para um estranho não saber que o projeto existe. Ser tirado do projeto vale no pedido
  seguinte.
- **Quem vê quem.** Os admins, quem cuida de pessoas e quem põe gente em algum projeto veem todas as pessoas da
  organização. Os outros veem só a si mesmos e quem divide projeto com eles.
- **Tirar alguém do projeto encerra a sessão aberta dele ali**, na mesma transação, mantendo o tempo trabalhado: fora
  do projeto, a pessoa não alcança mais a rota de parar o ponto.

**Hoje:** o responsável escolhido à mão precisa estar num time, menos a própria pessoa que pede e o responsável
atual. Em dados antigos, quem estava num time sem valor não conta mais como do projeto.

### Decisão 5: Integrações genéricas

**Escolha:** uma entidade `Integration` genérica, configurada por projeto, no lugar da antiga conexão só com o
GitHub. Cada integração guarda:

- `type`: a plataforma (`github`, `gitlab`, `trello`);
- `display_name`: um nome para as pessoas;
- `credentials`: o token, criptografado (Decisão 6);
- `metadata`: os campos só daquela plataforma (o repositório, o projeto, a chave e o quadro do Trello), em claro;
- `enabled`: ligar e desligar sem apagar.

As tarefas se ligam a itens externos, um por integração (linhas de `issue_syncs`, que a tarefa devolve em `links`).
Ao ler a tarefa, o sistema busca os detalhes do item na plataforma, na hora; se ela não responde, a tarefa volta sem
os detalhes, sem quebrar. Os detalhes ficariam num cache em memória de curta duração.

Cada tipo implementa uma interface comum (`internal/adapter`): `Descriptor()` (o rótulo, os campos do `metadata` e
como o item se chama), `CheckMetadata` (confere e normaliza os campos sem falar com a plataforma), `Validate` (testa
a conexão) e `FetchItemDetails` (título, estado, URL). Um `SendNotification` ficou para o futuro.

**Estrutura comum:** todo tipo é criado e editado com o mesmo corpo, `{type, display_name, enabled, token,
metadata}`, e responde com o mesmo, com `has_token` no lugar do token. O que é comum fica no primeiro nível; o que é
da plataforma vai em `metadata`, que o adapter converte num tipo próprio antes de qualquer pedido. Um `PATCH` aceita
qualquer parte: o `token` vazio mantém o guardado, e a plataforma só é chamada de novo quando chegou um token ou o
`metadata` mudou de fato, então renomear ou desativar uma integração não depende de o token ainda valer. A tela
desenha o formulário a partir dos descritores (`window.BOOT.integration_types`), então um tipo novo não pede
JavaScript.

**Entrada que vai para a URL da plataforma:** o repositório, o quadro, o número da issue e o cartão vêm de uma pessoa
e acabam no caminho de uma chamada à API. Cada adapter confere o formato antes, para ninguém apontar o pedido para
outro recurso que o token enxerga.

**Motivo:** desacopla o núcleo de qualquer plataforma. Um tipo novo é implementar a interface e registrá-lo, sem
mudar schema, regras nem tela. Buscar na hora evita dados velhos e a estrutura de webhooks.

**Alternativas:**
- Sincronizar por webhook: mais rápido, mas pede um endpoint, segredos e o cadastro do webhook em cada plataforma. E
  o servidor nem sempre é alcançável de fora.
- Guardar os detalhes e atualizá-los de tempos em tempos: uma rotina de fundo cedo demais.
- Colunas `config_github`, `config_gitlab`: cada plataforma nova pediria uma migração.

**Desdobramentos:**
- **Conectar com o GitHub por autorização.** Colar um token pessoal era o passo errado para quem talvez não tenha
  um. O GitHub se conecta autorizando em github.com e voltando, por redirecionamento (não por popup). O `state`, a
  pessoa, o projeto e, ao reconectar, a integração vão selados num cookie curto (AES-GCM com a chave das
  integrações). Na volta, o servidor confere tudo, troca o código pelo token e **cria a integração na hora,
  desativada e sem repositório**; o token vai direto para a credencial criptografada e nunca chega ao navegador.
  Escolher o repositório é o `PATCH` de sempre. O mesmo fluxo **reconecta** uma integração (só a credencial muda).
  O escopo `repo` é o único que um OAuth App tem para repositórios privados, e ele também permite escrever; um
  GitHub App, de permissões finas, é a saída se isso pesar.
- **Sincronizar as issues com as tarefas, nos dois sentidos.** Toda issue aberta vira uma tarefa, e as duas ficam
  iguais (título, corpo, etiquetas, responsável e o fechamento). Projetos, milestone e comentários ficam de fora; a
  prioridade, as horas e os status intermediários ficam só aqui.
  - **Snapshot do último acordo.** Cada item guarda o último estado em que os dois lados concordavam. É uma
    comparação de três vias, o único jeito de saber "o GitHub mudou" e "a tarefa mudou" sem um `updated_at` nas
    tarefas. A regra pura é `issuesync/merge.go`, testada em tabelas.
  - **Em conflito, vence a plataforma.** Pediu-se sincronização, não uma ferramenta de merge. As etiquetas não
    conflitam: cada lado soma e tira o que fez.
  - **O responsável se liga pelo e-mail público** do usuário no GitHub, dentro da organização e do projeto. Só o
    login ligado é mexido, então os responsáveis da issue que o sistema não reconhece ficam como estão.
  - **Gatilhos.** O botão Sincronizar agora (uma rodada completa), a rotina de fundo (incremental, com uma rodada
    completa a cada hora, espera crescente quando o token é recusado ou o limite acaba) e um gancho no store das
    tarefas, que só anota numa fila: um único worker junta os pedidos (1,5 s) e empurra para a plataforma. Sem
    webhook, porque o GitHub não alcança um servidor local.
  - **Escrita defensiva.** O cliente não segue redirecionamentos (um 301 viraria um GET que "dá certo" sem gravar),
    cria a etiqueta antes de usá-la, relê a issue antes de empurrar e compara a resposta com o que mandou. Um campo
    que o GitHub aceitou com 200 e não gravou (token sem escrita) fica registrado e não é repetido a cada rodada. Uma
    rodada sem diferença não escreve nada, e os testes contam os pedidos para provar.
  - **Cursor.** A rodada incremental pede o que mudou desde o início da anterior menos 2 minutos, e o cursor só anda
    quando todas as páginas chegaram; tudo o que a rodada escreve pode ser repetido.
  - **Limites.** A descrição da tarefa vai até 65.536 caracteres e a etiqueta até 50, os limites do GitHub, para a
    importação não cortar nada.
  - **Testes sem internet.** Os testes usam um GitHub falso com estado, e o transporte HTTP dos testes recusa
    qualquer endereço fora da máquina.
  - **Forma do servidor.** `server.Build` devolve o servidor e o `Syncer`; o `main` roda `Sync.Run` ao lado e espera
    por ele antes de sair.
- **Sincronizar uma tarefa e as listas.** A página da tarefa tem um cartão por item, com o botão Sincronizar, que
  relê só aquele item pelas mesmas regras; as listas de tarefas têm um Sincronizar que faz a rodada completa das
  integrações do projeto. Os dois são de qualquer pessoa do projeto, porque quem edita uma tarefa já escreve na
  plataforma pelo gancho; a trava por integração continua deixando uma rodada por vez.
- **Postar uma tarefa nova como issue.** O modal Nova tarefa ganhou a etapa Integrações. A tarefa é criada primeiro
  e postada depois (`POST /api/tasks/:taskId/publish`), então uma falha deixa a tarefa e um aviso, em vez de perder
  o formulário. Um token que não escreve é recusado: o GitHub abriria a issue sem as etiquetas e o responsável.
- **Um núcleo para várias plataformas.** A chave do item virou texto (o número da issue, o link curto do cartão), e o
  que um tipo faz além do básico é declarado em `Descriptor.Caps`: espelhar o prazo, o responsável, filtrar por data
  no servidor e postar sozinho. Um campo `Internal` do `metadata` é guardado pela conexão e não muda na edição.
- **Trello.** Um quadro por integração e um cartão por tarefa, com as diferenças que a plataforma impõe: a chave é o
  link curto do cartão; as listas ficam de fora (a tarefa criada aqui vai para a primeira lista aberta); o cartão
  arquivado ou com a entrega marcada como concluída fecha a tarefa; o prazo é a data de entrega; o responsável fica
  de fora (o Trello não mostra o e-mail dos membros). O Trello não filtra por data, então toda rodada é completa.
  A conexão é o fluxo implícito do Trello: o token volta no fragmento da URL, que o servidor não vê, então uma página
  de retorno o lê e o entrega por `POST`, com as mesmas conferências do GitHub. A chave do app fica guardada em cada
  integração, porque o token pertence à chave que o gerou: trocar a chave no `.env` não quebra o que existe.
- **Uma tarefa criada aqui vira cartão sozinha.** Um segundo gancho, de criação, faz o worker postar a tarefa na
  integração que declara `AutoPublish`. Criar a tarefa nunca espera o Trello. O que a plataforma recusa por si fica
  registrado e é descartado; o que é temporário deixa a tarefa esperando, para até dez tentativas. A fila é da
  memória: um reinício esquece as que esperavam.
- **Um intervalo por plataforma.** `SYNC_INTERVAL` é o padrão, e `GITHUB_SYNC_INTERVAL` e `TRELLO_SYNC_INTERVAL`
  valem só para a delas. A rotina acorda no menor intervalo e olha cada integração quando passou o do tipo dela.
- **Vários itens numa tarefa.** `issue_syncs` passou a ser o **único** lugar do vínculo: as colunas `external_*` da
  tarefa saíram, e o índice único virou `(task_id, integration_id)`. A tarefa é o centro: uma mudança numa
  plataforma chega à tarefa na rodada dela, e o gancho a leva ao outro item em segundos. Duas rodadas de integrações
  diferentes podem escrever a mesma tarefa, então há uma trava por tarefa, sempre depois da trava da integração. Um
  vínculo feito à mão nasce `pending` e é adotado pela primeira rodada em que o item está aberto.
- **O empurrão que espera.** Uma edição numa tarefa cuja integração está em espera (token recusado, limite) não se
  perde: fica guardada e volta à fila quando a espera acaba, e um Sincronizar que dá certo acaba a espera na hora.
- **Criar nos dois lugares e espelhar.** No modal Nova tarefa as caixas das plataformas não se excluem. O GitHub é
  postado pela tela; o Trello, pelo servidor, e desmarcá-lo manda `skip_publish` na criação. Na página da tarefa,
  cada integração em que a tarefa ainda não está oferece Publicar, para uma issue virar cartão e o contrário. A
  oferta é manual e por tarefa: postar tudo de um quadro interno num repositório talvez público, sem ninguém
  escolher, ficou de fora.
- **Excluir também nas plataformas.** O modal de exclusão tem uma caixa por plataforma, todas desmarcadas. A API
  REST do GitHub não apaga issue: só a mutação `deleteIssue` do GraphQL, e só para admin do repositório; sem isso a
  issue é fechada como não planejada, e o aviso diz. O Trello arquiva o cartão. A ordem é ler os vínculos, excluir a
  tarefa e só então chamar as plataformas, uma a uma: uma falha lá nunca traz a tarefa de volta.

**Hoje:** os detalhes do item são buscados a cada leitura, sem o cache de curta duração; o único cache é o da
sincronização (o e-mail público e o login no GitHub). O `SendNotification` não existe. O Trello é oferecido; o
GitLab tem o adapter pronto, mas segue "Em breve" (`Descriptor.ComingSoon`), e a API recusa criar uma integração
nova dele. O sistema escreve nas plataformas, com a conta de quem conectou. O funcionamento de hoje está em
[integrations/overview.md](../integrations/overview.md).

### Decisão 6: Credenciais — criptografia em repouso

**Escolha:** o token da integração fica criptografado na coluna `credentials`, com AES-GCM e a chave de
`INTEGRATION_ENCRYPTION_KEY`. Ele nunca volta numa resposta nem vai para o log; a API mostra só `enabled` e
`has_token`. O `metadata` não é segredo: fica em claro e volta nas respostas, e cada tipo só guarda os campos que
declara, então um segredo mandado ali por engano, com outro nome, é descartado.

**Motivo:** criptografia simétrica é simples para um serviço de uma instância e protege os segredos se o banco
vazar. Primeiro a configuração inteira era criptografada; com um token só para todos os tipos e o resto em
`metadata`, só o token precisa disso, e o resto pode ser mostrado na edição e sobrevive a uma troca de chave.

**Alternativas:**
- OAuth com refresh token: mais seguro, mas complexo demais para cada plataforma no começo. **Hoje:** o GitHub e o
  Trello se conectam por autorização; o token de um OAuth App do GitHub não vence, então cabe no mesmo token
  criptografado, sem refresh token nem mudança de schema.
- Guardar em texto puro: inaceitável.
- Criptografia por campo: acopla o schema aos tipos de integração.
- Marcar campos do `metadata` como secretos: nenhum tipo precisa de um segundo segredo hoje.

### Decisão 7: API — REST com JSON, por organização e projeto

**Escolha:** rotas REST com JSON, agrupadas por recurso, debaixo da organização e do projeto: `/api/orgs/:orgId/...`,
`/api/projects/:projectId/...`, e o recurso pelo próprio id quando ele já diz de quem é (`/api/tasks/:taskId`,
`/api/teams/:teamId`, `/api/integrations/:integrationId`).

**Motivo:** a hierarquia da URL segue o modelo, o que deixa simples as permissões e os filtros. O JSON serve tanto
ao Alpine.js quanto a scripts. A API continua sendo a fonte da verdade, e a interface é só mais um cliente dela.

**Hoje:** a lista de rotas que estava aqui envelheceu. A referência completa está em
[api/reference.md](../api/reference.md), e as convenções, em [api/overview.md](../api/overview.md).

### Decisão 8: Front-end — HTML renderizado no servidor + Alpine.js, servido pelo Echo

**Escolha:** o Echo renderiza templates (`html/template`) para cada tela, em rotas próprias do navegador. Os
templates carregam o Alpine.js de um arquivo vendorizado, e os componentes Alpine chamam a API JSON com `fetch` para
criar, mudar, bater o ponto e atualizar o que mudou. Templates e estáticos ficam embutidos no binário (`embed.FS`).

**Motivo:** um binário só, sem build de JavaScript. O Alpine dá conta dos formulários, das listas, do botão de ponto
e do cronômetro da sessão. A API continua sendo a fonte da verdade.

**Alternativas:**
- HTMX no lugar do Alpine: também sem build; o Alpine ganhou pelo estado no navegador (o cronômetro, a validação na
  hora, os formulários em etapas).
- Formulários puros, recarregando a página: mais simples, mas pior para o ponto e os totais ao vivo.
- SPA separado: recusado na Decisão 1.

**Desdobramentos** (os padrões de tela que ficaram; o detalhe de cada um está em
[frontend/patterns.md](../frontend/patterns.md)):
- **Ícones pelo CDN.** São a única exceção a "tudo vendorizado": o Font Awesome Free vem do cdnjs com Subresource
  Integrity. Sem internet os ícones somem, e todo controle mantém o texto, então a tela continua usável.
- **Um modal só**, no layout base, com Alpine e CSS, sem `<dialog>` nem plugin. As páginas entregam o conteúdo por
  `x-teleport`, e ele continua no escopo do componente da página. Com o modal aberto, o resto da página fica `inert`.
- **Modais editam um rascunho.** Nada vai ao servidor antes de Salvar. Um salvamento que faz vários pedidos não é
  atômico, mas cada passo deixa um estado válido, e salvar de novo manda só o que falta.
- **Listas no servidor quando crescem.** A lista de tarefas filtra e pagina no servidor e guarda a busca, os filtros
  e a página na URL. Ela tem duas listas: as sem responsável, para o time pegar, e as de outras pessoas (o filtro
  `assignee_id=others` sai da sessão, então a mesma chamada responde diferente a cada pessoa). As listas de pessoas
  mostram cinco por vez, paginadas no navegador, porque a tela já carregou todos.
- **Linha e cartão clicáveis.** A linha inteira de uma tabela abre o item, sem botão Detalhes; o nome continua um
  link de verdade, para o teclado e para abrir em outra aba.
- **Cores com significado e com moderação.** Cada prioridade e cada status tem a sua cor, um par de tokens para o
  tema claro e outro para o escuro, com contraste de 4,5:1, e um teste falha se faltar cor ou se duas se
  repetirem. Os cartões de projeto ficam neutros, com a cor só num toque.
- **A área de Gestão.** As abas que só quem cuida do projeto usa (visão geral, colaboradores, integrações,
  configurações) ficam numa área própria do projeto. As da organização são uma página por assunto, sem sub-abas, e
  o que é da conta (perfil, ajuda, idioma, sair) fica num menu no nome da pessoa.
- **A página da tarefa é uma tela própria**, sem as abas do projeto. A edição fica em três lugares, cada um com o que
  é dele (o nome e a descrição, os detalhes, a exclusão). Os detalhes vão por `PATCH /api/tasks/:taskId/attributes`,
  uma atualização parcial: o `PATCH` da tarefa reescreve a linha inteira, e uma edição rápida por ele desfaria uma
  tarefa que alguém acabou de pegar.
- **Visões gerais calculadas no servidor**, num domínio sem tabela (`overview`) que lê os outros. No navegador
  seriam vários pedidos e todas as sessões do projeto para mostrar uma dúzia de números, e as regras ficariam sem
  teste. Os totais somam o valor de cada sessão já arredondado; uma janela de 7 ou 30 dias **corta** a sessão que
  atravessa a borda e conta só o trecho de dentro.
- **Não comparar pessoas por horas.** Comparar tempo dá a entender que quem trabalhou mais merece mais
  reconhecimento, e o valor entregue nem sempre vem das horas. A equipe na primeira tela mostra quem está
  trabalhando agora e em quê, sem tempo. O bloco antigo de horas ficou guardado no código, sem uso, e um teste
  impede que volte sem alguém decidir.
- **Quem trabalha agora** é uma rota leve (`GET /api/orgs/:orgId/working-now`), sem tempo nem dinheiro, lida por um
  store do Alpine a cada 30 segundos com a aba à vista: um pedido por página, não um por linha.
- **O painel da pessoa** usa rotas próprias (`/me/overview`, `/me/tasks`), sem parâmetro de pessoa. Hoje e a semana
  dependem de onde a pessoa está, então o navegador manda o fuso dele. A tarefa não guarda quando foi fechada, então
  "fechadas nesta semana" não existe.
- **O balão da sessão** acompanha todas as páginas, no canto de baixo: a sessão saiu da barra superior, que ela
  alargava demais no celular.
- **O perfil é a mesma tela** para quem o abre: o meu e o de um colaborador. É só para ler, e editar abre um modal.
- **A descrição é Markdown**, renderizado só no navegador: a API guarda texto puro. O marked e o DOMPurify são
  vendorizados, com hash conferido por teste, e não há Content-Security-Policy, então o DOMPurify é a única defesa
  contra XSS ali.
- **Sessões por permissão.** Quem não cuida do projeto só vê as próprias sessões, e isso vale no servidor.

### Decisão 9: Mudanças de schema — migrações versionadas

**Escolha:** o schema muda só por arquivos SQL versionados em `internal/database/migrations`. Eles vão embutidos no
binário e são aplicados em ordem ao subir, pelo goose (no servidor e no seed), cada arquivo numa transação, sob uma
trava do banco. Os arquivos são gerados a partir de `ent/schema` por `go run ./cmd/migrate new <nome>`: ele aplica os
arquivos existentes num banco descartável, do mesmo jeito que o servidor, deixa o Atlas comparar o resultado com o
schema do Ent e escreve a diferença. Uma pessoa revisa o arquivo, e o edita quando a mudança pede um rename ou um
preenchimento de dados, antes de commitar. As migrações só andam para frente. Um banco com tabelas e sem histórico é
recusado, não adotado.

**Motivo:** antes, o servidor rodava a migração automática do Ent a cada subida, com remoção de coluna ligada. Um
campo renomeado virava `DROP COLUMN` + `ADD COLUMN`, e um valor que mudava de tabela se perdia; aconteceu quando a
jornada semanal passou da organização para o projeto. Isso não serve para um banco que guarda dinheiro. Com
arquivos, toda instrução destrutiva é escrita, revisada e commitada, e o preenchimento fica ao lado do DDL que o
pede. Um teste de deriva (depois das migrações, a diferença do Ent precisa ser vazia) impede que os dois divirjam.

**Alternativas:**
- A migração automática sem remoções: para a perda de dados, mas renames e preenchimentos continuam impossíveis, e
  colunas mortas se acumulam.
- Aplicar só por um comando: o certo com um pipeline de deploy, que não existe; um binário que se migra mantém a
  instalação num passo.
- golang-migrate: pares up/down e um estado "sujo" para limpar à mão; o goose roda cada arquivo numa transação.
- Atlas CLI para gerar e aplicar: aplicar ficaria fora do binário.
- Adotar bancos existentes marcando a base como aplicada: só havia bancos locais, e recriá-los é mais simples.

O passo a passo está em [development/workflow.md](../development/workflow.md).

### Decisão 10: Idiomas — catálogos YAML, idioma num cookie

**Escolha:** a interface fala português do Brasil (o padrão) e inglês. Todo texto que a pessoa lê mora num catálogo
YAML por idioma (`internal/i18n/locales/<idioma>.yaml`, embutido no binário); o código só conhece as chaves. O
servidor os lê com o go-i18n v2 e escolhe o idioma pelo cookie `wtt_lang`, depois pelo `Accept-Language`, depois o
padrão. Os templates usam `{{.T "chave"}}`. O navegador recebe o mesmo YAML como `window.I18N`
(`GET /i18n/<idioma>.js`, com o hash na URL para o cache) e o lê com um `WTT.t` / `$t` de umas 40 linhas, com
`Intl.PluralRules` para o plural. O botão de idioma é um link para `/lang/<código>?next=<página>`.

**Motivo:** as páginas são cascas do servidor com componentes Alpine, então o texto vive em dois lugares; um
catálogo serve aos dois, e um teste mantém os idiomas iguais. O go-i18n dá o plural do CLDR e os placeholders; o
navegador não precisa de biblioteca, porque só procura chaves. Um cookie funciona nas telas sem sessão, e um link
não precisa de JavaScript.

**Regras:** uma chave por frase inteira; valores por placeholder; plural com `one` e `other`. Dentro de uma expressão
Alpine o texto vem sempre de `$t`, nunca de `{{.T}}`: o `html/template` escapa HTML, não JavaScript, e um apóstrofo
em inglês quebraria a expressão. O conteúdo das pessoas e o seed não são traduzidos. O detalhe está em
[development/i18n-and-countries.md](../development/i18n-and-countries.md).

**Alternativas:**
- O idioma na pessoa (uma coluna): segue a pessoa entre aparelhos, mas não ajuda as telas sem sessão.
- `golang.org/x/text/message` com geração de código: um build e catálogos que não são YAML.
- Uma biblioteca de i18n no JavaScript: um segundo formato para o que é uma busca por chave.
- Só o `Accept-Language`, sem botão: a pessoa não escolhe.

### Decisão 11: Erros da API — códigos estáveis, o texto no cliente

**Escolha:** a API não manda mensagem de erro. Toda resposta de erro é `{"error": {"code": "dominio.motivo",
"params": {...}}}` com o status HTTP de sempre. Os códigos são declarados junto do domínio que os levanta
(`apperr.New(...)` em cada `errors.go`, com os nomes dos parâmetros que o texto usa), e todo handler responde por uma
função só, `apperr.Respond`. Um registro que não existe vira `request.not_found` (`404`), e um erro sem código vira
`internal.server_error` (`500`), com o detalhe só no log. O navegador monta a mensagem a partir do mesmo catálogo
YAML (`errors.<código>`). Os códigos estão em [api/errors.md](../api/errors.md), gerado por um teste que falha
quando o arquivo está velho.

**Motivo:** uma mensagem do servidor fixa o idioma da resposta. Um código é um contrato em que outro cliente pode
confiar e traduzir sozinho, e põe todo o texto nos catálogos, onde os testes mantêm os idiomas iguais.

**Alternativas:**
- Traduzir no servidor pelo `Accept-Language`: todo cliente dependeria do texto do servidor.
- Mandar o código e uma mensagem: a mensagem ainda teria um idioma só e tentaria os clientes a compará-la.
- Um código por campo nos erros de tamanho: multiplica códigos que só mudam de substantivo.

### Decisão 12: A sessão tem tarefas ao longo do tempo — WorkSessionTask

**Escolha:** uma `WorkSession` é de um projeto e tem uma ou mais tarefas por `work_session_tasks`, cada uma com o
intervalo em que esteve na sessão (`from_at`, `until_at`; nulo é "até o fim da sessão"). O clock-in cria a sessão e a
primeira tarefa numa transação; outras tarefas entram numa sessão aberta ou fechada, e a mesma tarefa pode voltar
desde que os intervalos não se sobreponham. Tarefas de projetos diferentes não dividem uma sessão, porque os valores
congelados nela vêm da alocação da pessoa num projeto.

**Motivo:** as pessoas trabalham em várias tarefas numa mesma sentada, às vezes em paralelo. O tempo vale para a
sessão e para cada tarefa: a pessoa, o projeto e os valores contam a sessão uma vez, e cada tarefa conta o tempo em
que esteve nela (tarefas em paralelo contam a sobreposição inteira cada uma). Por isso a visão por tarefa não soma, e
nada a soma entre tarefas. Mudar as tarefas de uma sessão não muda o tempo nem os valores, então não pede permissão
própria: quem bateu o ponto e os admins podem.

**Alternativas:**
- Fatias por tarefa (trocar de tarefa fecha uma e abre outra): exato e somável, mas não descreve trabalho em
  paralelo.
- Dividir o tempo à mão ao parar: um passo a mais em todo clock-out.
- Sessões entre projetos: pediria um valor por projeto dentro da sessão.

**Consequências:** excluir uma tarefa a tira das sessões; as sessões e as horas ficam, e uma sessão pode ficar sem
tarefa. Uma tarefa pode sair da sessão aberta sozinha (`{"stop": true}` no intervalo dela), mas a última tarefa em
andamento não sai (`work_session.last_task`: uma sessão aberta sempre tem uma), e a tela, nesse caso, para o ponto.
Editar o início e o fim de uma sessão ainda não existe.

### Decisão 13: Login — o Clerk diz quem é, a sessão é nossa

**Escolha:** com `CLERK_SECRET_KEY` e `CLERK_PUBLISHABLE_KEY`, o login, o cadastro e a página do convite oferecem
dois jeitos de entrar: o e-mail e a senha do sistema, em cima, e o Google, abaixo, que é o Clerk. O e-mail e a senha
do próprio Clerk não são oferecidos. Sem as chaves, nada muda. O Clerk só diz quem a pessoa é: depois de entrar lá,
`/auth/clerk/continue` manda o token de sessão do Clerk (um JWT de 60 segundos) ao servidor, que confere a
assinatura, o prazo e a origem, lê o usuário para pegar o e-mail **primário e verificado** e abre a **sessão do
sistema** de sempre. Tudo o que vem depois não sabe que o Clerk existe. A pessoa ganha `clerk_user_id` (único,
opcional). O lado do Clerk fica atrás de `adapter.ClerkProvider`, então os testes usam um Clerk falso.

Como um usuário do Clerk vira uma pessoa: já ligado, entra. Uma conta com o mesmo e-mail verificado e sem senha é
ligada. Uma **com senha** pede a senha dela uma vez, e só então liga. Sem conta, a tela mostra os convites pendentes
do e-mail, para a pessoa escolher, ou pede o nome da organização; nada é aceito sozinho. O link de um convite entra
direto, desde que o e-mail confira. Uma conta ligada a outro usuário do Clerk que ainda existe é recusada.

**Motivo:** o modelo é uma pessoa por organização com e-mail único, e os convites e as permissões já existiam; as
Organizations do Clerk não cabem nisso. Manter a nossa sessão evita o clerk-js nas páginas internas, um token de 60
segundos para renovar a cada pedido e mudanças nos middlewares. A senha na ligação fecha um buraco real: o cadastro
por senha não verifica o e-mail.

**Alternativas:**
- Aceitar o cookie `__session` do Clerk no `LoadSession`: toda página precisaria do clerk-js, e o middleware
  dependeria da rede a cada pedido.
- As Organizations do Clerk como fonte das organizações e papéis: uma segunda verdade.
- Trocar o login por senha pelo Clerk: levaria as contas que já existem e os cadastros dos testes.
- Ligar só pelo e-mail: sem passo a mais, e com o buraco acima.

**Consequências:** o e-mail não é sincronizado entre o Clerk e o sistema. Uma sessão nossa não cai quando o usuário
é bloqueado ou apagado no Clerk, e uma que venceu volta sozinha enquanto o Clerk estiver logado. Sair leva a
`/login?out=1`, que também desconecta do Clerk. Os cookies não são separados por porta, então uma segunda instância
na mesma máquina divide o cookie com a primeira e precisa estar em `CLERK_AUTHORIZED_PARTIES`.

**Desdobramentos:**
- **Convite por e-mail pelo Clerk.** Usa o convite de aplicação do Clerk (que tem o `notify`), não o de organização.
  O Clerk é só quem entrega o link: o servidor cria o convite lá e só então grava o daqui; uma falha lá é `502` e
  nada fica gravado. Convidar o mesmo e-mail de novo substitui o anterior, e revogar cancela lá também. O
  `__clerk_ticket` que o Clerk põe na URL **não é usado**: sai da URL antes de o Clerk carregar, porque abriria uma
  conta do lado do Clerk com código por e-mail, um jeito de entrar que o sistema não oferece.
  `INVITE_DELIVERY=terminal` não manda o e-mail e escreve o link no log, para desenvolver.
- **Convite que leva a um projeto.** O convite pode levar o projeto, o valor por hora, o time e o grupo
  (`POST /api/projects/:projectId/invites`, com as mesmas regras de pôr alguém no projeto). Ao aceitar, o `auth`
  cria a conta e chama um `ProjectApplier`, que põe a pessoa no projeto. Isso roda depois de a conta existir, fora
  da transação dela: uma falha vai para o log e não desfaz uma conta que já abriu sessão.

### Decisão 14: Dados por país — uma tabela, um cadastro de países

**Escolha:** a organização tem um `country` (`BR` ou `US`), e o que o perfil pede depende dele: o documento fiscal
(`cnpj` no Brasil, `ein` nos EUA), os textos das duas linhas do endereço, e a moeda e o fuso de partida de uma
organização nova. A tabela continua **uma**, com todas as colunas, e as de outro país ficam vazias. As regras moram
num cadastro, `internal/country`, que o navegador recebe em `window.BOOT.countries`, então rótulos, máscaras e a
validação do servidor saem da mesma entrada. O cliente tem país **próprio** (ISO 3166-1, começando no da
organização), e o documento dele é conferido por esse país: CNPJ, EIN, ou um texto livre curto num país sem regra.

**Motivo:** o primeiro perfil era do Brasil, e uma empresa dos EUA não tinha o EIN. Uma coluna por campo de país
mantém os dados tipados e o `PATCH` parcial, e o cadastro evita que um terceiro país vire `if` espalhado.

**Alternativas:**
- Uma coluna `tax_id` genérica com o tipo: renomearia o `cnpj` para todo cliente da API.
- Uma coluna JSON por país: sem tipos nem checagem no banco.
- O documento do cliente seguir o país da organização: uma empresa brasileira não cadastraria o EIN de um cliente
  americano.

**Consequências:** trocar o país da organização não muda a moeda nem o fuso, e não apaga o documento do outro país
(ele fica guardado e escondido). O perfil perdeu segmento, porte, ano de fundação, telefone, Instagram, cidade,
estado e CEP: ninguém usava. `Parse` ainda entende as grafias antigas ("Brasil", "EUA").

### Decisão 15: Pagamentos — uma regra por pessoa, períodos calculados, nada gravado

**Escolha:** a pessoa tem uma **regra de pagamento** opcional: `monthly` com um dia do mês (1 a 31) ou `biweekly`
com uma data de início (`payment_start`, um dia de calendário sem fuso). O pacote `payment` não tem tabela: da regra
e das sessões ele calcula o período corrente, as próximas datas e os períodos fechados, com horas e valor.

- **O período fecha no dia do pagamento.** Mensal no dia D paga em `min(D, último dia do mês)`; quinzenal a partir
  de S tem períodos `[S+15k, S+15(k+1))`, pagos no último dia, e nada antes de S.
- **As bordas são meia-noite no fuso da organização**, calculadas com `AddDate`, nunca com somas de 24 horas, então
  o horário de verão não desloca nada. A API manda os períodos como dias de calendário, nunca como instantes.
- **As horas e o valor reaproveitam a sessão**: o valor congelado no clock-in e o arredondamento ao centavo. Uma
  sessão que atravessa dois períodos é cortada e arredondada em cada um, e a soma dos pedaços pode diferir em um
  centavo da sessão inteira (aceito). A sessão aberta conta até agora.
- **O dono tem horas e não tem valor.**
- A resposta traz também `lifetime`: tudo o que a pessoa registrou desde a primeira sessão, com ou sem regra.

**Motivo:** o que se pediu foi saber quando cada pessoa recebe e quanto rendeu o período, não rodar uma folha.
Calcular das sessões não pede tabela, nada para manter em sincronia e nada a errar quando as horas mudam.

**Consequências:** o histórico é uma visão das sessões, não um registro: excluir um projeto apaga as sessões dele e
os períodos passados encolhem; trocar a regra recalcula onde os períodos caem. Não há status de pago. **A visão da
equipe mostra horas por pessoa**, o que a primeira tela de propósito não faz: quem paga precisa das horas de cada
um. Para respeitar o motivo daquela regra, a tabela segue a data de pagamento e depois o nome, sem barras, ranking
nem destaque por horas, e os testes travam a ordem.

**Hoje:** a regra é definida e lida só pelo dono e pela própria pessoa; os pagamentos de uma pessoa são dela e do
dono (um admin vê só as horas), e a visão da equipe é só do dono (Decisão 17). Os pagamentos de cada um ficam no
perfil. Os períodos continuam calculados, mas o pagamento feito passou a ser registrado (Decisão 18).

### Decisão 16: Validação — uma regra por campo, na tela e no servidor, com o erro embaixo do campo certo

**Escolha:** cada campo de texto tem **uma regra**, escrita uma vez e lida pelos dois lados. Os tetos ficam em
`internal/validate/limits.go`; o servidor confere por eles, os templates tiram o `maxlength` da função `limit`, e o
JavaScript, de `window.BOOT.limits` (um teste falha se um `maxlength="N"` literal voltar). Um erro de validação
**diz qual campo falhou**: o parâmetro `field` é a chave JSON do corpo, e todo campo tem um rótulo em `fields.*` nos
dois idiomas. Na tela, `WTT.form()` leva o erro ao campo, o primeiro campo com erro ganha o foco, e o erro some
quando a pessoa digita. Todo formulário tem `novalidate`: quem confere é o sistema, com as mesmas regras e no idioma
da página. O campo opcional diz "(opcional)" no rótulo e, preenchido, é validado. O corpo JSON é estrito: chave
desconhecida e corpo acima de 1 MiB são recusados.

**Motivo:** cada formulário tinha a sua validação, e o banco não garante nada além de `NOT NULL`, enums, únicos e
chaves estrangeiras: toda a defesa está no Go. A mesma regra dos dois lados impede a tela de aceitar o que o servidor
recusa.

**Consequências:** um campo novo pede um limite, um rótulo `fields.*` nos dois idiomas e o `data-field` com o
`field_error` na tela. Alguns conflitos passaram de `400` para `409`. As regras de dinheiro entraram aqui: um valor
por hora (pago ou cobrado) tem o piso de 10,00, zero é recusado menos para o dono, e um projeto ou tem cliente com
valor cobrado ou é interno sem os dois, nos dois sentidos.

**Hoje:** o admin cria só projetos internos; criar com cliente ou valor cobrado é do dono (`auth.owner_only`), e só
o dono é matriculado no projeto que cria (o admin já vê todos).

### Decisão 17: Cargos e dinheiro — o cargo dá as permissões da organização, e o dinheiro tem eixo próprio

**Contexto:** primeiro, o **dono** era um admin marcado com `persons.is_owner` (um por organização, por índice único
parcial), cujas horas valem o valor cobrado, sem custo: o que ele tira do projeto é a margem. Depois vieram as
**permissões por projeto e grupo**: cada pessoa, numa alocação, recebe um grupo (`allocations.preset`) com uma lista
de permissões do projeto, e a organização tinha três permissões dadas pessoa a pessoa. Mas `role: admin` ainda era
um atalho para "pode tudo", e a cobrança, a receita e os pagamentos ficavam ao alcance de qualquer admin. Rever o
fluxo organização → colaborador → cliente → projeto → colaborador no projeto → tarefa mostrou que "quem cuida de
gente e de projeto" e "quem vê o dinheiro" são perguntas diferentes.

**Decisão:** há quatro cargos: o **dono** (tudo, inclusive o dinheiro), o **admin** (cria projetos internos, convida
pessoas e administra todos os projetos, sem clientes, cobrança nem pagamentos), o **administrador de projeto** (o
grupo `manager` num projeto) e o **colaborador**. São dois eixos: a gestão é uma hierarquia, e o dinheiro (custo e
cobrança) não segue o cargo. Hoje o dono o concentra; um cargo Financeiro fica para depois, junto com o custo de
infraestrutura. O time é só organização visual e não dá autoridade.

- As permissões da organização decorrem do cargo (`permission.OrgKeysFor`) e não são gravadas. `payments.manage` é
  nova e só do dono, como `customers.manage`.
- "Vê todos os projetos" (`IsAdmin()`) ficou separado de "tem todas as permissões": `auth.BaseProjectSet` é o único
  lugar que diz o que o cargo dá num projeto (tudo para o dono; `permission.AdminProjectKeys`, tudo menos
  `billing.*`, para o admin; nada para os outros).
- Ficam com o dono: editar a organização, a página de pagamentos da equipe, a regra de pagamento de uma pessoa e os
  clientes. As visões gerais vão ao admin e ao administrador de projeto sem a cobrança, a receita e a margem. Criar
  um projeto com cliente ou valor cobrado é do dono. Convidar para um projeto não pede mais `people.manage`, então o
  administrador de projeto convida.
- O que um colega recebe de uma pessoa ficou menor: a jornada vai à própria pessoa, ao dono, aos admins e ao
  administrador de um projeto em que ela está; a regra de pagamento, à pessoa e ao dono. `project.delete` saiu de
  `project.edit` e fica fora do grupo `manager`.
- Estar num projeto é ter custo nele: a união "tem custo ou está num time", que servia a bancos antigos, saiu.

**Consequências:** os grupos `admin` e `finance` de um projeto continuam na API e no seed, escondidos das telas, e
quem não tem `billing.*` não os concede (`allocation.preset_above_yours`). Não houve migração de dados: o banco de
desenvolvimento é descartável e o seed foi refeito. A matriz dos quatro cargos contra as rotas está nos testes
`TestRoles_Matrix` e `TestRoles_Pages`. As regras de hoje estão em [permissions.md](../product/permissions.md).

### Decisão 18: Baixa de pagamentos — o período é calculado, o pagamento feito é gravado

**Contexto:** a Decisão 15 não guardava nada como pago, e a página Pagamentos mostrava o que pagar sem como dizer
que foi pago. O dono pediu para dar baixa, ver o que pagou há pouco e, junto, dar um bônus.

**Decisão:** uma tabela `payouts` guarda a baixa de um período de uma pessoa, identificado pelas datas
(`period_start`, `pay_date`). A baixa vale a qualquer momento depois que o período começou, inclusive no corrente,
antes da data; guarda as horas e o valor calculados naquele momento, o valor pago pelas horas (que o dono pode mudar)
e um bônus opcional. Um período fechado sem baixa nos últimos 60 dias fica **atrasado** em Próximos pagamentos; Pagos
recentemente mostra as baixas dos últimos 30 dias e o que o período ganhou depois de cada uma. Só o dono dá baixa e
desfaz (`payments.manage`). O dono não é pago e sai das tabelas e de "sem regra".

Horas registradas depois de uma baixa são o **restante** do período: ele volta para Próximos pagamentos só com elas
(o que já foi pago aparece ao lado), entra nos cartões e leva outra baixa. Cada baixa guarda o que cobriu (a primeira,
o período até ali; as seguintes, o que veio depois da anterior) e a posição dela no período (`seq`); o índice único em
pessoa, período e `seq` impede que duas baixas ao mesmo tempo paguem o mesmo restante. Sem restante, uma baixa nova é
recusada (`payment.already_paid`).

**Consequências:** o período continua sendo calculado, então trocar a regra pode deixar uma baixa sem período que
case com ela: a baixa fica nos números dela e o período novo aparece como a pagar. Desfazer uma baixa devolve o que ela
cobria como restante. Não houve migração de dados: os períodos fechados antes da baixa existir aparecem como atrasados
até o dono dar baixa neles.

### Decisão 19: Deploy — uma imagem só com o servidor, o banco fora dela

**Contexto:** o sistema vai ao ar para o dono e algumas pessoas, gastando o mínimo, sem um servidor para administrar:
uma imagem Docker que uma plataforma roda. O dono escolheu o Supabase grátis para o banco e o Railway (ou o Fly.io)
para o servidor.

**Decisão:** a imagem tem só o binário, numa base distroless sem shell, rodando sem privilégios. O banco fica fora:
dentro da imagem, cada deploy criaria um container novo sem os dados, e a maioria das plataformas não guarda disco. A
conexão com o Supabase é pelo pooler em session mode, porque a direta só tem IPv6 e o transaction mode não aceita os
prepared statements do pgx nem o advisory lock das migrações. O servidor roda numa **instância só, sempre ligada**: a
sincronização roda no processo e o limite de tentativas do login é da memória. Três ajustes para isso: `PORT` quando
não há `API_PORT`, `DB_MAX_CONNS` (o session mode recusa clientes além do pool) e `CLIENT_IP_HEADER`, porque atrás do
proxy o IP da conexão é o dele e todo mundo dividiria o mesmo limite do login. O cabeçalho é um de um IP só, que o
proxy reescreve; o `X-Forwarded-For`, que quem chama pode começar, é recusado.

**Consequências:** duas réplicas sincronizariam em dobro e dividiriam mal o limite do login, então escalar pede tirar
os dois do processo. Plataformas que desligam o container sem tráfego param a sincronização. Uma versão anterior não
sobe num banco com uma migração que ela não conhece, então voltar uma versão por cima de uma migração não funciona: a
saída é subir uma versão nova. O plano grátis do Supabase pausa depois de uma semana sem uso e não tem backup, que fica
por conta do `pg_dump`. Ver [Deploy](../getting-started/deploy.md).

## Riscos e trade-offs

- **Limite de requisições das plataformas.** Buscar detalhes e sincronizar gasta a cota de cada plataforma. →
  A rotina de fundo espera quando o limite acaba, e uma rodada sem diferença não escreve nada.
- **Clock-ins simultâneos.** Dois pedidos quase ao mesmo tempo para a mesma pessoa. → O índice único parcial é a
  garantia; o service devolve um erro claro.
- **Vazamento de credenciais.** Os tokens criptografados valem o que vale a chave. → A chave vem do ambiente, fora do
  repositório.
- **Mudanças nas APIs das plataformas.** → Cada tipo tem a sua implementação, e a leitura de uma tarefa nunca falha
  por causa da plataforma.
- **Latência da busca na hora.** Ler uma tarefa com item ligado pede uma ida à plataforma. → Uma falha volta sem os
  detalhes, sem travar a leitura.
- **Schema e migrações divergindo.** O `ent/schema` é editado à mão e as migrações são geradas dele. → Um teste aplica
  as migrações e falha se a diferença do Ent não for vazia.
- **Migrações que só falham com dados de verdade.** Os testes migram um banco vazio. → O gerador avisa dessas
  mudanças, todo arquivo é revisado, e cada migração roda numa transação: uma falha deixa o banco na versão anterior
  e o servidor não sobe.
- **Sem checagem de build na lógica do front-end.** O comportamento do Alpine mora em atributos do HTML. → Componentes
  pequenos, a validação do servidor como fonte da verdade e testes das páginas servidas.
- **A fila da sincronização é da memória.** Um reinício esquece as tarefas que esperavam para virar cartão. → Elas
  podem ser postadas pela página da tarefa (`POST /api/tasks/:taskId/publish`).
