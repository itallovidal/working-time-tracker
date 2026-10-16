# Refatoração: Domain-First Total

> **Status**: Proposta para validação antes da execução.
> **Estratégia**: Big-bang em commit único atômico.
> **Objetivo**: Reorganizar o código da atual arquitetura *layered flat* (tudo misturado por camada) para **domain-first**, onde cada domínio é um package Go autossuficiente contendo seu model, store, service e handler.

---

## 1. Decisões de design consolidadas

| # | Decisão | Detalhe |
|---|---|---|
| 1 | **Domain-first total** | Cada domínio vira um package Go em `internal/domain/<dominio>/` contendo `model.go`, `store.go`, `service.go`, `handler.go` e seus testes. |
| 2 | **team_membership fundido em team** | Fica como sub-comportamento do domínio `team` (`membership_*.go` dentro de `internal/domain/team/`). |
| 3 | **time_entry → work_session** | Domínio renomeado para refletir o nome real do model (`WorkSession`). Inclui structs, arquivos, rotas HTTP e coleção de testes manuais. |
| 4 | **Adaptadores externos isolados** | `internal/integration/` (github/gitlab/crypto/registry/interface) vira `internal/adapter/`, evitando colisão semântica com o domínio `integration`. |
| 5 | **Tipos sem prefixo (idiomático Go)** | Em cada package, structs são apenas `Handler`, `Service`, `Store`, `NewHandler`, `NewService`, `NewStore`. O package já contextualiza: `organization.Handler`, `work_session.Service`, etc. |
| 6 | **Rotas renomeadas** | `/api/.../time-entries/*` → `/api/.../work-sessions/*`. Atualizar `_test/routes.md` e `_test/insomnia-collection.json`. |
| 7 | **Big-bang atômico** | Tudo num único commit. Checkpoints de compilação entre passos para detectar erros cedo. |

---

## 2. Estrutura final de diretórios

```
working-time-tracker/
├── cmd/
│   └── main.go                        ← reescrito (imports + construtores)
├── internal/
│   ├── adapter/                       ← NOVO (era internal/integration/)
│   │   ├── interface.go               ← Integration interface, tipos externos
│   │   ├── registry.go
│   │   ├── github.go
│   │   ├── gitlab.go
│   │   ├── crypto.go
│   │   └── crypto_test.go
│   ├── config/
│   │   └── config.go                  ← mantém
│   ├── domain/                        ← NOVO
│   │   ├── organization/
│   │   │   ├── model.go               ← type Organization
│   │   │   ├── store.go               ← type Store
│   │   │   ├── service.go             ← type Service, type Handler
│   │   │   ├── handler.go
│   │   │   ├── service_test.go
│   │   │   └── handler_test.go        ← extraído de handler_test.go único
│   │   ├── person/
│   │   │   └── (mesmo padrão)
│   │   ├── project/
│   │   │   └── (mesmo padrão)
│   │   ├── team/                      ← inclui team_membership
│   │   │   ├── model.go               ← type Team
│   │   │   ├── membership_model.go    ← type TeamMembership
│   │   │   ├── store.go               ← type Store (Team)
│   │   │   ├── membership_store.go    ← type MembershipStore
│   │   │   ├── service.go             ← type Service (Team)
│   │   │   ├── membership_service.go  ← type MembershipService
│   │   │   ├── handler.go             ← type Handler (inclui AddMember, RemoveMember, ListMembers)
│   │   │   ├── service_test.go
│   │   │   ├── membership_service_test.go
│   │   │   └── handler_test.go
│   │   ├── task/
│   │   │   └── (mesmo padrão)
│   │   ├── work_session/              ← NOVO (era time_entry)
│   │   │   ├── model.go               ← type WorkSession (mantém nome)
│   │   │   ├── store.go               ← type Store (era WorkSessionStore)
│   │   │   ├── service.go             ← type Service (era TimeEntryService)
│   │   │   ├── handler.go             ← type Handler (era TimeEntryHandler)
│   │   │   ├── service_test.go
│   │   │   └── handler_test.go
│   │   └── integration/
│   │       ├── model.go               ← type Integration (struct GORM)
│   │       ├── store.go
│   │       ├── service.go
│   │       ├── handler.go
│   │       ├── service_test.go
│   │       └── handler_test.go
│   ├── routes/
│   │   ├── routes.go                  ← reescrito (imports + /work-sessions)
│   │   ├── healthcheck.go             ← recebe as 2 funções de healthcheck
│   │   └── healthcheck_handler.go     ← MOVIDO de internal/handlers/healthcheckHandler.go
│   └── template/
│       └── renderer.go                ← mantém
├── testutil/                          ← NOVO
│   └── db.go                          ← helper p/ subir PostgreSQL de teste (TestMain)
├── web/                               ← mantém
└── (removidos:)
    ├── internal/handlers/             ← DELETADO (conteúdo movido para domain/)
    ├── internal/service/              ← DELETADO
    ├── internal/store/                ← DELETADO (exceto db.go — ver §5.3)
    ├── internal/model/                ← DELETADO
    └── internal/integration/          ← DELETADO (movido para adapter/)
```

---

## 3. Mapeamento detalhado de arquivos (origem → destino)

### 3.1 Organization

| Origem | Destino |
|---|---|
| `internal/model/organization.go` | `internal/domain/organization/model.go` |
| `internal/store/organization_store.go` | `internal/domain/organization/store.go` |
| `internal/service/organization_service.go` | `internal/domain/organization/service.go` |
| `internal/service/organization_service_test.go` | `internal/domain/organization/service_test.go` |
| `internal/handlers/organization_handler.go` | `internal/domain/organization/handler.go` |
| (parte de) `internal/handlers/handler_test.go` | `internal/domain/organization/handler_test.go` |

### 3.2 Person

| Origem | Destino |
|---|---|
| `internal/model/person.go` | `internal/domain/person/model.go` |
| `internal/store/person_store.go` | `internal/domain/person/store.go` |
| `internal/service/person_service.go` | `internal/domain/person/service.go` |
| `internal/service/person_service_test.go` | `internal/domain/person/service_test.go` |
| `internal/handlers/person_handler.go` | `internal/domain/person/handler.go` |
| (parte de) `handler_test.go` | `internal/domain/person/handler_test.go` |

### 3.3 Project

| Origem | Destino |
|---|---|
| `internal/model/project.go` | `internal/domain/project/model.go` |
| `internal/store/project_store.go` | `internal/domain/project/store.go` |
| `internal/service/project_service.go` | `internal/domain/project/service.go` |
| `internal/service/project_service_test.go` | `internal/domain/project/service_test.go` |
| `internal/handlers/project_handler.go` | `internal/domain/project/handler.go` |
| (parte de) `handler_test.go` | `internal/domain/project/handler_test.go` |

### 3.4 Team (inclui team_membership)

| Origem | Destino |
|---|---|
| `internal/model/team.go` | `internal/domain/team/model.go` |
| `internal/model/team_membership.go` | `internal/domain/team/membership_model.go` |
| `internal/store/team_store.go` | `internal/domain/team/store.go` |
| `internal/store/team_membership_store.go` | `internal/domain/team/membership_store.go` |
| `internal/service/team_service.go` | `internal/domain/team/service.go` |
| `internal/service/team_membership_service.go` | `internal/domain/team/membership_service.go` |
| `internal/service/team_service_test.go` | `internal/domain/team/service_test.go` |
| `internal/handlers/team_handler.go` | `internal/domain/team/handler.go` (mantém AddMember/RemoveMember/ListMembers) |
| (parte de) `handler_test.go` | `internal/domain/team/handler_test.go` |

### 3.5 Task

| Origem | Destino |
|---|---|
| `internal/model/task.go` | `internal/domain/task/model.go` |
| `internal/store/task_store.go` | `internal/domain/task/store.go` |
| `internal/service/task_service.go` | `internal/domain/task/service.go` |
| `internal/service/task_service_test.go` | `internal/domain/task/service_test.go` |
| `internal/handlers/task_handler.go` | `internal/domain/task/handler.go` |
| (parte de) `handler_test.go` | `internal/domain/task/handler_test.go` |

### 3.6 Work Session (era time_entry)

| Origem | Destino |
|---|---|
| `internal/model/work_session.go` | `internal/domain/work_session/model.go` |
| `internal/store/work_session_store.go` | `internal/domain/work_session/store.go` |
| `internal/service/time_entry_service.go` | `internal/domain/work_session/service.go` |
| `internal/service/time_entry_service_test.go` | `internal/domain/work_session/service_test.go` |
| `internal/handlers/time_entry_handler.go` | `internal/domain/work_session/handler.go` |
| (parte de) `handler_test.go` | `internal/domain/work_session/handler_test.go` |

### 3.7 Integration (domínio)

| Origem | Destino |
|---|---|
| `internal/model/integration.go` | `internal/domain/integration/model.go` |
| `internal/store/integration_store.go` | `internal/domain/integration/store.go` |
| `internal/service/integration_service.go` | `internal/domain/integration/service.go` |
| `internal/service/integration_service_test.go` | `internal/domain/integration/service_test.go` |
| `internal/handlers/integration_handler.go` | `internal/domain/integration/handler.go` |
| (parte de) `handler_test.go` | `internal/domain/integration/handler_test.go` |

### 3.8 Adapter (era integration/)

| Origem | Destino |
|---|---|
| `internal/integration/interface.go` | `internal/adapter/interface.go` |
| `internal/integration/registry.go` | `internal/adapter/registry.go` |
| `internal/integration/github.go` | `internal/adapter/github.go` |
| `internal/integration/gitlab.go` | `internal/adapter/gitlab.go` |
| `internal/integration/crypto.go` | `internal/adapter/crypto.go` |
| `internal/integration/crypto_test.go` | `internal/adapter/crypto_test.go` |

### 3.9 Infrastructure

| Origem | Destino |
|---|---|
| `internal/store/db.go` | `internal/database/db.go` (ver §5.3) |
| `internal/handlers/healthcheckHandler.go` | `internal/routes/healthcheck_handler.go` |
| `internal/service/setup_test.go` | (descartar lógica; migrar helper para `testutil/db.go`) |
| `internal/service/setup_test.go.bak` | DELETAR |

---

## 4. Mudanças de código (resumo de padrões)

### 4.1 Package declarations

Cada arquivo movido passa a declarar o package do seu domínio:

```go
// ANTES (em internal/service/organization_service.go)
package service

// DEPOIS (em internal/domain/organization/service.go)
package organization
```

Tabela de packages:

| Domínio | Package |
|---|---|
| organization | `organization` |
| person | `person` |
| project | `project` |
| team (inclui team_membership) | `team` |
| task | `task` |
| work_session | `work_session` |
| integration (domínio) | `integration` |
| adaptadores externos | `adapter` |
| infraestrutura de DB | `database` |
| helpers de teste | `testutil` |

### 4.2 Tipos sem prefixo — tabela completa de rename

> Regra: dentro de cada package, tipos e construtores perdem o prefixo do domínio. Modelos GORM mantêm o nome (são entidades de negócio, não componentização técnica).

#### Organization

| Antes | Depois |
|---|---|
| `OrganizationStore` | `Store` |
| `OrganizationService` | `Service` |
| `OrganizationHandler` | `Handler` |
| `NewOrganizationStore` | `NewStore` |
| `NewOrganizationService` | `NewService` |
| `NewOrganizationHandler` | `NewHandler` |
| `Organization` (model) | `Organization` (mantém) |

#### Person

| Antes | Depois |
|---|---|
| `PersonStore` | `Store` |
| `PersonService` | `Service` |
| `PersonHandler` | `Handler` |
| `NewPersonStore` | `NewStore` |
| `NewPersonService` | `NewService` |
| `NewPersonHandler` | `NewHandler` |
| `Person` (model) | `Person` (mantém) |

#### Project

| Antes | Depois |
|---|---|
| `ProjectStore` | `Store` |
| `ProjectService` | `Service` |
| `ProjectHandler` | `Handler` |
| `NewProjectStore` | `NewStore` |
| `NewProjectService` | `NewService` |
| `NewProjectHandler` | `NewHandler` |
| `Project` (model) | `Project` (mantém) |

#### Team (+ team_membership)

| Antes | Depois |
|---|---|
| `TeamStore` | `Store` |
| `TeamService` | `Service` |
| `TeamHandler` | `Handler` |
| `NewTeamStore` | `NewStore` |
| `NewTeamService` | `NewService` |
| `NewTeamHandler` | `NewHandler` |
| `TeamMembershipStore` | `MembershipStore` |
| `TeamMembershipService` | `MembershipService` |
| `NewTeamMembershipStore` | `NewMembershipStore` |
| `NewTeamMembershipService` | `NewMembershipService` |
| `Team` (model) | `Team` (mantém) |
| `TeamMembership` (model) | `TeamMembership` (mantém) |

#### Task

| Antes | Depois |
|---|---|
| `TaskStore` | `Store` |
| `TaskService` | `Service` |
| `TaskHandler` | `Handler` |
| `NewTaskStore` | `NewStore` |
| `NewTaskService` | `NewService` |
| `NewTaskHandler` | `NewHandler` |
| `Task` (model) | `Task` (mantém) |

#### Work Session (era time_entry) — renomeia também o domínio

| Antes | Depois |
|---|---|
| `WorkSessionStore` | `Store` |
| `TimeEntryService` | `Service` |
| `TimeEntryHandler` | `Handler` |
| `NewWorkSessionStore` | `NewStore` |
| `NewTimeEntryService` | `NewService` |
| `NewTimeEntryHandler` | `NewHandler` |
| `WorkSession` (model) | `WorkSession` (mantém) |
| variável `timeEntrySvc`, `timeEntryHandler` | `wsSvc`, `wsHandler` (ou `workSessionSvc`) |

#### Integration (domínio)

| Antes | Depois |
|---|---|
| `IntegrationStore` | `Store` |
| `IntegrationService` | `Service` |
| `IntegrationHandler` | `Handler` |
| `NewIntegrationStore` | `NewStore` |
| `NewIntegrationService` | `NewService` |
| `NewIntegrationHandler` | `NewHandler` |
| `Integration` (model GORM) | `Integration` (mantém) |

#### Adapter (antigo integration/)

> ⚠️ **Atenção**: o package `adapter` define a **interface** `Integration` (contrato para GitHub/GitLab), e o package `integration` (domínio) define a **struct** `Integration` (entidade GORM). São coisas diferentes em packages diferentes — Go permite, mas exige qualificação completa ao referenciar (`adapter.Integration` vs `integration.Integration`).

| Antes (package `integration`) | Depois (package `adapter`) |
|---|---|
| `Integration` (interface) | `Integration` (mantém — interface do adapter) |
| `Registry` / `NewRegistry` | mantém |
| `GithubIntegration` | mantém |
| `GitlabIntegration` | mantém |
| `Encrypt`, `Decrypt` | mantém |
| `ExternalItem` (se existir) | mantém |

### 4.3 Imports — transformação sistemática

Substituições globais em todos os arquivos `.go`:

```go
// ANTES
import (
    "working-time-tracker/internal/handlers"
    "working-time-tracker/internal/service"
    "working-time-tracker/internal/store"
    "working-time-tracker/internal/model"
    "working-time-tracker/internal/integration"  // adaptadores
)

// DEPOIS (depende do domínio)
import (
    "working-time-tracker/internal/domain/organization"
    "working-time-tracker/internal/domain/task"
    "working-time-tracker/internal/adapter"
)
```

### 4.4 Rotas HTTP — `/time-entries` → `/work-sessions`

Em `internal/routes/routes.go`:

| Antes | Depois |
|---|---|
| `/api/projects/:projectId/time-entries/clock-in` | `/api/projects/:projectId/work-sessions/clock-in` |
| `/api/projects/:projectId/time-entries/clock-out` | `/api/projects/:projectId/work-sessions/clock-out` |
| `/api/projects/:projectId/time-entries` | `/api/projects/:projectId/work-sessions` |
| `/api/projects/:projectId/time-entries/total` | `/api/projects/:projectId/work-sessions/total` |

---

## 5. Pontos sensíveis

### 5.1 `handler_test.go` (459 linhas, único, com `TestMain` global)

Hoje o arquivo contém:
- `func TestMain(m *testing.M)` que sobe PostgreSQL de teste (via `testDB` global).
- Todos os testes end-to-end de todos os domínios misturados.

**Plano**:

1. Extrair o setup de DB para `testutil/db.go`:
   ```go
   package testutil

   func OpenTestDB(t *testing.T) *gorm.DB { ... }
   // ou
   func SetupTestDB() (*gorm.DB, func())  // returns cleanup
   ```
2. Quebrar `handler_test.go` em 7 arquivos `handler_test.go`, um por domínio em seu package. Cada um importa `testutil` para obter o DB.
3. Cada package de domínio terá seu próprio `TestMain` (Go permite um por package).

### 5.2 `cmd/main.go` (91 linhas de wiring manual)

Praticamente reescrito. Antes:

```go
orgStore := store.NewOrganizationStore(db)
orgSvc := service.NewOrganizationService(orgStore)
orgHandler := handlers.NewOrganizationHandler(orgSvc)
```

Depois:

```go
orgStore := organization.NewStore(db)
orgSvc := organization.NewService(orgStore)
orgHandler := organization.NewHandler(orgSvc)
```

Cadeia completa (todas as 7+):

```go
// Organization
orgStore := organization.NewStore(db)
orgSvc := organization.NewService(orgStore)
orgHandler := organization.NewHandler(orgSvc)

// Person
personStore := person.NewStore(db)
personSvc := person.NewService(personStore)
personHandler := person.NewHandler(personSvc)

// Project
projectStore := project.NewStore(db)
projectSvc := project.NewService(projectStore)
projectHandler := project.NewHandler(projectSvc)

// Team (inclui membership)
teamStore := team.NewStore(db)
membershipStore := team.NewMembershipStore(db)
teamSvc := team.NewService(teamStore)
membershipSvc := team.NewMembershipService(membershipStore)
teamHandler := team.NewHandler(teamSvc, membershipSvc)

// Task (cross-domain: depende de IntegrationService + MembershipStore)
taskStore := task.NewStore(db)
taskSvc := task.NewService(taskStore, membershipStore, integrationSvc)
taskHandler := task.NewHandler(taskSvc)

// Work Session (cross-domain: depende de TaskStore)
wsStore := work_session.NewStore(db)
wsSvc := work_session.NewService(wsStore, taskStore)
wsHandler := work_session.NewHandler(wsSvc)

// Integration (depende do adapter.Registry)
integrationStore := integration.NewStore(db)
integrationSvc := integration.NewService(integrationStore, registry)
integrationHandler := integration.NewHandler(integrationSvc)

routes.RegisterRoutes(e,
    orgHandler, personHandler, projectHandler,
    teamHandler, taskHandler, wsHandler, integrationHandler,
)
```

> ⚠️ **Ordem de instanciação importa**: `integration` antes de `task`; `task` antes de `work_session` por causa das dependências cross-domain. Confirmar assinaturas exatas dos construtores ao implementar.

### 5.3 Destino de `internal/store/db.go`

Contém `Open(cfg)` e `AutoMigrate(db)`. Não pertence a um domínio específico. Opções:

- **(recomendado)** `internal/database/db.go` com `package database`.
- Alternativa: `internal/store/db.go` mantido (só com a infraestrutura de conexão).

`AutoMigrate` precisará listar todos os models das referências qualificadas:

```go
db.AutoMigrate(
    &organization.Organization{},
    &person.Person{},
    &project.Project{},
    &team.Team{},
    &team.TeamMembership{},
    &task.Task{},
    &work_session.WorkSession{},
    &integration.Integration{},
)
```

### 5.4 `routes/routes.go` — assinatura de `RegisterRoutes`

A função recebe hoje 7 handlers posicionais. Após refatoração, continua recebendo 7 mas com tipos diferentes:

```go
func RegisterRoutes(
    e *echo.Echo,
    orgHandler *organization.Handler,
    personHandler *person.Handler,
    projectHandler *project.Handler,
    teamHandler *team.Handler,
    taskHandler *task.Handler,
    wsHandler *work_session.Handler,
    integrationHandler *integration.Handler,
) { ... }
```

### 5.5 Healthcheck

`internal/handlers/healthcheckHandler.go` define funções de pacote `RegisterHealthcheckHandler` e `RegisterHelloWorldHandler` (sem struct, sem service). Como não há domínio, mover para `internal/routes/healthcheck_handler.go` no mesmo package `routes`, onde já é consumido por `healthcheck.go`.

### 5.6 Dependências cross-domain (preservar)

- `task.Service` → depende de `integration.Service` e `team.MembershipStore`.
- `work_session.Service` → depende de `task.Store`.

No código, dentro de `internal/domain/task/service.go`:

```go
package task

import (
    "working-time-tracker/internal/domain/integration"
    "working-time-tracker/internal/domain/team"
)

type Service struct {
    taskStore       *Store
    membershipStore *team.MembershipStore
    integrationSvc  *integration.Service
}
```

### 5.7 Colisão de nomes `Integration`

- `adapter.Integration` = **interface** (contrato para GitHub/GitLab).
- `integration.Integration` = **struct GORM** (configuração persistida).

Sempre qualificar no uso. Em `internal/domain/integration/service.go`, é provável que ambos sejam referenciados:

```go
package integration

import (
    "working-time-tracker/internal/adapter"
)

type Service struct {
    store    *Store
    registry *adapter.Registry
    // se usar adapter.Integration em algum método, qualificar sempre
}
```

### 5.8 Atualização de artefatos auxiliares

- `_test/routes.md` — substituir todas as ocorrências de `/time-entries` por `/work-sessions`.
- `_test/insomnia-collection.json` — mesmo.
- `_docs/design.md` — se houver diagrama de pacotes ou descrição da arquitetura em camadas, atualizar para refletir domain-first.
- `_docs/tasks.md` — adicionar entrada de sprint para esta refatoração (se desejar rastreabilidade).

### 5.9 Lixo a remover

- `internal/service/setup_test.go.bak` — deletar.

---

## 6. Checklist de execução (big-bang com checkpoints)

Marque cada passo ao concluir. Compile (`go build ./...`) ao final de cada checkpoint para detectar erros cedo.

### Passo 1 — Criar nova árvore de pastas

```bash
mkdir -p internal/domain/{organization,person,project,team,task,work_session,integration}
mkdir -p internal/adapter
mkdir -p internal/database
mkdir -p testutil
```

- [ ] Criadas todas as pastas

### Passo 2 — Mover arquivos com `git mv` (preserva histórico)

Mover **sem editar conteúdo ainda**. Apenas reposicionar arquivos.

- [ ] Mover todos os arquivos de model
- [ ] Mover todos os arquivos de store (exceto `db.go`)
- [ ] Mover todos os arquivos de service
- [ ] Mover todos os arquivos de handler (exceto healthcheck)
- [ ] Mover adaptadores de `internal/integration/` → `internal/adapter/`
- [ ] Mover `internal/store/db.go` → `internal/database/db.go`
- [ ] Mover `healthcheckHandler.go` → `internal/routes/healthcheck_handler.go`
- [ ] Deletar `setup_test.go.bak`
- [ ] **Checkpoint**: `git status` mostra todos os renames

### Passo 3 — Ajustar `package` declarations

Em cada arquivo movido, trocar o `package X` pela declaração correta conforme tabela do §4.1.

- [ ] Todos os arquivos `.go` com package correto
- [ ] **Checkpoint**: `go build ./...` ainda pode falhar por imports, mas erro deve ser apenas "package mismatch"

### Passo 4 — Rename de tipos (sem prefixo)

Em cada arquivo de cada domínio, aplicar a tabela do §4.2. Usar find-and-replace com cuidado dentro de cada arquivo.

- [ ] organization: `OrganizationStore` → `Store`, `OrganizationService` → `Service`, etc.
- [ ] person
- [ ] project
- [ ] team + team_membership
- [ ] task
- [ ] work_session (inclui rename de `TimeEntryHandler`/`TimeEntryService`)
- [ ] integration (domínio)
- [ ] adapter (apenas package declaration + ajustes internos)

### Passo 5 — Atualizar imports em todos os arquivos

Aplicar a transformação do §4.3 em:

- [ ] Todos os arquivos dentro de `internal/domain/*` (referenciam uns aos outros e o `adapter`)
- [ ] `cmd/main.go`
- [ ] `internal/routes/routes.go`
- [ ] `internal/routes/healthcheck_handler.go` (se houver import)
- [ ] `internal/database/db.go` (para `AutoMigrate`)
- [ ] **Checkpoint**: `go build ./...` deve passar sem erros

### Passo 6 — Ajustar `cmd/main.go`

- [ ] Reescrever imports e construtores conforme §5.2
- [ ] Renomear variáveis `timeEntrySvc`/`timeEntryHandler` → `wsSvc`/`wsHandler`
- [ ] **Checkpoint**: `go vet ./...` limpo

### Passo 7 — Ajustar `routes/routes.go`

- [ ] Nova assinatura de `RegisterRoutes` (tipos qualificados por package)
- [ ] Renomear rotas `/time-entries` → `/work-sessions` (§4.4)
- [ ] **Checkpoint**: `go build ./...` passa

### Passo 8 — Refatorar testes

- [ ] Criar `testutil/db.go` com helper `OpenTestDB` (ou similar)
- [ ] Quebrar `handler_test.go` único em 7 arquivos `handler_test.go` (um por domínio)
- [ ] Em cada package de domínio, manter/recriar `TestMain` se necessário
- [ ] Atualizar todos os imports nos testes de service também
- [ ] **Checkpoint**: `go test ./...` todos passando

### Passo 9 — Atualizar artefatos auxiliares

- [ ] `_test/routes.md`: substituir `/time-entries` por `/work-sessions`
- [ ] `_test/insomnia-collection.json`: substituir `/time-entries` por `/work-sessions`
- [ ] `_docs/design.md`: atualizar descrição de arquitetura se aplicável
- [ ] `_docs/tasks.md`: registrar refatoração concluída

### Passo 10 — Validação final

- [ ] `gofmt -w .`
- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `git status` — confirmar que `internal/handlers/`, `internal/service/`, `internal/store/`, `internal/model/`, `internal/integration/` foram removidos
- [ ] Subir servidor manualmente e bater num endpoint de cada domínio (opcional, mas recomendado)

### Passo 11 — Commit

```bash
git add -A
git commit -m "refactor: reorganize codebase to domain-first architecture

- Move from layered-flat (handlers/, service/, store/, model/ per layer)
  to domain-first (internal/domain/<dominio>/ with model, store, service,
  handler in the same package).
- Rename time_entry domain to work_session to align with model name.
- Move external adapters (github/gitlab/crypto) from internal/integration/
  to internal/adapter/ to avoid semantic collision with integration domain.
- Rename types to drop domain prefix (organization.NewHandler instead of
  handlers.NewOrganizationHandler) — idiomatic Go with packages providing
  context.
- Rename HTTP routes /api/time-entries/* to /api/work-sessions/*.
- Extract shared test DB setup to testutil package.
- Remove orphan setup_test.go.bak."
```

---

## 7. Riscos e mitigações

| Risco | Mitigação |
|---|---|
| Ciclo de imports entre domínios | Go detecta em tempo de compilação. Se acontecer (ex.: `integration` importar `task` e vice-versa), extrair tipo compartilhado para um package `internal/domain/shared` ou mover regra para um domínio neutro. |
| Quebrar testes existentes por conta do `testDB` global | Implementar `testutil.OpenTestDB` primeiro, antes de mover os testes. Validar com 1 domínio antes de replicar para os outros. |
| Coleção Insomnia quebra para QA | Atualizar `_test/insomnia-collection.json` no mesmo commit. Comunicar mudança de rotas ao time. |
| Esquecer de migrar alguma referência | `go build ./...` e `go test ./...` ao final de cada checkpoint. Usar `grep -r "internal/handlers\|internal/service\b\|internal/store\b\|internal/model\b\|internal/integration" internal/ cmd/` para checar sobras. |
| Perder histórico do git | Usar `git mv` em vez de `mv` + `git add`. O GitHub preserva blame com `--follow`. |
| Adapters com nome `Integration` confundindo com domínio | Sempre qualificar (`adapter.Integration` vs `integration.Integration`). Se atrapalhar muito, renomear a interface do adapter para `Provider` ou `ExternalIntegration`. |

---

## 8. Pós-refatoração (não-escopo, mas recomendado)

Após validar a nova estrutura, considerar para sprint seguinte:

- Extrair interfaces para stores/services ( permitir mocking nos testes de handler sem subir Postgres ).
- Introduzir um `cmd/main.go` mais enxuto usando *functional options* ou factory por domínio (ex.: `organization.NewModule(db) (*Handler, error)` instancia toda a cadeia).
- Avaliar mover `team.TeamMembership` para domínio próprio se crescer em complexidade.
- Documentar a arquitetura em `_docs/design.md` com um diagrama de pacotes atualizado.
