# MCP: o Claude no sistema

O servidor MCP ([Model Context Protocol](https://modelcontextprotocol.io)) deixa o Claude agir como uma pessoa: ver e
criar tarefas, atualizá-las, pegar uma tarefa para si, sincronizá-la com a plataforma, bater o ponto, trocar de tarefa e
ver as próprias horas. Ele mora no mesmo binário, em `/mcp`. Quem decidiu e por quê está nas
[Decisões 22 e 25](../architecture/decisions.md).

Há dois jeitos de entrar:

- **OAuth**, para o **claude.ai**, o **Claude Desktop** e o **app do celular** (que só conectam um servidor remoto
  assim) e também para o **Claude Code**: a pessoa autoriza o Claude numa tela do sistema, e ele recebe tokens que se
  renovam sozinhos. Liga com um `PUBLIC_URL` em `https` (ver [OAuth](#oauth)).
- **Token pessoal**, para o **Claude Code**: um token gerado no perfil, que vai num cabeçalho.

## Conectar

O endereço do conector é o `PUBLIC_URL` mais `/mcp` (`https://<endereço do sistema>/mcp`). O perfil o mostra em
**Conectar ao Claude**, com o botão de copiar.

### claude.ai, Claude Desktop e celular

1. No claude.ai, abra **Personalizar → Conectores → Adicionar conector personalizado** e cole o endereço do conector.
   Em OAuth, deixe **Usar a identidade publicada do Claude** (o padrão). No Team e no Enterprise, quem adiciona o
   conector é o dono da conta do Claude; cada pessoa conecta a sua.
2. O Claude abre a tela de entrada do sistema. Entre (se ainda não estiver) e, na tela de consentimento, clique em
   **Permitir**.
3. Numa conversa, peça, por exemplo, "quais são as minhas tarefas?" ou "bate o ponto na tarefa X". O conector vale
   também no Claude Desktop e no app do celular, com a mesma conta do Claude.

A conexão aparece em **Perfil → Conectar ao Claude → Apps conectados**, de onde também se desconecta.

### Claude Code

Com o OAuth, sem token:

```bash
claude mcp add --transport http --scope user wtt https://<endereço do sistema>/mcp
```

Numa conversa do Claude Code, `/mcp`, escolha `wtt` e **Authenticate**: o navegador abre a mesma tela de
consentimento. Cada máquina vira um app conectado.

Ou com um **token pessoal**:

1. No sistema, abra **Perfil** e, em **Conectar ao Claude → Token pessoal**, clique em **Novo token**. Dê um nome
   (para reconhecê-lo depois) e escolha a validade: 30, 90 ou 365 dias.
2. O modal mostra o token **uma vez só**, com o comando pronto. Rode no terminal:

   ```bash
   claude mcp add --transport http --scope user wtt https://<endereço do sistema>/mcp --header "Authorization: Bearer wtt_…"
   ```

3. Numa conversa do Claude Code, peça o que quiser. `claude mcp list` mostra se a conexão está de pé.

O comando grava o token em claro na configuração do Claude Code (`~/.claude.json`). Para não deixá-lo num arquivo de
projeto, um `.mcp.json` pode ler o token de uma variável de ambiente:

```json
{
  "mcpServers": {
    "wtt": {
      "type": "http",
      "url": "https://<endereço do sistema>/mcp",
      "headers": { "Authorization": "Bearer ${WTT_TOKEN}" }
    }
  }
}
```

O endereço é o `PUBLIC_URL` do servidor; sem ele, o perfil mostra o endereço por onde a página foi aberta.

## O token pessoal

- É `wtt_` seguido de 32 bytes aleatórios. O banco guarda só o sha256 (`api_tokens.token_hash`), e a lista do perfil
  mostra só o começo (`prefix`).
- **Só vale no `/mcp`.** A API continua só com o cookie: um token não abre `/api`, não gera outro token e não mexe na
  conta.
- Vence (30, 90 ou 365 dias; não há "nunca"). Cada pessoa tem até 10 válidos.
- Revogar no perfil corta o acesso na hora. **Trocar a senha revoga todos**, como já encerra as outras sessões. Os
  tokens também somem com a pessoa.
- A identidade é lida de novo a cada requisição: um cargo que mudou, ou um projeto de que a pessoa saiu, vale na hora.
- As rotas da API do token estão em [reference.md](reference.md#tokens-do-claude).

## OAuth

O sistema é o próprio servidor de autorização (OAuth 2.1, como a
[especificação de autorização do MCP](https://modelcontextprotocol.io/specification/latest/basic/authorization) pede).
O código está em `internal/oauth` (o protocolo) e em `internal/domain/auth/oauth_grant.go` (os códigos e os tokens).

- **Liga sozinho** quando o `PUBLIC_URL` é uma origem pura em `https`, ou em `http` no próprio computador para
  desenvolver; `MCP_OAUTH=off` desliga ([configuration.md](../getting-started/configuration.md)). Desligado, o `/mcp`
  aceita só o token pessoal e o perfil não mostra o caminho do claude.ai.
- **Só os apps do Claude.** O claude.ai (com o Desktop e o celular) e o Claude Code se identificam pela URL de um
  documento público (CIMD): `https://claude.ai/oauth/mcp-oauth-client-metadata` e
  `https://claude.ai/oauth/claude-code-client-metadata`. O servidor busca o documento e o guarda pelo `max-age` (entre
  5 minutos e 24 horas); se a busca falha, vale o último ou a cópia embutida. Não há cadastro dinâmico (DCR). Os outros
  clientes MCP estão na [issue #36](https://github.com/itallovidal/working-time-tracker/issues/36).
- **O caminho:**
  1. o `/mcp` sem token responde `401` com
     `WWW-Authenticate: Bearer resource_metadata="<PUBLIC_URL>/.well-known/oauth-protected-resource/mcp", scope="mcp"`;
  2. os metadados do recurso apontam o servidor de autorização (o `PUBLIC_URL`), e os dele
     (`/.well-known/oauth-authorization-server`) dão os endereços;
  3. o Claude manda a pessoa a `/oauth/authorize` com o PKCE `S256` e o `resource` (o `/mcp`). Quem não está logado
     passa pelo login e volta;
  4. a tela de consentimento mostra o app, a pessoa, a organização e para onde volta (um aviso quando é um programa no
     próprio computador). O **Permitir** volta ao app com um código, o `state` e o `iss`;
  5. o app troca o código em `/oauth/token` pelos tokens.
- **Os tokens:** o de acesso é `wtta_…` e vale 1 hora, só no `/mcp` e só para o recurso em que foi emitido; o de
  renovação é `wttr_…` e gira a cada uso. A conexão **cai depois de 30 dias sem uso** (cada renovação empurra a data).
  O token de renovação anterior, mandado de novo em até 60 segundos, recebe o mesmo sucessor (dois processos renovando
  juntos); depois disso é reuso, e a conexão cai inteira.
- **O código** vale 2 minutos e uma vez só: a troca o gasta na primeira tentativa, e um segundo uso derruba a conexão.
- **Desconectar:** na lista **Apps conectados** do perfil, pelo `/oauth/revoke` ou trocando a senha, que desconecta
  todos os apps. Cada pessoa tem até 10 apps conectados; passando disso, sai o que está há mais tempo sem uso.
- **Igual ao token pessoal:** a identidade é lida a cada requisição, o token não abre a API, e o que o Claude pode é o
  mesmo ([O que o Claude pode](#o-que-o-claude-pode)).
- **O endereço importa:** a URL colada no Claude tem de ser exatamente o `PUBLIC_URL` mais `/mcp`. Trocar o
  `PUBLIC_URL` (um domínio próprio, por exemplo) derruba as conexões, e cada pessoa conecta de novo.
- As rotas estão em [reference.md](reference.md#oauth-do-mcp), e a segurança em
  [security.md](../architecture/security.md#oauth-do-mcp).

## Como funciona

- **Transporte:** Streamable HTTP **sem estado** (cada `POST /mcp` é completo, e as respostas vêm em JSON). Nenhuma
  sessão fica na memória do processo, então a troca de processo de um deploy não derruba quem está conectado. Um `GET`
  responde `405`.
- **Sem token, ou com um inválido, vencido ou revogado:** `401` com `WWW-Authenticate: Bearer scope="mcp"` (com o
  OAuth ligado, também o `resource_metadata`, que leva o Claude a autorizar de novo). O cookie de sessão não vale no
  `/mcp`.
- **Navegador de outro site:** um pedido com `Origin` (ou `Sec-Fetch-Site`) de outra origem recebe `403`. O Claude
  Code não manda esses cabeçalhos.
- **Corpo:** até 1 MiB, como na API.
- **Proxy na mesma máquina:** o SDK recusa com `403` um pedido que chega pela interface de loopback (`127.0.0.1`) com
  outro `Host`, contra DNS rebinding. Um nginx ou um túnel na mesma máquina que repassa para `localhost` cai nisso; no
  Render, no Railway e no Fly.io o pedido chega pela rede do container, e não acontece.
- **Erros:** uma ferramenta que falha devolve um resultado com `isError` e o texto do erro em inglês, com o código
  estável entre parênteses e os parâmetros (`this task already has an assignee (task.already_assigned)`). Alguns trazem
  uma dica do que fazer: o ponto que já está aberto diz em que projeto e sugere `add_task_to_session` ou `clock_out`. Os
  códigos são os da API, mais os `mcp.*` de [errors.md](errors.md).
- **Código:** `internal/mcp` (o servidor e as ferramentas), `internal/domain/auth/api_token.go` (o token pessoal),
  `internal/oauth` e `internal/domain/auth/oauth_grant.go` (o OAuth), e os testes em `internal/server/mcp_test.go`,
  `internal/server/api_tokens_test.go`, `internal/server/oauth_test.go` (inclusive de ponta a ponta com o cliente OAuth
  do SDK) e `internal/oauth/oauth_test.go`.

## O que o Claude pode

O Claude age **como a pessoa do token** e alcança o que ela alcança pela API: cada projeto e cada tarefa passam pela
mesma checagem do `RequireOrg` (`auth.Resolver.Reach`), e um recurso de outra organização ou de um projeto em que a
pessoa não está responde `not_found`, como um que não existe. As regras de cada ação são as dos services, iguais às da
tela.

### Leitura

| Ferramenta | O que faz |
|---|---|
| `whoami` | A pessoa, o cargo, a organização (com o fuso), a sessão de ponto aberta e quando o token vence |
| `list_projects` | Os projetos em que a pessoa está (todos, para admin) |
| `list_labels` | As etiquetas de um projeto, pelo nome |
| `list_tasks` | As tarefas de um projeto, por página, com busca, status, prioridade, responsável (`me`, `none`, `any`, `others`) e etiquetas. Sem status, só as abertas |
| `my_tasks` | As tarefas da pessoa em todos os projetos, abertas (por prazo) ou fechadas |
| `get_task` | Uma tarefa, com a descrição em Markdown e os itens ligados (GitHub, Trello…) |
| `get_active_session` | A sessão aberta, com o trecho de cada tarefa e qual está rodando |
| `my_hours` | Horas de hoje e da semana, dia a dia e por projeto, as contagens de tarefas e no que a pessoa trabalha agora |

### Escrita

| Ferramenta | O que faz |
|---|---|
| `create_task` | Cria a tarefa (nome, descrição, prioridade, prazo, etiquetas pelo nome, e `assign_to_me`). Nasce em backlog; sem prazo, o prazo é daqui a 7 dias, como na API; a publicação automática das integrações vale como na tela |
| `update_task` | Muda só o que vem: nome, descrição, status, prioridade, prazo (ou `clear_deadline`), etiquetas e responsável (`me` ou `none`). Numa tarefa sincronizada, a mudança vai à plataforma |
| `claim_task` | Pega a tarefa sem responsável, sem bater o ponto. A de outra pessoa é recusada |
| `sync_task` | Sincroniza agora a tarefa com os itens ligados a ela, nos dois sentidos (o botão Sincronizar da tarefa) |
| `clock_in` | Abre o ponto numa tarefa. Ela passa a em progresso e, sem responsável, vira da pessoa |
| `add_task_to_session` | Põe outra tarefa do mesmo projeto na sessão aberta, a partir de agora. Com `switch`, encerra os outros trechos ativos: é trocar de tarefa |
| `stop_session_task` | Encerra agora o trecho de uma tarefa na sessão; as horas ficam |
| `remove_session_task` | Tira a tarefa da sessão, com o trecho dela |
| `clock_out` | Encerra o ponto |

- **O ponto é sempre o da pessoa.** Nenhuma ferramenta recebe outra pessoa, e as da sessão agem na sessão aberta
  dela. Por isso o MCP não precisa das checagens de quem registra o ponto de outro (`personFor`) nem de quem mexe na
  sessão (`editable`). Mas, como as rotas da sessão na API, as que mudam a sessão (`add_task_to_session`,
  `stop_session_task`, `remove_session_task`, `clock_out`) conferem o projeto dela: quem perdeu o acesso ao projeto
  com o ponto aberto recebe `project.not_found`, e só um admin encerra aquela sessão. `get_active_session` e `whoami`
  continuam a mostrá-la. (Tirar a pessoa do projeto pela tela já encerra a sessão dela.)
- **Dinheiro fica de fora.** Nenhuma saída tem valor pago, cobrado, receita ou margem, nem para o dono.
- **Datas:** o prazo entra e sai como dia (`AAAA-MM-DD`), às 23:59 do relógio no fuso da organização, como a tela
  grava (também no dia em que o horário de verão começa ou termina). Os instantes voltam em RFC 3339, no mesmo fuso.
- **Ids:** um id em outra forma que o UUID aceita (maiúsculas, por exemplo) vale o mesmo id.
- **Etiquetas** vão pelo nome, sem diferenciar maiúsculas. Um nome que o projeto não tem é recusado
  (`mcp.unknown_label`) com a lista das que existem: criar etiqueta continua sendo da tela, com `labels.manage`.
- O `initialize` devolve instruções curtas sobre o sistema (uma sessão por pessoa, como trocar de tarefa, os status e
  as prioridades), e as ferramentas de leitura vêm marcadas como só leitura.

## Fora do MCP

Ainda não estão no MCP:

- **Outros clientes MCP** (ChatGPT, Cursor e afins): o OAuth aceita só os apps do Claude
  ([issue #36](https://github.com/itallovidal/working-time-tracker/issues/36)).
- Sincronizar o projeto inteiro, publicar uma tarefa numa plataforma, excluir tarefa, atribuir a outra pessoa, o ponto
  de outra pessoa e tudo o que é dinheiro.
