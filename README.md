# ops-platform-starter

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Project Board](https://img.shields.io/badge/GitHub_Project-Active-6e40c9?logo=github&logoColor=white)](https://github.com/users/CynthiaWahome/projects/7)
[![Issues](https://img.shields.io/github/issues/CynthiaWahome/ops-platform-starter)](https://github.com/CynthiaWahome/ops-platform-starter/issues)

Reusable backend for role-based operations software, built in Go. REST/JSON — bring your own frontend. (Reference frontend: [ops-platform-starter-frontend](https://github.com/CynthiaWahome/ops-platform-starter-frontend), private.)

> **Note:** this README covers the current state honestly, but it's a working document — a full rewrite is planned as the last task in this project (see `OPS_PLATFORM_STARTER_BUILD_TICKETS.md`, OPS-049), once the repo's final shape is settled.

## Overview

This repository exists to capture the recurring technical core behind projects such as:

- service dispatch systems
- field task tracking tools
- assignment and verification workflows
- provider or worker operations dashboards

The intent is not to build a giant generic SaaS.

The intent is to build a strong internal starter that can be adapted quickly to multiple client or internal operations products.

## What this starter is meant to solve

Most operations products repeat the same backbone:

```text
work item created
-> assigned to a worker/provider
-> assignee updates progress
-> evidence is uploaded
-> internal reviewer verifies or flags
-> work item closes
```

This starter is being shaped around that workflow.

## Initial product goals

The first reusable implementation target is a shared core for:

- authentication
- roles and permissions
- work item lifecycle
- assignment and reassignment
- status tracking
- evidence uploads
- verification flows
- reusable dashboard shells

## Tech stack

- Go, standard `net/http` (no framework — deliberate, see `notes/`)
- PostgreSQL, optional — every store also has an in-memory implementation, which is the zero-setup default
- Frontend: none in this repo, by design. Plain REST/JSON, so any language/framework can consume it. See [ops-platform-starter-frontend](https://github.com/CynthiaWahome/ops-platform-starter-frontend) for a reference Next.js implementation.

## Repository structure

```text
ops-platform-starter/
├── cmd/api/         # entrypoint
├── internal/        # config, http, workitems, teams, notifications, attachments, db
└── docker-compose.yml
```

## Current status

- Auth (JWT, 4 bootstrap roles: admin/supervisor/assignee/requester), full work item lifecycle (create → assign → accept/decline → in progress → submitted for review → verify/flag → completed), teams with scoped supervisor authority, notifications, attachments, and real Postgres persistence (optional, in-memory by default) are all built and tested.
- No frontend in this repo (see above) — that's the immediate next phase, tracked in the separate frontend repo.

## Planning and backlog

The implementation backlog is already defined in GitHub:

- Project board: [`Ops Platform Starter`](https://github.com/users/CynthiaWahome/projects/7)
- Issues: [`OPS-001` to `OPS-052`](https://github.com/CynthiaWahome/ops-platform-starter/issues)

The issues are grouped into these milestones:

1. `M0 - Design Lock`
2. `M1 - Skeleton and Foundations`
3. `M2 - Work Item and Assignment Core`
4. `M3 - Evidence and Verification Loop`
5. `M4 - Hardening`
6. `M5 - Mapping and Adoption`

## Getting started

```bash
go mod tidy
go run ./cmd/api
```

By default this runs entirely in memory — no database required, data resets
every restart. That's deliberate: every test, and every first-time `go run`,
should work with zero setup.

To run against real Postgres instead (OPS-048):

```bash
docker compose up -d
DATABASE_URL="postgres://postgres:postgres@localhost:5432/ops_platform?sslmode=disable" go run ./cmd/api
```

Setting `DATABASE_URL` is what opts a run into real, restart-surviving
persistence — the schema is migrated automatically on startup (see
`internal/db`). See `.env.example` for every other configurable value
(bootstrap users, JWT secret, etc.).

## Development approach

Built in thin vertical slices rather than broad abstract layers — each ticket is a real, tested, manually-verified feature, not a stub. The core workflow loop:

```text
work item created
-> assigned to a worker/provider (by admin or their team's supervisor)
-> assignee accepts, works it, uploads evidence
-> admin or the team's supervisor verifies or flags it
-> work item closes
```

## Next steps

- Frontend (see [ops-platform-starter-frontend](https://github.com/CynthiaWahome/ops-platform-starter-frontend))
- `requester` role signup flow (self-service account creation)
- README rewrite as the last task in this project (OPS-049)

## Notes

Planning docs exist locally during active architecture work and are not necessarily intended to be pushed with every scaffold iteration.
