# go-spa-sample

An experiment in rebuilding [go-classic-mpa-sample](https://github.com/podhmo/go-classic-mpa-sample)
("zatudan" — an issue tracker + discussions + git-backed pull requests)
as a **single-page application**.

## Context

The source project is a deliberately *classic* web app: server-rendered
HTML, plain HTML forms, and POST → Redirect → GET everywhere, backed by
SQLite — plus a `/api/v1` JSON surface that mirrors the web app.

The far goal here:

1. Copy the API-server side of go-classic-mpa-sample (SQLite store +
   `internal/api`-style JSON handlers) into this repo.
2. Re-implement the whole frontend as an SPA on top of that API.

Before any porting, this repo first establishes the *toolchain*:

- **Go** — the API server (`net/http`, `http.ServeMux`).
- **Vite + React + TypeScript** — the SPA, developed against the Go
  server through a dev-server proxy (same-origin `/api`, no CORS).
- **TanStack Router** — routing/data loading; `useReducer` for local
  state (`useState` is banned by convention).
- **Vitest** — unit tests.
- **playwright-cli** — browser-level verification.
- **context7 MCP** / **modern-web-guidance** — docs lookup for the agent
  (setup notes: `docs/sketch/ja/environment.md`).

## Current state: connectivity spike

A throwaway prototype to prove the Go ↔ SPA path works end to end.
The fake API is intentionally *unrelated* to zatudan — its signatures
are ad-hoc and will be discarded.

```sh
go run ./cmd/fakeapi            # fake API on 127.0.0.1:8080
npm --prefix web run dev        # Vite dev server on :5173, /api proxied to :8080
```
