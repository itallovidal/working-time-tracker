# Working Time Tracker

Sistema de rastreamento de tempo de trabalho para organizações. Permite que times registrem sessões de trabalho em tarefas, organizadas por projetos e times, com integração a plataformas externas (GitHub, GitLab, Slack, Trello, etc.).

## Funcionalidades

### Organizações, Projetos e Times
- Cadastro de **organizações** (empresas) como tenant isolado
- **Projetos** dentro de cada organização, com configurações próprias:
  - Duração da sprint (dias)
  - Horário do daily standup
  - Dia da weekly sync
- **Times** dentro de cada projeto para agrupar pessoas
- Uma pessoa pode pertencer a múltiplos times dentro de um projeto

### Pessoas
- Cadastro de pessoas vinculadas a uma organização
- Email único por organização
- Associação a times via membership

### Tarefas
- CRUD de tarefas dentro de um projeto
- Atribuição de uma pessoa responsável (deve ser membro de algum time do projeto)
- Deadline com valor padrão de 7 dias a partir da criação
- Vinculação opcional a um item externo (issue do GitHub, card do Trello, etc.) via integration
- Recuperação sob demanda dos detalhes do item externo (título, status, URL)

### Registro de Tempo (Work Sessions)
- **Clock-in**: inicia uma sessão de trabalho em uma tarefa
- **Clock-out**: finaliza a sessão de trabalho ativa
- Apenas **uma sessão ativa por pessoa** por vez (garantido por índice único parcial no banco)
- Listagem de sessões por projeto com filtros por tarefa e/ou pessoa
- Cálculo de **tempo total** trabalhado por tarefa ou pessoa

### Integrações
- Sistema genérico de integrações configurado por projeto
- Tipos suportados: GitHub, GitLab, Slack, Trello (extensível)
- Credenciais armazenadas criptografadas (AES-GCM) no banco
- Validação de credenciais via chamada à API da plataforma
- Fetch sob demanda de detalhes de itens externos com cache em memória (TTL 60s)
- Degradação graciosa: se a API da integração estiver indisponível, retorna null sem quebrar

## API REST

Todas as chamadas são JSON. Endpoints são escopados por organização e projeto.

### Organizações
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/orgs` | Criar organização |
| GET | `/api/orgs` | Listar organizações |
| GET | `/api/orgs/:orgId` | Detalhar organização |
| PATCH | `/api/orgs/:orgId` | Atualizar organização |
| DELETE | `/api/orgs/:orgId` | Excluir organização (rejeita se houver projetos ativos) |

### Pessoas
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/orgs/:orgId/persons` | Criar pessoa |
| GET | `/api/orgs/:orgId/persons` | Listar pessoas da organização |
| GET | `/api/persons/:personId` | Detalhar pessoa |
| PATCH | `/api/persons/:personId` | Atualizar pessoa |

### Projetos
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/orgs/:orgId/projects` | Criar projeto |
| GET | `/api/orgs/:orgId/projects` | Listar projetos da organização |
| GET | `/api/projects/:projectId` | Detalhar projeto |
| PATCH | `/api/projects/:projectId` | Atualizar projeto |
| DELETE | `/api/projects/:projectId` | Excluir projeto (cascade em times, tarefas, sessões, integrações) |

### Times
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/projects/:projectId/teams` | Criar time |
| GET | `/api/projects/:projectId/teams` | Listar times do projeto |
| GET | `/api/teams/:teamId` | Detalhar time |
| PATCH | `/api/teams/:teamId` | Atualizar time |
| DELETE | `/api/teams/:teamId` | Excluir time |

### Team Memberships
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/teams/:teamId/members` | Adicionar membro |
| DELETE | `/api/teams/:teamId/members` | Remover membro |
| GET | `/api/teams/:teamId/members` | Listar membros |

### Tarefas
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/projects/:projectId/tasks` | Criar tarefa |
| GET | `/api/projects/:projectId/tasks` | Listar tarefas do projeto |
| GET | `/api/tasks/:taskId` | Detalhar tarefa (inclui detalhes do item externo se vinculado) |
| PATCH | `/api/tasks/:taskId` | Atualizar tarefa |
| DELETE | `/api/tasks/:taskId` | Excluir tarefa (cascade em sessões) |
| POST | `/api/tasks/:taskId/link-external-item` | Vincular item externo |
| DELETE | `/api/tasks/:taskId/link-external-item` | Desvincular item externo |
| GET | `/api/tasks/:taskId/external-details` | Buscar detalhes do item externo via integração |

### Work Sessions (Registro de Tempo)
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/projects/:projectId/work-sessions/clock-in` | Iniciar sessão (body: task_id, person_id) |
| POST | `/api/projects/:projectId/work-sessions/clock-out` | Finalizar sessão (body: person_id) |
| GET | `/api/projects/:projectId/work-sessions` | Listar sessões (filtros: task_id, person_id) |
| GET | `/api/projects/:projectId/work-sessions/total` | Tempo total (filtros: task_id, person_id) |

### Integrações
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/projects/:projectId/integrations` | Criar integração (valida credenciais) |
| GET | `/api/projects/:projectId/integrations` | Listar integrações do projeto |
| GET | `/api/integrations/:integrationId` | Detalhar integração |
| PATCH | `/api/integrations/:integrationId` | Atualizar integração |
| DELETE | `/api/integrations/:integrationId` | Excluir integração |

## Stack

| Camada | Tecnologia |
|--------|-----------|
| Backend | Go + Echo v5 |
| ORM | GORM |
| Banco | PostgreSQL |
| Frontend | Server-rendered HTML (Go templates) + Alpine.js |
| Build | Single binary via `embed.FS` |

## Pré-requisitos

- Go 1.26+
- PostgreSQL 14+

## Configuração

Variáveis de ambiente (ou arquivo `.env`):

```
API_PORT=8080
DATABASE_URL=postgres://user:password@localhost:5432/working_time_tracker?sslmode=disable
INTEGRATION_ENCRYPTION_KEY=uma-chave-hex-de-32-bytes
```

## Como rodar

```bash
# Instalar dependências
go mod tidy

# Configurar variáveis de ambiente
cp .env.example .env
# Editar .env com suas credenciais

# Iniciar servidor
go run ./cmd

# Build single binary
go build -o wtt ./cmd
./wtt
```

O servidor inicia na porta configurada (padrão `8080`). Acesse `http://localhost:8080` para a interface web. A API REST está disponível em `http://localhost:8080/api/`.

## Estrutura do Projeto

```
cmd/                        # Entry point do servidor
  main.go
internal/
  config/                   # Carregamento de configuração do ambiente
  handlers/                 # Handlers HTTP (REST)
  integration/              # Implementações de integrações (GitHub, GitLab, etc.)
  model/                    # Modelos GORM (Organization, Person, Project, Team, etc.)
  routes/                   # Definição de rotas
  service/                  # Lógica de negócio
  store/                    # Conexão com banco e migrações
  template/                 # Renderer de templates para Echo
web/
  embed.go                  # Embed dos assets via embed.FS
  templates/                # Templates HTML (Go templates)
    base.gohtml             # Layout base
  static/                   # Assets estáticos
    alpine.min.js           # Alpine.js vendored
```

## Modelo de Dados

```
Organization (1) ──── (N) Project
Organization (1) ──── (N) Person
Project (1) ──── (N) Team
Project (1) ──── (N) Task
Project (1) ──── (N) Integration
Team (N) ──── (N) Person  (via TeamMembership)
Task (1) ──── (N) WorkSession
Task (0..1) ──── (0..1) Integration (via external_integration_id)
```

Uma `WorkSession` pertence a uma `Task` e a uma `Person`. Apenas uma sessão ativa por pessoa é permitida (garantido por índice único parcial `one_active_session`).

## Segurança

- Credenciais de integrações são criptografadas em repouso usando AES-GCM
- Credenciais nunca são retornadas em respostas da API
- Chave de criptografia configurada via variável de ambiente (`INTEGRATION_ENCRYPTION_KEY`)
