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
2. **開発前準備** (done)
3. **開発開始** ← いまここ

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
- [x] **Vitest 導入**: `web/` に vitest を入れ `npm test` が通る。
  **browser mode 採用** (jsdomではなく実Chrome、playwright provider +
  `channel: 'chrome'` でDL不要) — componentの見た目確認はこれを主軸にし
  Storybookは見送り (詳細: `docs/sketch/ja/libraries.md`)
- [x] **ライブラリ用法の整理**: context7 で TanStack Router/Query/Form、
  React (Suspense/`use`/`useTransition`) のdocsを収集し
  `docs/sketch/ja/libraries.md` に整理。使い方は `dev-workflow.md` にも
- [x] **context7 OAuth login**: `devin mcp login context7` 完了
  (2026-11-24)

### 開発 (far goal)

- [x] **zatudan API サーバーの移植 (issues のみ)**: `cmd/zatudan` +
  `internal/{store,auth,events,export,api}` を移植。MPA向け機能
  (SSE live updates, `X-Partial` fragments) は移植せず JSON API のみ
- [x] **SPA 実装 (issues)**: 一覧/詳細/作成/コメント/状態変更/削除を実装・
  playwright-cli で実サーバー疎通確認済み。modern-web-guidance を適用
  (ネイティブ `<dialog>` の削除確認、`:user-invalid` + aria 同期、`<search>`)
- [ ] **discussions/pulls の移植判断**: store には残存。機能 spec は
  `docs/ja/features.md` 参照
- [ ] **`cmd/fakeapi` の撤去**: スパイク用コードの整理
- [x] **サーバー状態管理の再検討**: `sketch/tanstack-query` で評価実装済み。
  判定は「現状は loader 継続」(再検討条件は `docs/sketch/ja/tanstack-query.md`)
- [ ] **i18n**: UI 文言の日本語化方針
