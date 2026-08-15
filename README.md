# ops-platform-starter

![-Go-](https://img.shields.io/badge/-Go-00ADD8?logo=go&logoColor=white&style=for-the-badge)
![-PostgreSQL-](https://img.shields.io/badge/-PostgreSQL-336791?logo=postgresql&logoColor=white&style=for-the-badge)
![-JWT-](https://img.shields.io/badge/-JWT-000000?logo=jsonwebtokens&logoColor=white&style=for-the-badge)
![-Docker-](https://img.shields.io/badge/-Docker-2496ED?logo=docker&logoColor=white&style=for-the-badge)
[![-GitHub_Project-](https://img.shields.io/badge/-GitHub_Project-6e40c9?logo=github&logoColor=white&style=for-the-badge)](https://github.com/users/CynthiaWahome/projects/7)
[![-Issues-](https://img.shields.io/github/issues/CynthiaWahome/ops-platform-starter?style=for-the-badge&label=Issues&color=orange)](https://github.com/CynthiaWahome/ops-platform-starter/issues)
[![-MIT_License-](https://img.shields.io/badge/-MIT_License-yellow?style=for-the-badge)](LICENSE)

**A lightweight, self-hostable backbone for work order management, field service dispatch, and task assignment software — the slice of an enterprise ERP a small operations team actually uses, without the rest.**

Open source, Go, REST/JSON. Bring your own frontend, your own database (or none — it runs entirely in memory out of the box).

## Table of Contents

- [Why this exists](#why-this-exists)
- [Who this is for](#who-this-is-for)
- [What it does](#what-it-does)
- [Roles](#roles)
- [Tech stack](#tech-stack)
- [Getting started](#getting-started)
- [Repository structure](#repository-structure)
- [Frontend](#frontend)
- [Planning and backlog](#planning-and-backlog)
- [Contributing](#contributing)
- [License](#license)

## Why this exists

Most small and mid-sized operations businesses — dispatch services, field task tracking, provider/worker coordination — end up choosing between two bad options: build a bespoke internal tool from scratch every time, or adopt something the size of SAP or a full ERP suite to get one workflow they actually need.

That workflow repeats across almost every ops business:

```text
work item created
-> assigned to a worker/provider
-> assignee updates progress and uploads evidence
-> internal reviewer verifies or flags it
-> work item closes
```

This project is that one workflow, built properly, reusable, and nothing else bolted on. Not a finance module, not an HR suite, not a CRM — those are legitimately separate systems, and pretending otherwise is exactly how ops software becomes bloated and expensive. If you need the full SAP-style suite, this isn't trying to replace it. If you need *this one piece* of it, done well, this is that piece.

## Who this is for

- Small service dispatch companies (home services, repairs, field visits)
- Field task or verification-heavy operations — construction site inspections, delivery confirmation, compliance checks
- Internal ops teams that need real work-assignment and sign-off, without buying an enterprise platform to get it
- Anyone prototyping this exact pattern who doesn't want to rebuild auth, roles, and a status engine from zero

## What it does

- **Full work item lifecycle**: created → assigned → accepted/declined → in progress → submitted for review → verified/flagged → completed, enforced by a real status-transition state machine, not just a free-text field
- **Role-scoped visibility and permissions**, enforced at the backend (see [Roles](#roles))
- **Teams**: assignees belong to exactly one team; supervisors run their own team's day-to-day work without every action routing through a single admin account
- **Evidence upload and verification** — attachments tied to a work item, required before it can move to review
- **Assignment and status audit trails** — every reassignment and every status change is a permanent, queryable record, not overwritten in place
- **In-app notifications** for the events people actually wait on (assigned, evidence submitted, flagged, verified, completed)
- **Real persistence, optional** — every store has a Postgres implementation *and* an in-memory one; run with zero setup, or set `DATABASE_URL` for a real, restart-surviving database

## Roles

Four roles, each a genuinely different view over the same data — not three copies of the same screen with different labels:

| Role | Can see | Can do |
| --- | --- | --- |
| **Admin** | Everything, every team | Everything, everywhere — the permanent fallback, never locked out of anything a supervisor can do |
| **Supervisor** | Their own team's work | Create, assign, reassign, verify, flag, and complete work — scoped to their team only |
| **Assignee / worker** | Only their own assigned work | Accept, decline, work it, submit evidence |
| **Requester** | Only what they created | Create a request, track its status — nothing else |

Full detail (visibility matrix, action matrix, status-transition permissions) lives in the planning pack — see [Planning and backlog](#planning-and-backlog).

## Tech stack

- **Go**, standard library `net/http` — no framework. Deliberate: this project locks the backend's shape and learns the stdlib primitives before reaching for something that hides them. Swapping to a framework later costs one file (`internal/http/router`), since nothing else depends on how routing works.
- **PostgreSQL**, optional. Every store also has an in-memory implementation, which is the zero-setup default — every test, and every first `go run`, works without a database.
- **JWT** authentication, bcrypt password hashing.
- **No frontend in this repo**, by design — plain REST/JSON, so any language or framework can consume it. See [Frontend](#frontend).

## Getting started

```bash
go mod tidy
go run ./cmd/api
```

Runs entirely in memory by default — no database required, data resets on restart.

To run against real Postgres instead:

```bash
docker compose up -d
DATABASE_URL="postgres://postgres:postgres@localhost:5432/ops_platform?sslmode=disable" go run ./cmd/api
```

Setting `DATABASE_URL` is what opts a run into real, restart-surviving persistence — the schema migrates itself on startup. See `.env.example` for every other configurable value (bootstrap users for each of the four roles, JWT secret, etc.).

## Repository structure

```text
ops-platform-starter/
├── cmd/api/         # entrypoint
├── internal/
│   ├── auth/        # JWT, roles, bootstrap users
│   ├── workitems/    # the core lifecycle + state machine
│   ├── teams/        # team membership and supervision scoping
│   ├── notifications/
│   ├── attachments/  # evidence upload
│   ├── db/           # migrations, Postgres wiring, transactions
│   └── http/         # handlers, router, middleware
└── docker-compose.yml # local Postgres, one command
```

## Frontend

None in this repo, on purpose — see [Why this exists](#why-this-exists). A reference Next.js implementation lives in a separate repo: [ops-platform-starter-frontend](https://github.com/CynthiaWahome/ops-platform-starter-frontend).

## Planning and backlog

The full implementation backlog, permission matrix, domain model, and design rationale are tracked in GitHub:

- Project board: [`Ops Platform Starter`](https://github.com/users/CynthiaWahome/projects/7)
- Issues: [`OPS-001` onward](https://github.com/CynthiaWahome/ops-platform-starter/issues)

Grouped into milestones:

1. `M0 - Design Lock`
2. `M1 - Skeleton and Foundations`
3. `M2 - Work Item and Assignment Core`
4. `M3 - Evidence and Verification Loop`
5. `M4 - Hardening`
6. `M5 - Mapping and Adoption`

## Contributing

Issues and pull requests are welcome. This project is built in thin, fully-tested vertical slices — each PR is expected to include real tests and, where it touches behavior, manual verification against a running server, not just passing CI.

## License

[MIT](LICENSE) — free to use, fork, and adapt for your own operations product.
