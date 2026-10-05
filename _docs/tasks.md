# Working Time Tracker — Tasks

## 📦 FASE 1: BACKEND

---

### Sprint 1: Foundation — Setup & Database

- [X] S1.1 Initialize Go module and add dependencies: Echo web framework, GORM (`gorm.io/gorm`, `gorm.io/driver/postgres`), UUID lib, env config loader; vendor Alpine.js as a local static asset
- [X] S1.2 Create base project structure: `cmd/server/main.go`, `internal/handler/`, `internal/service/`, `internal/store/`, `internal/model/`, `internal/config/`, `internal/integration/`, `web/templates/`, `web/static/`
- [X] S1.3 Add configuration loading from environment (DATABASE_URL, PORT, INTEGRATION_ENCRYPTION_KEY)
- [X] S1.4 Set up GORM database connection (Postgres via `gorm.io/driver/postgres`), Echo server, and graceful shutdown in `main.go`
- [X] S1.5 Configure Echo to render HTML templates and serve static assets (Alpine.js); embed `web/` via `embed.FS`
- [X] S1.6 Define GORM model struct for `Organization` (id UUID, name) with appropriate GORM tags
- [X] S1.7 Define GORM model struct for `Person` (id UUID, name, email, organization_id FK) with unique index tag on (organization_id, email)
- [X] S1.8 Define GORM model struct for `Project` (id UUID, organization_id FK, name, description, github_repo_url nullable, gitlab_repo_url nullable, sprint_duration_days, daily_time nullable, weekly_sync_day nullable) with foreign key constraints
- [X] S1.9 Define GORM model struct for `Team` (id UUID, name, project_id FK)
- [X] S1.10 Define GORM model struct for `TeamMembership` (person_id FK, team_id FK) with unique composite constraint
- [X] S1.11 Define GORM model struct for `Task` (id UUID, project_id FK, name, description, assignee_id FK, deadline, external_integration_id FK nullable, external_item_id nullable, external_item_url nullable)
- [X] S1.12 Define GORM model struct for `TimeEntry` (id UUID, task_id FK, person_id FK, start_at, end_at nullable)
- [X] S1.13 Define GORM model struct for `Integration` (id UUID, project_id FK, type, display_name, config JSONB with custom serializer, enabled) with JSONB serialization
- [X] S1.14 Run GORM AutoMigrate for all models on startup; apply partial unique index on `time_entries (person_id) WHERE end_at IS NULL` via `db.Exec()`

---

### Sprint 2: Organization & Person

- [X] S2.1 Implement `OrganizationService` with Create, List, Get, Update, Delete methods
- [X] S2.2 Implement `POST /api/orgs` handler (validate name required)
- [X] S2.3 Implement `GET /api/orgs` and `GET /api/orgs/:orgId` handlers
- [X] S2.4 Implement `PATCH /api/orgs/:orgId` handler
- [X] S2.5 Implement `DELETE /api/orgs/:orgId` handler (reject if org has active projects)
- [X] S2.6 Implement `PersonService` with Create, List, Get, Update methods (scoped to organization)
- [X] S2.7 Implement create logic: validate unique email within the organization
- [X] S2.8 Implement `POST /api/orgs/:orgId/persons` handler (validate name + email required)
- [X] S2.9 Implement `GET /api/orgs/:orgId/persons` handler
- [X] S2.10 Implement `GET /api/persons/:personId` and `PATCH /api/persons/:personId` handlers

---

### Sprint 3: Project & Team

- [X] S3.1 Implement `ProjectService` with Create, List, Get, Update, Delete methods (scoped to organization)
- [X] S3.2 Implement create logic: default sprint_duration_days = 14, validate config fields
- [X] S3.3 Implement `POST /api/orgs/:orgId/projects` handler (validate name required)
- [X] S3.4 Implement `GET /api/orgs/:orgId/projects` handler
- [X] S3.5 Implement `GET /api/projects/:projectId` and `PATCH /api/projects/:projectId` handlers
- [X] S3.6 Implement `DELETE /api/projects/:projectId` handler (cascade delete teams, tasks, time entries, integrations)
- [X] S3.7 Implement `TeamService` with Create, List, Get, Update, Delete methods (scoped to project)
- [X] S3.8 Implement `POST /api/projects/:projectId/teams` handler (validate name required)
- [X] S3.9 Implement `GET /api/projects/:projectId/teams` handler
- [X] S3.10 Implement `GET /api/teams/:teamId` / `PATCH /api/teams/:teamId` / `DELETE /api/teams/:teamId` handlers
- [X] S3.11 Implement `TeamMembershipService` with Add, Remove, ListByTeam methods
- [X] S3.12 Implement `POST /api/teams/:teamId/members` handler (validate person exists in org)
- [X] S3.13 Implement `DELETE /api/teams/:teamId/members` handler
- [X] S3.14 Implement `GET /api/teams/:teamId/members` handler

---

### Sprint 4: Task Management

- [X] S4.1 Implement `TaskService` with Create, Get, List, Update, Delete methods (scoped to project)
- [X] S4.2 Implement create logic: default deadline to created_at + 7 days when not provided
- [X] S4.3 Implement `POST /api/projects/:projectId/tasks` handler (validate name + assignee required; assignee must be team member of the project)
- [X] S4.4 Implement `GET /api/projects/:projectId/tasks` handler
- [X] S4.5 Implement `GET /api/tasks/:taskId` handler (include external item details if linked)
- [X] S4.6 Implement `PATCH /api/tasks/:taskId` handler
- [X] S4.7 Implement `DELETE /api/tasks/:taskId` handler (cascade delete associated time entries)
- [X] S4.8 Implement `POST /api/tasks/:taskId/link-external-item` handler (body: integration_id, external_item_id, external_item_url)
- [X] S4.9 Implement `DELETE /api/tasks/:taskId/link-external-item` handler
- [X] S4.10 Implement `GET /api/tasks/:taskId/external-details` handler (fetch via integration)

---

### Sprint 5: Time Tracking

- [X] S5.1 Implement `TimeEntryService` with ClockIn, ClockOut, ListByTask, ListByPerson, TotalTime methods
- [X] S5.2 Implement ClockIn: validate task exists in project; check no active entry for person (global); insert entry with start_at = now, end_at = NULL
- [X] S5.3 Implement ClockOut: find active entry for person; set end_at = now; error if none exists
- [X] S5.4 Implement `POST /api/projects/:projectId/time-entries/clock-in` handler (body: task_id, person_id)
- [X] S5.5 Implement `POST /api/projects/:projectId/time-entries/clock-out` handler (body: person_id)
- [X] S5.6 Implement `GET /api/projects/:projectId/time-entries` handler with task_id and person_id query filters
- [X] S5.7 Implement `GET /api/projects/:projectId/time-entries/total` handler computing total duration for task or person
- [X] S5.8 Return friendly error when clock-in is attempted while already clocked in

---

### Sprint 6: Integrations

- [X] S6.1 Implement `IntegrationService` with Create, List, Get, Update, Delete, ValidateConfig methods
- [X] S6.2 Define `Integration` interface: `ValidateConfig(config) error`, `FetchItemDetails(config, itemID) (ItemDetails, error)`
- [X] S6.3 Implement GitHub integration type (validates token via GitHub API, fetches issue by number)
- [X] S6.4 Implement GitLab integration type (validates token via GitLab API, fetches issue by IID)
- [X] S6.5 Implement credential encryption/decryption helper (AES-GCM using INTEGRATION_ENCRYPTION_KEY)
- [X] S6.6 Implement `POST /api/projects/:projectId/integrations` handler (validates config via type-specific implementation)
- [X] S6.7 Implement `GET /api/projects/:projectId/integrations` handler
- [X] S6.8 Implement `GET /api/integrations/:integrationId` / `PATCH /api/integrations/:integrationId` / `DELETE /api/integrations/:integrationId` handlers
- [X] S6.9 Graceful degradation: return null external details with error indicator when integration API is unreachable

---

### Sprint 7: Security & Backend Tests

- [X] S7.1 Ensure integration credentials are never included in any API response, log, or error message
- [X] S7.2 Add redaction for credential fields in request/response logging
- [X] S7.3 Validate that integration type matches a known, whitelisted type before persisting
- [X] S7.4 Write tests for OrganizationService: create, list, update, delete, delete-with-projects-rejected
- [X] S7.5 Write tests for PersonService: create, unique email within org, list by org
- [X] S7.6 Write tests for ProjectService: create with defaults, create with explicit config, update config, delete cascade
- [X] S7.7 Write tests for TeamService + TeamMembershipService: create, add/remove members, duplicate membership rejected
- [X] S7.8 Write tests for TaskService: create with default deadline, create with explicit deadline, assignee must be team member, update, delete cascade, external item link/unlink
- [X] S7.9 Write tests for TimeEntryService: clock in success, clock in on missing task, clock out success, clock out with no active session, overlapping active session rejection, total time calculation
- [X] S7.10 Write tests for IntegrationService: create with valid/invalid config, credentials encrypted at rest, credentials never returned on get
- [X] S7.11 Write tests for credential encryption helper: round-trip encrypt/decrypt, wrong key fails
- [X] S7.12 Write handler/integration tests for all API endpoints covering happy path and error cases

---

### Correções pós-migração GORM → Ent

- [X] C1 FKs com `ON DELETE CASCADE` (projeto → times/tarefas/integrações, tarefa → sessões, time → membros, organização → pessoas)
- [X] C2 Índice parcial `one_active_session` declarado no schema do Ent em vez de SQL cru após o auto-migrate
- [X] C3 `POST/GET /api/orgs` respondiam 404 (rota registrada como `"/"`); `RemoveTrailingSlash` para aceitar os dois formatos
- [X] C4 Desvincular item externo não limpava `external_item_id`/`external_item_url`
- [X] C5 `external_integration_id` não era lido do banco: `/external-details` nunca funcionava e editar a tarefa apagava o vínculo
- [X] C6 Suíte verde: adapters com URL base injetável e GitHub fake nos testes; `testutil.Truncate` com os nomes reais das tabelas

### Autenticação (pré-requisito da Fase 2)

- [X] A1 `Person` com senha (bcrypt) e papel `admin|member`; email único no sistema
- [X] A2 Signup cria organização + admin; login, logout, `/api/auth/me` e troca de senha
- [X] A3 Sessões por cookie (`wtt_session`, HttpOnly, SameSite=Lax), token guardado como hash, validade de 7 dias
- [X] A4 Convites por link (uso único, 7 dias, email opcional), listagem e revogação
- [X] A5 Isolamento por organização em toda rota com ID (404) e rotas de admin (403)
- [X] A6 Ponto da pessoa logada por padrão; só admin registra o ponto de outra pessoa
- [X] A7 API só aceita corpo JSON em POST/PUT/PATCH (CSRF) e limita tentativas de login, signup e convite

---

## 🎨 FASE 2: FRONTEND

---

### Sprint 8: Frontend — Core Views (Layout, Org, Person, Project, Team)

- [X] S8.1 Create base HTML layout template (header with org/project navigation, vendored Alpine.js include) and register the Echo template renderer
- [X] S8.2 Build Organization management UI: list orgs, create org form, org settings page
  - Como cada pessoa pertence a uma organização só, virou a visão geral e as configurações da própria organização; a criação de organização é o signup.
- [X] S8.3 Build Person management UI within org: list persons, create person form
- [X] S8.4 Build Project management UI: list projects within org, create project form (with sprint/daily/sync config fields), project settings page
- [X] S8.5 Build Team management UI: list teams within project, create team form, manage team members (add/remove)

---

### Sprint 9: Frontend — Tasks & Time Tracking

- [X] S9.1 Build Task list UI scoped to project (name, assignee, deadline, external item badge) with create/edit/delete
- [X] S9.2 Build Task detail UI: edit form, external item link/unlink via integration selector
- [X] S9.3 Build Time Tracking UI within project: clock in (select task) and clock out button, active-session indicator, live elapsed timer
- [X] S9.4 Build work-sessions list and total-time display filtered by task and/or person within a project
- [X] S9.5 Write integration tests for frontend-serving routes: templates render, static assets served, main views return 200

---

### Sprint 10: Frontend — Integrations & Documentation

- [X] S10.1 Build Integration config UI: list integrations per project, create with type selection and dynamic config form, credential mask, enable/disable toggle
- [X] S10.2 Wire Alpine.js client-side error handling and inline validation feedback from API errors
- [X] S10.3 Write README with setup instructions (env vars, database setup, how to run, org/project/team workflow)
- [X] S10.4 Document the REST API endpoints with example requests/responses
- [X] S10.5 Document the web UI: browser routes, navigation flow, and that Alpine.js is vendored with no build step

---

## 💼 FASE 3: PERFIL DA ORGANIZAÇÃO E VALORES

---

### Sprint 11: Organization Profile

- [X] S11.1 Add profile fields to the Organization schema (identity, contact, legal data, operating defaults) with validation: CNPJ check digits (numeric and alphanumeric), http/https links, IANA timezone, currency
- [X] S11.2 Make `PATCH /api/orgs/:orgId` a partial update of the profile; `GET` returns the full profile to any member
- [X] S11.3 Use the organization's default sprint length for new projects and expose the organization currency in the logged-in identity
- [X] S11.4 Build the organization settings form (identity, contact, legal data, defaults) and the "Sobre" tab that every member can read; show the summary in the organization header
- [X] S11.5 Tests for validators, service, handler, router access and pages; seed the demo organization profile; update README, design and API reference

---

### Sprint 12: Clients & Hourly Rates

- [ ] S12.1 Add `Client` (per organization) and link projects to a client, with the hourly rate the client pays for the project
- [ ] S12.2 Add `Allocation`: the hourly rate the organization pays a person in a project, unique per person and project
- [ ] S12.3 Expose clients, project billing and allocations through the API: admins see and change everything, members only read their own rate
- [ ] S12.4 Build the Clients tab (organization), the Rates tab and billing card (project) and "Meus valores" on the profile page
- [ ] S12.5 Tests for the new domains and for rate visibility between roles and organizations; update seed and docs

---

### Sprint 13: Rates on Work Sessions

- [ ] S13.1 Snapshot the pay and bill rates on each work session at clock-in, so later rate changes do not rewrite past hours
- [ ] S13.2 Refuse clock-in for a person without an allocation in the project
- [ ] S13.3 Return per-session amounts, hiding other people's pay and every bill value from non-admins
- [ ] S13.4 Show rate, earnings, cost, revenue and margin on the time tracking screen according to the role
- [ ] S13.5 Tests for the snapshot, the clock-in rule and amount visibility; update seed and docs

---

### Refatoração: Domain-First Architecture (pré-Sprint 9)

- [X] R1 Reorganizar código de layered-flat para domain-first: `internal/domain/<dominio>/` com `model.go`, `store.go`, `service.go`, `handler.go` por domínio
- [X] R2 Fundir `team_membership` em `team` (sub-domínio)
- [X] R3 Renomear domínio `time_entry` → `work_session` (structs, rotas HTTP `/api/time-entries/*` → `/api/work-sessions/*`)
- [X] R4 Mover adaptadores externos (GitHub/GitLab/crypto) de `internal/integration/` para `internal/adapter/` para evitar colisão semântica com o domínio `integration`
- [X] R5 Tipos sem prefixo em cada package (`Handler`, `Service`, `Store` em vez de `OrganizationHandler`, etc.)
- [X] R6 Extrair helper de DB de teste para `testutil/` + tests em package externo `<dom>_test` para evitar ciclos de import
- [X] R7 Adicionar JSON tags em todos os models para API retornar snake_case
- [X] R8 Atualizar `cmd/main.go`, `routes/routes.go`, `database/db.go` com novos imports e construtores
- [X] R9 Atualizar `_docs/design.md` com nova nomenclatura
