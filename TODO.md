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
- [x] **issues scope の機能パリティ**: `docs/ja/features.md` §6 の
  Issues/コメント・モデレーション/Labels の全項目 + 横断の大半をカバー
  （一覧フィルタ+pagination、issue 編集、コメント編集/削除/hide/unhide/
  edited/permalink、GFM+alert Markdown 描画、markdown エクスポート、
  404、labels 管理画面）。Go 側に `GET /labels?archived=1` を追加。
  残りは workspace スコープ (`/w/{slug}`) と login 相当の横断項目のみ
- [x] **サーバー状態管理の再検討**: `sketch/tanstack-query` で評価実装済み。
  判定は「現状は loader 継続」(再検討条件は `docs/sketch/ja/tanstack-query.md`)
**順序**: まず issues tracker だけの見た目を classic-mpa に揃え
modern-web-guidance を適用しきる (次の2項目)。それができたら
workspace → discussions → pulls → accounts へ展開する。

- [x] **[milestone] issues の見た目を classic-mpa に揃える**: SPA の
  issues scope を元アプリ `internal/web/static/app.css` +
  `docs/ja/ui-rules.md` (go-classic-mpa-sample) 準拠の見た目にした。
  MPA(:8090) vs SPA(:5173) の playwright-cli スクリーンショットで
  4画面を比較確認。記録: `docs/sketch/ja/ui-parity.md`
  (残差分も文書化 — nav は issues 限定、label 削除は dialog 確認等)
- [x] **[milestone] modern-web-guidance の issues 反映を記録**:
  適用ガイド・パターンを `docs/sketch/ja/ui-parity.md` +
  `libraries.md` 追記として記録済み
- [x] **workspace スコープ `/w/{slug}` 対応** (features.md 3.1, §6-全体):
  URL を `/w/{slug}/issues…` 構造に再編 (`WS='default'` 固定を撤去)。
  workspace なしアクセスは default/先頭へ redirect (検索クエリ保持)、
  別 workspace のリソースへの GET/HEAD は 404 の `error.workspace` +
  `X-Workspace-Slug` ヒントで正規スコープへ rebase (mutation はヒント無し
  404 で拒否)。`/workspaces` 一覧/作成 UI と最後の1個の削除抑止も実装。
- [x] **Discussions 移植** (features.md 3.5, §6-Discussions)
  - [x] **Go API 復元**: 移植時に外した discussions ハンドラを
    `internal/api` に再移植 (threads/replies/answer/categories ルート)
  - [x] **SPA**: 一覧 (category/answered/q/sort/page)・詳細
    (threads+replies 1段ネスト)・作成 (category 必須)・編集/削除・
    thread/reply モデレーション・qa カテゴリの answer マーク
- [x] **Pull Requests 移植** (features.md 3.6, §6-Pull Requests):
  git バック + repo ゲーティングが最大の残り塊
  - [x] **Go**: `internal/gitrepo` + pulls API 移植、`-repo` フラグと
    `.git` 自動検出 (repo 無しは `/pulls*` 404)
  - [x] **SPA**: 一覧 (status/q/sort/page)・詳細 (commits/merge可否)・
    作成 (base/head)・Files (diff + 行アンカー review コメント)・
    status 遷移・merge (merge/squash/rebase)
- [x] **アカウント・セッション** (features.md 3.7, §6-アカウント):
  register/login/logout + cookie セッション (SHA-256, 30d sliding
  TTL、HttpOnly/SameSite=Lax)。session cookie は Bearer より優先。
  SPA は /login /register + nav のユーザー名/ログアウト + `?next=`
  誘導。投稿の author 付与は保存済み (actorID → created_by)；残りは
  表示側 (UI での author 名表示・共有 token 投稿との区別)
- [x] **`zatu` CLI 移植判断** (features.md 3.11): **移植**。store の
  メソッド面は MPA と完全一致なのでローカル (SQLite直叩き) /
  リモート (`--server`/`--token` → `/api/v1`) 両バックエンドを
  `internal/cli` + `internal/client` + `cmd/zatu` として移植
- [x] **i18n** (features.md 3.10): 元は en/ja 2ロケール (cookie +
  Accept-Language 解決)。SPA では URL/localStorage/Accept-Language の
  どれで持つか方針から → `web/src/i18n.tsx` (localStorage('lang') >
  navigator.language > 'en'、辞書は typed `ja: typeof en`) + nav 言語
  スイッチ。方針は `docs/sketch/ja/i18n.md`
- [x] **markdown エクスポート拡張** (features.md 3.12): issue は済。
  discussions/PR 移植時に対応分を追加 (PR の `Copy as markdown` 済)
- [x] **`cmd/fakeapi` の撤去**: スパイク用コードの整理 (dev-workflow.md
  の記述も zatudan 直起動に更新)
- [x] **SPA版 ui-rules.md の策定** (`docs/ja/ui-rules.md`)
- [x] **入力中の離脱ガード** ([`docs/ja/ui-rules.md` §3](docs/ja/ui-rules.md)):
  `web/src/leave-guard.ts` の `useLeaveGuard` が `useBlocker` でアプリ内遷移を
  confirm、`beforeunload` でブラウザ離脱をガード (submit 後の遷移は
  reducer の `submitted` flag で解除)
- [x] **コメント空入力の送信 Disabled** ([`docs/ja/ui-rules.md` §6](docs/ja/ui-rules.md)):
  トップレベル・返信・編集インラインとも本文が空/空白のみなら disabled
  (MPA の comment-submit-disabled.js 相当)
- [x] **live 更新（SSE イベント購読）**: 複数エージェントがローカル CLI
  (DB直叩き) でコメント/スレッドを書いたとき、開いている issues /
  discussions 画面が追加・編集・削除に追随するようにする。
  description/title 変更の細かい追随は MPA/SPA 双方 future work
- [ ] **CSSのcomponent co-location移行**: `docs/sketch/ja/css.md` の方針に従い、
  leaf component → app shell → feature固有style → shared primitive の順で
  `src/index.css` のglobal ruleを `*.module.css` へ移す。目安は
  `src/index.css` 300行以内、feature固有selectorはglobal fileに残さない
  ([#22](https://github.com/podhmo/go-spa-sample/issues/22))
  - [x] **leaf componentのmodule化**: `Markdown` とdialog系component
    (`ConfirmDialog`/`HideDialog`/pull merge) のstyleをcomponent隣の
    `*.module.css` へ移す
  - [ ] **app shellのmodule化**: `__root.tsx` のheader/nav/footer/progress/error
    pageのstyleを `routes/-components/SiteChrome.module.css` へ移す
  - [ ] **feature固有styleのmodule化**: workspace list、labels admin、pulls
    diff/PR styleをroute-local moduleへ移す
  - [ ] **共有primitiveのmodule化**: `btn`, `field`, `form-actions`, `card`,
    `page-head`, `filter`, `pagination`, `breadcrumb`をexplicit importへ移す
  - [ ] **list/comment/detailのmodule化**: issue/discussion/pull跨ぎのlist row、
    comment、detail、copy/label関連styleを共有関係ごと整理して移す
- [ ] **本番構成の決定**: dev proxy の `Bearer dev-token` 注入は開発専用。
  embed 配信+セッション or 別配備+token のどちらにするか
