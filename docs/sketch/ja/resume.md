# 作業再開用プロンプト (resume)

新しいセッション/エージェントがこのリポジトリの作業を再開するための要約。
詳細はリンク先を読むこと (ここには判断の「場所」だけを書く)。

## このリポジトリは

[go-classic-mpa-sample](https://github.com/podhmo/go-classic-mpa-sample)
(古典的 POST→redirect MPA の issue tracker "zatudan") の
API サーバー部分を移植し、フロントを **SPA として作り直す**実験。
元アプリの機能 spec / 受け入れチェックリストは `docs/ja/features.md`。

## 現在地 (2026-11-24 時点)

- **issues スコープは機能パリティ達成**。SPA: 一覧(全フィルタ+page)/
  詳細/作成/編集/削除/状態変更/コメント CRUD/hide・unhide/permalink/
  labels管理/404/Markdown描画(GFM+alert)/markdownエクスポート。
  API: `cmd/zatudan` + `internal/` (issues/labels/comments/workspaces/
  markdown)。17 browser-mode tests 緑、CI 有り (`.github/workflows/ci.yml`)
- **スコープ外として捨てたもの**: SSE live updates、`X-Partial` 部分描画
  (SPA は JSON API のみ)、`zatu` CLI、accounts/セッション
- **サブエージェント運用**: 大きな実装・検証は worktree + 別ブランチで
  サブエージェントに任せ、結果を master にマージしてきた
  (sketch/tanstack-query、sketch/react-markdown)

## 残りタスク (`TODO.md` が正)

| タスク | 補足 |
| --- | --- |
| discussions/pulls の移植判断 | spec: `docs/ja/features.md` 3.5/3.6。store には残存。API routes は削除済み |
| `cmd/fakeapi` 撤去 | 疎通スパイク用の残骸 |
| i18n | 元は en/ja 2ロケール (features.md 3.10)。SPA 側方針未定 |
| workspace scoping `/w/{slug}` | SPA は `WS='default'` 固定。URL 構造を変える大きめの変更 |
| 本番構成 | dev proxy の `Bearer dev-token` 注入は開発専用。embed 配信+認証 or 別配備+token |
| (保留) TanStack Query | 評価済み・見送り。再検討条件は `docs/sketch/ja/tanstack-query.md` |

## 決まっていること

- **React ローカル状態は `useReducer` のみ** (useState 禁止)
- サーバーデータ: Router loader + `router.invalidate()` (mutation系の
  pending は useReducer busy flag — `useTransition`+invalidate は
  pending が固まる flake あり。navigate を包む useTransition はOK)
- Markdown: `react-markdown` + `remark-gfm` + `remark-github-blockquote-alert`
  (vdomネイティブ、innerHTML不使用。比較: `docs/sketch/ja/markdown.md`)
- CSS: `src/index.css` (+必要なら CSS Modules)。Panda は将来案 (`css.md`)
- UI は modern-web-guidance 準拠 — 適用パターン集:
  `docs/sketch/ja/libraries.md` (dialog/`commandfor`、`:user-invalid`+aria
  同期 `<search>`、copy-button の role=status bubble 等)
- Lint = oxlint、Formatter = oxfmt (`npm run format` を編集後に必ず。
  `format:check` は CI)。TS は `~7.0.2` (native)
- ライブラリ追加時は「公開から7日以上経ったバージョン」を選ぶ

## 開発ループ

```sh
go run ./cmd/zatudan -seed -api-token dev-token -db /tmp/spa-zatudan.db  # API :8080
npm --prefix web run dev      # SPA :5173 (/api proxy、dev-token 注入)
cd web && npm run format && npm run lint && npm run build && npm test
```

- component 単位の確認 → `web/src/*.test.tsx` (vitest browser mode、実 Chrome)
- アプリ全体の確認 → playwright-cli でブラウザ操作
- 手順詳細: `docs/sketch/ja/dev-workflow.md`、規約集約: `AGENTS.md`

## ハマりどころ (gotchas)

- **:8080 を掴んだままの古い Go プロセス**が zatudan をシャドウすることがある
  → 起動前に port を確認
- **vitest browser mode が稀に iframe 初期化タイムアウト** (Chrome 起動競合)
  → 再実行で大体通る
- `commandfor`/`command`/`closedby` は React 型にまだない
  → attribute spread / setAttribute で渡す (`ConfirmDialog.tsx` 参照)
- `getByLabelText` は exact match → ラベルに `*` を足すとテストが壊れる
  → テスト側は `/Title/` のような正規表現で
- 作業ブランチ worktree は `/tmp/gss-*`、作業後は `git worktree remove`
