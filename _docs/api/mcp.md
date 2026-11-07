# MCP: o Claude no sistema

O servidor MCP ([Model Context Protocol](https://modelcontextprotocol.io)) deixa o Claude Code agir como uma pessoa:
ver e criar tarefas, atualizá-las, pegar uma tarefa para si, sincronizá-la com a plataforma, bater o ponto, trocar de
tarefa e ver as próprias horas. Ele mora no mesmo binário, em `/mcp`, e entra com um **token pessoal** que a pessoa gera
no perfil. Quem decidiu e por quê está na [Decisão 22](../architecture/decisions.md).

Nesta etapa o caminho é o **Claude Code**, que aceita o token num cabeçalho. O claude.ai, o Claude Desktop e o
celular só conectam um servidor remoto por OAuth, que ainda não existe aqui (ver [Fora do MCP](#fora-do-mcp)).

## Conectar

1. No sistema, abra **Perfil** e, em **Conectar ao Claude**, clique em **Novo token**. Dê um nome (para reconhecê-lo
   depois) e escolha a validade: 30, 90 ou 365 dias.
2. O modal mostra o token **uma vez só**, com o comando pronto. Rode no terminal:

   ```bash
   claude mcp add --transport http --scope user wtt https://<endereço do sistema>/mcp --header "Authorization: Bearer wtt_…"
   ```

3. Numa conversa do Claude Code, peça, por exemplo, "quais são as minhas tarefas?" ou "bate o ponto na tarefa X".
   `claude mcp list` mostra se a conexão está de pé.

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

## O token

- É `wtt_` seguido de 32 bytes aleatórios. O banco guarda só o sha256 (`api_tokens.token_hash`), e a lista do perfil
  mostra só o começo (`prefix`).
- **Só vale no `/mcp`.** A API continua só com o cookie: um token não abre `/api`, não gera outro token e não mexe na
  conta.
- Vence (30, 90 ou 365 dias; não há "nunca"). Cada pessoa tem até 10 válidos.
- Revogar no perfil corta o acesso na hora. **Trocar a senha revoga todos**, como já encerra as outras sessões. Os
  tokens também somem com a pessoa.
- A identidade é lida de novo a cada requisição: um cargo que mudou, ou um projeto de que a pessoa saiu, vale na hora.
- As rotas da API do token estão em [reference.md](reference.md#tokens-do-claude).

## Como funciona

- **Transporte:** Streamable HTTP **sem estado** (cada `POST /mcp` é completo, e as respostas vêm em JSON). Nenhuma
  sessão fica na memória do processo, então a troca de processo de um deploy não derruba quem está conectado. Um `GET`
  responde `405`.
- **Sem token, ou com um inválido, vencido ou revogado:** `401` com `WWW-Authenticate: Bearer scope="mcp"`. O cookie
  de sessão não vale no `/mcp`.
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
- **Código:** `internal/mcp` (o servidor e as ferramentas), `internal/domain/auth/api_token.go` (o token) e os testes
  em `internal/server/mcp_test.go` e `internal/server/api_tokens_test.go`.

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
  sessão (`editable`).
- **Dinheiro fica de fora.** Nenhuma saída tem valor pago, cobrado, receita ou margem, nem para o dono.
- **Datas:** o prazo entra e sai como dia (`AAAA-MM-DD`), às 23:59 no fuso da organização, como a tela grava. Os
  instantes voltam em RFC 3339, no mesmo fuso.
- **Etiquetas** vão pelo nome, sem diferenciar maiúsculas. Um nome que o projeto não tem é recusado
  (`mcp.unknown_label`) com a lista das que existem: criar etiqueta continua sendo da tela, com `labels.manage`.
- O `initialize` devolve instruções curtas sobre o sistema (uma sessão por pessoa, como trocar de tarefa, os status e
  as prioridades), e as ferramentas de leitura vêm marcadas como só leitura.

## Fora do MCP

Ainda não estão no MCP:

- **O claude.ai, o Desktop e o celular.** Eles só conectam por OAuth 2.1: os metadados do recurso
  (`/.well-known/oauth-protected-resource`), o servidor de autorização com a tela de consentimento, `/oauth/token` com
  renovação e o cadastro do cliente (CIMD ou DCR). O verificador do token (`internal/mcp/auth.go`) é onde isso encaixa,
  com o mesmo escopo `mcp`.
- Sincronizar o projeto inteiro, publicar uma tarefa numa plataforma, excluir tarefa, atribuir a outra pessoa, o ponto
  de outra pessoa e tudo o que é dinheiro.
