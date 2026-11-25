# 改善アイデア (オーナー視点のバックログ案)

2026-11-25。issues / discussions / pulls / accounts / live updates /
CSS module 化まで一巡した時点で、コードを読み直して出した改善案。
**決定ではなく候補リスト**。着手するものは TODO.md に移す。

各項目に **効果 / コスト** を S・M・L で付けた。末尾に推奨の着手順。

---

## 1. コードの改善

### 1.1 live updates のヒントを「関係するルートだけ」に効かせる (効果 L / コスト M)

現状 `web/src/live-updates.ts` はヒントが1件来るたびに
`router.invalidate()` で **アクティブな全 loader** (root loader の
`pullsAvailable` + `me` を含む) を再実行する。別 workspace の issue に
コメントが付いただけでも、開いている PR 詳細 (diff 取得込み) が再取得される。

- サーバー: `events.Change` に `scope` (`ws` slug or `pulls`) と
  `resource` (`issue:5` 等) を足す。changes テーブルにも列追加
- クライアント: `router.invalidate({ filter: (m) => … })` で、
  一覧ルートは同 workspace の `new/edit/status/delete`、詳細ルートは
  自分の resource id のヒントだけで再取得。root loader は対象外にする
- 効果: 無関係な再フェッチ・再レンダリングの削減、diff の多い PR 画面の
  ちらつき抑止。複数エージェントが CLI で書き込み続ける用途で特に効く

### 1.2 SQLite を WAL モードにする (効果 M / コスト S)

`internal/store/store.go` の DSN は `busy_timeout` と `foreign_keys` のみ。
API サーバーと `zatu` CLI (別プロセス) が同じファイルに書くのが設計の前提
なので、`_pragma=journal_mode(WAL)` + `synchronous(NORMAL)` にすれば
読み取りが書き込みをブロックしなくなり、`WatchChanges` の 250ms ポーリングも
writer と競合しにくくなる。`-wal`/`-shm` ファイルが増える点は README に記載。

### 1.3 API クライアントの型をサーバーから生成する (効果 M / コスト M)

`web/src/api.ts` (534行) の型は Go の DTO を手で写している。
discussions/pulls 追加で DTO が増え、ずれの検出がテスト頼み。

- 案A: Go の DTO から TypeScript 型を生成 (`tygo` 等)
- 案B: OpenAPI を書いて (or 生成して) `openapi-typescript`
- まずは CI で「Go の JSON タグ一覧と api.ts のキー一覧を突き合わせる」
  簡易チェックだけでも価値がある

### 1.4 一覧ルートの重複を共通化 (効果 M / コスト M)

`issues/index.tsx` / `discussions/index.tsx` / `pulls/index.tsx` は
「tabs + `<search>` フォーム (`key=JSON.stringify(search)` で remount) +
range 表示 + pagination」がほぼ同形。
`ListPage` 的な presentational component と `useFilterForm(route)`
にまとめると、フィルタ追加時の3重修正がなくなる。
旧 URL リダイレクト (`/issues`, `/issues/new`, `/discussions…`) も
`homeSlug` を呼ぶだけの同形ファイルなので、ヘルパ1つ + 薄い route に寄せる。

### 1.5 `api.Store` interface の分割 (効果 S / コスト S)

`internal/api/api.go` の `Store` は 70 メソッド超の単一 interface。
`IssueStore` / `DiscussionStore` / `PullStore` / `AuthStore` に分けて埋め込むと
テスト用 fake を部分的に書けるようになり、ハンドラごとの依存が読みやすくなる。

### 1.6 エラー境界とログの整理 (効果 S / コスト S)

- Go: `log.Printf` 直書き → `log/slog` に統一し、リクエスト ID・
  method・path・status・latency のアクセスログ middleware を足す
- SPA: route の `errorComponent` で 401 を検知したら `/login?next=` へ
  誘導 (production.md の「未ログイン着地でエラーページに落ちる」課題の解消)

---

## 2. 機能の追加 / 洗練

### 2.1 Markdown 入力にプレビュータブ (効果 L / コスト S)

issue/discussion/PR 本文・コメントの textarea に「Write / Preview」切替。
`Markdown` component は既に lazy 化されているので再利用するだけ。
状態は `useReducer` に `mode: 'write' | 'preview'` を足す。

### 2.2 下書きの自動保存 (効果 M / コスト S)

離脱ガード (`useLeaveGuard`) は「失わないように止める」だけ。
`sessionStorage` に `draft:{ws}:{route}:{id}` で debounce 保存し、
再訪時に復元 + 「下書きを破棄」ボタン。submit 成功で削除。
タブクラッシュやリロードでも長文コメントを失わない。

### 2.3 author 表示の仕上げ (効果 M / コスト S)

TODO の残り: 共有 token 投稿 (anonymous) とログインユーザー投稿の区別、
一覧行への author 表示、「自分の投稿だけ編集/削除可」の UI 制御
(サーバー側の権限チェックとセットで)。

### 2.4 一覧の追加フィルタ (効果 M / コスト M)

- `author:me` / `author:{name}` フィルタ、`sort=updated`
  (updated_at 順。インデックス追加込み)
- 検索クエリ構文の軽量版: `is:open label:bug foo` を `<search>` の
  1つの input で受け、既存の search params に分解 (URL は今のまま)

### 2.5 キーボードショートカット (効果 M / コスト S)

`/` で検索にフォーカス、`c` で新規作成、`j`/`k` で一覧行移動、`?` で
ヘルプ `<dialog>`。入力中 (textarea/input フォーカス時) は無効。
`aria-keyshortcuts` を付けて支援技術にも通知。

### 2.6 issue ↔ PR / discussion の相互参照 (効果 M / コスト M)

Markdown 内の `#123` を issue リンクに、`!12` を PR に自動リンク
(remark プラグイン)。サーバー側で参照を抽出して「この issue を参照している」
タイムラインを出すのは次段階。

### 2.7 未読 / 変更バッジ (効果 S / コスト M)

live updates のヒントを捨てずに、一覧で「前回閲覧後に更新あり」の点を表示。
最初は localStorage に `lastSeenAt[issueId]` を持つだけで十分 (per-viewer)。

---

## 3. UX の洗練

### 3.1 mutation のフィードバックを即時に (効果 L / コスト M)

現状はコメント投稿・status 切替・hide が「サーバー応答 → invalidate →
loader 再取得」まで画面が変わらない (`useTransition` の pending 表示のみ)。
React 19 の `useOptimistic` (useState 禁止ルールには抵触しない) で、
送信直後にコメントを半透明で差し込み、失敗時は自動で巻き戻してエラー表示。
TanStack Query を入れずに体感を大きく改善できる (tanstack-query.md の
再検討条件「楽観的更新が必要になったら」の手前で止められる)。

### 3.2 トースト / ライブリージョンで結果を伝える (効果 M / コスト S)

「コメントを削除しました」「マージしました」等を `role="status"` の
共有ライブリージョンで通知。削除系は「元に戻す」を出せると理想
(hide は unhide があるので即実装可)。

### 3.3 View Transitions (効果 S / コスト S)

一覧 → 詳細、タブ切替で `document.startViewTransition` (TanStack Router の
`defaultViewTransition: true`)。`prefers-reduced-motion` では無効化。

### 3.4 未ログイン時の着地 (効果 M / コスト S)

1.6 と同じ課題。トップで 401 → エラーページではなく、ログイン画面 or
「ログインしてください」の空状態へ。`?next=` は既存の仕組みを流用。

### 3.5 空状態と skeleton (効果 S / コスト S)

一覧 0 件時に「フィルタを解除」「最初の issue を作る」導線。
初回ロードは `<progress>` のみなので、`pendingComponent` に
レイアウトを保った skeleton を出して CLS を抑える
(`defaultPendingMs` を調整して高速時は出さない)。

### 3.6 ダークモード (効果 S / コスト S)

トークンは `src/index.css` に集約済みなので `prefers-color-scheme: dark` で
トークンを差し替えるだけ。`color-scheme: light dark` も宣言する。

---

## 4. 性能の改善

### 4.1 root loader を live update の再取得対象から外す (効果 M / コスト S)

`__root.tsx` の loader (`pullsAvailable` + `me`) は全ヒントで再実行される。
ログイン状態は logout/login 時にしか変わらないので、`staleTime` 相当を
`shouldReload: false` で表現するか、1.1 のフィルタで root を除外する。

### 4.2 一覧の COUNT + OFFSET (効果 M / コスト M)

`ListIssues` は毎回 `SELECT COUNT(*)` + `LIMIT/OFFSET`。データが増えると
後ろのページほど遅い。

- `(created_at, id)` を cursor にした keyset pagination を API に追加
  (`?before=`)。UI は「前/次」中心にし、総数はフィルタ無しのときだけ
- 現状規模では問題にならないので、seed を 10万件にしたベンチを先に作る

### 4.3 全文検索を FTS5 に (効果 M / コスト M)

`q` は `instr(lower(title))` / `instr(lower(body))` のフルスキャン。
`issues_fts` (FTS5, trigram tokenizer で日本語も部分一致) を
トリガで同期すれば件数が増えても一定。discussions も同様。

### 4.4 HTTP レイヤ (効果 M / コスト S)

- JSON レスポンスの gzip/zstd 圧縮 (PR diff は大きい)
- GET に `ETag` (例: `updated_at` + 子要素の max `created_at` のハッシュ)
  を付け、`If-None-Match` で 304。live update による再取得の大半は
  「変化なし」になるはずなので効果が大きい (1.1 と相補的)
- 本番 embed 配信 (production.md 案A) 時は hashed asset に
  `Cache-Control: immutable`、index.html は `no-cache`

### 4.5 詳細ページのレンダリング (効果 S / コスト S)

コメントが多いと invalidate のたびに全 `Markdown` が再パースされる。
`CommentItem` を `memo` 化し、`Markdown` を `body` 単位で memo する。
長いスレッドは `content-visibility: auto` + `contain-intrinsic-size` で
画面外の描画をスキップ (CSS だけで済む)。

### 4.6 bundle 分析と preload の調整 (効果 S / コスト S)

`autoCodeSplitting` は有効。`rollup-plugin-visualizer` で一度可視化し、
i18n の ja/en 辞書を両方同梱している点 (各 ~300 行) を必要なロケールだけ
動的 import にするか判断する。`defaultPreload: 'intent'` は PR diff の
ような重い loader では hover だけで重い取得が走るので、そのルートだけ
`preload: false` を検討。

---

## 推奨の着手順

1. **1.2 WAL** と **4.1 root loader 除外** — 数行で済み、リスクが小さい
2. **1.1 live update のスコープ化** + **4.4 ETag** — 「複数エージェントが
   CLI で書き続ける」本リポジトリの主用途で効く。セットでやる
3. **3.1 useOptimistic** + **3.2 ライブリージョン** — 体感改善が最大
4. **2.1 プレビュー** / **2.2 下書き保存** / **3.4 未ログイン着地**
5. **1.4 一覧共通化** / **1.3 型生成** — 次に機能を足す前の地ならし
6. 4.2 / 4.3 はベンチ用 seed を作ってから数値で判断

各項目は ui-rules.md / dev-workflow.md (component を browser-mode で先に
検証 → playwright-cli で実サーバー確認) の手順で進める。
