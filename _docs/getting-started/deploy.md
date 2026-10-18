# Deploy

O sistema é um binário Go com os templates, os estáticos, os textos e as migrações embutidos, então a imagem Docker
é só ele. O banco fica fora da imagem: um container novo a cada deploy apagaria os dados, e a maioria das
plataformas que rodam imagens não guarda disco. Este guia usa o **Supabase** para o banco e o **Railway** ou o
**Fly.io** para o servidor, mas qualquer lugar que rode uma imagem e dê um PostgreSQL 14+ serve. O porquê está na
[Decisão 19](../architecture/decisions.md#decisão-19-deploy--uma-imagem-só-com-o-servidor-o-banco-fora-dela).

```mermaid
flowchart LR
    U["Navegador"] -->|"HTTPS"| P["Proxy da plataforma<br/>(Railway ou Fly.io)"]
    P -->|"X-Real-IP ou Fly-Client-IP"| S["Container<br/>com o binário"]
    S -->|"Session pooler, porta 5432, TLS"| D["PostgreSQL<br/>no Supabase"]
    S -->|"sincronização"| I["GitHub, Trello"]
```

## Quanto custa

Os preços de outubro de 2026, para uma pessoa e alguns amigos:

| Peça | Plano | Por mês |
|---|---|---|
| Banco | Supabase Free: 500 MB, pausa depois de uma semana sem uso, sem backup | US$ 0 |
| Servidor | Railway Hobby: inclui US$ 5 de uso, que o servidor sozinho não costuma passar | US$ 5 |
| Servidor | Fly.io: uma máquina de 256 MB em São Paulo, sem plano grátis | ~US$ 3,50 |

## O que o servidor precisa

- **Uma instância só.** A sincronização com o GitHub e o Trello roda dentro do processo, e o limite de tentativas do
  login guarda os contadores em memória. Com duas réplicas, as duas sincronizariam ao mesmo tempo.
- **Ficar ligado.** Uma plataforma que desliga o container sem tráfego (o Render grátis, o Koyeb grátis, o Cloud Run
  escalando a zero) para a sincronização junto.
- **O banco na mesma região.** Cada página faz várias consultas, e a distância entre o servidor e o banco se soma em
  cada uma. A distância até quem usa conta uma vez por página.

## A imagem

O [`Dockerfile`](../../Dockerfile) compila o binário e o põe numa imagem distroless (sem shell, com os certificados e
os fusos horários), que roda sem privilégios. O servidor aplica as migrações ao subir.

```bash
docker build -t wtt .
docker run --rm -p 8080:8080 --env-file .env -e API_PORT= \
  -e DATABASE_URL='postgres://wtt:wtt@host.docker.internal:5432/working_time_tracker?sslmode=disable' wtt
```

O `-e API_PORT=` deixa valer a porta da imagem (`PORT=8080`), e o `host.docker.internal` é o Postgres do
`docker compose` visto de dentro do container. O Railway e o Fly.io constroem a imagem a partir do `Dockerfile`; não
precisa publicá-la num registro.

## O banco no Supabase

1. Crie o projeto no plano Free, na **mesma região do servidor**: East US (North Virginia) para o Railway na
   Virgínia, South America (São Paulo) para o Fly.io em `gru`.
2. **Desligue a Data API** (desmarque "Enable Data API" ao criar o projeto, ou desligue depois nas configurações da
   API). O Supabase expõe o schema `public` por uma API REST, e as migrações criam as tabelas nele sem RLS. O sistema
   não usa essa API.
3. Em **Connect**, copie a URL do **Session pooler** (porta `5432`, host `aws-…pooler.supabase.com`, usuário
   `postgres.<ref>`) e acrescente `?sslmode=require`. Essa URL é o `DATABASE_URL`:
   - a conexão direta só tem IPv6 no plano Free, e as plataformas costumam sair por IPv4;
   - o Transaction pooler (porta `6543`) não aceita prepared statements, que o pgx usa, nem os advisory locks de
     sessão que travam as migrações.
4. O session mode recusa clientes além do tamanho do pool do projeto. O servidor abre no máximo `DB_MAX_CONNS`
   conexões (padrão `10`); se aparecer "max clients reached", baixe esse número.

O plano Free tem 500 MB de banco, que sobram para o ponto de algumas pessoas, e duas pegadinhas:

- **Pausa depois de uma semana sem uso.** O Supabase avisa por email antes; os dados ficam e o projeto volta pelo
  painel, mas enquanto ele está pausado o sistema não abre.
- **Não tem backup.** Faça um de tempos em tempos, da sua máquina:

  ```bash
  pg_dump "$DATABASE_URL" -Fc -f wtt-$(date +%F).dump
  ```

Nunca aponte o `TEST_DATABASE_URL` para o Supabase: cada pacote de teste apaga o schema do banco dele.

## As variáveis de produção

| Variável | Valor |
|---|---|
| `DATABASE_URL` | A URL do Session pooler, com `?sslmode=require` |
| `INTEGRATION_ENCRYPTION_KEY` | Uma chave nova (`openssl rand -hex 32`), guardada fora da plataforma. Trocá-la depois faz cada integração pedir o token de novo |
| `COOKIE_SECURE` | `true` |
| `PUBLIC_URL` | O endereço público, sem barra no fim (`https://wtt.up.railway.app`, `https://wtt.fly.dev` ou o seu domínio) |
| `CLIENT_IP_HEADER` | `X-Real-IP` no Railway, `Fly-Client-IP` no Fly.io |
| `GITHUB_*`, `TRELLO_*`, `CLERK_*` | Se usar as integrações ou o Clerk, como em [Configuração](configuration.md). `INVITE_DELIVERY` fica em `email` |

A porta não precisa de variável: a imagem traz `PORT=8080`, e a plataforma que diz outra porta em `PORT` a troca.

## Railway

Tudo pelo painel, e cada push na branch escolhida faz um deploy novo.

1. **New Project → Deploy from GitHub repo** e escolha o repositório. O Railway acha o `Dockerfile` na raiz e
   constrói a imagem com ele.
2. Em **Variables**, preencha as variáveis de produção.
3. Em **Settings**:
   - **Networking → Generate Domain** dá o endereço público; ele detecta a porta em que o servidor escuta.
   - **Deploy → Region**: a mesma do banco (US East, na Virgínia, com o Supabase em North Virginia).
   - **Deploy → Healthcheck Path**: `/healthcheck`.
   - Deixe uma réplica e o "Serverless" (o servidor dormir sem tráfego) desligado.
4. Volte em **Variables** e ponha o `PUBLIC_URL` com o domínio gerado.

O Railway não tem região na América do Sul; da Virgínia, cada página leva uns 120–150 ms a mais para quem está no
Brasil. O `railway.toml` não é usado: o Railway está trocando esse formato e deixa de lê-lo em dezembro de 2026.

## Fly.io

Pela linha de comando, com o `fly.toml` no repositório. A imagem também é construída pelo Fly a partir do
`Dockerfile`.

```bash
brew install flyctl
fly auth login
fly launch --no-deploy        # cria o app e o fly.toml; escolha a região gru (São Paulo)
```

Deixe o `fly.toml` assim (o nome do app é o que o `fly launch` criou):

```toml
app = "wtt"
primary_region = "gru"

[env]
  COOKIE_SECURE = "true"
  CLIENT_IP_HEADER = "Fly-Client-IP"
  PUBLIC_URL = "https://wtt.fly.dev"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "off"

  [[http_service.checks]]
    grace_period = "10s"
    interval = "30s"
    method = "GET"
    timeout = "5s"
    path = "/healthcheck"

[[vm]]
  memory = "256mb"
  cpu_kind = "shared"
  cpus = 1
```

Os segredos vão por `fly secrets`, fora do repositório, e o primeiro deploy pede uma máquina só (sem o `--ha=false`
o Fly cria duas):

```bash
fly secrets set DATABASE_URL='postgresql://…' INTEGRATION_ENCRYPTION_KEY="$(openssl rand -hex 32)"
fly deploy --ha=false
```

Depois, cada `fly deploy` constrói e sobe a versão nova. `fly logs` mostra o log do servidor.

## Depois do primeiro deploy

- Abra o `PUBLIC_URL` e clique em **Crie uma organização**. Não rode o `cmd/seed` em produção.
- No log, a linha `server` mostra a porta, o `db_max_conns` e de onde vem o IP (`client_ip_from`), e cada requisição
  traz o `remote_ip`: ele deve ser o IP de quem acessou, não o do proxy.
- Se usar as integrações ou o Clerk, troque o endereço de retorno para o `PUBLIC_URL`: a "Authorization callback URL"
  do app OAuth do GitHub, as "Allowed origins" da chave do Trello e o domínio no Clerk.

## Atualizar e voltar uma versão

- **Railway:** cada push na branch ligada ao serviço (a `main`) faz um deploy novo. Com o healthcheck, a versão
  anterior só sai quando a nova responde. Para voltar, abra **Deployments**, escolha um deploy anterior e use
  **Rollback**.
- **Fly.io:** `fly deploy` sobe a versão nova. Para voltar, `fly releases --image` lista as imagens já publicadas, e
  `fly deploy --image <imagem>` sobe uma delas.

O servidor aplica as migrações novas ao subir, e cada uma roda numa transação: se uma falha, o banco fica como estava
e o servidor não sobe. **Voltar por cima de uma migração não funciona:** uma versão anterior recusa um banco com uma
migração que ela não conhece (`database has a migration this build does not know`). Em produção, ignore a dica da
mensagem de recriar o banco, que é para o banco de desenvolvimento: corrija e suba uma versão nova. Antes de um deploy
com migração, faça o `pg_dump`.

## Problemas comuns

| O que aparece | Por quê | O que fazer |
|---|---|---|
| `max clients reached` no log | Mais conexões que o pool do Supabase em session mode | Baixe o `DB_MAX_CONNS` |
| `Tenant or user not found` | O usuário do pooler é `postgres.<ref>`, não só `postgres` | Copie de novo a URL do Session pooler em **Connect** |
| O servidor não conecta e o Supabase manda email de pausa | Uma semana sem uso no plano Free | Restaure o projeto pelo painel do Supabase |
| Todo mundo leva `429` no login depois de alguém errar a senha | O IP de todos é o do proxy | Defina o `CLIENT_IP_HEADER` |
| A plataforma diz que o servidor não responde | A porta em que ele escuta não é a que a plataforma espera | Veja a porta na linha `server` do log e não defina `API_PORT` |
| `database has a migration this build does not know` | Voltou para uma versão anterior a uma migração | Suba uma versão nova (ver acima) |
