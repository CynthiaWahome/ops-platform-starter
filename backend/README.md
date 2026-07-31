# Backend

This backend is intentionally starting small.

The goal of the first backend pass is not to add a framework and a pile of dependencies.
The goal is to lock clean boundaries first:

- `cmd/` for entrypoints
- `internal/config` for environment-driven configuration
- `internal/http` for handlers and routing
- `internal/server` for HTTP server setup and lifecycle

## Layout

```text
backend/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── config/
│   ├── http/
│   │   ├── handlers/
│   │   └── router/
│   └── server/
└── .env.example
```

## Why this shape

This keeps transport, configuration, and startup concerns separated early.

That matters because the repository is meant to become a reusable starter.

If everything starts in one `main.go`, the first slices may feel fast, but the codebase becomes harder to extend once auth, permissions, assignments, and status transitions arrive.

## Current state

Current implementation includes:

- environment-backed config
- HTTP server bootstrap
- one health endpoint

The health endpoint exists only to prove the wiring:

```text
config
-> router
-> handlers
-> server
```

## Next backend step

After this foundation, the next meaningful work should be:

1. auth foundation
2. role resolution
3. work item module shell
4. assignment flow
