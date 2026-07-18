# Working Time Tracker — Tasks

## 📦 FASE 1: BACKEND

---

### Sprint 1: Foundation — Setup & Database

- [ ] S1.1 Initialize Go module and add dependencies: Echo web framework, Postgres driver, UUID lib, env config loader; vendor Alpine.js as a local static asset
- [ ] S1.2 Create base project structure: `cmd/server/main.go`, `internal/handler/`, `internal/service/`, `internal/store/`, `internal/config/`, `internal/integration/`, `web/templates/`, `web/static/`
- [ ] S1.3 Add configuration loading from environment (DATABASE_URL, PORT, INTEGRATION_ENCRYPTION_KEY)
- [ ] S1.4 Set up PostgreSQL connection pool, Echo server, and graceful shutdown in `main.go`
- [ ] S1.5 Configure Echo to render HTML templates and serve static assets (Alpine.js); embed `web/` via `embed.FS`
- [ ] S1.6 Create migration for `organizations` table (id UUID PK, name, created_at)
- [ ] S1.7 Create migration for `persons` table (id UUID PK, name, email, organization_id FK, created_at) with unique index on (organization_id, email)
- [ ] S1.8 Create migration for `projects` table (id UUID PK, organization_id FK, name, description, github_repo_url nullable, gitlab_repo_url nullable, sprint_duration_days int default 14, daily_time TIME nullable, weekly_sync_day VARCHAR nullable, created_at)
- [ ] S1.9 Create migration for `teams` table (id UUID PK, name, project_id FK, created_at)
- [ ] S1.10 Create migration for `team_memberships` table (person_id FK, team_id FK, created_at) with unique constraint on (person_id, team_id)
- [ ] S1.11 Create migration for `tasks` table (id UUID PK, project_id FK, name, description, assignee_id FK, deadline timestamptz, external_integration_id FK nullable, external_item_id VARCHAR nullable, external_item_url TEXT nullable, created_at)
- [ ] S1.12 Create migration for `time_entries` table (id UUID PK, task_id FK, person_id FK, start_at timestamptz, end_at timestamptz nullable, created_at) with partial unique index on (person_id) WHERE end_at IS NULL
- [ ] S1.13 Create migration for `integrations` table (id UUID PK, project_id FK, type VARCHAR, display_name, config JSONB, enabled boolean default true, created_at)
- [ ] S1.14 Add migration runner that applies all migrations on startup

---

### Sprint 2: Organization & Person

- [ ] S2.1 Implement `OrganizationService` with Create, List, Get, Update, Delete methods
- [ ] S2.2 Implement `POST /api/orgs` handler (validate name required)
- [ ] S2.3 Implement `GET /api/orgs` and `GET /api/orgs/:orgId` handlers
- [ ] S2.4 Implement `PATCH /api/orgs/:orgId` handler
- [ ] S2.5 Implement `DELETE /api/orgs/:orgId` handler (reject if org has active projects)
- [ ] S2.6 Implement `PersonService` with Create, List, Get, Update methods (scoped to organization)
- [ ] S2.7 Implement create logic: validate unique email within the organization
- [ ] S2.8 Implement `POST /api/orgs/:orgId/persons` handler (validate name + email required)
- [ ] S2.9 Implement `GET /api/orgs/:orgId/persons` handler
- [ ] S2.10 Implement `GET /api/persons/:personId` and `PATCH /api/persons/:personId` handlers

---

### Sprint 3: Project & Team

- [ ] S3.1 Implement `ProjectService` with Create, List, Get, Update, Delete methods (scoped to organization)
- [ ] S3.2 Implement create logic: default sprint_duration_days = 14, validate config fields
- [ ] S3.3 Implement `POST /api/orgs/:orgId/projects` handler (validate name required)
- [ ] S3.4 Implement `GET /api/orgs/:orgId/projects` handler
- [ ] S3.5 Implement `GET /api/projects/:projectId` and `PATCH /api/projects/:projectId` handlers
- [ ] S3.6 Implement `DELETE /api/projects/:projectId` handler (cascade delete teams, tasks, time entries, integrations)
- [ ] S3.7 Implement `TeamService` with Create, List, Get, Update, Delete methods (scoped to project)
- [ ] S3.8 Implement `POST /api/projects/:projectId/teams` handler (validate name required)
- [ ] S3.9 Implement `GET /api/projects/:projectId/teams` handler
- [ ] S3.10 Implement `GET /api/teams/:teamId` / `PATCH /api/teams/:teamId` / `DELETE /api/teams/:teamId` handlers
- [ ] S3.11 Implement `TeamMembershipService` with Add, Remove, ListByTeam methods
- [ ] S3.12 Implement `POST /api/teams/:teamId/members` handler (validate person exists in org)
- [ ] S3.13 Implement `DELETE /api/teams/:teamId/members` handler
- [ ] S3.14 Implement `GET /api/teams/:teamId/members` handler

---

### Sprint 4: Task Management

- [ ] S4.1 Implement `TaskService` with Create, Get, List, Update, Delete methods (scoped to project)
- [ ] S4.2 Implement create logic: default deadline to created_at + 7 days when not provided
- [ ] S4.3 Implement `POST /api/projects/:projectId/tasks` handler (validate name + assignee required; assignee must be team member of the project)
- [ ] S4.4 Implement `GET /api/projects/:projectId/tasks` handler
- [ ] S4.5 Implement `GET /api/tasks/:taskId` handler (include external item details if linked)
- [ ] S4.6 Implement `PATCH /api/tasks/:taskId` handler
- [ ] S4.7 Implement `DELETE /api/tasks/:taskId` handler (cascade delete associated time entries)
- [ ] S4.8 Implement `POST /api/tasks/:taskId/link-external-item` handler (body: integration_id, external_item_id, external_item_url)
- [ ] S4.9 Implement `DELETE /api/tasks/:taskId/link-external-item` handler
- [ ] S4.10 Implement `GET /api/tasks/:taskId/external-details` handler (fetch via integration)

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
