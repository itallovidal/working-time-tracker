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

- [X] S11.1 Add profile fields to the Organization schema (identity, contact, legal data, and how it works: work mode, timezone, currency) with validation: CNPJ check digits (numeric and alphanumeric), http/https links, IANA timezone, currency
- [X] S11.2 Make `PATCH /api/orgs/:orgId` a partial update of the profile; `GET` returns the full profile to any member
- [X] S11.3 Expose the organization currency in the logged-in identity
- [X] S11.4 Build the organization settings form (identity, contact, legal data, defaults) and the "Sobre" tab that every member can read; show the summary in the organization header
- [X] S11.5 Tests for validators, service, handler, router access and pages; seed the demo organization profile; update README, design and API reference
- [X] S11.6 Keep weekly hours and sprint length on the project instead of the organization, since projects can work differently: add `weekly_hours` to the project (create, edit and card badge)
- [X] S11.7 Show "Não informado" for empty profile fields on the "Sobre" tab and the customers table instead of hiding them

---

### Sprint 12: Clients & Hourly Rates

- [X] S12.1 Add `Customer` (per organization; `Client` is reserved by Ent) and link projects to a customer, with the hourly rate the customer pays for the project
- [X] S12.2 Add `Allocation`: the hourly rate the organization pays a person in a project, unique per person and project
- [X] S12.3 Expose customers, project billing and allocations through the API: admins see and change everything, members only read their own rate
- [X] S12.4 Build the Clients tab (organization), the Rates tab and billing card (project) and "Meus valores" on the profile page
- [X] S12.5 Tests for the new domains and for rate visibility between roles and organizations; update seed and docs

---

### Sprint 13: Rates on Work Sessions

- [X] S13.1 Snapshot the pay and bill rates on each work session at clock-in, so later rate changes do not rewrite past hours
- [X] S13.2 Refuse clock-in for a person without an allocation in the project
- [X] S13.3 Return per-session amounts, hiding other people's pay and every bill value from non-admins
- [X] S13.4 Show rate, earnings, cost, revenue and margin on the time tracking screen according to the role
- [X] S13.5 Tests for the snapshot, the clock-in rule and amount visibility; update seed and docs

---

### Sprint 14: Organization Admin Screens

- [X] S14.1 Drop the "Geral" tab: the organization form becomes an edit screen opened by the "Editar" button on the "Sobre" tab, and saving returns to it with a toast; rename the "Pessoas" tab to "Colaboradores"; the "Sobre" tab and the edit screen use the full page width like the other tabs
- [X] S14.2 Add UI icons: Font Awesome Free from cdnjs with SRI and an `icon` template partial, used on the organization and project tabs and on the buttons of the organization screens
- [X] S14.3 Add the application-wide modal: one host in the base layout, an Alpine store to open and close it, content handed over with `x-teleport`
- [X] S14.4 Move the customer form into the modal: the Clients tab becomes a full-width table with a "Novo cliente" button
- [X] S14.5 Page tests for the new navigation, the modal host and the icon stylesheet; update README and design
- [X] S14.6 Rework the Collaborators tab: an "Integrantes" table and a "Convites pendentes" table with an empty state, and the invite form in the modal behind an "Adicionar colaborador" button
- [X] S14.7 Make the admin Projects tab a management table (customer, people in the teams, tasks) whose rows open the project; the project JSON gains `member_count` and `task_count`; the home page keeps the cards
- [X] S14.8 Create projects in the modal, with the customer and the hourly bill rate in the same form (the rate field only shows once a customer is chosen; the billing is saved right after the project)

---

### Sprint 15: Versioned Migrations

- [X] S15.1 Generate migrations from `ent/schema` with `cmd/migrate new`: replay the existing files on a throwaway database, diff against the Ent schema and warn about changes that touch existing data
- [X] S15.2 Baseline migration with the whole current schema; migrations embedded in the binary and applied at startup by goose, one transaction per file
- [X] S15.3 Remove the Ent auto migration (with column and index drops) from the server, the seed and the tests
- [X] S15.4 Refuse a database that has tables and no migration history, or a migration the build does not know
- [X] S15.5 Tests for schema drift between `ent/schema` and the migrations, the refusals and the checksum file; the test helper rebuilds the schema of the `_test` database for each package
- [X] S15.6 Document the migration workflow in the README and the decision in `design.md`

---

### Sprint 16: Task List Filters and Pages

- [X] S16.1 Filter and paginate the task list on the server: `q`, `assignee_id`, `deadline_to`, `page` and `per_page` on `GET /api/projects/:projectId/tasks`; without `page` the route still returns the whole array, which the time tracking screen needs
- [X] S16.2 Tasks tab: search by name, assignee and deadline filters and 10 tasks per page, with the state kept in the URL and restored by the "Voltar" button of a task
- [X] S16.3 Tests for the filters, the pages and the query parameters; more tasks in the seed; update README, design and routes
- [X] S16.4 After the manual test: the "Só as minhas tarefas" box sits below the filters and, when checked, turns off the name search and the assignee filter, leaving only the deadline one; the new task form opens in the modal

---

### Sprint 17: Project Collaborators

- [X] S17.1 Collaborators API: `GET /api/projects/:projectId/collaborators` lists who has an hourly rate in the project or is in one of its teams, with the teams and the rate (colleagues' rates only for admins); `DELETE .../collaborators/:personId` removes the rate and every team membership of the project in one transaction
- [X] S17.2 Rename the "Times" tab to "Colaboradores" (same route): summary tiles, a people table with search, the hourly rate edited inline and the margin, and removal from the project
- [X] S17.3 "Adicionar pessoa" in the modal, with a search over the people of the organization, the hourly rate and an optional team; "Novo time" in the modal
- [X] S17.4 Icon-only buttons for renaming and deleting a team and for removing a member, with `aria-label` and `title`
- [X] S17.5 Remove the "Valores" tab (template, page route and component); the customer's bill rate stays in the project settings
- [X] S17.6 `member_count` of a project counts its collaborators, not only the people in teams
- [X] S17.7 Tests for the union, the visibility by role and the removal; a person with a rate and no team in the seed; update README, design and routes

---

### Sprint 18: Time Tracking Tab Review

- [X] S18.1 Move the "no hourly rate" warning out of the clock card to the top of the page, and keep the clock card and the "Seu tempo neste projeto" card at the same height
- [X] S18.2 Filter the sessions by date (the day the session started), next to the person and task filters, with a button to clear them
- [X] S18.3 Move the totals (time, cost, revenue and margin, or the member's own amount) out of the table footer into their own card; the table only lists the sessions
- [X] S18.4 Page test for the new layout; update README

---

### Sprint 19: Team Card and Edit Modal

- [X] S19.1 The team card only shows the team: the name and each member's avatar, name and e-mail, with a single pencil button for admins; renaming, deleting and adding or removing members leave the card
- [X] S19.2 "Editar time" in the modal: the name, the members as a searchable checklist over the people of the organization (the team first, then the project, then the rest, with a "fora do projeto" badge) and "Excluir time" with a confirmation
- [X] S19.3 Nothing is sent before "Salvar", so "Cancelar" discards the draft; saving applies the difference with the existing team routes and can be repeated after a failure
- [X] S19.4 Page test for the card and the modal; update README and design

---

### Sprint 23: Integrations Modal, Trello and a Shared Structure with Metadata

- [X] S23.1 One body for every integration type: `{type, display_name, enabled, token, metadata}`, with the fields of each platform in `metadata`; the answer carries `has_token` and the `metadata`, never the token
- [X] S23.2 Adapter contract: a `Descriptor` per type (label, metadata fields, how the linked item is called), `CheckMetadata` that checks and normalizes the fields without calling the platform, and `Validate` and `FetchItemDetails` over a `Connection{Token, Metadata}`
- [X] S23.3 Column `config` renamed to `credentials` (the encrypted token) and a new `metadata` column, in a hand-edited migration (`RENAME COLUMN`, so no credential is lost)
- [X] S23.4 `PATCH /api/integrations/:id` keeps the stored token when none is sent and only calls the platform again when the token or the metadata changed; `enabled` defaults to true on creation
- [X] S23.5 Trello: token plus `api_key` and `board_id` in the metadata (the board as its address, short link or id), credentials in the `Authorization` header, the list of the card as its state, and a card from another board refused
- [X] S23.6 What reaches a platform URL is checked first: the repository, the project, the board, the issue number and the card id
- [X] S23.7 "Nova integração" and "Editar integração" in one modal: the platforms as a radio group, the metadata fields drawn from the descriptor, the token optional on edit, and disabling and "Excluir integração" with a confirmation inside it
- [X] S23.8 The integration card only shows the integration, with a single pencil for admins; a card of a row from before the metadata says what is missing
- [X] S23.9 The integration types reach the project pages in `window.BOOT.integration_types`; the hardcoded lists in `app.js` and `project.js` are gone, and the task page takes the label of the link field from them
- [X] S23.10 The task list loads the integration of each linked item, so the badge names the platform ("GitHub #42", "Trello H0TZyzbK") instead of "Item #42"
- [X] S23.11 Tests with fake GitHub, GitLab and Trello servers (`testutil/platforms.go`), for the adapters, the service, the handler and the pages; update README, design, routes and the Insomnia collection

---

### Sprint 24: Internationalization — Interface

- [X] S24.1 `internal/i18n`: YAML catalogs (`pt-BR`, `en`) read with go-i18n, language chosen by cookie, then `Accept-Language`, then Portuguese; `{{.T "key"}}` in templates, `TitleKey` in the Go pages
- [X] S24.2 Language toggle (PT | EN) in the top bar and, without a session, in the corner of the screen; `GET /lang/:code` sets the cookie and redirects, `GET /i18n/:lang.js` serves the texts to the browser
- [X] S24.3 `WTT.t` and the Alpine `$t` magic over the same YAML, with plural forms from `Intl.PluralRules`
- [X] S24.4 Migrated: layout, top bar, modal, toasts, active session, sign-in, sign-up, invitation, profile and the 404 page
- [X] S24.5 Tests: same keys and placeholders in every language, every used key exists, language by cookie / header / toggle, script cache headers; check in the browser in both languages
- [X] S24.6 Locale-aware formats: dates, times and currency follow the language (the currency itself stays the organization's), money fields show and read the language's decimal separator, option labels (weekdays, sizes, work modes, currencies, integration fields) come from the catalogs
- [X] S24.7 Organization screens (`org_*`, `org.js`): about, edit, collaborators and invitations, customers, projects; a test renders every migrated page in English and fails on any accented word left
- [X] S24.8 Project: tasks, task detail and time tracking (project header and tabs, filters, pager with plural forms, deadline badges, toasts, totals); page titles come from the catalogs
- [X] S24.9 Project: integrations and settings (the project field labels are shared with the new-project form in `project.fields`)
- [ ] S24.10 Project: collaborators and teams
- [ ] S24.11 Update README, design and the status page

### Sprint 25: Internationalization — API Error Codes

- [ ] S25.1 `internal/apperr`: errors with a stable code and parameters; sentinels and loose messages of every domain become coded errors
- [ ] S25.2 One helper builds `{"error": {"code", "params"}}` for every handler and middleware; unknown errors become `internal.server_error` without leaking text
- [ ] S25.3 The front end translates by code (`errors.*` in the catalogs); `api()` keeps `e.message`
- [ ] S25.4 Document the codes (`_docs/error-codes.md`, README, routes, Insomnia) and test that every code has a text in every language

---

### Sprint 26: Project Overview

- [X] S26.1 `GET /api/projects/:projectId/overview`, for admins only: people, teams, hours, cost, revenue and margin, the age of the project, tasks, integrations and the hours of each person, in a read-only `overview` domain that reads the other services and has no table
- [X] S26.2 The totals add the amount of each session, already rounded, so they match the time tracking tab; an open session counts up to `generated_at`, the 7 and 30 day windows only count the part of a session inside them, and whoever left the project stays in the hours per person, flagged
- [X] S26.3 The start of the project is the day it was registered (`created_at`), shown with the first and the last clock-in; the age comes from the server as elapsed days and weeks and completed calendar months
- [X] S26.4 "Visão geral" is the first tab of the project and the only one for admins alone: members do not see the link and get "Página não encontrada"; `/projects/:projectId` opens on it for an admin and on the tasks for a member
- [X] S26.5 The screen: summary tiles, "Tempo de projeto", "Atividade", "Integrações", "Tarefas e cliente" and "Horas por pessoa" with five people per page; it is a snapshot with an "Atualizar" button, and it reloads when a session opens or closes
- [X] S26.6 Seed: each project registered from a month to almost a year ago, older sessions so the 7 day, 30 day and total hours differ, and three integrations without a token
- [X] S26.7 Tests for the totals with exact values, the age, the route and the page by role; update README, design, routes and the Insomnia collection

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
