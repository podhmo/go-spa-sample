# TODO — go-spa-sample

> **Note on updating this file:** upkeep rules live in
> [`.agents/skills/todo-md/SKILL.md`](.agents/skills/todo-md/SKILL.md) —
> follow them whenever you add or complete a task. Short version: this file
> lists actionable work only (`[ ]` / `[-]`); mark finished work `[x]` — a
> periodic GC sweep appends such entries to `TODO-archive.md`.

ゴール: [go-classic-mpa-sample](https://github.com/podhmo/go-classic-mpa-sample)
(古典的な POST→redirect→GET の MPA) の API サーバー部分を移植し、
フロントエンドを SPA として作り直す (see `idea.md`, `README.md`)。

進行の流れ:

1. **環境セットアップ** (done): context7 MCP / `.agents/skills/` (modern-web-guidance,
   playwright-cli, todo-md) / `@playwright/cli`。詳細は `docs/sketch/ja/environment.md`
2. **開発前準備** ← いまここ
3. **開発開始**

## To Be Implemented

### 開発前準備

「AI がブラウザで動作確認しながらコードを作成する」ワークフローを確立する。
完了条件: idea.md のツールスタック (Vite + React + TS + TanStack Router +
useReducer / Vitest / playwright-cli) を使った試行錯誤が完全に行える状態。

- [x] **Go API ↔ SPA 疎通スパイク**: zatudan とは無関係の独自 API と最小 SPA
  で通信を確立するプロトタイプ (signature はテキトーでよい)
  - [x] **fake Go API server**: `cmd/fakeapi` (`GET /api/ping`, `GET/POST /api/counter*`)。
    動作確認済み・コードは未コミット
  - [x] **SPA scaffold**: `web/` (Vite react-ts) + TanStack Router (code-based
    routing) + vite proxy `/api` → `127.0.0.1:8080`
  - [x] **playwright-cli での動作確認**: `/` で `/api/ping` の表示、
    `/counter` で POST→invalidate の反映をブラウザで確認
    (`docs/sketch/ja/spike-counter.png`)
- [x] **Vitest 導入**: `web/` に vitest + jsdom + testing-library、
  `npm test` で smoke test 通過
- [ ] **ライブラリ用法の整理**: modern-web-guidance の使い方、
  Async React (Suspense/`use`/transitions)、React 本体、TanStack
  Router/Query/Form あたりのドキュメントを context7 で収集し
  `docs/sketch/ja/` にまとめる
- [ ] **context7 OAuth login**: `devin mcp login context7` をブラウザで実行
  (環境セットアップの残件・ユーザー操作)

### 開発 (far goal)

- [ ] **zatudan API サーバーの移植 (issues のみ)**: go-classic-mpa-sample の
  SQLite store + `/api/v1` 相当ハンドラをこのリポジトリへコピー。
  まず issue tracker の機能だけから始める (discussions/pulls は後回し)
- [ ] **SPA 実装 (issues)**: TanStack Router + React。ローカル状態は useReducer
  (useState 禁止)。サーバー状態の扱い (loader のみ vs TanStack Query) は要決定
