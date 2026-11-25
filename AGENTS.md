# AGENTS.md

SPA rebuild of [go-classic-mpa-sample](https://github.com/podhmo/go-classic-mpa-sample)
("zatudan"). The Go API server (`/api/v1`, issues scope) is ported
as-is; the frontend is a fresh SPA. See `README.md` for layout.

## Docs

- `docs/ja/features.md` — full feature spec/acceptance checklist of the
  source app; the porting target reference.
- `docs/ja/ui-rules.md` — UI conventions consolidated (SPA adaptation of the
  MPA's rules). Read before touching UI; new UI work must follow it.
- `docs/ja/libraries.md` — library usage table + applied modern-web-guidance
  patterns.
- `docs/sketch/ja/` — decision notes: `environment.md` (setup),
  `dev-workflow.md` (component-first + verification loop),
  `css.md`, `tanstack-query.md` (evaluation: rejected for now).
- `TODO.md` — task tracker; follow `.agents/skills/todo-md/SKILL.md` for
  add/complete/GC rules.

## Skills

- `.agents/skills/modern-web-guidance/` — run
  `npx -y modern-web-guidance@latest search "<query>"` before touching
  HTML/CSS/client UI; applied patterns are recorded in
  `docs/ja/libraries.md`.
- `.agents/skills/playwright-cli/` — drive a real browser against the
  running app for E2E verification.
- `.agents/skills/chrome-devtools-cli/` — DevTools-level inspection:
  performance traces, insights, console/network dumps.
- `.agents/skills/todo-md/` — TODO.md upkeep rules.
- `devin mcp` configured: context7 — **use proactively** whenever unsure
  about a library/framework API (`resolve-library-id` → `query-docs`,
  one topic per query; see `docs/ja/libraries.md` for query tips).

## Commands

```sh
# Go API
go run ./cmd/zatudan -seed -api-token dev-token -db /tmp/spa-zatudan.db  # :8080
go build ./... && go vet ./... && go test ./... && gofmt -l .

# SPA (in web/)
npm run dev           # Vite dev server :5173, /api proxied → :8080 (injects dev-token)
npm run build         # tsc -b && vite build
npm test              # vitest browser mode (system Chrome)
npm run lint          # oxlint
npm run format        # oxfmt — run after every edit (CI checks format:check)
```

CI (`.github/workflows/ci.yml`) runs both stacks on push/PR.

## Conventions

- React local state: **`useReducer` only — `useState` is banned.**
- Server data: route loaders + `router.invalidate()` after mutations
  (TanStack Query evaluated and deferred — see `docs/sketch/ja/tanstack-query.md`).
- Pending UI via `useTransition`; form errors via `useReducer`.
- Presentational components take props + callbacks; routes wire loaders
  and mutations. Verify components in vitest browser-mode tests *before*
  connecting to the real server, then confirm with playwright-cli.
- UI follows modern-web-guidance: native `<dialog>` for confirmations,
  `:user-invalid` + `aria-invalid` sync (`src/a11y.ts`), `<search>` for
  filter forms, `.visually-hidden` labels, `data-error` field errors.
- Styling: plain CSS in `src/index.css` (+ CSS Modules as needed);
  Panda CSS is a future option (`docs/sketch/ja/css.md`).
- MPA-era `X-Partial` fragments are NOT ported — data flows as JSON.
  Live updates ARE: the server streams hint-only SSE on `/api/v1/events`
  (fed by the changes table, so local-CLI writes in other processes are
  covered) and the client just runs `router.invalidate()` per hint
  (`web/src/live-updates.ts`).
- Commits: English, granular (commit after each coherent change).
