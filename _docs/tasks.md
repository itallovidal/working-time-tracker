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

- [ ] S5.1 Implement `TimeEntryService` with ClockIn, ClockOut, ListByTask, ListByPerson, TotalTime methods
- [ ] S5.2 Implement ClockIn: validate task exists in project; check no active entry for person (global); insert entry with start_at = now, end_at = NULL
- [ ] S5.3 Implement ClockOut: find active entry for person; set end_at = now; error if none exists
- [ ] S5.4 Implement `POST /api/projects/:projectId/time-entries/clock-in` handler (body: task_id, person_id)
- [ ] S5.5 Implement `POST /api/projects/:projectId/time-entries/clock-out` handler (body: person_id)
- [ ] S5.6 Implement `GET /api/projects/:projectId/time-entries` handler with task_id and person_id query filters
- [ ] S5.7 Implement `GET /api/projects/:projectId/time-entries/total` handler computing total duration for task or person
- [ ] S5.8 Return friendly error when clock-in is attempted while already clocked in

---

### Sprint 6: Integrations

- [ ] S6.1 Implement `IntegrationService` with Create, List, Get, Update, Delete, ValidateConfig methods
- [ ] S6.2 Define `Integration` interface: `ValidateConfig(config) error`, `FetchItemDetails(config, itemID) (ItemDetails, error)`
- [ ] S6.3 Implement GitHub integration type (validates token via GitHub API, fetches issue by number)
- [ ] S6.4 Implement GitLab integration type (validates token via GitLab API, fetches issue by IID)
- [ ] S6.5 Implement credential encryption/decryption helper (AES-GCM using INTEGRATION_ENCRYPTION_KEY)
- [ ] S6.6 Implement `POST /api/projects/:projectId/integrations` handler (validates config via type-specific implementation)
- [ ] S6.7 Implement `GET /api/projects/:projectId/integrations` handler
- [ ] S6.8 Implement `GET /api/integrations/:integrationId` / `PATCH /api/integrations/:integrationId` / `DELETE /api/integrations/:integrationId` handlers
- [ ] S6.9 Graceful degradation: return null external details with error indicator when integration API is unreachable

---

### Sprint 7: Security & Backend Tests

- [ ] S7.1 Ensure integration credentials are never included in any API response, log, or error message
- [ ] S7.2 Add redaction for credential fields in request/response logging
- [ ] S7.3 Validate that integration type matches a known, whitelisted type before persisting
- [ ] S7.4 Write tests for OrganizationService: create, list, update, delete, delete-with-projects-rejected
- [ ] S7.5 Write tests for PersonService: create, unique email within org, list by org
- [ ] S7.6 Write tests for ProjectService: create with defaults, create with explicit config, update config, delete cascade
- [ ] S7.7 Write tests for TeamService + TeamMembershipService: create, add/remove members, duplicate membership rejected
- [ ] S7.8 Write tests for TaskService: create with default deadline, create with explicit deadline, assignee must be team member, update, delete cascade, external item link/unlink
- [ ] S7.9 Write tests for TimeEntryService: clock in success, clock in on missing task, clock out success, clock out with no active session, overlapping active session rejection, total time calculation
- [ ] S7.10 Write tests for IntegrationService: create with valid/invalid config, credentials encrypted at rest, credentials never returned on get
- [ ] S7.11 Write tests for credential encryption helper: round-trip encrypt/decrypt, wrong key fails
- [ ] S7.12 Write handler/integration tests for all API endpoints covering happy path and error cases

---

## 🎨 FASE 2: FRONTEND

---

### Sprint 8: Frontend — Core Views (Layout, Org, Person, Project, Team)

- [ ] S8.1 Create base HTML layout template (header with org/project navigation, vendored Alpine.js include) and register the Echo template renderer
- [ ] S8.2 Build Organization management UI: list orgs, create org form, org settings page
- [ ] S8.3 Build Person management UI within org: list persons, create person form
- [ ] S8.4 Build Project management UI: list projects within org, create project form (with sprint/daily/sync config fields), project settings page
- [ ] S8.5 Build Team management UI: list teams within project, create team form, manage team members (add/remove)

---

### Sprint 9: Frontend — Tasks & Time Tracking

- [ ] S9.1 Build Task list UI scoped to project (name, assignee, deadline, external item badge) with create/edit/delete
- [ ] S9.2 Build Task detail UI: edit form, external item link/unlink via integration selector
- [ ] S9.3 Build Time Tracking UI within project: clock in (select task) and clock out button, active-session indicator, live elapsed timer
- [ ] S9.4 Build time-entries list and total-time display filtered by task and/or person within a project
- [ ] S9.5 Write integration tests for frontend-serving routes: templates render, static assets served, main views return 200

---

### Sprint 10: Frontend — Integrations & Documentation

- [ ] S10.1 Build Integration config UI: list integrations per project, create with type selection and dynamic config form, credential mask, enable/disable toggle
- [ ] S10.2 Wire Alpine.js client-side error handling and inline validation feedback from API errors
- [ ] S10.3 Write README with setup instructions (env vars, database setup, how to run, org/project/team workflow)
- [ ] S10.4 Document the REST API endpoints with example requests/responses
- [ ] S10.5 Document the web UI: browser routes, navigation flow, and that Alpine.js is vendored with no build step
