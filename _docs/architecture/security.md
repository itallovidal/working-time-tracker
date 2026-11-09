# Segurança

O que protege as contas, os dados de cada organização, o dinheiro e os tokens das integrações. Quem pode fazer o
quê está em [permissions.md](../product/permissions.md); aqui está como isso é garantido.

## Contas e sessões

- **Senhas** com bcrypt (custo padrão), de 8 a 72 caracteres: o bcrypt ignora o que passa de 72 bytes, então senhas
  maiores são recusadas. Um login com e-mail que não existe gasta o mesmo tempo de uma senha errada, para o tempo de
  resposta não revelar quais contas existem. Contas sem senha (as que entram só pelo Clerk) não entram por senha.
- **Sessões e convites** usam tokens aleatórios de 32 bytes, e o banco guarda só o sha256 deles. A sessão dura 7
  dias, e o convite também. Trocar a senha encerra as outras sessões da pessoa.
- **Cookie** `wtt_session`: `HttpOnly`, `SameSite=Lax` e `Secure` quando `COOKIE_SECURE=true` (use em produção,
  atrás de HTTPS).
- **CSRF.** A API só aceita corpo JSON em `POST`, `PUT` e `PATCH` (`JSONOnly`; outro `Content-Type` recebe `415`).
  Um formulário de outro site não consegue mandar `application/json` sem o preflight do CORS, e isso completa o
  `SameSite=Lax`.
- **Corpo estrito.** A API recusa chave desconhecida no JSON e corpo acima de 1 MiB.
- **Limite de tentativas** por IP no signup, no login, nos convites e nas rotas do Clerk: 10 por minuto, em memória
  (`429` acima disso). Atrás de um proxy, o IP vem do cabeçalho que ele escreve (`CLIENT_IP_HEADER`); sem isso, todo
  mundo teria o IP do proxy e uma pessoa errando a senha travaria o login das outras.
- **Redirecionamentos.** O `?next=` do login e da troca de idioma só aceita um caminho do próprio sistema.

## Tokens do Claude (MCP)

O servidor MCP (`/mcp`) é a porta do Claude. O Claude Code entra com um token pessoal, e o claude.ai (com o Desktop e o
celular) e o Claude Code entram também pelo OAuth ([OAuth do MCP](#oauth-do-mcp)); nunca com o cookie
([api/mcp.md](../api/mcp.md)).

- **Como a sessão:** `wtt_` mais 32 bytes aleatórios, e o banco guarda só o sha256. O token aparece em claro uma vez,
  na criação, e a lista mostra só o começo dele.
- **Só no `/mcp`.** A API não aceita o token: com ele não se cria outro token, não se troca a senha nem se mexe em
  pessoas ou dinheiro. E o `/mcp` não aceita o cookie, então um site de fora não usa a sessão do navegador para chamá-lo.
- **Vence sempre** (30, 90 ou 365 dias), no máximo 10 válidos por pessoa, revogável no perfil. **Trocar a senha revoga
  todos**: quem trocou a senha por desconfiar de alguém também tira dele um token que ele tenha gerado.
- **A identidade é lida a cada requisição**, então um cargo que mudou ou um projeto de que a pessoa saiu valem na hora.
- **O mesmo acesso da API:** cada projeto e cada tarefa passam pela checagem do `RequireOrg` (`Resolver.Reach`), e o
  que a pessoa não alcança responde `not_found`. Uma falha do banco ao ler o projeto do recurso não deixa passar: a
  checagem devolve o erro (e o `RequireOrg`, `500`). O ponto é sempre o dela, as ferramentas que mudam a sessão também
  conferem o projeto dela, e nenhuma saída tem dinheiro.
- **Outra origem:** um pedido de navegador vindo de outro site recebe `403` (`http.CrossOriginProtection`).
- O token não vai para o log: o log de requisições não grava cabeçalhos.

## OAuth do MCP

O claude.ai, o Claude Desktop e o celular só conectam um servidor remoto por OAuth 2.1. O sistema é o próprio servidor
de autorização (`internal/oauth`, [Decisão 24](decisions.md#decisão-24-oauth-no-mcp--o-claudeai-o-desktop-e-o-celular-pela-identidade-publicada-do-claude)).

- **Só os apps do Claude.** O client_id tem de ser uma das duas URLs de identidade publicadas do Claude (o claude.ai e o
  Claude Code). O documento é buscado dessas URLs e de nenhuma outra: sem redirecionamento, em até 5 segundos e até
  64 KiB; o client_id dele tem de ser a própria URL, o app tem de ser público (`none`) e cada endereço de volta, https
  ou http no próprio computador. Se a busca falha, vale a última cópia boa ou a embutida.
- **O endereço de volta é exato,** menos no http do próprio computador (o Claude Code abre uma porta qualquer): ali a
  porta não conta, mas o nome, o caminho e a query, sim. Um app desconhecido, ou um endereço de volta que não é dele,
  para numa página de erro, sem redirecionar: mandar o erro a um endereço desconhecido seria um redirecionamento aberto.
- **PKCE S256 obrigatório.** Sem o desafio, com outro método ou com um verificador errado, não há código nem token.
- **O código vale uma vez, por 2 minutos.** A troca o gasta na primeira tentativa, mesmo errada (outro app, outro
  endereço de volta, outro verificador, outro recurso). Um código usado de novo derruba a conexão, com os tokens que ela
  já deu.
- **O token de acesso (`wtta_`) só vale no `/mcp` e só para o recurso em que foi emitido** (o `PUBLIC_URL` mais
  `/mcp`, comparado na forma canônica). Vale 1 hora e, como o pessoal, relê a pessoa a cada requisição.
- **O token de renovação (`wttr_`) gira a cada uso.** O anterior, mandado de novo em até 60 segundos, recebe o mesmo
  sucessor (guardado cifrado com a `INTEGRATION_ENCRYPTION_KEY`); depois disso, é reuso, e a conexão cai inteira. A
  conexão cai também depois de 30 dias sem uso.
- **Todos os tokens e códigos são guardados como sha256,** como a sessão.
- **A tela de consentimento** sempre pergunta (não há aprovação automática), mostra o app, a pessoa, a organização e o
  host para onde volta, e avisa quando a volta é um programa no próprio computador. Ela responde com
  `X-Frame-Options: DENY`, `frame-ancestors 'none'`, `Referrer-Policy: no-referrer` e `Cache-Control: no-store`.
- **O "Permitir" é da API** (`POST /api/oauth/consent`): pede o cookie, o JSON (`JSONOnly`) e a mesma origem
  (`http.CrossOriginProtection`), e confere o pedido de novo. A concessão é sempre de quem está logado.
- **O `/oauth/token` e o `/oauth/revoke`** aceitam só o corpo do formulário (parâmetros na URL não contam, e um
  parâmetro repetido é recusado), respondem com `Cache-Control: no-store` e têm o próprio limite por IP (1 por segundo,
  rajada de 30), mais folgado que o do login porque o claude.ai renova os tokens de todo mundo pelos mesmos IPs.
- **Desconectar** é na lista "Apps conectados" do perfil (cada pessoa só vê e desconecta os seus) ou no
  `/oauth/revoke`. **Trocar a senha desconecta todos os apps**, como revoga os tokens pessoais. Uma conta só com o
  Clerk não troca a senha: para ela, o corte é desconectar no perfil.
- **Os documentos de descoberta** (`/.well-known/*`) são públicos e respondem a qualquer origem; um que não existe é um
  `404` em JSON, nunca o redirecionamento para o login.

## Entrar pelo Clerk

O Clerk só diz quem a pessoa é; a sessão continua sendo a do sistema. O token de sessão do Clerk só vale se:

- a assinatura confere com as chaves públicas do app;
- não venceu (com uma folga de 10 segundos para relógios diferentes);
- vem de uma origem listada em `CLERK_AUTHORIZED_PARTIES` (o `azp`). Um token sem origem, ou de outro app da mesma
  instância, é recusado;
- o usuário existe no Clerk e não está bloqueado.

Só o e-mail **primário e verificado** vale. Uma conta que já tem senha só é ligada ao Clerk depois de a pessoa
digitar essa senha, uma vez: o cadastro por senha não verifica o e-mail, e sem essa prova quem cadastrou o e-mail de
outra pessoa ficaria com a conta dela. As rotas `/api/auth/clerk/*` levam o token em `Authorization: Bearer`, não
em cookie, então um site de fora não as chama em nome de ninguém. A chave secreta fica só no servidor, e nenhum
token vai para o log.

Uma sessão do sistema já aberta não cai quando o usuário é bloqueado ou apagado no Clerk: ela vale até vencer. O
fluxo completo está em [integrations/clerk.md](../integrations/clerk.md).

## Convite por e-mail

O link `/invite/<token>` viaja na `redirect_url` do convite do Clerk e no e-mail, então o token também fica no Clerk.
Ele só serve a quem prova a caixa de entrada: num convite com e-mail, o e-mail verificado do Clerk precisa ser o do
convite. Revogar cancela o convite aqui e tenta cancelá-lo no Clerk; um convite que ficou no Clerk não dá acesso a
nada, porque o daqui já não existe. Com `INVITE_DELIVERY=terminal` os links dos convites vão para o log do servidor:
é só para desenvolver, e o servidor avisa ao subir se isso estiver ligado junto com `COOKIE_SECURE=true`.

## Isolamento e acesso

- **Entre organizações.** Toda rota com id confere se o recurso é da organização de quem chama
  (`RequireOrg`) e responde `404` quando não é, como se ele não existisse: um `403` revelaria que existe.
- **Entre projetos.** Quem não é dono nem admin só alcança os projetos em que tem valor por hora. Num projeto,
  time, tarefa ou integração de fora, a resposta também é `404`, antes de qualquer permissão.
- **Entre pessoas.** Quem não é admin, não cuida de pessoas e não põe gente em nenhum projeto só vê a si mesmo e
  quem divide projeto com ele; as outras pessoas respondem `404`.
- **Telas.** Uma página que a pessoa não pode abrir responde "Página não encontrada". A tela esconde o que a pessoa
  não pode usar, mas quem garante é sempre a API.

O caminho de uma requisição pelos middlewares está em [overview.md](overview.md#caminho-de-uma-requisição).

## Dinheiro

O dinheiro tem um eixo próprio: o **dono** vê tudo, e os outros cargos veem só parte. A API apaga os campos antes de
responder, não a tela:

| O quê | Quem recebe |
|---|---|
| O valor cobrado do cliente (`GET /api/projects/:projectId/billing`) | Quem tem `billing.view` no projeto: o dono, ou quem está nos grupos `finance` ou `admin` do projeto |
| Valor cobrado, receita e margem nas sessões e na visão geral do projeto | Também só quem tem `billing.view`; os outros recebem a visão sem cobrança (`HideBilling`) |
| Receita e margem na visão geral da organização | Só o dono |
| O valor pago de uma pessoa (na sessão, na lista de colaboradores) | A própria pessoa e quem tem `rates.view` ou `rates.manage` no projeto (o dono, os admins, o administrador de projeto) |
| A regra de pagamento de uma pessoa | A própria pessoa e o dono |
| Os pagamentos de uma pessoa (períodos e valores) | A própria pessoa e o dono. Um admin que abre o perfil de outra pessoa recebe só as horas (`HideMoney`) |
| Os pagamentos da equipe | Só o dono |
| As contas a receber (modo solo) | Só o dono, e só numa organização no modo solo: fora dele a rota responde 404 |
| A jornada semanal de uma pessoa | A própria pessoa, o dono, os admins, quem cuida de pessoas e o administrador de um projeto em que ela está |

Os clientes e a edição da organização também são só do dono.

## Integrações

- **Tokens** criptografados com AES-GCM; a chave é o sha256 de `INTEGRATION_ENCRYPTION_KEY`. O token nunca volta
  nas respostas (a API diz só `has_token`) nem vai para o log. Trocar a chave torna os tokens salvos ilegíveis: cada
  integração pede o token de novo, e o resto dela continua.
- **`metadata` fica em claro** e volta nas respostas, então não é lugar de segredo: cada tipo só guarda os campos
  que declara, e um campo a mais é descartado.
- **O que vem de quem usa e entra numa URL da plataforma** (o repositório, o quadro, o número da issue, o cartão) é
  conferido antes, para ninguém apontar o pedido para outro recurso que o token enxerga.
- **Conectar com o GitHub** usa um `state` aleatório, selado (AES-GCM) num cookie `wtt_oauth` `HttpOnly`,
  `SameSite=Lax`, limitado ao caminho do retorno e válido por 10 minutos, que serve a uma volta só. Na volta o
  servidor confere o `state` em tempo constante e que a pessoa e a permissão `integrations.manage` no projeto
  continuam valendo. Como é uma navegação `GET`, e não uma chamada da API, a proteção contra CSRF dessas rotas é o
  `state`.
- **Conectar com o GitLab** é o mesmo vai e volta do GitHub, com o cookie limitado a `/integrations/gitlab`. O par
  de tokens (o de acesso vence em 2 horas) fica cifrado como os outros tokens, e a renovação trava a linha da
  integração (`SELECT ... FOR UPDATE`): o token de renovação gira a cada uso, e duas renovações juntas deixariam a
  integração com um par já gasto.
- **Conectar com o Trello** usa o mesmo cookie, limitado a `/integrations/trello` e marcado com a plataforma (o do
  GitHub não serve para terminar uma conexão do Trello). O Trello devolve o token no fragmento da URL, que o
  navegador não manda ao servidor: a página de retorno lê o token, apaga o endereço do histórico e o entrega por
  `POST`, e responde com `Referrer-Policy: no-referrer` e `Cache-Control: no-store`.
- **As escritas nas plataformas** saem com a conta de quem conectou a integração. Com a sincronização ligada,
  qualquer pessoa do projeto que edita uma tarefa sincronizada escreve no GitHub, no GitLab ou no Trello com essa conta.

## Conteúdo de quem usa

- Os templates usam `html/template`, que escapa o HTML.
- A descrição da tarefa é Markdown, renderizado no navegador pelo marked e limpo pelo DOMPurify. Não há
  Content-Security-Policy, então **o DOMPurify é a única defesa contra XSS** ali: só uma lista fixa de tags, nenhuma
  imagem, links só para `http`, `https` e `mailto`, sempre com `target="_blank"` e `rel="noopener noreferrer"`. As
  duas bibliotecas são vendorizadas e o hash delas é conferido por teste.
- Num repositório público, quem abre uma issue cria uma tarefa (com a sincronização ligada); o Markdown dela passa
  pela mesma limpeza.

## Testes que não saem da máquina

O transporte HTTP padrão dos testes recusa qualquer endereço que não seja da própria máquina: um teste que
apontasse para o GitHub de verdade falha, em vez de escrever nas issues de alguém. As plataformas dos testes são
servidores fake em `testutil`.
