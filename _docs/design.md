## Context

This is a greenfield project. There is no existing code, database, or API. The system will be built from scratch to track worked time against tasks within projects, where tasks are linked to people, teams, and optionally to external items (issues, cards, tickets) through generic integrations. The project lives in a standalone repository (`working-time-tracker`).

The system supports multiple organizations (companies), each with multiple projects. A project has teams of people, a configurable sprint duration, daily standup time, and weekly sync schedule. People register within an organization and are assigned to teams. Projects can have integrations with external platforms (GitHub, GitLab, Slack, Trello, etc.) to link tasks to external items and push notifications.

Stakeholders are teams that work by project and want accurate time-per-task data, visibility into project progress, task deadlines, and linked external items. The main audience is software development companies (software houses and consultancies) that serve several customers and allocate people to one or more projects; the demo seed is one of them. The model itself stays generic: nothing in the organization profile requires a software company.

## Goals / Non-Goals

**Goals:**
- Provide an organizational structure: organizations → projects → teams → people.
- Implement clock-in/clock-out as the core time-tracking primitive within a project, enforcing one active session per person.
- Manage tasks within projects, with default one-week deadline and single-person assignment.
- Support generic integrations with external platforms (GitHub, GitLab, Slack, Trello, etc.) configured per project, replacing any single-platform coupling.
- Allow linking tasks to external items (issues, cards, tickets) through integrations and retrieving their details on demand.
- Support project-level configuration: sprint duration, daily standup time, and weekly sync schedule.
- Model how the organization is paid and how it pays: a customer hires a project at an hourly bill rate, and each person has an hourly pay rate per project. Admins see and change every value; a member sees only their own pay rate.
- Define a minimal REST API covering all CRUD and clock-in/out operations, scoped by organization and project.
- Serve a lightweight web UI (server-rendered HTML + Alpine.js) from the Go binary for administration and daily use.

**Non-Goals:**
- Real-time collaboration or SSE/WebSocket updates.
- A heavy SPA framework or separate frontend build pipeline (server-rendered HTML + Alpine.js is sufficient for v1).
- Webhook-driven automatic issue sync (external item details are fetched on demand for this iteration).
- Complex RBAC with fine-grained permissions (simple member/admin roles within an organization is sufficient for v1).
- Invoicing or payroll: the system stores the hourly rate a customer pays per project and the hourly rate each person is paid per project, and shows what each work session is worth, but it does not issue invoices, close pay periods or handle taxes.
- Fixed-price projects and rate history with effective dates: billing is hourly only, and a rate change applies from the next clock-in on.
- Multi-region or sharded deployment (single PostgreSQL instance for v1).

## Decisions

### Decision 1: Tech stack — Go (Echo) + Ent + PostgreSQL + Alpine.js frontend
**Choice:** Go backend using the Echo web framework, with PostgreSQL as the datastore and Ent as the ORM (the project started on GORM and moved to Ent). The Go server also serves the frontend as server-rendered HTML templates enhanced with Alpine.js for interactivity. The frontend consumes the JSON REST API exposed by the same server.
**Rationale:** Ent generates a typed client from the schema declared in `ent/schema` — eliminating raw SQL boilerplate while keeping the data model in Go. Schema changes reach the database through versioned migrations (Decision 9), not through the ORM at startup. Echo provides a lightweight HTTP router with middleware, static-asset serving, and HTML template rendering. Together they keep the backend fast to iterate on. PostgreSQL gives reliable timestamp handling, unique constraints, and JSONB columns for flexible integration config storage. Alpine.js adds reactivity with no build step.
**Alternatives considered:**
- Node.js + SQLite: simpler to start but weaker concurrency guarantees and no native single-binary deployment.
- Python + Postgres: viable but heavier runtime; Go's compiled output suits a small tracker service better.
- Raw `database/sql` without an ORM: more control but slower to develop; Ent's generated client and edge handling justify the abstraction for this project's data-model complexity.
- Separate SPA (React/Vue) frontend: more capable but adds a JavaScript build toolchain and a second deployment artifact; overkill for a small internal tracker.

### Decision 2: Data model — Seven core entities
**Choice:** Seven entities: `Organization`, `Project`, `Team`, `Person`, `Task`, `WorkSession`, `Integration`, with a join table `TeamMembership`.

```
Organization: id (UUID PK), name, created_at,
              profile: summary, description, industry, founded_year, size,
                       website, contact_email, phone, linkedin_url, instagram_url,
                       legal_name, cnpj, address_line1, address_line2, city, state,
                       postal_code, country (all optional)
              how it works: work_mode (remote | hybrid | onsite, optional),
                            timezone (default America/Sao_Paulo), currency (default BRL)
              (sprint length is per project, not per organization: different
               projects can work differently; weekly hours are per person)
Project: id (UUID PK), organization_id (FK Organization), name, description,
         github_repo_url (nullable), gitlab_repo_url (nullable),
         sprint_duration_days (int, default 14),
         daily_time (VARCHAR "HH:MM", nullable — null means no daily),
         weekly_sync_day (VARCHAR nullable — null means no weekly),
         weekly_sync_time (VARCHAR "HH:MM", nullable — only with a weekly day),
         customer_id (FK Customer, nullable — internal projects have none),
         bill_rate_cents (int, nullable — what the customer pays per hour;
         admin-only, served by /billing and never in the project JSON),
         created_at
Customer: id (UUID PK), organization_id (FK Organization), name, document (CNPJ),
          contact_name, contact_email, contact_phone, created_at
          (named Customer in code because Ent reserves "Client")
Team: id (UUID PK), name, project_id (FK Project), created_at
Person: id (UUID PK), name, email, organization_id (FK Organization),
        weekly_hours (int, nullable — the hours per week agreed with the person,
        for the whole organization and not per project), created_at
TeamMembership: person_id (FK Person), team_id (FK Team), created_at
               (unique constraint on person_id + team_id)
Allocation: id (UUID PK), project_id (FK Project), person_id (FK Person),
            pay_rate_cents (int >= 0), created_at
            (unique constraint on project_id + person_id; the rate lives here and
             not on TeamMembership because one person can be in two teams of the
             same project)
            A collaborator of a project is a person with an Allocation in
            it: the rate is required to join. A TeamMembership is only
            accepted for a person who already has the Allocation; the service
            enforces it, there is no foreign key between the two tables.
Integration: id (UUID PK), project_id (FK Project), type (VARCHAR — github,
             gitlab, trello), display_name,
             credentials (JSONB, the encrypted token),
             metadata (JSONB, the fields of that platform, in clear),
             enabled (boolean, default true), created_at
Task: id (UUID PK), project_id (FK Project), name, description,
      assignee_id (FK Person, nullable), deadline (TIMESTAMP, default created_at + 7 days),
      external_integration_id (FK Integration, nullable),
      external_item_id (VARCHAR, nullable),
      external_item_url (TEXT, nullable),
      created_at
WorkSession: id (UUID PK), task_id (FK Task), person_id (FK Person),
             start_at (TIMESTAMP), end_at (TIMESTAMP nullable),
             pay_rate_cents (int, nullable), bill_rate_cents (int, nullable),
             created_at
             (the two rates are copied from the Allocation and the Project at
              clock-in, so changing a rate later never rewrites past hours;
              clock-in is refused for a person without an Allocation in the
              project)
```

A partial unique index enforces one active session per person:
`CREATE UNIQUE INDEX one_active_session ON work_sessions (person_id) WHERE end_at IS NULL`

**Rationale:** The model maps directly to the organizational hierarchy. A person belongs to an organization, which has projects. Each project has teams, integrations, and tasks. Links to external items are stored on the task itself (one link per task), referencing which integration provides the connection. The partial unique index on WorkSession is the database-level guarantee against concurrent clock-in races. JSONB for Integration.metadata allows type-specific fields without schema changes per platform.

**Alternatives considered:**
- Separate `TaskLink` table for external item references: more flexible for multiple links per task but over-normalized for v1 where one link per task is sufficient.
- Separate per-integration tables: not extensible; adding a new integration type would require a migration.

### Decision 3: Single active session enforcement — Partial unique index
**Choice:** Enforce the "one active session per person" rule with a PostgreSQL partial unique index: `CREATE UNIQUE INDEX one_active_session ON work_sessions (person_id) WHERE end_at IS NULL`. The index is declared in the Ent schema (`ent/schema/worksession.go`) and created by the baseline migration (Decision 9).
**Rationale:** Database-level guarantee that prevents race conditions even if two concurrent clock-in requests arrive. The service layer checks first for a friendly error, but the index is the hard guarantee. This decision is unchanged from the original design because the constraint is per-person regardless of project scope.
**Alternatives considered:**
- Application-level lock/check only: vulnerable to concurrent request races.
- A `currently_active_entry_id` column on Person: denormalized state that can drift from the truth.

### Decision 4: Organizational hierarchy — Org → Project → Team → Person
**Choice:** Model the hierarchy as Organization (top-level tenant), Project (work container), Team (group of people within a project), Person (individual, belongs to one organization).

- A person registers under one organization.
- An organization has many projects; each project belongs exclusively to one organization.
- A project has many teams; each team belongs exclusively to one project.
- A person can be a member of multiple teams within the same project via TeamMembership.
- Tasks are scoped to a project. A task may have no assignee: it is then available, and the first person to clock in on it becomes the assignee, team or not. An assignee chosen by hand must be a member of at least one team within that project.
- Time entries are scoped to tasks, which are scoped to projects. The clock-in endpoint requires a project context.

**Rationale:** Clean tenant isolation at the organization level. Project is the natural scope for configuration (sprint, daily, sync) and integration settings. Teams allow grouping without duplicating data — a person can be on multiple teams.
**Alternatives considered:**
- Flat project list without organizations: simpler but no multi-tenant support.
- Person directly assigned to project without teams: loses the grouping concept and makes per-team reporting harder.
- Hierarchical orgs (parent/child organizations): over-engineering for v1; single-level orgs are sufficient.

### Decision 5: Generic integration system
**Choice:** Replace the previous GitHub-specific connection with a generic Integration entity configured per project.

Each Integration stores:
- `type`: a string identifier for the platform (`github`, `gitlab`, `trello`).
- `display_name`: a human-readable label (e.g., "Production Repo", "Client board").
- `credentials`: the token, encrypted at rest (Decision 6).
- `metadata`: a JSONB column holding the fields only that platform has (the repository, the project, the Trello key and board), in clear.
- `enabled`: flag to enable/disable without deleting.

Tasks can be linked to external items by setting `external_integration_id`, `external_item_id`, and `external_item_url`. When reading a task, the system fetches current item details from the integration's API on demand. If the API is unavailable, the task returns a null detail block (graceful degradation). Per-integration API responses are cached in-process with a short TTL (e.g., 60s).

Each integration type implements a common interface (`internal/adapter`):
- `Descriptor() Descriptor` — what the type is and what it asks for: its label, the metadata fields (key, label, hint, required) and how the linked item is called.
- `CheckMetadata(raw) (metadata, error)` — without calling the platform, check that the metadata has every field the type needs and return what is stored: only the declared fields, normalized.
- `Validate(conn) error` — verify the connection by calling the platform API.
- `FetchItemDetails(conn, itemID) (ItemDetails, error)` — retrieve title, state, URL.
- (Future) `SendNotification(conn, message)` — push notifications.

**Shared structure (Sprint 23):** Every type is created and edited with the same body, `{type, display_name, enabled, token, metadata}`, and answers with the same one, `{id, project_id, type, display_name, enabled, has_token, metadata, created_at}`. What is common sits at the first level; what belongs to the platform goes into `metadata`: `repo` for GitHub, `project_url` for GitLab, `api_key` and `board_id` for Trello. The adapter receives it as a `Connection{Token, Metadata}` and parses the metadata into its own typed struct, through `CheckMetadata`, before any request, so the check lives inside each integration. Before this, the body carried a `config` map that mixed the token with those fields and was encrypted whole: nothing of it could be shown back, and editing meant typing everything again. `PATCH` now takes any subset; an empty `token` keeps the stored one, a `metadata` replaces the stored one, and the platform is only called again when a token came or the metadata really changed, so renaming or disabling an integration does not depend on its token still being valid. The type catalog is also the backend's: the project pages receive the descriptors in `window.BOOT.integration_types` and draw the form and the labels from them, so a new adapter needs no JavaScript.

**Trello (Sprint 23):** A Trello integration is bound to a board and links tasks to cards. The token goes in the shared field and the API key, which Trello documents as safe to be public, in the metadata. Both travel in the `Authorization: OAuth` header, never in the URL. One request (`GET /1/cards/{id}` with `list=true&board=true`) brings the card with its list and board: the state of a card is the name of the list it is in ("arquivado" when closed), and a card from another board is refused.

**Input that reaches a platform URL (Sprint 23):** The repository, the board, the issue number and the card id come from a person and end up in the path of an API call. Each adapter checks their format first (an issue number is digits only, a repository is `owner/name`), so a member linking an item cannot point the request at another resource the token can read.

**Rationale:** Decouples the core tracking system from any single external platform. Adding a new integration type only requires implementing the interface and registering it; no schema, core logic or frontend changes. The JSONB metadata allows per-type fields while the token stays encrypted. On-demand fetch avoids stale data and webhook plumbing in v1.
**Alternatives considered:**
- Webhook-driven sync: more responsive but adds endpoint, secret management, and per-platform webhook registration complexity — deferred.
- Persisting external item details and refreshing on a schedule: background job overhead for v1.
- Separate `config_github`, `config_gitlab` etc. columns: not extensible; each new platform requires a migration.

### Decision 6: Credential security — Encryption at rest
**Choice:** Store the integration token encrypted at rest in the Integration.credentials JSONB column using symmetric AES-GCM with a key from environment configuration (`INTEGRATION_ENCRYPTION_KEY`). Never return it in any API response or log; expose only an `enabled` boolean and a `has_token` indicator per integration. The metadata is not a secret: it is stored in clear and returned, and each type only keeps the fields it declares, so a secret sent there by mistake under another key is dropped.
**Rationale:** Symmetric encryption is simple for a single-instance service and keeps secrets safe if the database is compromised. The boolean indicator lets the UI show connection status without leaking secrets. Until Sprint 23 the whole config was encrypted, to avoid per-field handling; with one token shared by every type and the rest in `metadata`, only the token needs it, and the rest can be read, shown in the edit form and kept when the encryption key changes. A row written before that still has token and fields together in the blob: the token stays readable, the missing metadata is reported as the reason when details are fetched, and editing the integration once rewrites it in the new shape.
**Alternatives considered:**
- OAuth flow with refresh tokens: more secure/scalable but far more complex per-platform for v1.
- Plaintext storage: unacceptable security posture.
- Column-level encryption per field: more granular but couples schema to integration types.
- Marking some metadata fields as secret and encrypting those: no type needs a second secret today; a type that does would add it then.

### Decision 7: API style — REST with JSON, scoped by organization and project
**Choice:** RESTful JSON endpoints grouped by resource, scoped under organizations and projects.

- `POST /api/orgs` / `GET /api/orgs` — create / list organizations
- `GET /api/orgs/:orgId` / `PATCH /api/orgs/:orgId` — read / update organization
- `POST /api/orgs/:orgId/projects` / `GET /api/orgs/:orgId/projects` — project CRUD within an org
- `GET /api/projects/:projectId` / `PATCH /api/projects/:projectId` / `DELETE /api/projects/:projectId` — project detail / update / delete
- `GET /api/projects/:projectId/overview` — the project in numbers: people, teams, hours, cost, revenue, margin, age, tasks and integrations (admins only)
- `POST /api/projects/:projectId/teams` / `GET /api/projects/:projectId/teams` — team CRUD
- `GET /api/teams/:teamId` / `PATCH /api/teams/:teamId` / `DELETE /api/teams/:teamId`
- `POST /api/teams/:teamId/members` / `DELETE /api/teams/:teamId/members` — team membership
- `GET /api/projects/:projectId/collaborators` / `DELETE /api/projects/:projectId/collaborators/:personId` — who is in the project, with the hourly rate and the teams / remove a person from both
- `POST /api/orgs/:orgId/persons` / `GET /api/orgs/:orgId/persons` — person CRUD scoped to org
- `GET /api/persons/:personId` / `PATCH /api/persons/:personId`
- `PATCH /api/persons/:personId/weekly-hours` — the person's weekly hours (admin only)
- `POST /api/projects/:projectId/tasks` / `GET /api/projects/:projectId/tasks` — task CRUD scoped to project (list filters: q, assignee_id — a person or `none` —, deadline_to; pages: page, per_page)
- `GET /api/tasks/:taskId` / `PATCH /api/tasks/:taskId` / `DELETE /api/tasks/:taskId`
- `POST /api/projects/:projectId/work-sessions/clock-in` — clock in (body: task_id, person_id)
- `POST /api/projects/:projectId/work-sessions/clock-out` — clock out (body: person_id)
- `GET /api/projects/:projectId/work-sessions` — list (filters: task_id, person_id)
- `GET /api/projects/:projectId/work-sessions/total` — total time (filters: task_id, person_id)
- `POST /api/projects/:projectId/integrations` / `GET /api/projects/:projectId/integrations` — integration CRUD
- `GET /api/integrations/:integrationId` / `PATCH /api/integrations/:integrationId` / `DELETE /api/integrations/:integrationId`
- `POST /api/tasks/:taskId/link-external-item` — link task to external item (body: integration_id, external_item_id, external_item_url)
- `DELETE /api/tasks/:taskId/link-external-item` — unlink external item
- `GET /api/tasks/:taskId/external-details` — fetch linked item details from integration

**Rationale:** Scoping under orgs and projects provides natural access boundaries. The URL hierarchy mirrors the data model, making permissions and filtering straightforward. JSON is universally supported by both the Alpine.js frontend and CLI/script clients. The same Echo server additionally serves the frontend (see Decision 8); the `/api/*` endpoints remain the source of truth and are reusable by non-browser clients.

### Decision 8: Frontend delivery — Server-rendered HTML + Alpine.js, served by Echo
**Choice:** The Echo server renders HTML templates (Go `html/template`) for each main view and serves them at browser-friendly routes (e.g., `/`, `/orgs/:orgId`, `/projects/:projectId`, `/projects/:projectId/tasks`, `/projects/:projectId/time-tracking`). Templates load Alpine.js from a vendored local static asset for client-side interactivity. Alpine components call the JSON REST API (under `/api/*`) via `fetch` for create/update/clock-in/out actions and re-render the relevant sections. Templates and static assets are embedded into the binary via `embed.FS` so deployment is a single self-contained artifact.
**Rationale:** Keeps a single deployable binary with no JavaScript build step. Alpine.js provides enough reactivity for forms, lists, the clock in/out button, and an active-session timer without a framework. The JSON API remains the source of truth and stays reusable by CLI/script clients. Organization/project scoping in the URL is reflected in the navigation structure.
**Icons (Sprint 14):** UI icons are the one exception to "everything is vendored": Font Awesome Free is loaded from cdnjs with Subresource Integrity, as the CSS + webfont build. It is the only third-party request the pages make. In a deployment without internet access the icons do not render; every control keeps its text label, so the interface stays usable. Templates use the `icon` partial (`{{template "icon" "pen"}}`) instead of writing the markup.
**Modal (Sprint 14):** There is a single modal, in the base layout, built with Alpine and CSS (`x-show`, `x-transition`), with no native `<dialog>` and no Alpine plugin. Pages hand it content with `x-teleport` and open it through `Alpine.store('modal')`; the teleported content stays in the scope of the page component, so a form in the modal still reads and writes that component's state. While it is open the rest of the page is `inert`, which keeps focus inside without a focus-trap plugin.
**Personal overview and sessions (Sprint 29):** The Ponto tab used to hold the clock, everyone's sessions and the totals, and a member saw colleagues' sessions (only the money was hidden). Now `GET /api/projects/:projectId/work-sessions` and `/total` are limited on the server: an admin sees everyone or whoever `person_id` names, a member only sees their own (no `person_id` means theirs, someone else's is a 403 `work_session.other_person_admin_only`, and a total by task alone adds only their time). Three screens share one `sessionList()` factory in `project.js` (list, date/task/person filters, totals that follow the filters, ten per page): the personal "Visão geral" (the first tab of everyone, today and this week plus own sessions, with only the person's own amount), the Gestão overview (everyone's sessions with cost, revenue and margin, replacing the "Horas por pessoa" table; `by_person` stays in the API for the activity card), and no longer the Ponto, which is now the clock plus the tasks assigned to the person, each with a Start button. `/projects/:projectId` opens on the personal overview for everyone.

**Daily and weekly are optional, and the weekly has a time (Sprint 31):** A project may have only a daily, only a weekly, both or neither (the daily with some people at 10:00, the weekly with more people at 13:00). The model already had "no daily" and "no weekly" as null, so the change is `projects.weekly_sync_time` and how the forms ask: "O projeto tem daily" and "O projeto tem weekly" are checkboxes that reveal the time (and, for the weekly, the day), instead of a blank field that reads as "forgot to fill". The API still takes `daily_time`, `weekly_sync_day` and `weekly_sync_time` with the same convention (omitted keeps, `""` clears), and the weekly time cannot exist without the day: clearing the day clears the time, and sending a time with no day is a 400 (`project.weekly_time_without_day`). Weeklies created before the column keep the day with no time; the forms ask for the time when someone edits the project. The checkbox state lives in the form draft (`WTT.routine`), not in the database, and the shared markup of the two modals is the `project_routine` partial. The modals group their fields in three fieldsets: Projeto, Cliente e cobrança, Rotina do time. Participants of each meeting are not modelled.

**Read-only collaborators for everyone (Sprint 30):** After the admin tabs moved to the Gestão, regular people had no way to see who works on a project. `/projects/:projectId/collaborators` renders the same `project_teams` page and component, but `Data.CanManage()` (`Management && IsAdmin`) is false there, so every admin tool is left out, for admins too: no rates, margin, summary money tiles, add/edit modals or buttons. The page passes `readonly: true` in the boot data, and `projectTeams` then skips the admin-only API calls (billing, the organization's people, all sessions). The full tab stays in the Gestão.

**Gestão area (Sprint 28):** The project tabs mixed what everyone does every day with what only admins need. Overview, Collaborators, Integrations and Settings moved to a "Gestão" area, `/projects/:projectId/management/{overview,teams,integrations,settings}`, behind the same admin guard the overview already used, so a member gets the "not found" page; the API keeps its permissions. The tab bar has two modes (`Data.Management`): the everyday tabs (Quadro de tarefas, Ponto) with a "Gestão" button at the end for admins, and the Gestão tabs with a "Voltar ao projeto" link. `/projects/:projectId` opens on the Ponto for everyone, the root of the Gestão redirects to its overview, and the old paths (`/overview`, `/teams`, `/integrations`, `/settings`) answer 303 to the new ones, keeping the query string (`?view=teams`).

**The Ponto tab folds into the Início (Sprint 33):** The Ponto tab (the clock and "Suas tarefas") and the personal Visão geral were two screens about the same person's day. The Ponto tab is gone, and its clock, its warning for people without an hourly rate and its task list are the top of the personal overview, which is now called **Início** (house icon; the Gestão keeps its own "Visão geral"). `/projects/:projectId/time-tracking` redirects to `/overview` (303), and the links that pointed to it (the profile's rates, the clock pill, "Voltar ao projeto" in the Gestão) go straight to the Início. One Alpine component, `projectMyOverview`, absorbed `timeTracking`; no API route changed. The layout: the clock card (with the open session's value, `rate × elapsed` with the server's rounding, from the rate locked at clock-in) beside the "Seu tempo neste projeto" card, then "Suas tarefas" as small cards with the name, View details and Start (no deadline), then the sessions.

**Task page and edit modal (Sprint 32):** The board row had a Start button and the task link opened a form. Now the row has a Details button, and `/tasks/:taskId` only shows the task (name, description, assignee, deadline, time tracked, the linked external item), like the project settings tab: the pen opens an Edit task modal with the same assignee choices as New task and, inside it, Delete task with confirmation. Starting the clock moved to the task page (the Ponto tab keeps its own button). No API route changed; the page has room for what comes next (tags, a longer description).

**Tasks without an assignee (Sprint 27):** `tasks.assignee_id` is nullable (the foreign key still refuses to delete a person who has tasks). `POST /api/projects/:projectId/tasks` no longer asks for `assignee_id`, and `PATCH /api/tasks/:taskId` with an empty `assignee_id` clears it; `task.assignee_required` is gone. `assignee_id=none` in the list returns only the available tasks. `ClockIn` hands an unassigned task to the person who clocked in with one conditional `UPDATE ... WHERE assignee_id IS NULL`, after the session is created, so a refused clock-in takes nothing and two simultaneous ones cannot both win. Team membership is not asked at that moment, only when an admin picks the assignee by hand. The "Tarefas" tab is now the "Quadro de tarefas" and lists everything.

**Task list (Sprint 16):** The task list is the first one that filters and paginates on the server instead of loading everything and filtering in the browser. `GET /api/projects/:projectId/tasks` takes `q`, `assignee_id`, `deadline_to`, `page` and `per_page`. Without `page` it still returns the plain array, because the time tracking screen needs every task of the project; with `page` it returns `{items, total, page, per_page, assignees}`. The browser turns a deadline shortcut ("until the end of next week") into an instant in the viewer's time zone and sends that, so the server never interprets a date. The page keeps the search, the filters and the page number in its own URL, so a reload, a shared link or the way back from a task lands on the same place.
**Collaborators tab (Sprint 17):** A person is tied to a project in two independent ways: an Allocation (the hourly rate, which allows clocking in) and a TeamMembership (which allows being assigned tasks). The project used to show them on two tabs, "Times" and "Valores", so adding someone meant visiting both. The "Colaboradores" tab shows one list: a collaborator is whoever has at least one of the two. There is no table for it; `GET /api/projects/:projectId/collaborators` computes the union, and `DELETE .../collaborators/:personId` removes both ties in one transaction. Adding a person reuses the existing calls (`PUT .../allocations/:personId`, then `POST /api/teams/:teamId/members` when a team was chosen): the state between the two is valid, a collaborator without a team. The tab keeps the `/teams` route. Members open it and see people and teams; rates, margins and every action are for admins. The customer's bill rate is edited in the project settings.
**Edit team modal (Sprint 19):** The team card used to carry every action: rename, delete, a remove button per member and a select to add one. It now only shows the team, and one "Editar time" modal holds the name, the members and the deletion. The modal edits a draft, and nothing reaches the server before "Salvar", so "Cancelar" means what it says. There is no new route: saving compares the draft with what the server has and calls `PATCH /api/teams/:teamId`, then `DELETE` and `POST .../members` for whoever left and joined, one request per change. The save is therefore not atomic. If a request fails, what was already applied stays, the page behind the modal is reloaded, and the modal stays open with the error; saving again only sends what is still missing, because the difference is computed again. A single transactional route was left out because a team changes a few people at a time and every intermediate state is valid.
**Hourly rate required to join a project (Sprint 20):** The two independent ties were a requirement mistake: a person could be put in a team, and so in the project, without an hourly rate, and then could not clock in. Now the Allocation is what puts a person in a project, and the rate is mandatory at that moment. `POST /api/teams/:teamId/members` refuses a person without an Allocation in the team's project, the "Editar time" modal only lists people who are already in the project, and the "Adicionar pessoa" form requires the rate (one field per line, the rate above the optional team). `DELETE .../allocations/:personId` was removed, since deleting only the rate would leave the person in the teams without one; `DELETE .../collaborators/:personId` is the only way out. Adding a person still takes two calls, in an order that keeps every intermediate state valid: the rate first, then the team. Rows from before the rule (a team membership without an Allocation) are not deleted: `GET .../collaborators` still returns them with a null rate, and the tab warns the admin, who sets the rate or removes the person. `member_count` keeps counting them for the same reason.
**Two views and the collaborator modal (Sprint 21):** The tab showed the people table and the team cards one after the other, with the hourly rate edited in the table row and a remove button next to it. In a small project the teams add little, and in a large one the table is the slow way to find someone, so the tab now has a switch below the summary tiles: "Pessoas" (the table) or "Times" (the cards). It is a segmented control, not a second row of underlined tabs right under the project tabs, and the choice goes in the URL (`?view=teams`). No list shows more than five people at a time: the table has pages and so does each team card. The paging is done in the browser, because the tab already loads every collaborator for the tiles and the cards; the browser also sorts the names in Portuguese order, since the API sorts by bytes and would put "Íris" after "Nuno". The row and the card no longer carry fields or direct actions. Everything about one person changes in one modal, "Editar colaborador", opened from the table row or from the person inside a team card: the hourly rate, the teams as checkboxes and "Tirar do projeto" with a confirmation. Like "Editar time", it edits a draft and sends nothing before "Salvar"; saving sends the rate first (a team only accepts a person with a rate) and then one request per team left or joined, so it is not atomic and can be repeated after a failure. For admins the summary gained two money tiles: the sum of the hourly margins at today's rates (what the project earns in an hour in which everyone works; a dash without a bill rate) and what the recorded hours are worth, the sum of the closed sessions, each one's time multiplied by the pay rate stored at clock-in. A session still running only counts after clock-out, so the second tile can be below "Custo" on the time tracking tab, which follows the clock. It is summed in the browser from the project's sessions, like that tab does; `/work-sessions/total` still requires a task or a person. The tiles show only the figure and a short label: the explanation of each sum sits behind a small info button (`tip` in `app.js`), a balloon that stays inside the row and the screen. No route was added.
**Project settings as a read-only tab, weekly hours on the person (Sprint 22):** The settings tab was two forms always open, side by side, plus a delete card. It now only shows the project, like the organization's "Sobre" tab: one card with the description, the sprint, the daily and the weekly, another with the customer and, for admins, the bill rate. Admins get one "Editar" button, which opens the "Editar projeto" modal with the same fields in the same order as "Novo projeto", and the deletion with its confirmation. The modal edits a draft; saving sends `PATCH /api/projects/:projectId` and then, only when the customer or the rate changed, `PUT .../billing`. The two calls are not atomic: if the second fails the first stays, the tab behind the modal already shows it, and saving again repeats what is missing. The sprint length became a list (7 days, 14 days, one month) in both modals instead of a free number. The API still accepts 1 to 90 days, so a project that already has another length keeps it: that length shows up as one more option and saving does not change it; "one month" is stored as 30 days. Weekly hours left the project and went to the person: they are what the organization agrees with someone when hiring, whatever the projects, and a person in two projects had two of them. `persons.weekly_hours` is set by `PATCH /api/persons/:personId/weekly-hours`, an admin-only route like `/role`, because the profile `PATCH` is open to the person and the hours are not theirs to change; everyone in the organization reads it. The migration adds the column and drops `projects.weekly_hours` without copying anything, since a project's expected hours do not say what each person's are. In the organization's "Colaboradores" tab the row shows the role and the hours: the role still changes with the "Tornar admin" button on the row, and the hours in a modal opened by a pencil; the person reads their own hours on the profile. Nothing computes with the hours yet.
**Integrations tab (Sprint 23):** The tab had the creation form inline in the page and, on each card, the buttons to disable, edit and delete plus an inline edit form. It now follows the team card: the card only shows the integration (platform, status, the metadata field that identifies the connection, a masked token) with a single pencil for admins, and one modal creates and edits. The modal shows the platforms as a radio group on creation and draws the metadata fields of the chosen one from its descriptor; on edit the platform is fixed, an empty token keeps the stored one, and disabling and deleting (with a confirmation) are there too. Nothing is sent before "Salvar". The task page takes the label of the link field from the same descriptors ("Número da issue" or "Cartão"), and the task list now loads the integration of each linked item, which it did not, so the badge names the platform ("GitHub #42", "Trello H0TZyzbK").
**Project overview (Sprint 26):** An admin who wanted to know how a project was doing had to visit several tabs and add things up. "Visão geral" is a new first tab that puts it on one screen: people, teams, hours, revenue, cost and margin, how long the project has existed, its integrations, the activity of the last 7 and 30 days, overdue tasks and the hours of each person. It is the only project tab for admins alone, because nearly everything on it is money: members do not get the link, the page answers "not found" and the route answers 403, and `/projects/:projectId` now opens on it for an admin and on the tasks for everyone else. One route, `GET /api/projects/:projectId/overview`, computes everything, in a read-only `overview` domain with no table that reads the services of the other domains (the shape of `collaborator`). It was not done in the browser, like the totals of the time tracking tab, because it would take seven requests and every session of the project to show a dozen numbers, and the rules would have no test. Nothing is aggregated in the database: the totals add the amount of each session, already rounded, so they match the rows of the time tracking tab. Decisions that are not obvious: (1) the project has no start date, so its start is `created_at`, the day it was registered, and the first clock-in is shown next to it; (2) the age is computed on the server, as elapsed days and weeks and completed calendar months, each rounded down on its own; (3) an open session counts up to the instant of the answer (`generated_at`), and the screen is a snapshot with an "Atualizar" button, not a running clock; (4) "people" is the same union as the collaborators tab, and someone who left the project stays in the hours per person, flagged, so the rows still add up to the totals; (5) a task has no completion state, so "overdue" is every task past its deadline, the same count the task list calls "Atrasadas". The seed backdates each project and adds older sessions and integrations without a token, so the screen has different numbers to show.
**Alternatives considered:**
- HTMX instead of Alpine.js: also viable and build-free; Alpine.js chosen for finer-grained client state (e.g., active-session elapsed timer, inline validation feedback, multi-step forms for integration config).
- Pure server-rendered forms with full page reloads: simpler but poorer UX for clock in/out and live totals.
- Separate SPA frontend: rejected per Decision 1 (build toolchain + second deployment artifact).

### Decision 9: Schema changes — Versioned migrations
**Choice:** The database schema changes only through versioned SQL files in `internal/database/migrations`. They are embedded in the binary and applied in order at startup by goose (server and seed), one transaction per file, under an advisory lock. The files are generated from `ent/schema` by `go run ./cmd/migrate new <name>`: Ent's Atlas engine replays the existing files on a throwaway database, diffs the result against the Ent schema and writes the difference. A person reviews the file, and edits it when the change needs a rename or a backfill, before committing it. Migrations are forward-only. A database that has tables but no migration history is refused, not adopted.
**Rationale:** Before this decision the server ran Ent's auto migration at every start with column and index drops enabled: it diffed the Ent schema against the live database and applied whatever came out. A renamed field became `DROP COLUMN` + `ADD COLUMN`, and a value that moved between tables was lost — it happened when the weekly hours moved from the organization to the project. That is not acceptable now that the database stores money (the hourly rates on allocations, projects and work sessions). With versioned files, every destructive statement is written down, reviewed and committed, and a backfill sits next to the DDL that needs it. A drift test (after the migrations run on an empty database, Ent's diff must be empty) keeps `ent/schema` and the files from diverging.
**Alternatives considered:**
- Keep the auto migration without drops: stops the data loss, but renames, type changes and backfills stay impossible to express, and dead columns pile up.
- Apply migrations only through an explicit command: the right call with a deploy pipeline, but there is none; a binary that migrates itself keeps setup to one step.
- golang-migrate: up/down file pairs and a "dirty" state to clear by hand after a failed migration; goose runs each file in a transaction.
- Atlas CLI for both generating and applying: no new Go dependency, but applying would live outside the binary.
- Adopting existing databases by marking the baseline as applied: only local databases existed, and recreating them is simpler than proving an old schema matches the baseline.

### Decision 10: Internationalization — YAML catalogs, language in a cookie
**Choice:** The interface is available in Brazilian Portuguese (the default) and English. Every text a person reads lives in one YAML catalog per language (`internal/i18n/locales/<lang>.yaml`, embedded in the binary); code only references keys. The server loads them with go-i18n v2 and resolves the language of each request from the `wtt_lang` cookie, then `Accept-Language`, then the default. Templates call `{{.T "key"}}`. The browser gets the very same YAML as `window.I18N` from `GET /i18n/<lang>.js` (the content hash goes in the URL, so it is cached for a year and renewed only when the text changes) and reads it with a ~40-line `WTT.t` / Alpine `$t`, picking plural forms with `Intl.PluralRules`. The toggle is a plain link to `GET /lang/<code>?next=<page>`, which sets the cookie and redirects (the target goes through the same `safeNext` as the login).
**Rationale:** The pages are server-rendered shells with Alpine components, so text lives in two places, templates and JavaScript; a single catalog serves both and a test keeps the languages in sync. go-i18n gives CLDR plural rules, placeholders and nested YAML for free; the browser needs no library because it only looks keys up. A cookie works on the screens that have no session (login, sign-up, invitation), and a link needs no JavaScript.
**Rules:** one key per whole sentence (word order differs between languages), values through `{{.name}}` placeholders, plurals through `one`/`other` with `{{.count}}`. Inside an Alpine expression texts always come from `$t`, never from `{{.T}}`: `html/template` escapes HTML, not JavaScript, so an apostrophe in English would break the expression. Enum values stay as the backend codes; only their labels are translated. go-i18n reads `id`, `description`, `hash`, `zero`..`other`, `translation`, `leftdelim` and `rightdelim` as parts of a message, so they cannot be key names (the loader refuses the file). User-entered content and the demo seed are not translated.
**Tests:** every language has exactly the same keys, placeholders and plural forms; every key used in templates, JavaScript and Go pages exists in every language (a scan of the sources); the server picks the language by cookie, header and toggle.
**Alternatives considered:**
- Language stored on the person (a column plus a migration): follows the person across devices, but needs a schema change and does not help the screens without a session, which still need a cookie. Left out for now; the cookie can later be seeded from the profile.
- golang.org/x/text/message with `gotext` code generation: a build step and catalogs that are not YAML.
- A JavaScript i18n library (i18next and similar): a second dictionary format and a vendored dependency for what is a key lookup.
- `Accept-Language` only, without a toggle: gives people no way to choose.

### Decision 11: API errors — stable codes, the text on the client
**Choice:** The API no longer sends error messages. Every error response is `{"error": {"code": "domain.reason", "params": {...}}}` with the usual HTTP status. Codes are declared next to the domain that raises them (`apperr.New("organization.invalid_cnpj", 400)` in each `errors.go`, with the names of the parameters the text uses); services return the sentinel (`ErrX` or `ErrX.With("max", 160)`), and every handler answers through one function, `apperr.Respond(c, status, err)`. A record that does not exist (`database.ErrNotFound`) becomes `request.not_found` (404), and any error without a code becomes `internal.server_error` (500) with the detail only in the log, so no internal text leaves the server. The browser builds the message from the same YAML catalog as the rest of the interface (`errors.<code>`, with `fields.<field>` for the `field` parameter), so `ApiError.message` keeps working for every existing toast and form. The codes are documented in `_docs/error-codes.md`, generated from the registry and the catalogs by a test that fails when the file is stale.
**Rationale:** A message sent by the server fixes the language of the answer, and the API had ~230 Portuguese texts spread over services and handlers. A code is a contract another client (a CLI, a script, a mobile app) can rely on and translate by itself, and it moves all the text to the catalogs, where the languages are kept in sync by tests (same codes, same placeholders). Declaring the parameters at the code keeps the text, the API and the documentation from drifting apart.
**Alternatives considered:**
- Translate on the server from `Accept-Language`: keeps clients trivial, but every client then depends on the server's wording, the API doc cannot list stable identifiers, and the browser would have two translation paths.
- Send both the code and a message: easy, but the message would still be in one language and would tempt clients to match on it.
- One code per field for the length errors: avoids the `field` parameter but multiplies codes that only differ by a noun.

## Risks / Trade-offs

- **[Per-integration API rate limits]** On-demand external item fetches consume API quota per platform. → Mitigation: cache item details in-process with a short TTL (e.g., 60s); document rate-limit handling per integration type.
- **[Concurrent clock-in race]** Two near-simultaneous clock-in requests for the same person. → Mitigation: partial unique index is the hard guarantee; service returns a clear "already clocked in" error.
- **[Credential compromise]** Encrypted-at-rest integration tokens are only as safe as the encryption key. → Mitigation: key from env var / secrets manager, not committed; document rotation procedure; each integration stores only necessary permissions.
- **[Integration API drift]** External platforms may change their APIs, breaking item detail fetching. → Mitigation: integration interface allows per-type implementation; graceful degradation (null detail block) ensures task reads never fail.
- **[On-demand fetch latency]** Reading a task with a linked external item adds an API round-trip. → Mitigation: in-process TTL cache and graceful null-on-failure so task reads never block indefinitely.
- **[Schema and migrations drifting apart]** `ent/schema` is edited by hand and the migration files are generated from it, so one can change without the other. → Mitigation: a test applies the migrations and fails if Ent's diff against the result is not empty, or if the set of tables differs.
- **[Migrations that only fail on real data]** The test suite migrates an empty database, so a migration that breaks on existing rows (a new NOT NULL column, a unique index over duplicates) passes the tests. → Mitigation: the generator warns about these changes and every file is reviewed; each migration runs in a transaction, so a failure leaves the database at the previous version and the server refuses to start.
- **[No build-time checks on frontend logic]** Alpine.js behavior lives in HTML attributes and inline scripts, so client-side errors only surface at runtime. → Mitigation: keep Alpine components small; rely on server-side validation as the source of truth; cover served routes with integration tests.
