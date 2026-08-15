# ops-platform-starter

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.9-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![Project Board](https://img.shields.io/badge/GitHub_Project-Active-6e40c9?logo=github&logoColor=white)](https://github.com/users/CynthiaWahome/projects/7)
[![Issues](https://img.shields.io/github/issues/CynthiaWahome/ops-platform-starter)](https://github.com/CynthiaWahome/ops-platform-starter/issues)

Reusable starter architecture for role-based operations software built with Go on the backend and Next.js on the frontend.

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

### Backend

- Go
- standard module structure under `backend/`

### Frontend

- Next.js
- TypeScript
- Tailwind CSS
- App Router structure under `frontend/`

## Repository structure

```text
ops-platform-starter/
├── backend/   # Go API
└── frontend/  # Next.js application
```

This scaffold is intentionally thin right now. The real domain structure will be added incrementally as the starter matures.

## Current status

Current branch state:

- repo initialized
- Go module scaffold started
- Next.js frontend scaffolded
- PR template added
- GitHub milestones, issues, and project board created

What is not built yet:

- auth flow
- work item model
- assignment flow
- status engine
- evidence upload loop
- verification workflow

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

### Frontend

```bash
cd frontend
npm install
npm run dev
```

### Backend

```bash
cd backend
go mod tidy
go run ./cmd/api
```

By default this runs entirely in memory — no database required, data resets
every restart. That's deliberate: every test, and every first-time `go run`,
should work with zero setup.

To run against real Postgres instead (OPS-048):

```bash
cd backend
docker compose up -d
DATABASE_URL="postgres://postgres:postgres@localhost:5432/ops_platform?sslmode=disable" go run ./cmd/api
```

Setting `DATABASE_URL` is what opts a run into real, restart-surviving
persistence — the schema is migrated automatically on startup (see
`internal/db`). See `.env.example` for every other configurable value
(bootstrap users, JWT secret, etc.).

## Development approach

This repository will be built in thin vertical slices rather than broad abstract layers.

The first meaningful slice is:

```text
admin creates or reviews a work item
-> admin assigns it
-> assignee accepts it
-> assignee uploads evidence
-> admin verifies or flags it
```

That slice is the first proof that the starter is actually reusable.

## Next steps

The immediate next engineering steps are:

1. lock the backend foundation shape in Go
2. add base API structure under `backend/`
3. establish frontend app shell structure beyond scaffold defaults
4. implement the first auth and role-aware workflow slice

## Notes

Planning docs exist locally during active architecture work and are not necessarily intended to be pushed with every scaffold iteration.
