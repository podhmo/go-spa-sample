# go-spa-sample

An experiment in rebuilding [go-classic-mpa-sample](https://github.com/podhmo/go-classic-mpa-sample)
("zatudan" — an issue tracker + discussions + git-backed pull requests)
as a **single-page application**.

The source project is a deliberately *classic* web app: server-rendered
HTML, plain HTML forms, and POST → Redirect → GET everywhere, backed by
SQLite — plus a `/api/v1` JSON surface that mirrors the web app.

This repo ports the API-server side (SQLite store + `/api/v1` JSON
handlers) and re-implements the frontend as an SPA on top of it.
Feature parity covers workspaces, issues, discussions, git-backed pull
requests, cookie-session auth (`/login` + `/register`), the `zatu` CLI
(local SQLite and remote backends), and en/ja i18n. MPA-era SSE live
updates ARE ported as a hint-only stream (`/api/v1/events` fed by the
changes table — including writes made by the CLI against the DB
directly); the SPA subscribes and re-fetches through `router.invalidate()`.
Only the `X-Partial` fragment swap is dropped — data flows as JSON.

## Layout

```
cmd/zatudan/    API server (-seed, -db, -api-token, -repo flags)
cmd/zatu/       gh-style CLI — local (SQLite) and remote (/api/v1) backends
internal/       store / api / auth / events / export (ported from the MPA)
web/            the SPA — Vite + React 19 + TS 7 (@next) + TanStack Router
docs/ja/        porting reference (full feature spec of the source app)
docs/sketch/ja/ decision notes: environment, dev-workflow, libraries, css
```

## Develop

```sh
# terminal 1 — API on 127.0.0.1:8080 (dev token injected by the vite proxy)
go run ./cmd/zatudan -seed -api-token dev-token -db /tmp/spa-zatudan.db

# terminal 2 — SPA on :5173, /api proxied to :8080
npm --prefix web run dev
```

## Verify

```sh
# web (in web/)
npm run build   # tsc -b + vite build
npm test        # vitest browser mode (system Chrome, real DOM)
npm run lint    # oxlint

# go
go build ./... && go vet ./... && go test ./...
```

For full-browser checks against the running app, use the `playwright-cli`
skill (`npx playwright-cli ...`) — that's the agent's "hands" for E2E.
Component-level visual checks happen inside vitest browser-mode tests
instead of Storybook. See `docs/sketch/ja/dev-workflow.md`.

## Conventions

- Local React state: `useReducer` only — `useState` is banned.
- Server data: Router loaders + `router.invalidate()` on mutation.
- UI code follows modern-web-guidance (native `<dialog>`, `:user-invalid`
  + aria sync, `<search>`) — applied patterns in `docs/ja/libraries.md`.
- Presentational components take props + callbacks; routes wire loaders
  and mutations. Verify components in browser-mode tests *before*
  connecting them to the real server.
- Task tracking lives in `TODO.md` (see `.agents/skills/todo-md/`).
