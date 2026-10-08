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

### Sprint 20: Hourly Rate Required to Join a Project

- [X] S20.1 A person joins a project with an hourly rate: `POST /api/teams/:teamId/members` refuses a person without an allocation in the team's project
- [X] S20.2 Remove `DELETE /api/projects/:projectId/allocations/:personId`: deleting only the rate left the person in the teams without one; `DELETE .../collaborators/:personId` is the way out
- [X] S20.3 "Adicionar pessoa" form: the hourly rate is a required field and sits above the team, one field per line
- [X] S20.4 The "Editar time" modal only lists people who are already in the project, without the "fora do projeto" badge; the "Sem valor por hora" tile is gone, and the warning stays for rows from before the rule
- [X] S20.5 Seed: rates before teams, and nobody in a team without a rate
- [X] S20.6 Tests for the refusal and the form; fixtures give a rate before a team; update README, design and routes

---

### Sprint 21: Collaborators Tab Views and Collaborator Modal

- [X] S21.1 A switch below the summary tiles chooses between two views of the same people: "Pessoas" (the table) and "Times" (the cards); the choice is kept in the URL (`?view=teams`) and the switch works with the arrow keys
- [X] S21.2 No list shows more than five people at a time: the people table has pages, and so does each team card
- [X] S21.3 "Editar colaborador" in the modal: the hourly rate, the teams as checkboxes and "Tirar do projeto" with a confirmation; nothing is sent before "Salvar", and the rate goes before the teams
- [X] S21.4 The table row no longer has the rate field or the remove button; for admins the row and its pencil open the modal, and so does a person inside a team card
- [X] S21.5 Names are sorted in Portuguese order in the browser, so an accented initial lands on the right page
- [X] S21.6 Page test for the views, the pages and the modal; update README and design
- [X] S21.7 Two more summary tiles for admins: the sum of the hourly margins at today's rates and what the recorded hours are worth (the closed sessions, time multiplied by the rate stored on each); the explanation of each sits behind an info button

---

### Sprint 22: Project Settings as a Read-Only Tab, Weekly Hours on the Person

- [X] S22.1 The project settings tab only shows the project, like the organization's "Sobre" tab: a card with the description, the sprint, the daily and the weekly, and a card with the customer and, for admins, the bill rate
- [X] S22.2 "Editar projeto" in the modal, opened by a single "Editar" button for admins: every field, in the order of "Novo projeto", and "Excluir projeto" with a confirmation; nothing is sent before "Salvar", and the billing is only sent when the customer or the rate changed
- [X] S22.3 The sprint length is a list in "Novo projeto" and "Editar projeto": 7 days, 14 days (the default) and one month (30 days); a project with another length keeps it as one more option
- [X] S22.4 Weekly hours leave the project and go to the person: `persons.weekly_hours`, `PATCH /api/persons/:personId/weekly-hours` (admin only), and a migration that adds the column and drops `projects.weekly_hours`
- [X] S22.5 Organization "Colaboradores" tab: a weekly hours column and a pencil that opens a modal with the hours; the role still changes with the button on the row. The profile shows the person's own hours
- [X] S22.6 Seed: weekly hours on the people instead of the projects
- [X] S22.7 Tests for the route, the permissions and the pages; update README, design, routes and the Insomnia collection

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
- [X] S24.10 Project: collaborators and teams (counters with plural forms, rate and margin table, the add-person, new-team and edit-team modals)
- [X] S24.11 Update README, design and the status page
- [X] S24.12 After the rebase onto sprints 23 and 26: the integration tab and the platform descriptors (texts in `integration_types.*`, localized on the server), and the project Overview tab

### Sprint 25: Internationalization — API Error Codes

- [X] S25.1 `internal/apperr`: errors with a stable code and parameters; the sentinels and the loose messages of every domain became coded errors (104 codes), the adapters included
- [X] S25.2 One helper, `apperr.Respond`, builds `{"error": {"code", "params"}}` for every handler and middleware; unknown errors become `internal.server_error` without leaking text, and the external-details `error` field has the same shape
- [X] S25.3 The front end translates by code (`errors.*` and `fields.*` in the catalogs); `api()` keeps `e.message`, and `ApiError` also carries `code` and `params`
- [X] S25.4 Document the codes (`_docs/error-codes.md` generated by a test, README, routes) and test that every code has a text in every language, that texts only use declared parameters and that no text is left without a code

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

### Sprint 27: Tasks Without an Assignee and the Task Board

- [X] S27.1 `tasks.assignee_id` is nullable (migration `task_assignee_optional`, with the foreign key still `NO ACTION`); `task.assignee_required` is removed
- [X] S27.2 Create a task without an assignee; `PATCH` with an empty `assignee_id` clears it; a chosen assignee is still checked against the project teams
- [X] S27.3 `assignee_id=none` filters the available tasks
- [X] S27.4 Clocking in on an unassigned task makes the person its assignee, team or not, with a conditional update; a refused clock-in takes nothing
- [X] S27.5 The "Tarefas" tab is the "Quadro de tarefas": "Sem responsável" in the new task modal (not required), in the assignee filter and as a badge in the list; new tasks no longer wait for team members; the list reloads when a clock opens
- [X] S27.6 Task detail can clear the assignee; seed with two available tasks
- [X] S27.7 Tests for the domain, the claim, the API and the page; update README, design, routes and the error codes

---

### Sprint 28: Gestão Area

- [X] S28.1 The admin-only tabs move to `/projects/:projectId/management/{overview,teams,integrations,settings}` behind the admin page guard (404 for members); the API permissions do not change
- [X] S28.2 Two tab bars: Quadro de tarefas and Ponto for everyone, plus a "Gestão" button for admins; inside the Gestão, Visão geral, Colaboradores, Integrações, Configurações and a "Voltar ao projeto" link
- [X] S28.3 `/projects/:projectId` opens on the Ponto for everyone; the old tab paths redirect to the new ones with the query string; page titles say "Gestão · ..."
- [X] S28.4 Internal links updated (rate warnings, settings link, new project redirect, task detail)
- [X] S28.5 Tests for the redirects, the 404 for members and the two tab bars; update README and design

---

### Sprint 29: Personal Overview, Ponto and Sessions

- [X] S29.1 The session list and total are limited on the server: a member only gets their own sessions (no `person_id` means theirs, another person's is a 403); admins keep seeing everyone
- [X] S29.2 `sessionList()` in `project.js`: list, person/date/task filters from the sessions themselves, totals that follow the filters and ten sessions per page, shared by two screens
- [X] S29.3 "Visão geral" is the first tab of everyone and the page `/projects/:projectId` opens on: today and this week, and the person's own sessions, the same screen for admin and member
- [X] S29.4 The Ponto is the clock and "Suas tarefas" (assigned to the person, with deadline and a Start button); the sessions and totals left it
- [X] S29.5 The Gestão overview swaps "Horas por pessoa" for everyone's sessions, with person filter and cost, revenue and margin
- [X] S29.6 Tests for the member scope, the three screens and the redirects; update README, design and routes

---

### Sprint 30: Read-Only Collaborators Tab for Everyone

- [X] S30.1 `/projects/:projectId/collaborators`, for everyone, between Ponto and the Gestão button: people (with search) and teams, five per page, nothing to edit, not even for admins
- [X] S30.2 `Data.CanManage()` gates the admin tools of the collaborators template (only in the Gestão, only for admins); with `readonly` in the boot data the component skips the admin-only calls
- [X] S30.3 Tests for the page by role and in both languages; update README and design

---

### Sprint 31: Optional Daily and Weekly, Weekly Time

- [X] S31.1 `projects.weekly_sync_time` (nullable `HH:MM`) with a migration; `weekly_sync_time` in the project create/update API, cleared with the day, refused without it (`project.invalid_weekly_time`, `project.weekly_time_without_day`)
- [X] S31.2 "O projeto tem daily" and "O projeto tem weekly" in the New project and Edit project modals: the time (and the weekly day) only appear when the project has the meeting, and are required then
- [X] S31.3 The two modals group their fields in Projeto, Cliente e cobrança and Rotina do time; the daily and weekly markup is the `project_routine` partial
- [X] S31.4 The settings card and the project card badge show "Weekly Sexta às 14:00" and "Sem daily"/"Sem weekly"; the seed has a project with only a weekly and one with only a daily
- [X] S31.5 Tests for the service rules and the modals; update README, design, routes, error codes and the Insomnia collection
- [X] S31.6 "Nova tarefa" asks the assignee with three choices, "Atribuir a mim", "Sem responsável" and "Outra pessoa"; "Outra pessoa" lists everyone on the project, with or without a team (a rate or a team membership), and assigning to oneself works even outside the project (`CreateAs`/`UpdateAs`)

---

### Sprint 32: Task Page and Edit Modal

- [X] S32.1 The board row swaps the Start button for a Details button that opens `/tasks/:taskId`
- [X] S32.2 The task page only shows the task (description, assignee, deadline, time tracked, linked external item) and has the Start button and a pen in its card
- [X] S32.3 The pen opens the Edit task modal (name, the three assignee choices, deadline, description) with Delete task and its confirmation inside; nothing goes to the server before Save
- [X] S32.4 Tests for the board and the page, in both languages; update README and design

---

### Sprint 33: The Ponto Tab Folds into the Início

- [X] S33.1 The Ponto tab is removed: the clock, the rate warning and "Suas tarefas" are the top of the personal overview, which is renamed Início (house icon)
- [X] S33.2 `/projects/:projectId/time-tracking` redirects to `/overview`; the links that used it (profile, clock pill, Gestão back link) point to the Início
- [X] S33.3 `timeTracking` is merged into `projectMyOverview`; tests for the page, the missing tab and the redirect; update README and design
- [X] S33.4 The clock card shows the value of the open session next to the timer; "Suas tarefas" moves below the clock as small cards (name, Ver detalhes, Iniciar, no deadline) and "Seu tempo neste projeto" sits beside the clock

---

### Sprint 34: Priority and Labels on Tasks

- [X] S34.1 `tasks.priority` (enum, default `none`) and the `labels` / `task_labels` tables, with a migration; labels belong to a project, with a unique name (case-insensitive in the service)
- [X] S34.2 `GET/POST /api/projects/:projectId/labels` and `PATCH/DELETE .../labels/:labelId` (create, rename and delete are admin only); `priority` and `label_ids` in the task create/update bodies, `priority` and `labels` in the task JSON
- [X] S34.3 The task list filters by `priority` and `label_id`, each with several comma-separated values (any of them), alone or together
- [X] S34.4 The board: rows of toggle chips for priority and label, a priority column, labels under the task name; filters kept in the URL
- [X] S34.5 New task and Edit task modals: priority select and label checkboxes, and, for admins, a field that creates a label on the spot; the task page shows both
- [X] S34.6 Seed with priorities and labels; tests for the service, the API, the page markup and the migration; update README, design, routes, error codes and the Insomnia collection

---

### Sprint 35: Markdown Descriptions and Task Modals in Steps

- [X] S35.1 marked and DOMPurify vendored with a hash test each; loaded only on the board and the task page
- [X] S35.2 `WTT.markdown()`: a fixed list of tags, no images, links only to http, https and mailto and opened in another tab; the task page shows the description rendered
- [X] S35.3 New task and Edit task in two steps (name and description, then deadline, priority, labels and assignee), with the shared `task_fields` partial and the `taskWizard()` mixin
- [X] S35.4 A Write / Preview toggle for the description in both modals
- [X] S35.5 Description limit of 10,000 characters (`task.description_too_long`)
- [X] S35.6 Tests (hashes, page markup, the limit) and docs; checked in the browser with a hostile description (script, onerror, javascript: and data: links, image, iframe)

---

### Sprint 36: Gestão Overview Without Repeated Numbers

- [X] S36.1 The strip of six totals leaves the top of the Gestão overview; the "Tempo de projeto" card becomes **Projeto** with people, teams, age and dates
- [X] S36.2 Hours, revenue, cost and margin stay only in the sessions' Totais card, with the margin share of the revenue computed from the filtered totals
- [X] S36.3 Test for the page (no repeated cards), locale keys cleaned up, README and design updated; checked in the browser

---

### Sprint 37: Weekly Meeting with the Customer

- [X] S37.1 `projects.customer_meeting_day` and `customer_meeting_time` (optional), with a migration; `project.Routine` replaces the loose daily and weekly arguments of `Create` and `Update`
- [X] S37.2 Same rules as the weekly: weekday and `HH:MM` checked, no time without the day, an omitted field keeps the value and empty text clears it; new codes `project.invalid_customer_meeting_time` and `project.customer_meeting_time_without_day`
- [X] S37.3 Taking the customer off a project (`PUT .../billing` with no customer) clears the meeting
- [X] S37.4 "Possui reunião semanal com o cliente" in the "Cliente e cobrança" group of the New project and Edit project modals, shown only with a customer chosen; the meeting in the Configurações customer card and a badge on the project card
- [X] S37.5 Seed with meetings in four projects; tests for the service, the API and the page markup; README, design, routes, error codes and the Insomnia collection updated; checked in the browser

---

### Sprint 38: Organization Owner

- [X] S38.1 `persons.is_owner` (a partial unique index: one per organization) and `work_sessions.owner_hours`, with a migration that makes the oldest admin of each organization its owner
- [X] S38.2 Signup creates the owner; an invite never does; the owner cannot be demoted (`person.owner_is_admin`); `is_owner` in the identity, the person and the collaborator JSON
- [X] S38.3 The owner has no paid rate: `Set` stores 0, the clock-in needs no allocation and snapshots `pay 0`, the project's billed rate and `owner_hours`; creating a project enrolls the owner
- [X] S38.4 Screens: the owner's hours count as what they earned (Início clock and totals, own sessions), the Colaboradores tab shows "Dono" without a rate field, the people page and the top bar say "Dono", the profile explains it
- [X] S38.5 Seed with Ana as the owner; tests for the service, the API and the migration; README, design, routes and error codes updated; checked in the browser

---

### Sprint 39: Permissions by Project and Group

- [X] S39.1 `internal/domain/permission`: the catalog (9 project and 3 organization permissions), the four groups (`member`, `manager`, `finance`, `admin`), `Set`, and `GET /api/permissions`
- [X] S39.2 `allocations.permissions` and `allocations.preset`, `persons.permissions`, with a migration; `PATCH /api/persons/:personId/permissions` (owner only)
- [X] S39.3 `RequireOrg` leaves the person's project permissions in the context; `RequireProjectPermission`, `RequireOrgPermission`, `RequireOwner` and the page versions replace `RequireAdmin` route by route
- [X] S39.4 `PUT .../allocations/:personId` takes `preset`; each change asks for its permission, and nobody gives a group with a permission they lack
- [X] S39.5 Rates, billed rate and sessions follow the permissions; roles, organization permissions, admin invites and deleting the organization are the owner's
- [X] S39.6 Pages and buttons follow the permissions (`Data.Can`, `WTT.can`); the Gestão opens on the first tab the person can see
- [X] S39.7 Seed with managers and a finance person; tests for the catalog, each group, the anti-escalation rule and the organization scope; `_docs/permissions.md`, README, design, routes, error codes and the Insomnia collection
- [X] S39.8 The modals: Adicionar pessoa in two steps (who, then the group), Editar colaborador with the group, and the organization permissions in the people modal (owner only); checked in the browser

---

### Sprint 40: Task Status

- [X] S40.1 `tasks.status` (enum `backlog`, `in_progress`, `awaiting_closure`, `closed`; default `backlog`, so existing tasks land in the backlog), with a migration
- [X] S40.2 Every new task is created in `backlog` (the create body does not read `status`); `PATCH /api/tasks/:taskId` takes `status` (omitted keeps it; another value is `task.invalid_status`); `status` in the task JSON
- [X] S40.3 The task list filters by `status`, with several comma-separated values (any of them), alone or with the other filters
- [X] S40.4 The board: a row of status chips (kept in the URL) and a Status column; the task page shows the status; the Edit task modal has a status select, and New task does not
- [X] S40.5 Clocking in on a task sets it to `in_progress` from any status (after the session exists, so a refused clock-in moves nothing); clocking out leaves it; the task page reloads the task when the clock changes
- [X] S40.6 Seed with tasks in the four statuses; tests for the service, the clock-in and the API, and the page markup; update README, design, routes, error codes and the Insomnia collection

---

### Sprint 41: Task Page in Cards

- [X] S41.1 The task page splits what the task says from what it has: one card for the name and the description, and a **Detalhes** card with one row each for status, priority, assignee (avatar, name and email, or the "Sem responsável" badge with its hint), deadline, labels and the creation date
- [X] S41.2 Two columns of cards on a wide screen (task and time tracked; details and linked item), one column on a narrow one, in the order task, details, time, item; light and dark themes and the phone width checked in the browser
- [X] S41.3 The task page is its own screen, not a tab of the project: no tab bar and no project name as the title; a header with Voltar ao quadro (back to the board, with its filters), the way (Projetos / the project), the task name as the title and the Start button and the pen; the first card is now only the description
- [X] S41.4 Test of the page markup (the cards, the rows, the details after the description and outside the card with the name); texts in both languages; update README and design; no API change

---

### Sprint 42: Task List and My Tasks

- [X] S42.1 The Quadro de tarefas tab becomes **Lista de tarefas** (menu, page title, back link and the texts that named it), in both languages
- [X] S42.2 `assignee_id=any` in the task list filter (only tasks with an assignee, of anyone), next to `none`; `ListFilter.Assigned`
- [X] S42.3 The task list page opens with a sentence saying what it is and has two lists, each with its own page and URL parameter (`free_page`, `taken_page`): **Sem responsável**, for the team to pick up, and **Com responsável**; picking a person in the filter hides the first; the "Sem responsável" option leaves the assignee select
- [X] S42.4 **Minhas tarefas** tab (`/projects/:projectId/my-tasks`), between Início and Lista de tarefas: the person's tasks in one list per status (Backlog, Em progresso, Aguardando fechamento, Fechada, the order the work moves), the closed one collapsed, each list foldable, a Start button on the open ones, and an empty state that points to the task list
- [X] S42.5 Tests for the filter (service and API), both pages' markup and the tab order; update README, design, routes and the Insomnia collection

---

### Sprint 43: Sessions with Several Tasks

- [X] S43.1 A session belongs to a project (`work_sessions.project_id`) and has tasks through `work_session_tasks` (`session_id`, `task_id`, `from_at`, `until_at`; null `until_at` is "until the session ends"); the migration moves the existing sessions (one interval with the old task, start to end) and drops `work_sessions.task_id`
- [X] S43.2 Clock-in creates the session and its first task in one transaction; `POST`/`PATCH`/`DELETE /api/projects/:projectId/work-sessions/:sessionId/tasks[/:linkId]` add a task, change its interval (or `stop` it now) and remove it, on an open or a closed session, for the person who clocked in and for admins; the session keeps at least one task and, while open, one in progress
- [X] S43.3 The time counts for the session and for each task (parallel tasks both count the overlap; the person, the project and the amounts count the session once): each task carries `seconds` and its amounts, the list filters by task, and `total` by task sums the task's time
- [X] S43.4 Deleting a task takes it out of the sessions and keeps the sessions and the hours (before, it deleted them)
- [X] S43.5 The session modal (one for the whole app, inside the layout modal): summary, the tasks with a time bar each, add, edit the interval, stop now, remove with confirmation; opened from the clock pill (first task and "+N"), the Início, the Gestão and the task page; the Início, the My tasks tab and the task page add a task to the open session instead of failing with `already_open`
- [X] S43.6 Seed with parallel tasks in about three in ten sessions; tests for the migration (old sessions kept), service, API (who can change a session), markup and texts in both languages; checked in the browser (light, English, phone width); update README, design, permissions, routes, error codes and the Insomnia collection

---

### Sprint 44: Task List Rows Open the Task

- [X] S44.1 In both tables of the **Lista de tarefas** (Sem responsável and Com responsável) the whole row opens the task page (`row-link`: pointer cursor, hover colour, click anywhere but on a link); the **Detalhes** button is removed and the task name is bold text that takes the accent colour on hover, no longer an underlined link
- [X] S44.2 A hint above each table (`tasks.row_hint`, in both languages, hidden when the list is empty) says that clicking a task opens its details; the running badge moves next to the name
- [X] S44.3 The **Item externo** column is removed from the list (the link stays on the task page); the tasks tab no longer gets `integration_types`
- [X] S44.4 Test of the board markup (the row click, no Details button, no Start button, no external item column, the hint, no integration types); checked in the browser (pointer cursor, click on the name, on a cell of each table and with the clock running, light and dark, English, phone width); update README and design; no API change

---

### Sprint 45: Colors for Priority and Status

- [X] S45.1 A palette of tokens for the light and the dark theme (orange, green, blue, turquoise, yellow, purple and grey, next to the red that already existed), each with a soft background and a contrast of at least 4.5:1 in both themes, and `.tone` with `.prio-*` and `.status-*` variants; the blue (low) and the purple (closed) are far enough apart, and the blue far from the turquoise, to not be mistaken
- [X] S45.2 Priority: urgent red, high orange, medium green, low blue, none white with a dashed border; status: backlog grey, in progress turquoise, awaiting closure yellow, closed purple; no colour shared between the two groups
- [X] S45.3 Everywhere they appear: the filter chips (soft when off, filled when on), the badges of the task list, My tasks (rows and the count of each status) and the task page, and the Status and Prioridade selects of the New and Edit task modals; "sem prioridade" has a (dashed) badge instead of a dash
- [X] S45.4 Test that reads `app.css` and `app.js` (every priority and status has a colour, none repeats, each exists in both themes) and test of the markup of the four pages; checked in the browser (light and dark, filters on and off, the modal, My tasks); update README and design; no API change

---

### Sprint 46: Com Responsável Is for Other People

- [X] S46.1 `assignee_id=others` in the task list filter (only tasks that have an assignee who is not the person asking, taken from the session), next to `none` and `any`; `ListFilter.OthersOf`, applied in the query so the total and the pages are right; `Unassigned` and `Assigned` win over it, and it wins over one person
- [X] S46.2 The **Com responsável** list of the Lista de tarefas asks for `others` when no person is chosen, so the logged-in person's own tasks stay in Minhas tarefas; choosing a person (the person included) or ticking "Só as minhas tarefas" still shows that person's tasks
- [X] S46.3 Texts in both languages: the description "Tarefas que já estão com alguém que não é você" (and "Tarefas que estão com você" when the filter is the person), the empty state "Nenhuma tarefa com outras pessoas ainda", and the notice "Tarefa criada. Ela está em Minhas tarefas." for a new task of the person
- [X] S46.4 Tests for the service (the new filter and its precedence) and the API (different answers for two people, total and pages without their own tasks) and the page markup; checked in the browser (the default view, "Só as minhas tarefas", a chosen person and creating a task for oneself); update README, design, routes and the Insomnia collection

---

### Sprint 47: Filters by List

- [X] S47.1 The "Só as minhas tarefas" box is removed from the Lista de tarefas (state, URL parameter `mine`, texts), since the person's tasks have their own tab; this replaces the part of S46.2 and S46.3 about choosing yourself
- [X] S47.2 The assignee filter moves into the **Com responsável** section and filters only that list; the **Sem responsável** list is no longer filtered by it nor hidden when a person is chosen; the select does not list the logged-in person, and an `assignee` with the person's own id in the URL is ignored
- [X] S47.3 At the top stay the filters of both lists: search by name, deadline, priority, status and label; the empty message of each list considers only the filters that apply to it
- [X] S47.4 Test of the page markup (no box, nothing turned off, the select inside the lists and only for the list with an assignee); checked in the browser (the top bar, the people in the select, a chosen person with the free list untouched, Limpar filtros, old URLs with `mine=1` and the person's id, and the search over both lists); update README and design; no API change

---

### Sprint 48: No Labels in the List Rows

- [X] S48.1 The rows of the **Lista de tarefas** (both lists) no longer show the labels under the task name; each row is the name (with the running badge), status, priority and deadline, plus the assignee in the Com responsável list; the Etiquetas filter stays
- [X] S48.2 Minhas tarefas and the task page keep showing the labels; no API change (the list still carries `labels`)
- [X] S48.3 Test of the page markup (the list rows have no label badges, and the label filter is still there); checked in the browser; update README and design

---

### Sprint 49: Take a Task and Quick Update

- [X] S49.1 `POST /api/tasks/:taskId/claim`: sets the assignee to the logged-in person only if the task has none, without clocking in and without touching the status; a task that is already yours comes back as it is, and one that is someone else's is 409 `task.already_assigned` (new error code, texts in both languages and the generated error docs); `Store.TryClaim` decides a simultaneous claim in the database (the clock-in's `ClaimIfUnassigned` now uses it)
- [X] S49.2 **Pegar** in the rows of the Sem responsável list (a column of actions only there; the row click ignores buttons) and **Pegar tarefa** in the task header while the task has no assignee: the list reloads and the toast says the task is in Minhas tarefas, and when someone was faster the screen reloads and shows the error
- [X] S49.3 `PATCH /api/tasks/:taskId/attributes`: only `priority`, `status` and `label_ids`, each optional, written by `Store.UpdateAttrs` without rewriting the rest of the task (the task `PATCH` requires the name and replaces the description)
- [X] S49.4 **Atualização rápida** on the task page: a modal with the status, the priority and the labels (the same fields as the edit modal, which keeps them), with `Data.WithIDPrefix` so the two modals do not repeat an `id`
- [X] S49.5 Tests for the service (claim, the rules, the race, the store-level atomicity checked by mutation, the quick update leaving name, description, assignee and deadline alone), the API (claim 200, repeated 200, 409, 404 for another organization, 401, no session opened; the attributes, partial and invalid bodies) and the markup of the list and the task page (no repeated id); checked in the browser with two people (claim from the list and the page, a task taken by someone else first, the quick update and its cancel, light and dark); update README, design, routes, error codes and the Insomnia collection

---

### Sprint 50: Início as an Overview

- [X] S50.1 `GET /api/orgs/:orgId/overview` (admins only; 403 for a member, 404 for another organization): the sessions of every project of the organization and its people, summed in three windows at once (`last_7_days`, `last_30_days`, `all_time`); each has the seconds of everybody and of whoever asked (`my_seconds`), the number of sessions and the project overview's `Money` (pay, bill, margin; `null` without a rate, margin `null` without revenue); `by_person` has every person of the organization, with zeros for those with no session, and `people.working_now` counts the open sessions
- [X] S50.2 `WorkSession.Within(since, now)` cuts a session at the window's border: the seconds inside and the two amounts of that part, rounded to the cent, so a session inside the window gives its own amounts and the organization's total equals the sum of the projects'; `work_session.Store.ListByOrganization` reads the sessions without the task links; `overview.Deps.Now` fixes the clock in the tests, and the project overview's margin rule became `Money.addMargin`, shared with the new one
- [X] S50.3 `GET /api/orgs/:orgId/projects?page=&per_page=`: without `page` the array it always was; with it `{items, total, page, per_page}`, newest first with the id as a tie-break, a page past the last comes back as the last, `per_page` 10 by default and at most 100, and `400 project.invalid_page` or `project.invalid_per_page` (new error codes, texts in both languages and the generated error docs)
- [X] S50.4 The home page (`/orgs/:orgId`, now `org_home`) for admins opens with the overview: a 7 days / 30 days / all switch (30 by default, no new request), six numbers (your hours with your share of everybody's, the team's hours, who is working now, revenue, cost and margin with its share of the revenue, red when negative) and the **Equipe** list with each person's hours and a bar, five per page, with a link to the collaborators (replaced in Sprint 53 by who is working now); "Atualizado às" and a refresh button as in the project overview
- [X] S50.5 For everybody, the projects as coloured cards, six per page with the page in the address (`?page=`): a stripe and an initials square in a hue drawn from the project id, the name, the customer (or "Projeto interno"), the description in two lines, and the number of people and tasks; the sprint, daily, weekly and customer meeting badges are gone (and their texts); a member sees no numbers and no "Novo projeto"
- [X] S50.6 Six decorative hues (`.hue-*`, from the palette of priority and status, with both themes) and a test that reads `app.css`, `org.js` and the template (each hue exists in the CSS, in both themes, and the initials and the tone over its soft background keep a contrast of 4.5:1; checked by mutation); the new-project form moved to the `project_form` partial and `projectCreation`, shared with the Projetos tab of the organization, which stays a table
- [X] S50.7 Tests for the service (the scenario with sessions outside and inside the windows, one crossing the border, the owner's, an open one without rates and another organization's, which counts for nothing; as two different viewers; no sessions; no revenue; malformed ids; checked by mutation on the organization filter) and the API (admin 200 and its shape, member 403, other organization 404, no session 401, malformed id 404; the pages, their order and total, the clamps and every invalid value) and the markup of the home page for an admin and for a member; checked in the browser against the seeded organization (the numbers against the sum of the eight project overviews, the three windows, the team and project pagers and `?page=` in the address, the modal, a member, light, dark, English and a phone width); update README, design, routes, error codes and the Insomnia collection

---

### Sprint 51: Colaboradores in the Top Bar

- [X] S51.1 The top bar is **Início, Colaboradores, Organização** (the first was "Projetos"): Colaboradores goes straight to `/orgs/:orgId/people`, shown only to who can open it (`people.manage`, always true for admins), so a member without it sees no link to a 404
- [X] S51.2 `Data.Section` has the value `people`, set for the people tab only: the Colaboradores item is the lit one there, and Organização stays lit on About, Customers, Projects and the edit screen; the Organização tab bar keeps its own Colaboradores tab
- [X] S51.3 Test of the bar in every case (admin on the home page, on the people tab, on About, on Customers and inside a project; a member without the permission; a member the owner allowed to manage people); checked in the browser; update README and design; no route, API or permission change

---

### Sprint 52: A Calmer Home and Two Shortcuts

- [X] S52.1 The project cards of the home page are neutral: no stripe, the initials square a soft tint of the project's hue, the counts muted chips with the icon in the hue, and the border takes the hue only on hover and focus (the owner found them too colourful)
- [X] S52.2 `TestProjectCardsStayCalm` (fails if the stripe, a solid-tone initials square or tinted chips come back) and the contrast test no longer asks for white over the tone; checked that both fail with the old CSS
- [X] S52.3 The **Novo projeto** button moves from the page header to the projects section, next to its title and count; test of where it sits for admins, and for the member who has no button
- [X] S52.4 **Adicionar colaborador** beside Ver colaboradores in the team block (`people.manage`): link to `/orgs/:orgId/people?add=1`; the people page opens the invite modal on load with that parameter and removes it from the address, so a reload does not reopen it
- [X] S52.5 Test that the home link and `org.js` agree on the parameter, and that the people page answers with the invite form; checked in the browser (light, dark, English, phone, the member's view, the modal opening and not reopening after a reload); update README and design; no route, API, permission or migration change

---

### Sprint 53: The Team Block Shows Who Is Working Now

- [X] S53.1 The Equipe block of the home page no longer ranks people by hours (no bars, no hours, no order by who worked the most): it lists who has the clock running and on which task, working people first and then the others by name, five per page with next and previous; each row has the name, "Trabalhando agora", "Na tarefa <task> · <project>" (the task opens its page; "e mais N" when the session has more) or "Sem ponto aberto"
- [X] S53.2 `GET /api/orgs/:orgId/overview` gains `working_on` on each `by_person` row: the tasks the person has in the open session right now (the intervals with no end), in the order they entered, each with the task and project `{id, name}`; `[]` and never `null` for anybody else and for an open session left with no task
- [X] S53.3 `work_session.Store.ListOpenByOrganization` (open sessions of the organization with their task intervals); the overview reads it only when someone is working, and fills `working_on` only for people the main count marked as working
- [X] S53.4 The hours ranking is kept, unused: the `team_hours` partial and the `teamHours()` mixin in `org.js`; the shared buttons (See collaborators, Add collaborator) are the `team_actions` partial
- [X] S53.5 Tests: the store (open only, the organization's own, task intervals in order), the overview (two projects, a task that left the session, a session with no task, a deleted one, another organization, the JSON), the API shape, the page (no hours or bars in the card) and a test that the stored partial still matches `org.js` and is not spread into `orgHome`; mutation checks on all of them; checked in the browser (light, dark, English, phone, paging, member); update README, design and `_test/routes.md`; no migration, no new route or permission

---

### Sprint 54: Richer Task Cards on the Project Início

- [X] S54.1 The task cards of "Suas tarefas" on the project Início have a left line (4px) in the priority colour (`.tone` and `.prio-*`, the same as the badges; no priority is the grey line), the priority name in the card's `title` and in a screen-reader-only text, the name, the status badge (`.status-*`), the deadline and the labels
- [X] S54.2 The deadline badge shows only when there is one, with the overdue (red) and due-in-48h (yellow) cues of the lists; on a closed task it is only the date, because "overdue" does not apply to what is finished
- [X] S54.3 The running badge moves to the top right of the card, the buttons stay at the bottom whatever the height of the row (`task-card-actions`), the cards are at least 270px wide, and the priority, status and deadline helpers are one `taskBadges` shared by the Início and Minhas tarefas
- [X] S54.4 Tests that read `app.css`, the template and `project.js` (the left line is the priority colour and no literal colour, the card shows priority, status, deadline and labels, and every name the card calls exists in the script), with three mutation checks; checked in the browser (every priority and status, overdue, due soon, far, none and closed deadlines, no and several labels, running, light, dark, English, phone); update README and design; no API, route or migration change

---

### Sprint 55: The Task Card Is the Link

- [X] S55.1 The left priority line is gone (the owner did not like it): the priority is a badge next to the status, in the same colours as the list
- [X] S55.2 The whole task card opens the task (`card-link`: pointer cursor, a darker background on hover, border a notch stronger, focus outline when the name is focused by keyboard); the name has no underline and the **Ver detalhes** button is removed (`time.view_details` too); clicks on the Iniciar button and on the name's link do not double-navigate
- [X] S55.3 New layout, top to bottom: name; status and priority; air; deadline on the left and Iniciar on the right (the "Em andamento" badge takes the button's place); a divider; the labels ("Sem etiquetas" when none, new key `time.no_labels`)
- [X] S55.4 `--surface-hover` token, darker than the card in both themes (the test compares the luminance); the four parts of every card sit on the same grid lines as the neighbours (subgrid with `span 4`, which a test ties to the number of children of the card) so deadline and Iniciar line up in a row even when one card's labels wrap to three lines
- [X] S55.5 Tests: the order of the parts, the styles (no left line, no underline, pointer, hover token in both themes, no literal colour), the subgrid span, the names the card calls, and the old Início page test brought up to date; four mutation checks plus one on the span; checked in the browser (hover, click on the body, on the name and on Iniciar, keyboard focus, running, light, dark, English, phone); update README and design; no API, route or migration change

---

### Sprint 56: The Timer Panel Holds the Tasks in Progress

- [X] S56.1 Starting a task from a card moves the card into the **timer panel** (`clock_card`): the clock, the value of the session, the cards of the tasks in the open session and, at the bottom, "Desde as HH:MM", Parar and Tarefas da sessão; stopped, the panel shows 00:00:00 and invites to start one of the tasks below
- [X] S56.2 "Suas tarefas" on the Início lists only the person's tasks that are not in the session (a note says when all of them are); stopping puts the cards back, and the cards of the session that are not the person's (or are from another project) are fetched whole so they have their badges
- [X] S56.3 The timer panel is one partial used by the Início (next to "Seu tempo neste projeto") and by Minhas tarefas, where it spans the full width above the lists; the task card is one partial (`task_card`) used by the Início list and inside the panel; the select that started a task is gone
- [X] S56.4 The state the two pages share is one mixin, `taskClock()` in `project.js` (tasks, rate, start, stop, session value, running and idle tasks, the card's colours and deadline), and the row Iniciar of Minhas tarefas is the same `startTask`; `time.since` reads "Desde as …" and there are two new texts; the front-end tests were not extended at the owner's request (the card test only follows the markup to the partial)

---

### Sprint 57: Each Person of the Team in a Card

- [X] S57.1 On the Início overview, each person of the Equipe block is a full-width card (avatar, name, "Trabalhando agora", the task and the project, or "Sem ponto aberto"); the block itself lost its frame, like the Projetos section
- [X] S57.2 The task is plain text in the card, and the link is an icon at the other end (`arrow-up-right-from-square`, `aria-label` and `title` "Abrir a tarefa <nome>"), only for who is working on a task; no front-end tests added, as the owner asked

---

### Sprint 58: One Task Table, Clickable Session Rows and a Session Modal in Cards

- [X] S58.1 The task table is one partial, `task_table`, used by the Lista de tarefas (Sem responsável with Pegar, Com responsável with the assignee column) and by each status of Minhas tarefas (task, status, priority, deadline and Iniciar; the assignee column is off because it is always the person); the whole row opens the task; the labels under the name and the Detalhes link of Minhas tarefas are gone; partials take several arguments through a new `dict` template function
- [X] S58.2 On the Início, the whole row of a session opens its modal (pointer cursor, hover as in the task rows); the eye column is gone, the start time is the button for the keyboard, and the task names in the row are plain text; the Gestão overview and the task page still have the eye button
- [X] S58.3 The session modal is wider (780px) and is split in groups with the name on the border, like the new project modal: Resumo, Tarefas da sessão (one card per task with the name, the running badge, the time, the interval line and range, an **Abrir tarefa** button with an icon and the edit pencil) and Adicionar tarefa; the task name in the modal is no longer a link
- [X] S58.4 No front-end tests were added, as the owner asked; the existing assertions follow the markup (the escaped quotes of the expressions that the partial writes, the row Iniciar of Minhas tarefas); checked in the browser (both tables, row clicks, the Iniciar button not navigating, keyboard Enter on the start time, the modal in light, dark and phone, Abrir tarefa, the pencil)

---

### Sprint 59: GitLab and Trello Coming Soon

- [X] S59.1 `Descriptor.ComingSoon` (in `window.BOOT.integration_types` as `coming_soon`), set for GitLab and Trello; the adapters, their tests and the integrations that already exist (the seed has some) are untouched
- [X] S59.2 `POST /api/projects/:projectId/integrations` refuses a coming-soon type with `integration.type_coming_soon` (400, names the provider); the check is in the handler, not in the service, so reactivating a type is one flag
- [X] S59.3 "Nova integração": the coming-soon platforms are disabled in the picker, dimmed, with a neutral "Em breve" badge, and the modal opens on the first available one; the intro and the empty-state texts name only GitHub
- [X] S59.4 Handler test for the refusal (both types, nothing left behind); no front-end tests added; update README and `error-codes.md`

---

### Sprint 60: The Session Bubble

- [X] S60.1 The open-session pill leaves the top bar: `active_session` is now `.session-dock`, a bubble fixed at the bottom right of every page of a logged-in person (`layouts/base.gohtml`), closed as one green strip "Sessão ativa • cronômetro" with the pulse and a chevron
- [X] S60.2 Clicking it opens, upwards, one card per task in progress with the task's own time in the session and an arrow (the card is the link to the task), and at the bottom Tarefas da sessão and Parar; Escape and a click outside close it, and it closes when the session ends (`sessionDock` in `app.js`; `clock.label()`, `labelTitle()` and `session.no_task` removed, `session.active` added)
- [X] S60.3 With a session open the `body` gets `has-dock`: more bottom padding on the pages and the toasts rise above the bubble, whatever its height (`--dock-h`, written by a `ResizeObserver`); the top bar no longer overflows a 390 px screen
- [X] S60.4 No front-end tests added, as the owner asked; the session pages test now looks for the bubble outside the top bar; checked in the browser (closed and open, light and dark, phone, a toast over it, the modal over it, Esc, click outside, the task link, Parar, navigating between pages); update README and `design.md`

---

### Sprint 61: Connect with GitHub

- [X] S61.1 `Descriptor.Auth` (`token` by default, `oauth` for GitHub) and `Configured` (filled by the page: the server has the OAuth app); optional adapter capabilities `AccountLookup` (who the token is) and `RepositoryLister`, implemented by GitHub only
- [X] S61.2 Config: `PUBLIC_URL`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` and, for a GitHub Enterprise or a fake, `GITHUB_URL` and `GITHUB_API_URL`; all optional, and without them the button says the connection is not set up
- [X] S61.3 `GitHubOAuth` client (authorization URL with scope `repo`, code exchange that reads the `error` GitHub sends with a 200) and `GitHubIntegration.Account` and `ListRepositories` (100 most recently pushed)
- [X] S61.4 `GET .../integrations/github/connect` and `GET /integrations/github/callback`: sealed cookie with the `state`, the person, the project and the integration to reconnect (10 minutes, one use); the callback re-checks the state, the person, the organization and `integrations.manage`, and sends every failure back to the tab with `?github_error=<code>`
- [X] S61.5 `Service.Connect` (the integration is born disabled and without a repository, named `GitHub · @login`, token sealed), `Reauthorize` (replaces the token only, validating it against the repository when there is one) and `Repositories`; `GET /api/integrations/:id/repositories`
- [X] S61.6 Modal: for an OAuth type creating is only the **Conectar com o GitHub** button; the return opens "Escolha o repositório" (repository offered from a `datalist`, "Ativa" already checked), editing has **Reconectar** instead of the token field, and the card shows "Conexão: Autorizada"; a reconnect of a complete integration only toasts "Acesso renovado."
- [X] S61.7 Tests: the OAuth client and the account and repositories calls against the fake GitHub, `Connect`, `Reauthorize` and `Repositories` in the service, and the whole flow through the real router (full path, every refusal, permissions lost on the way, reconnect, server without the app); no front-end tests added; the existing page assertion follows the token field
- [X] S61.8 README (variables, how to register the app, caveats), design, routes, the Insomnia collection and `.env.example`; checked in the browser against a fake GitHub (connect, pick the repository, reconnect, cancel)

---

### Sprint 62: Stop One Task of the Session

- [X] S62.1 `clock.stopTask(taskId)` in `app.js`: sends `PATCH .../work-sessions/:sessionId/tasks/:linkId` with `{"stop": true}` for the running interval of the task, refreshes the clock and fires `wtt:sessions-changed`; when the task is the only one running it calls the clock-out instead (the server still refuses it with `work_session.last_task`) and says "Ponto encerrado."; `stopTask` in the `taskClock()` mixin, in `taskDetail` and in `sessionDock`
- [X] S62.2 **Parar tarefa** (`stop` icon, secondary style) on the task page in the place of Iniciar and Adicionar à sessão, on the task card of the timer panel in the place of the "Em andamento" badge, on the row of Minhas tarefas, and as an icon button on each card of the session bubble (`.session-dock-item`: the link and the button side by side, since a button cannot go inside a link)
- [X] S62.3 Texts `session.stop_task`, `session.stop_task_label` and `session.stop_task_hint` in both languages; no API, route, permission or migration changed
- [X] S62.4 No front-end tests added, as the owner asked; checked in the browser (each of the four places with two tasks running, the session keeps going with the other one, the task offers Adicionar à sessão again and can be added back, the last task ends the session, light, dark, phone); update README and `design.md`

---

### Sprint 63: Sync GitHub Issues with the Tasks

- [X] S63.1 Limits: `MaxDescriptionLen` 10,000 → 65,536 and `maxLabelLen` 30 → 50 (the size of a GitHub issue body and label), in the service and in the `maxlength` of both fields, so importing and pushing back never truncates
- [X] S63.2 Schema and migration `issue_syncs` (`integration_id` CASCADE, `task_id` unique SET NULL, `issue_number`, `state`, the snapshot, `stuck_sig`, `last_error`) and, on `integrations`, `sync_issues`, `sync_cursor`, `last_synced_at`, `last_sync_error`; FK rules and uniqueness tested
- [X] S63.3 `testutil.GitHub`: a GitHub fake with state (issues with `updated_at`, `since`, pagination and `Link`, pull requests, labels, assignees, public and private emails, user search, PATCH that silently discards, 403 without push, 404/410/301, 429, request counters) plus `FakeGitHub()`; `testutil.Setup` makes the default HTTP transport refuse non-loopback hosts
- [X] S63.4 Adapter capability `IssueSyncer` with `context` (`Repo`, `ListIssues`, `GetIssue`, `UpdateIssue`, `ListLabels`, `CreateLabel`, `UserEmail`, `FindLoginByEmail`): no redirects, API version header, next links only on the same server, a stop at the rate-limit reserve, `ErrIssueGone`, `ErrRateLimited`, `ErrForbidden`
- [X] S63.5 Plugging in: `person.FindByEmailInOrg`; `task.Store.ApplyRemote`, `CreateImported`, `ListLinked`, `FindLinked`, `FindOrCreateLabels` and `SetChangeHook` (fired by `Update`, `UpdateAttrs`, `TryClaim`, `StartProgress`, and by renaming or deleting a label); `integration.Service.Connection`, `ListSyncing`, `RecordSync`, `Edit` with `sync_issues` (GitHub only, repository chosen, integration active; the repository is locked while the sync is on)
- [X] S63.6 `internal/domain/issuesync`: the pure merge in tables (CRLF, label case, several assignees, the linked login only, the push signature), the round (full and incremental, import, adoption of a task linked by hand, tombstone, gone issues, read-only mode, silent-discard detection, push cap, cursor only after a complete round) and the single-task path for the hook; tested against the fake with the database, with `-race`
- [X] S63.7 `server.Build` (returns the server and the `Syncer`; `New` stays), the hook wired to the task store, `POST /api/integrations/:integrationId/sync` (`integrations.manage`; `409` `integration.sync_running`, `400` `integration.sync_off`), new error codes in both languages and in `_docs/error-codes.md`, `TestRoutes_Table`
- [X] S63.8 Background routine in `cmd/main.go` (`GITHUB_SYNC_INTERVAL`, `5m` by default, `0` off; jitter, backoff, a full round every hour), a hook worker that debounces and works in batches, and a shutdown that waits for them (`signal.NotifyContext`, `echo.StartConfig`); the base URL of GitHub in use is logged
- [X] S63.9 Screen: **Sincronizar agora**, last sync, last warning and "N responsáveis sem correspondência" on the integration card; the sync box in the edit modal and in the repository step (checked after a connection, off on the integrations that already exist; the repository field locks while it is on); the `GitHub #N` badge on the task list, Minhas tarefas and the cards; the sync note in the edit and quick-update modals; the sync line in the Gestão overview; texts in both languages; the existing page assertions adjusted, no new front-end tests; fixed the edit modal of a task with no deadline (the Go zero time broke the date field)
- [X] S63.10 `cmd/fakegithub` (the fake as a dev server with a control panel under `/_fake`); README, design, routes, the Insomnia collection and `.env.example`; checked in the browser against it (connect, pick the repository, sync, edit here and see it on the fake, edit there and wait for the cycle, close, remove the assignee, read-only warning, phone), and once against the owner's real repository on a copy of the database (import, close and reopen the test issue from here, a second round with nothing to do)

---

### Sprint 64: The Integration Card on the Task

- [X] S64.1 `POST /api/tasks/:taskId/sync` and `Syncer.SyncTaskNow`: rereads the issue of one task and reconciles it like a round does (the integration lock with `TryLock`, `409` while a round runs; `integration.sync_off`, `task.no_external_item`, `integration.issue_gone`; a task linked by hand to an open issue is adopted); the answer is the round summary plus `problem`; tested against the fake (a change on GitHub, nothing to do, closing, adoption, deleted issue, other organization, no session, sync off) and in the route table
- [X] S64.2 `external_integration.name` in the task JSON (the name the integration has in the project)
- [X] S64.3 Task page: the "Item externo" card became one **Integração** card per integration (platform and its name, **Sincronizar** or, with no sync, **Atualizar**, the "Abrir no GitHub" tile with the brand icon and the issue number, the state as an Aberta or Fechada pill, the warning a sync left); **Desvincular** removed from the card; the card of a task with no link is "Integrações" with the link form; texts in both languages
- [X] S64.4 No front-end tests added; checked in the browser against `cmd/fakegithub` (the card, a change and a close made on the fake coming in with the button, the toast, no Desvincular, the unlinked card; light, dark, phone); README, design, routes and the Insomnia collection

---

### Sprint 65: Sincronizar Button on the Task Lists

- [X] S65.1 `POST /api/projects/:projectId/sync` and `Syncer.SyncProject`: a full round on every integration of the project with the sync on, one after the other, with the summaries added up (`partial` when one could not run); `409`, `integration.sync_off` and the first error as described in `_test/routes.md`; open to anyone on the project; tested against the fake with two repositories, a member pressing it, a round with nothing to do writing nothing, another organization, no session, and in the route table
- [X] S65.2 **Sincronizar** on the task list (next to Nova tarefa) and on Minhas tarefas (above the lists): the `sync_button` partial and the `projectSync()` mixin, shown only when the project has an enabled integration with the sync on; the list and the labels reload after the round, with the toast of the integration button; texts in both languages
- [X] S65.3 `GITHUB_SYNC_INTERVAL=1m` in the owner's local `.env` (not versioned)
- [X] S65.4 No front-end tests added; checked in the browser against `cmd/fakegithub` (the button on both lists, a new issue coming in with it and the toast, "Tudo igual" with nothing to do, no button in a project without the sync, phone); README, design, routes and the Insomnia collection

---

### Sprint 66: Post a New Task as an Issue

- [X] S66.1 Adapter: `IssueSyncer.CreateIssue` (`POST /repos/{o}/{r}/issues`, `NewIssue`), `ErrIssuesDisabled` (410); the fake GitHub got the POST route (labels and assignees dropped without push), `DisableIssues`; tested in the adapter
- [X] S66.2 `Syncer.Publish` and `POST /api/tasks/:taskId/publish`: refuses a read-only token, creates the missing labels, maps the assignee by public email, creates the issue, links the task and adopts it so the next round writes nothing; `task.already_linked`, `integration.sync_off`, `task.integration_other_project`, `issue_sync.publish_read_only`, `integration.issues_disabled`; the answer carries `problem`; error texts in both languages and in `_docs/error-codes.md`; tested against the fake (the issue's title, body, label and assignee, a round after it with zero writes, posting twice, an assignee with no GitHub user, a read-only token, issues off, another project, another organization, no session) and in the route table
- [X] S66.3 New task modal: the **Integrações** step (only with an active integration; the edit modal keeps two steps), one card per integration with the checkbox, the platform disclaimer (deadline and priority do not go, the assignee only by public email, labels do) and, disabled with the reason, GitLab, Trello and a GitHub with the sync off; the task is created and then posted, with a toast for the result or the failure; texts in both languages
- [X] S66.4 No front-end tests added; checked in the browser against `cmd/fakegithub` (a project with no integration keeps two steps, the disabled cards, posting a task with description and assignee, the failure toast, light, dark, phone); README, design, routes and the Insomnia collection

---

### Sprint 67: Generalize the Sync Core (for Trello)

- [X] S67.1 `issue_syncs.issue_number` (bigint) became `item_id` (text) and gained a nullable `deadline`, with a hand-written migration (`RENAME COLUMN`, `ALTER ... TYPE ... USING`, `RENAME` of the unique index) that keeps every existing link, tombstone and the unique index; `TestMigrate_IssueSyncItemIDKeepsTheLinks` replays the old schema with data and runs the migration over it
- [X] S67.2 Adapter contract: `Descriptor.Caps` (`SyncCaps`: deadline, assignee, server-side since, auto publish), `Field.Internal` and `Field.Picker`, `Issue.ID` (the key of the item as text; GitHub fills it with the number) with `Deadline` and `CreatedAt`, `IssuePatch`/`NewIssue` with a deadline, `GetIssue`/`UpdateIssue` take the id as text, `Repository.ID`, the optional `ItemNormalizer`, `StopsSync(err)` (the sync core delegates to it) and `Descriptor.SummaryKey()`; the GitHub adapter sets `Caps{Assignee, ServerSince}`
- [X] S67.3 Sync core: the rows, the maps of a round and the hand-made links are keyed by the item id (`Row.ItemID`, `run.src`, a `keyer` that turns the id stored on the task into the key); `Merge` takes options (`WithDeadline`, `WithoutAssignee`) and mirrors the deadline with the same remote-wins rule as the title (compared in UTC, to the second, and "no deadline" is the zero time or any date before 1971); a type without `ServerSince` always runs a full round; import and publish carry the deadline and the creation date when the type has them. No behavior change for GitHub
- [X] S67.4 `task.Store`: `RemotePatch.Deadline`, `Imported.Deadline` and `Imported.CreatedAt` (a task imported from an item is born with the date the item was); `integration.Service`: `ConnectWith`/`ReauthorizeWith` (the app brings part of the metadata, e.g. the Trello key), `keepInternal` (an edit cannot change an internal field) and the lock of the sync now follows the summary field of the type instead of `repo`
- [X] S67.5 Tests: the GitHub suite passes unchanged (only the call sites of the id got a `strconv.Itoa`); new cases for the deadline in `Merge` (remote wins, task pushes, milliseconds and zones, year-1 task, off without the option, assignee skipped), the signature, the task store, `StopsSync`, `SummaryKey`, `keepInternal`, `ConnectWith`; no front-end; design and this list

---

### Sprint 68: Trello Cards as an IssueSyncer (the adapter, still "Em breve")

- [X] S68.1 Trello adapter as an `IssueSyncer`: `Repo` (the board and whether the token writes: a member of the board as admin or normal, an observer does not), `ListIssues` (one call, `GET /boards/{b}/cards/open`), `GetIssue`, `UpdateIssue` (one `PUT /cards/{id}` with only what changed), `CreateIssue` (the first open list by position), `ListLabels`/`CreateLabel`; the card key is its short link, `NormalizeItemID` takes it from a pasted card URL; JSON bodies, no redirect followed, the key and the token only in the `Authorization` header
- [X] S68.2 What is mirrored: name, description, labels (a label with no name appears by its color; the ones the board lacks are created, with a color taken from the name), due date (to the second, null clears it) and the creation date of the card (from its id) into `tasks.created_at` on import. **Closed** is an archived card **or** a card whose due date is marked done: closing the task archives the card and, if it has a due date, completes it; reopening undoes both. No assignee (Trello does not show member emails) and no lists yet
- [X] S68.3 `AccountLookup` (`GET /members/me`) and `RepositoryLister` (`GET /members/me/boards`, with the workspace) for the connection of Sprint 69; `FetchItemDetails` now answers `open`/`closed` instead of the list name and two Portuguese words; the descriptor says `Sync` with `Caps{Deadline, AutoPublish}` (still `ComingSoon`)
- [X] S68.4 Errors: `integration.trello_no_list` (a board with no open list); the `issue_sync.*` warnings that said "GitHub" are worded for any platform (they are stored as a code, with no parameters, so the text cannot name the platform); `_docs/error-codes.md` regenerated
- [X] S68.5 `testutil.Trello`, a Trello with state in the mold of the fake GitHub (boards, lists, labels, cards, `members/me`, the authorization that returns the token in the URL fragment, the board that the token does not see or cannot write to, a token revoked, a PUT that drops fields, a card deleted or moved, rate limit, request log), served at the root and under `/1`; `cmd/faketrello` (port 8092, panel in `/_fake`, a seeded board)
- [X] S68.6 Tests: the adapter (every method, the label resolution, close and reopen, the errors, no redirect followed, the rate limit) and the sync core against the fake (import, changes in, changes out, Trello wins, gone cards, read-only token, silent discard, revoked token, every round is a full one, a hand-linked task, posting a task); no front-end; README and design

---

### Sprint 69: Connect with Trello and Pick the Board

- [X] S69.1 Config and wiring: `TRELLO_API_KEY` (letters and digits only, or the start is refused), `TRELLO_APP_NAME`, `TRELLO_URL`, `TRELLO_API_URL`; `SYNC_INTERVAL` is the name of the interval now (`GITHUB_SYNC_INTERVAL` still works when it is not set); `adapter.TrelloAuth` (the authorization URL: `key`, `name`, `scope=read,write`, `expiration=never`, `response_type=token`, `callback_method=fragment`, and the `return_url` carrying our `state`), `server.Options.TrelloAuth`, `OAuthConfigured["trello"]`
- [X] S69.2 The OAuth handler learned two flows (`oauthFlow`: type, cookie path, the parameters of the way back; the GitHub URLs, cookie and parameters are the same, and the sealed cookie now carries the platform, so one platform's cookie does not serve the other's return); `TrelloConnect` (the same checks and reconnection as GitHub) and `TrelloToken` (`POST /integrations/trello/token` with `{state, token}`, JSON only: it checks the cookie, the `state`, the person, the organization and the permission again, saves the integration through `ConnectWith`/`ReauthorizeWith` with the app key in the metadata, and always answers `200 {"redirect"}`); the return page `/integrations/trello/callback` (`trello_callback.gohtml` and `.js`: reads the token from the fragment, clears the address from the history, posts it, follows the redirect; `Referrer-Policy: no-referrer`, `Cache-Control: no-store`); errors `trello_oauth_not_configured`, `_denied`, `_state`
- [X] S69.3 Trello is offered (no longer `ComingSoon`): `Auth: oauth`, the app key is `Internal` (hidden, an edit cannot change it), the board is a `select` fed by `GET /api/integrations/:id/repositories` (the open boards, with the workspace) and keeps its readable name in `board_name` (`Field.NameKey`/`Hidden`: the screen fills it when a board is picked, the card shows it instead of the id, and changing the board without a new name drops the old one); the texts that depend on the platform (pick title and hint, sync box and explanation, "not configured") moved from `integrations.*` to `integration_types.<type>.*`, and the other sync texts no longer say GitHub or issue
- [X] S69.4 Tests: the whole Trello connection against the fake (the authorization URL, the sealed cookie, the return page, the token endpoint, the board list, picking the board and the locked board with the sync on, the refusals, another person, a cookie of the other platform, a form post, permissions lost on the way, reconnecting with a new token and with a token that cannot see the board, another project or type, not configured), config, the descriptor, the board name; `TestHandler_Create_ComingSoonType` is GitLab only; no front-end tests, checked in the browser against `cmd/faketrello` (connect, pick the board, sync, the tasks with their badge, English, phone width, dark); README, design, routes, the Insomnia collection and `.env.example`

---

### Sprint 70: A Task Made Here Becomes a Trello Card by Itself

- [X] S70.1 `task.Store.SetCreateHook` (fired by `Service.CreateAs`, never by `CreateImported`, the sync or the seed) and `Syncer.NotifyCreated`; the sync queue keeps the new tasks apart from the changed ones (`Pending()` still counts only the changed), and the worker drains both
- [X] S70.2 `autoPublish` (`issuesync/autopublish.go`): a task made here, not yet linked, in a project with an enabled integration that syncs and declares `Caps.AutoPublish` (Trello), is posted with the same `Publish` as the button, in the background (creating the task does not wait for Trello); a task already linked (the modal posted it to GitHub first) or with nothing to post to is left alone; what Trello refuses by itself (no open list, a token that only observes) is recorded as the warning of the integration and not retried; what is temporary (the platform down, the token refused, the rate limit) backs the integration off and parks the task, which the background routine and the Sincronizar button retry (ten times at most; the queue lives in memory, so a restart forgets it and the publish route posts by hand)
- [X] S70.3 New task modal, step Integrações: the Trello card with the sync on has no checkbox, an "Automático" badge and what happens (name, description, labels and deadline go, the card is made on the first list, priority, hours and the assignee stay, archiving or completing the due date closes the task, and a task links to one item only); a Trello with the sync off shows the reason; GitHub is as before
- [X] S70.4 The task edit no longer rewrites the deadline: the date field holds only the day, so saving sent the end of that day and a due date that came from Trello with a time lost it on every edit; the deadline is sent only when the person changed the day
- [X] S70.5 The texts that said "GitHub" or "issue" for every platform are neutral (the task page sync texts and notes, the sync errors); `_docs/error-codes.md` regenerated
- [X] S70.6 Tests: the create hook; a new task posted by itself with its fields, a second Flush and a round that write nothing, and the imported card not coming back; nothing to post (already linked, sync off, integration disabled); Trello refusing; Trello waiting and the button posting it after the wait, once; and end to end through the API with the real router (create, edit, close, a card made on Trello, an archived card, GitHub winning the single link, Trello down not blocking the task); no front-end tests, checked in the browser against `cmd/faketrello` (the step, the card appearing within seconds, editing a task with a due time); README, design, routes

---

### Sprint 71: One Polling Interval per Platform

- [X] S71.1 `GITHUB_SYNC_INTERVAL` and `TRELLO_SYNC_INTERVAL` set the interval of one platform and `SYNC_INTERVAL` is the default for the rest (five minutes when nothing is set; `0` turns the background routine off for everyone or for that platform only); `GITHUB_SYNC_INTERVAL` stops being the interval of everything, as it was since Sprint 69. `config.Config.SyncIntervals`, `issuesync.Config.Intervals`
- [X] S71.2 The routine wakes at the smallest interval that is on and looks at each integration when the interval of its type has passed (`lastTick`); the button and the hook are not affected; the startup log prints the interval of each platform
- [X] S71.3 Tests: the environment (each variable alone, the precedence, `0`, refusing a bad value) and the routine against the fake Trello (its own short interval with a long default; off with a short default, and the button still working); README, `.env.example`, design

---

### Sprint 72: Editing a Task in Three Places

- [X] S72.1 The edit modal of the task page was a two-step wizard that kept growing, so the wizard stays for **Nova tarefa** only and the page edits in three places: the pen at the right corner of the name card opens **Editar nome e descrição** (the name, the description with Escrever / Pré-visualizar), the **Editar** button on the Detalhes card (its accessible name is "Editar detalhes") opens a modal with the status, the priority, the assignee, the deadline and the labels, in the order of the card (the creation date is not editable), and a full-width **Excluir tarefa** button, apart from both and from the cards (between the Detalhes card and the integrations; last on a phone), opens a confirmation modal
- [X] S72.2 The page: the name is the `h1` of the first card (name, then the description under it, the pen at the right corner), the header keeps the way back, the breadcrumb and the clock actions, and the time tracked stays under the description; the **Atualização rápida** button and the pen of the header are gone, with their texts (`task_detail.quick*`, `sync_note_quick`); new texts `edit_details`, `details_saved`, `sync_note_details`; `edit_title` now says "Editar nome e descrição"
- [X] S72.3 `PATCH /api/tasks/:taskId/attributes` also takes `assignee_id` (empty takes the assignee off, the rule of the task `PATCH`: someone outside the teams is refused unless it is the person asking, or already the assignee) and `deadline`, each optional and written alone by `Store.UpdateAttrs`; `Service.UpdateAttrsAs` (and `UpdateAttrs` as the wrapper without them, like `Update` and `UpdateAs`) and `checkAssignee`, now shared with `UpdateAs`
- [X] S72.4 The partials: `task_fields` (the wizard) now uses `task_text` (name and description) and `task_assignee`, and `task_attrs` is split into `task_status`, `task_priority` and `task_labels`, so the new modals reuse the same markup; `taskWizard` shares Escrever / Pré-visualizar through `mdTools`; the ID prefix helper (`WithIDPrefix`) and the `hasStatus` switch are removed, since no page has two copies of the fields any more
- [X] S72.5 The modals send only what they own: the name and the description go through the task `PATCH` with those two fields, and the details through `attributes`; the assignee and the deadline are sent only when the person changed them (the day of the deadline, as since Sprint 70, and the assignee as it was when the modal opened)
- [X] S72.6 Tests: the service (assigning another member, the deadline, nothing sent keeping both, yourself outside the teams, the current assignee, someone outside the teams, a bad id, unassigning) and the API (unassign and move the deadline, outside the teams 400, a bad id 400, taking it back); the existing task page and tasks page assertions follow the new markup, no new front-end tests; checked in the browser (details with and without a change, "Outra pessoa" without a person, name required, the preview, cancel, delete, light and dark, phone width); README, design, routes, the Insomnia collection

---

### Sprint 73: One Task, Several Items

- [X] S73.1 A task can be linked to **one item per integration** (an issue of GitHub and a card of Trello at the same time), and an item serves one task: `issue_syncs` is the only home of the link, the unique index is `(task_id, integration_id)`, the row has `url` and a new state `pending` (a hand link not yet adopted); the `external_integration_id`, `external_item_id` and `external_item_url` columns of the task and the `Task.external_integration` edge are gone. The migration `issue_sync_links` moves the old links (the rows that still match, the hand links without a row, a discarded row reclaimed by a task, the oldest task wins for an item), normalizes a pasted Trello URL to the short link and drops the columns
- [X] S73.2 The task JSON has `links` (`integration_id`, `integration {id, type, name}`, `item_id`, `url`, `last_error`), never null, in place of the flat `external_*` fields; `link-external-item` (POST/DELETE), `external-details` and `POST /api/tasks/:taskId/sync` take `integration_id` (optional with one item, required to unlink with several); new `task.item_taken`, and `task.already_linked` now means "this integration"; `adapter.ItemKeyer` is the one place that turns what was typed into the key of an item
- [X] S73.3 The sync core works per row: a `pending` row is adopted by the first round in which the item is open (a closed item is not adopted, and an unlinked pending row disappears instead of becoming a discarded item); `load` reads the tasks of the rows in one query; `Rows.Save` is conditional on the task still owning the row (an unlink in the middle of a round is not undone); `importIssue` creates the task and the row in one transaction; `Publish` checks the link per integration after it waits for the lock, creates the row itself (with retries) and no longer rereads the integration or rewrites the task; `SyncTaskNow` takes the integration or syncs all the items, one integration at a time
- [X] S73.4 The task is the hub: when a round changes a task that has more than one item, the task hook is told, so the other item is pushed in seconds; a task with more than one item is changed under a lock of the task (64 mutexes by id, always after the lock of the integration) on a fresh read, so two rounds of different integrations do not overwrite each other's labels
- [X] S73.5 The task page shows one card per item, each with its own state, warning (`last_error` of the link, or the one the Sincronizar left), button and error; the manual link form lists only the integrations the task is not in; the badge of the lists joins the items ("GitHub #42 + Trello H0TZyzbK"); `ResetSync` also forgets the hand links of the integration
- [X] S73.7 A push that could not go out is not lost (found after delivery, when GitHub refused a token and an edit reached Trello only): the hook defers the task while its integration is backed off or when the push fails with an error that stops the sync, and the routine (at every tick) or a round that works (`SyncNow`, the task card button) puts it back in the queue; a successful `SyncNow` or `SyncTaskNow` also ends the wait. The card of a platform whose sync is stopped (a refused token, the rate limit, the platform unreachable, a repository or board gone) says so, with the reason, and that the edit is kept until it is back, and the warning goes away after a Sincronizar that works. Tests: an edit reaches both platforms; an edit made while GitHub is backed off goes out once it recovers, with no round of GitHub (the test fails without the fix); checked in the browser against the fakes (GitHub taken down: its card warns and Trello's does not; GitHub back: the warning goes)
- [X] S73.6 Tests: the migration over old data (every case above), the unique rules, the link rules in the service and the API (two integrations, an item taken, a second item in one integration, unlink one of two), and `TestDual_*` over a GitHub and a Trello at once (post to both, a change in each direction through the task, close and reopen, labels, unlinking one, deleting the task, a single item never queues, a closed issue is not adopted, two posts at once make one item, a stale save does not bring back an unlink), with the race detector; checked in the browser against `cmd/fakegithub` and `cmd/faketrello` (two cards, the Sincronizar of each, both directions, the list badge, phone width); README, design, routes, the Insomnia collection, the seed (two tasks in both platforms)

---

### Sprint 74: Creating in Both Places

- [X] S74.1 The Integrações step of the New task modal has a box on every card whose integration has the sync on, and the boxes do not exclude each other: the task goes to every ticked integration. The GitHub box starts unticked (the screen posts the issue after creating the task, one platform at a time, with a toast for each and a failure on one not stopping the others) and the Trello box starts **ticked** (the server posts the card by itself); unticking Trello is how a task is made in GitHub only
- [X] S74.2 `POST /api/projects/:projectId/tasks` takes `skip_publish` (integration ids, optional, not stored, at most 20; a non-UUID is `400 task.integration_not_found`, an id that is not an integration of the project matches nothing): `Attrs.SkipPublish` goes through `Store.SetCreateHook(func(id, skip))` and `Syncer.NotifyCreated(id, skip)` into the queue, and stays with the task through its retries until it is released or given up
- [X] S74.3 The background post is per integration: the new task is posted to the chosen auto target unless it is in the skip list (no fall to another integration) or the task already has an item in that integration; the issue the modal posted on GitHub no longer keeps the Trello card from being made
- [X] S74.4 The draft keeps `publish_to` and `skip_publish`, so the Trello box starts ticked without waiting for the integrations; the texts `tasks.publish.auto`, `auto_intro` and `auto_other` and the CSS of the card without a box are gone, `intro` says more than one can be ticked and `trello_after` says the card comes out a few seconds later
- [X] S74.5 Tests: the create hook carries the skip list, the handler (valid, unknown, bad and too many ids), the background post per integration (a task posted on GitHub also becomes a card, a hand-linked card is not doubled, an unticked Trello is skipped without falling to another, an unknown id skips nothing) and the server end to end (both at once, GitHub only, a bad id, Trello down); checked in the browser (default: Trello only; both; GitHub only; none; the modal reopening with Trello ticked; phone width); README, design, routes, the Insomnia collection

---

### Sprint 75: Mirroring an Item on the Other Platform

- [X] S75.1 The task page offers, for every enabled integration with the sync on whose type creates items and in which the task has no item yet, a card **Publicar no Trello** / **Publicar no GitHub** with a two-line note of what goes to the item and a button that calls `POST /api/tasks/:taskId/publish`; the new item shows as an integration card at once, and the warning the platform left is toasted (`mirrorable()`, `publish()`)
- [X] S75.2 The offer is manual and per task: an issue imported from GitHub becomes a Trello card, and a card from Trello a GitHub issue, without the task moving; a closed task gets no offer; an error of the platform (a token that does not write, a board with no list) shows in the card; the automatic option per integration is left out
- [X] S75.3 Tests: `TestDual_MirrorAnImportedItem` (an issue made into a card and a card into an issue, the round after settled, a change on the mirrored card reaching the other through the task, mirroring twice refused); checked in the browser (the offer, the button, the card made with the name and description, the offer gone, no offer on a closed task, the platform error in the card, English and 390 px); README, design

---

### Sprint 79: Deleting the Item on the Platforms Too

- [X] S79.1 The delete modal of the task page gets, when the task is in a platform, a box for each one (all unticked; a platform with the sync off is disabled with the reason): **Apagar a issue no GitHub** and **Arquivar o cartão no Trello**, with a note on what each does; with nothing ticked only the task goes (`removable()`, `pickRemove()`, `removeIn`)
- [X] S79.2 `DELETE /api/tasks/:taskId?remove_in=<integration>` (repeated, up to 20) always answers `200` `{"remote":[{integration_id, provider, outcome, problem}]}` (it was `204`); the links are read before the task is deleted and the platforms are called after, each on its own, so a failure never brings the task back; `task.Service.Delete(ctx, id, removeIn)` and the `Remover` set with `SetRemover` (wired to the `Syncer` in `server.go`)
- [X] S79.3 `adapter.ItemRemover` (optional): GitHub deletes the issue with the GraphQL `deleteIssue` (the REST API cannot) and, when the account is not a repository admin, closes it as `not_planned` and warns (`issue_sync.remove_only_closed`); Trello archives the card with a `PUT closed=true` that leaves the due date alone; an item already gone is `gone`, not an error; a type with no remover gets the generic fallback of closing the item
- [X] S79.4 `Syncer.Remove`: the lock of the integration, the same rules as posting (`issue_sync.remove_read_only` for a token that does not write, `integration.sync_off`), a result for each item; the screen turns the answer into one message kept across the redirect with `flash()`
- [X] S79.5 Error codes `issue_sync.remove_read_only` and `issue_sync.remove_only_closed` (both languages, `_docs/error-codes.md`); the fakes gained `POST /graphql`, `node_id`, `admin` and `Deleted` (`testutil.GitHub`) and `/_fake/admin` (`cmd/fakegithub`)
- [X] S79.6 Tests: the adapters against the fakes (delete as admin, close without admin, already gone, a refused token, the GraphQL address on GitHub Enterprise; archive without touching the due date, gone, forbidden); `TestRemove_*` end to end (both platforms, nothing ticked leaves them alone, only one, a platform the task is not in, without admin, already gone, a failure that does not stop the other nor keep the task, read-only/sync off/refused token, no remover); the service (items read before the delete, handed over after it) and the route (`remove_in` validation, the answer); checked in the browser against the fakes (the boxes, both ticked, nothing ticked, only Trello, GitHub without admin, one platform, no link, English, 390 px); README, design, routes, the Insomnia collection


### Sprint 80: An Integration Column and a Wider Page

- [X] S80.1 `task_table` (Lista de tarefas and Minhas tarefas) gets a column **Integração** after the deadline: for each item the task is linked to, a grey badge with the brand icon and the number or id of the item (`#42`, `H0TZyzbK`), the platform name in the `title` and for screen readers; the badge next to the name is gone, so a long name no longer pushes it into a second line (`tasks.col_integration` in both languages, `.task-links`)
- [X] S80.2 `linkLabel`, `linkIcon`, `linkProvider` and `linkItem` in `taskBadges`; the inner `x-for` is keyed by `integration_id` (the link has no `id`)
- [X] S80.3 `.page` and `.topbar-inner` go from 1120 to 1200px, with the side padding as it was
- [X] S80.4 The Início cards keep the text badge (not asked); the one existing assertion in `pages_test.go` that looked for the badge next to the name follows the column; checked in the browser against a project with GitHub and Trello items (one, both, none, a name long enough for four lines) at 1440, 1100 and 390 px, with no script error; README, design

### Sprint 81: A Help Page

- [X] S81.1 `GET /help`: a single page with all the help, public (no session needed: the global `LoadSession` already puts whoever is logged in on the context, and that person sees the top bar, which gets a new **Ajuda** item); `/ajuda` redirects to it (`page.Help` and `page.ToHelp`, `routes/pages.go`)
- [X] S81.2 `web/templates/pages/help.gohtml`: a contents column on the left (sticky; it scrolls by itself when taller than the screen; under 860px it becomes a collapsible `<details>` above the text) and the help on the right, in four groups: Getting started (nine steps, from the sign-up to following the numbers, each with the path on screen and a tip), The screens (fifteen), Good to know (the hourly rate, who sees and does what, the integrations) and Quick answers. Every section has a stable anchor (`#new-project`, `#hourly-rate`) that can be sent to someone
- [X] S81.3 The paths ("Organização › Sobre › Editar") are built in the template from the catalog keys of the screens themselves (`nav.*`, `org.*`, `project.head.*`, `tasks.new`, `time.start`…), so the guide cannot drift from the button names; the labels quoted inside the sentences were checked one by one against the catalog, in both languages
- [X] S81.4 `web/static/pages/help.js` (`helpPage`): marks in the contents the section being read (the last one whose top passed the reading line); keeps a clicked item marked until the person scrolls on their own, because the last questions cannot reach the top of the page; follows the real height of the top bar (it wraps to 170px on a phone) for both the sticky contents and the scroll margin, so a title never hides behind it; collapses the contents on narrow screens and reopens them when the window grows. Without JavaScript the links are plain anchors
- [X] S81.5 For a visitor without a session the language switch goes inside the page header (the floating one would cover the cards while scrolling); `html:has(.help)` turns on smooth scrolling for this page only, and `prefers-reduced-motion` turns it off
- [X] S81.6 `help.*` in `pt-BR.yaml` and `en.yaml` (136 keys each, one per sentence), `titles.help` and `nav.help`; README: the `/help` row in the browser routes, the public-page exception and the Ajuda item in the menu
- [X] S81.7 The existing tests that list pages follow the change (`help` in `TestPages_AllTemplatesLoad`; `/help` logged out and logged in in the English-leftovers and language-toggle tests); no new screen test, since the front end is still moving. Checked in the browser (Chromium, desktop and 390px wide, light and dark, both languages, with and without a session)
---

### Seed with a Full Demo

- [X] D1 `cmd/seed` fills every screen: twelve people (two admins), four customers, eight projects (one internal, one with negative margin), 62 tasks (11 unassigned, some overdue, 24 linked to GitHub, GitLab and Trello items), eight integrations, about 1,400 closed sessions over 75 days from a fixed random sequence, and two people with the clock open

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
