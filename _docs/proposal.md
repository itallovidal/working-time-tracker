## Why

There is no structured way to track how much time is spent on each task across the organization. Teams need to know when a person starts and stops working on a given task, which external items (issues, cards, tickets) that task relates to, and whether the task is on track to meet its deadline. A clock-in/clock-out system tied to tasks, projects, teams, and generic integrations to external platforms gives accurate time data and keeps engineering work visible across the organization.

## What Changes

- Introduce **time tracking** with clock-in and clock-out actions that record start and end timestamps for a work session on a specific task within a project.
- Introduce **task management** where each task has a unique id, name, description, an assigned person, an optional deadline (defaults to one week from creation), and belongs to a project.
- Introduce **organization and project structure**: an organization (company) has multiple projects; each project has its own teams, tasks, configurations (sprint duration, daily time, weekly sync), and integrations.
- Introduce **teams** within a project to group people working together; a person can belong to multiple teams.
- Introduce a generic **integrations** system supporting multiple external platforms (GitHub, GitLab, Slack, Trello, etc.) instead of a single GitHub connection. Integrations are configured per project and provide external item linking for tasks.
- Introduce **project-level configuration** for sprint duration, daily standup time, and weekly sync.
- A work session (time entry) is always tied to a task, which belongs to a project and optionally links to external items through integrations.

## Capabilities

### New Capabilities
- `time-tracking`: Clock in and clock out actions that capture work-session start/end timestamps and associate each session with a task and a person. Scoped within a project. Supports viewing total time worked per task and per person.
- `task-management`: Create, read, update, and delete tasks within a project. Each task has an id, name, description, an assigned person, a deadline (default one week from creation), and optional external item links through integrations.
- `integrations`: Configure and manage integrations with external platforms (GitHub, GitLab, Slack, Trello, etc.) per project. Link tasks to external items (issues, cards, tickets) and retrieve their details. Replaces the previous GitHub-specific integration.
- `org-project-team-management`: Create and manage organizations, projects, and teams. Person registration within an organization, team membership, and project-scoped configuration (sprint duration, daily time, weekly sync).

### Modified Capabilities
<!-- No existing specs to modify - this is a greenfield project. -->

## Impact

- **New data models**: Organization, Project, Team, Person (now org-scoped), Task (project-scoped), WorkSession, Integration, TeamMembership.
- **New API surface**: Endpoints for organization/project/team CRUD, integration configuration, plus all task/time endpoints scoped to projects. Existing GitHub-specific endpoints are replaced by generic integration endpoints.
- **External dependencies**: Integration-specific third-party APIs (GitHub API, GitLab API, Slack API, Trello API, etc.) depending on configured integrations.
- **New web UI**: A server-rendered frontend (Alpine.js) served by the Go backend for organization/project/team management, clock in/out, task management, and integration configuration.
- **Greenfield project**: No existing code or specs are affected; all layers (data model, services, handlers, REST API, and a served web UI) are built from scratch.
