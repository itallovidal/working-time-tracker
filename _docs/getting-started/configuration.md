# Configuração

Todas as variáveis de ambiente que o sistema lê. O servidor carrega o `.env` da pasta em que roda (`internal/config`);
o `.env.example` traz todas, comentadas.

Um valor inválido **derruba a subida** com a mensagem do motivo, em vez de o servidor rodar com uma configuração que
ninguém pediu.

## Servidor e banco

| Variável | Obrigatória | Para que serve |
|---|---|---|
| `API_PORT` | sim, ou `PORT` | Porta HTTP. O `.env.example` usa `1243`: a `8080` é comum demais e costuma estar ocupada por outro serviço |
| `PORT` | não | A porta quando não há `API_PORT`. É onde as plataformas que rodam a imagem (Railway, Render, Cloud Run) dizem a porta; a imagem traz `8080` |
| `DATABASE_URL` | sim | Conexão com o PostgreSQL. O seed e o `cmd/migrate` usam a mesma |
| `DB_MAX_CONNS` | não | Máximo de conexões abertas com o banco. Padrão `10`. Um pooler em session mode, como o do Supabase, recusa clientes além do tamanho do pool dele. Ver [Deploy](deploy.md) |
| `CLIENT_IP_HEADER` | atrás de um proxy | O cabeçalho em que o proxy da frente põe o IP de quem chamou: `X-Real-IP` no Railway, `Fly-Client-IP` no Fly.io. O limite de tentativas do login conta por esse IP; sem ele, atrás de um proxy, todo mundo divide o mesmo limite. Vazio (padrão), vale o IP da conexão. Só se define quando o servidor só é alcançado pelo proxy. `X-Forwarded-For` derruba a subida: é uma lista que quem chama pode começar |
| `INTEGRATION_ENCRYPTION_KEY` | sim | Chave da criptografia AES-GCM dos tokens das integrações (`openssl rand -hex 32`). Trocá-la torna os tokens salvos ilegíveis: cada integração pede o token de novo, e o resto dela (nome e `metadata`) continua |
| `COOKIE_SECURE` | não | `true` marca os cookies como `Secure`. Use em produção, atrás de HTTPS |
| `PUBLIC_URL` | com GitHub, Trello ou Clerk | O endereço em que as pessoas abrem o sistema, sem barra no fim (`http://localhost:1243`). As plataformas devolvem a pessoa para ele depois de autorizar |

## Integrações

| Variável | Obrigatória | Para que serve |
|---|---|---|
| `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` | para conectar o GitHub | O app OAuth do GitHub. Sem eles, o botão Conectar com o GitHub avisa que não está configurado. Ver [GitHub](../integrations/github.md) |
| `GITHUB_URL`, `GITHUB_API_URL` | não | Só para um GitHub Enterprise ou um servidor fake. Os padrões são `https://github.com` e `https://api.github.com` |
| `TRELLO_API_KEY` | para conectar o Trello | A chave de API do app do Trello (só letras e dígitos). Ver [Trello](../integrations/trello.md) |
| `TRELLO_APP_NAME` | não | O nome que a tela de autorização do Trello mostra. Padrão: `Working Time Tracker` |
| `TRELLO_URL`, `TRELLO_API_URL` | não | Só para um servidor fake. Os padrões são `https://trello.com` e `https://api.trello.com/1` |
| `SYNC_INTERVAL` | não | De quanto em quanto tempo a rotina de fundo olha as integrações com a sincronização ligada (`5m`, `30s`). Padrão `5m`; `0` desliga a rotina. Ver [Integrações](../integrations/overview.md) |
| `GITHUB_SYNC_INTERVAL`, `TRELLO_SYNC_INTERVAL` | não | O mesmo, só para aquela plataforma, por cima do `SYNC_INTERVAL` |

## Login pelo Clerk

| Variável | Obrigatória | Para que serve |
|---|---|---|
| `CLERK_SECRET_KEY`, `CLERK_PUBLISHABLE_KEY` | para entrar pelo Clerk | As duas chaves do app (`sk_…` e `pk_…`). Sem elas o login é só por email e senha. Uma sem a outra, ou as duas sem `PUBLIC_URL`, derrubam a subida. Ver [Clerk](../integrations/clerk.md) |
| `CLERK_AUTHORIZED_PARTIES` | não | As origens cujas sessões do Clerk o servidor aceita, separadas por vírgula. Padrão: a origem do `PUBLIC_URL` |
| `INVITE_DELIVERY` | não | `email` (padrão): o Clerk manda o email do convite. `terminal`: não manda e escreve o link no log do servidor, para desenvolver. Sem o Clerk não vale nada |
| `CLERK_API_URL` | não | Só para um servidor fake: a Backend API do Clerk, sem o `/v1`. Padrão: `https://api.clerk.com` |

## Desenvolvimento e testes

Estas não são lidas pelo servidor.

| Variável | Quem lê | Para que serve |
|---|---|---|
| `TEST_DATABASE_URL` | `go test` | O banco dos testes. O nome precisa terminar em `_test`. Ver [Fluxo de trabalho](../development/workflow.md) |
| `UPDATE_ERROR_DOCS` | `go test ./internal/server` | Qualquer valor regrava o [`_docs/api/errors.md`](../api/errors.md) em vez de compará-lo |
| `POSTGRES_PORT` | `docker compose` e o `Makefile` | A porta do Postgres na sua máquina, do ambiente ou do `.env`. Padrão: `5432` |
