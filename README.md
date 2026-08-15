# ops-platform-starter

![-Go-](https://img.shields.io/badge/-Go-00ADD8?logo=go&logoColor=white&style=for-the-badge)
![-PostgreSQL-](https://img.shields.io/badge/-PostgreSQL-336791?logo=postgresql&logoColor=white&style=for-the-badge)
![-JWT-](https://img.shields.io/badge/-JWT-000000?logo=jsonwebtokens&logoColor=white&style=for-the-badge)
![-Docker-](https://img.shields.io/badge/-Docker-2496ED?logo=docker&logoColor=white&style=for-the-badge)
[![-CI-](https://img.shields.io/github/actions/workflow/status/CynthiaWahome/ops-platform-starter/ci.yml?branch=dev&style=for-the-badge&label=CI)](https://github.com/CynthiaWahome/ops-platform-starter/actions)
[![-MIT_License-](https://img.shields.io/badge/-MIT_License-yellow?style=for-the-badge)](LICENSE)

**A production-grade backend for work order management, field service dispatch, and task assignment software — the slice of an enterprise ERP a small operations team actually uses, without the rest.**

Go. REST/JSON. Real role-based access control, a real status-transition engine, real Postgres persistence — bring your own frontend, in whatever stack you already run.

## Table of Contents

- [Why this exists](#why-this-exists)
- [Who this is for](#who-this-is-for)
- [What it does](#what-it-does)
- [Roles](#roles)
- [Tech stack](#tech-stack)
- [Getting started](#getting-started)
- [Repository structure](#repository-structure)
- [Contributing](#contributing)
- [License](#license)

## Why this exists

Most small and mid-sized operations businesses — dispatch services, field task tracking, provider and worker coordination — get forced into a bad tradeoff: build a bespoke internal tool from scratch, or adopt something the size of SAP to get one workflow they actually need.

That workflow repeats across almost every ops business:

```text
work item created
-> assigned to a worker/provider
-> assignee updates progress and uploads evidence
-> internal reviewer verifies or flags it
-> work item closes
```

This is that workflow, built once, built properly, and reusable — not a finance module, not an HR suite, not a CRM bolted on for good measure. Those are legitimately separate systems. Bundling them is exactly how ops software ends up bloated and expensive for a business that only ever needed the one core loop.

## Who this is for

- Small service dispatch companies — home services, repairs, field visits
- Field task or verification-heavy operations — construction site inspections, delivery confirmation, compliance checks
- Internal ops teams that need real work-assignment and sign-off without buying an enterprise platform to get it
- Teams building this exact pattern who don't want to rebuild auth, roles, and a status engine from zero

## What it does

- **Full work item lifecycle** — created → assigned → accepted/declined → in progress → submitted for review → verified/flagged → completed, enforced by a real status-transition state machine, not a free-text field
- **Role-scoped visibility and permissions**, enforced server-side (see [Roles](#roles))
- **Teams** — assignees belong to exactly one team; supervisors run their own team's day-to-day work without every action routing through a single admin account
- **Evidence upload and verification** — attachments tied to a work item, required before it can move to review
- **Assignment and status audit trails** — every reassignment and every status change is a permanent, queryable record
- **In-app notifications** for the events people actually wait on
- **Real persistence, optional** — every store has a Postgres implementation *and* an in-memory one. Run with zero setup, or set `DATABASE_URL` for a real, restart-surviving database

## Roles

Four roles, each a genuinely different view over the same data:

| Role | Can see | Can do |
| --- | --- | --- |
| **Admin** | Everything, every team | Everything, everywhere — the permanent fallback |
| **Supervisor** | Their own team's work | Create, assign, reassign, verify, flag, and complete work — scoped to their team |
| **Assignee / worker** | Only their own assigned work | Accept, decline, work it, submit evidence |
| **Requester** | Only what they created | Create a request, track its status — nothing else |

## Tech stack

- **Go**, standard library `net/http` — no framework. Minimal dependency surface, predictable request handling, nothing hidden between the code and the wire.
- **PostgreSQL**, optional. Every store also has an in-memory implementation, which is the zero-setup default.
- **JWT** authentication, bcrypt password hashing.
- **REST/JSON only** — no frontend shipped in this repo. Any language or framework can consume the API directly.

## Getting started

```bash
git clone git@github.com:CynthiaWahome/ops-platform-starter.git
cd ops-platform-starter
cp .env.example .env
go mod tidy
go run ./cmd/api
```

Runs entirely in memory by default — no database required, data resets on restart.

To run against real Postgres instead:

```bash
docker compose up -d
DATABASE_URL="postgres://postgres:postgres@localhost:5432/ops_platform?sslmode=disable" go run ./cmd/api
```

Setting `DATABASE_URL` is what opts a run into real, restart-surviving persistence — the schema migrates itself on startup. `.env.example` documents every other configurable value (bootstrap users for each of the four roles, JWT secret, etc.).

## Repository structure

```text
ops-platform-starter/
├── cmd/api/           # entrypoint
├── internal/
│   ├── auth/          # JWT, roles, bootstrap users
│   ├── workitems/      # the core lifecycle + state machine
│   ├── teams/          # team membership and supervision scoping
│   ├── notifications/
│   ├── attachments/    # evidence upload
│   ├── db/              # migrations, Postgres wiring, transactions
│   └── http/            # handlers, router, middleware
└── docker-compose.yml   # local Postgres, one command
```

## Contributing

1. Fork the repo and create a branch off `dev` (`feat/your-feature`, `fix/your-fix`).
2. Write tests for anything you change — CI runs the full suite (in-memory and against a real Postgres service container) on every PR and fails closed.
3. Commit using [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`, `docs:`, etc.).
4. Open a PR against `dev` using the pull request template — link the issue it closes, describe how it was tested.

## License

[MIT](LICENSE) — free to use, fork, and adapt for your own operations product.
