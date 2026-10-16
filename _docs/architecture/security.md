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
  (`429` acima disso).
- **Redirecionamentos.** O `?next=` do login e da troca de idioma só aceita um caminho do próprio sistema.

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
- **Conectar com o Trello** usa o mesmo cookie, limitado a `/integrations/trello` e marcado com a plataforma (o do
  GitHub não serve para terminar uma conexão do Trello). O Trello devolve o token no fragmento da URL, que o
  navegador não manda ao servidor: a página de retorno lê o token, apaga o endereço do histórico e o entrega por
  `POST`, e responde com `Referrer-Policy: no-referrer` e `Cache-Control: no-store`.
- **As escritas nas plataformas** saem com a conta de quem conectou a integração. Com a sincronização ligada,
  qualquer pessoa do projeto que edita uma tarefa sincronizada escreve no GitHub ou no Trello com essa conta.

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
