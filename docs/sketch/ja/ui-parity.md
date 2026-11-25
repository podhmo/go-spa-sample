# UI パリティ: SPA を MPA (zatudan) の見た目に揃える

issue tracker のSPA実装 (`web/src`) を、参照実装である
`go-classic-mpa-sample` (zatudan MPA) の見た目・構造に揃えた記録。
正は MPA 側の次のファイル:

- `internal/web/static/app.css` — design token と component スタイル
- `internal/web/templates/*.html` — マークアップ構造と class 語彙
- `internal/web/i18n.go` — UI 文言 (英語)
- `docs/ja/ui-rules.md` — UI 規約 (ボタン配置・確認モーダル等)
- `internal/web/static/*.js` — enter-submit.js 等の小さな振る舞い

方針: **class 語彙とDOM構造を MPA に寄せる**。SPA 側の a11y 強化
(`<search>` / `.error-msg` / `aria-*` / copy status bubble) は
そのまま活かし、見た目と文言だけ MPA に合わせる。

## 移植したもの

### CSS (`web/src/index.css`)

MPA の `app.css` の design token と component スタイルを移植:

- `:root` の GitHub 風 neutral パレット (`--bg`, `--surface`, `--border`,
  `--text`, `--accent`, `--open`/`--closed`/`--danger` など) と
  `prefers-color-scheme` による light/dark の raw value 切替
- `.container` (max-inline-size 60rem), `.site-header`/
  `.header-inner`/`.site-title`/`.site-nav`, `.main-content`
  (`container: main / inline-size`), `.site-footer`, `.skip-link`
- `.btn` / `.btn-primary` / `.btn-quiet` / `.btn-danger` /
  `.btn-disabled`, `.card`
- `.badge badge-open|closed`, `.label-badge` (inline `--label-color`)
- `.issue-list`/`.issue-row`/`.issue-row-main`/`.issue-row-meta`/
  `.issue-title`/`.issue-num`/`.issue-time`
- `.tabs`/`.tab`/`.tab-active`, `.filter-bar` (auto-fit grid)/
  `.filter-field`/`.filter-actions`, `.result-count`,
  `.pagination`/`.page-info`, `.empty-state`/`.empty-lead`
- `.field`/`.req`/`.field-hint`/`.form-note`/`.error-msg`,
  `.label-picker`/`.label-option`, `.form-actions`
  (左: cancel / 右: 確定、ui-rules 準拠)
- `.comment-list`/`.comment`/`.comment-meta`/`.comment-permalink`/
  `.comment-hidden` (+`-note`/`-body`), `.comment-actions`/
  `.comment-action`, `.issue-actions`, `.confirm-dialog`,
  `.breadcrumb`, `.markdown-body`/`.markdown-alert`,
  `.copy-wrap`/`.copy-tip`/`.copy-btn` (hover-reveal は opacity で
  a11y tree に残す), `.error-page`/`.error-code`, `.visually-hidden`
- textarea は `field-sizing: content` + `#comment-body`/`#issue-body`
  の `lh` 単位上下限 (MPA の `#f-body` 相当)

### マークアップ (`web/src/routes.tsx`, `web/src/components/*.tsx`)

- root route が MPA `layout.html` の chrome を再現: skip-link →
  `.site-header` (site-title + nav: Issues / Labels / `New issue` btn) →
  `<main class="main-content" id="main-content">` → `.site-footer`
- issues 一覧: `.page-head` h1 + status `.tabs` (Open/Closed/All、
  他フィルタを保持する link) + `<search>` 内の `.filter-bar`
  (Search/Label/Sort + Apply、フィルタ適用中のみ Clear) +
  `.result-count` + `.issue-list` + `.pagination`
  (Previous/Next、`page-info`)
- issue 詳細: `.breadcrumb` → `article.card.issue-detail`
  (badge+title+`.issue-num`、label badges、`.issue-meta`) +
  `.issue-actions` (Edit link / Close issue / Copy as markdown / Delete)
  + `Comments (n)` + `.comment-list` + comment form
- コメント: `.comment-meta` (author commented + permalink time +
  edited + copy-btn)、`.comment-actions` (Edit / Hide|Unhide / Delete)、
  hidden は `<details>` で畳む (ui-rules: display:none しない)
- フォーム: `Fields marked * are required.` note、`*` は
  `.req` (aria-hidden)、`.form-actions` の cancel/confirm 順、
  label picker は fieldset+`.label-option` checkbox
- labels 管理: `.label-admin-form.card` + `.form-inline`
  (MPA 同様 素の縦積み、submit はラッパーなし)、`.label-list`/
  `.label-row` は MPA 同様 **意図的に未スタイル** (箇条書きの素の li)、
  `.label-scope` (archived 行は `scope · kind`)、
  Archived section は非空のときだけ表示
- 404/エラーは `.error-page`/`.error-code`

### 文言 (i18n.go 準拠)

- `Showing 1–3 of 3 issues` / `Page %d of %d` / Previous / Next
- `No issues match the current filters.` / `No issues here yet.` /
  `Create the first issue` / `No comments yet.` / `Add a comment`
- `Close issue` / `Reopen issue` / `Delete issue #N?` /
  `Delete comment #N?` / `Hide comment #N?`
- `Copy as markdown` / `copy link` / `No description provided.`
- `Opened by %s <time>` / `· Last updated <time>` /
  `opened <time>` / `%s commented`
- labels: `New label` / `Name` / `Color` / `Kind` / `Scope` /
  `This workspace` / `Shared (all workspaces)` / `Create label` /
  `Issue labels` / `Discussion labels` / `Archived` /
  `No issue labels yet.` など
- `fmtTime` は `Jan 2, 2006 15:04` 相当 (local time、`format.ts`)

### 振る舞い

- `enter-submit.js` 相当: textarea で Ctrl+Enter / Cmd+Enter
  (`submitOnCtrlEnter` in `enter-submit.ts`) — modifier combo なので
  IME composition と衝突しない。hint 文言も `Ctrl+Enter (Cmd+Enter
  on macOS) submits.` に合わせた
- 削除/hide は `<dialog>` + Invoker Commands (`commandfor`/
  `command="show-modal"`) + `<form method="dialog">` + `returnValue`、
  Cancel が先・初期フォーカス (ui-rules 準拠)
- `input[type=text]` を明示 (MPA テンプレ同様; 属性なし input は
  `input[type='text']` セレクタに掛からない)

## 意図的に残した差分

- **ヘッダー nav は issues subset**: Discussions / Workspaces /
  Log in / Register / 言語切替は SPA の対象外機能のため未配置。
  サイト名は `zatudan` (MPA は `Issue Tracker`)
- **label 削除は `<dialog>` 確認を挟む**: MPA は archive POST を
  無確認で投げるが、ui-rules「破壊的操作は必ず確認」の方を優先。
  ボタン文言は MPA に合わせて `Delete` (実際は archive)
- **コメント/ラベルの編集は行内フォーム**: MPA は別ページ
  (`/labels/{id}/edit` 等)。SPA は遷移なしで畳み込み
- **`.error-msg` + `:user-invalid` 連動**: MPA は `.field-error` を
  常時描画+pending class で隠す。SPA は `src/a11y.ts` の delegated
  listener と `:user-invalid ~ .error-msg` で同等を実現 (既存 a11y
  実装を維持)
- **markdown の hint/ダイアログ等**: `role="status"` の copy bubble や
  `<search>` ラッパなど SPA 側の a11y 構造は残す
- **seed データは別実装** (issue id や件数が違う) — 見た目比較のみ

## 検証

playwright-cli で実サーバー同士を撮影して目視比較
(SPA: `go run ./cmd/zatudan -seed -api-token dev-token` + vite dev、
MPA: `go run ./cmd/zatudan -seed`):

| 画面 | MPA (:8090) | SPA (:5174) |
| --- | --- | --- |
| issues 一覧 | `images/ui-parity-mpa-issues.png` | `images/ui-parity-spa-issues.png` |
| issue 詳細 | `images/ui-parity-mpa-issue.png` | `images/ui-parity-spa-issue.png` |
| issue 作成 | `images/ui-parity-mpa-new.png` | `images/ui-parity-spa-new.png` |
| labels 管理 | `images/ui-parity-mpa-labels.png` | `images/ui-parity-spa-labels.png` |
