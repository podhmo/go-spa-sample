# zatudan 機能一覧・受け入れ仕様

このドキュメントは MPA 版 zatudan（`podhmo/go-classic-mpa-sample`）の機能棚卸しであり、
SPA 版（go-spa-sample）への移植時の受け入れチェックリストとして使う。
挙動の出典は各項目末尾に `internal/...` `docs/...` 形式で記す。

## 1. 概要

zatudan は Go 製のクラシック MPA（サーバーレンダリング HTML + 全状態変更が HTML フォーム + POST→Redirect→GET）で実装された issue tracker。workspaces / issues / comments / labels / discussions（スレッド + Q&A answer）/ git バックの pull requests / アカウントとセッション / JSON API（`/api/v1`）+ gh 風 CLI（`zatu`）を持つ。永続化は単一 SQLite ファイル。

- 技術スタック: Go 1.27 / `net/http` + `http.ServeMux`（メソッド+パスパターン）/ `html/template` / `database/sql` + `modernc.org/sqlite`（CGO 不要）/ goldmark（GFM Markdown）/ argon2id（パスワード）。ORM・ビルドステップなし（`README.md`）。
- サーバー: `cmd/zatudan`（`cmd/zatudan/main.go`）。CLI: `cmd/zatu`（別バイナリ）。
- **認可は存在しない**: アカウントは投稿者名の帰属のためだけ。ログイン有無に関わらず全操作が可能。デフォルト bind は `127.0.0.1:8080`。

## 2. 起動・設定

`cmd/zatudan/main.go`、README「Configuration」。

| フラグ | 環境変数 | デフォルト | 意味 |
|---|---|---|---|
| `-addr` | `ZATUDAN_ADDR` | `127.0.0.1:8080` | listen アドレス |
| `-db` | `ZATUDAN_DB` | `zatudan.db` | SQLite ファイルパス（`:memory:` 可、空文字は起動エラー） |
| `-init` / `init` サブコマンド | — | `false` | スキーマ + マスターデータ投入 + `ANALYZE` して終了（起動時にも毎回実行されるので任意のプロビジョニング用） |
| `-seed` | `ZATUDAN_SEED` | `false` | デモ issue/discussion/ユーザー投入（マスターデータは常に投入） |
| `-api-token` | `ZATUDAN_API_TOKEN` | 未設定 | `/api/v1` の共有 Bearer トークン |
| `-repo` | `ZATUDAN_REPO` | 自動検出 | PR 機能用 git リポジトリ。`off` で機能ごと無効化 |

- 起動時にスキーマ初期化は冪等に毎回行われ、`SeedMasterData`（default workspace + discussion categories）も常に実行（`cmd/zatudan/main.go`）。
- graceful shutdown あり（SIGINT/SIGTERM、10s）。`WriteTimeout` 15s（SSE ハンドラは個別に解除）。

## 3. 機能一覧

### 3.1 Workspaces

出典: `internal/store/workspaces.go`、`internal/web/workspaces.go`、`internal/api/api.go`。

- issue と discussion は必ず 1 workspace に属する（GitHub repository 相当のコンテナ）。PR は workspace 非依存（リポジトリに紐づく）。
- 全 URL は `/w/{slug}/...` が正。`/issues` `/discussions` など workspace なし URL は「`default` があればそこ、なければ最初の workspace」へリダイレクト（GET/HEAD は 302、それ以外は 307）。
- 別 workspace のリソースを `/w/{他slug}/...` で GET すると正しい workspace の URL へ 302。**POST 系は 404**（誤った workspace への mutation を防ぐ）。
- `GET /` は 303 で `/issues` へ。
- 操作: 一覧 `GET /workspaces`、作成 `GET/POST /workspaces/new` + `POST /workspaces`、削除 `GET/POST /w/{ws}/delete`（中身ごと CASCADE 削除、**最後の 1 個は削除不可** `ErrLastWorkspace`）。存在しない slug は 404。
- slug は一意。重複は `ErrDuplicateSlug`。
- API: `GET/POST /api/v1/workspaces`、`GET/DELETE /api/v1/workspaces/{slug}`。

### 3.2 Issues

出典: `internal/store/issues.go`、`internal/web/issues.go`、`internal/api/issues.go`、`docs/ui.md`。

- 一覧 `GET /w/{ws}/issues`: フィルタは全てクエリパラメータ。
  - `status`: `open`（デフォルト）/`closed`/`all`（未知値は `all` 扱い）
  - `q`: title OR body の大小無視 substring 検索
  - `label`: ラベル名で絞り込み
  - `sort`: `newest`（デフォルト、created_at DESC + id DESC）/`oldest`
  - `page`: 1-based、1 未満は 1 にクランプ。1 ページ 20 件
- 作成 `GET/POST /w/{ws}/issues/new` + `POST /w/{ws}/issues` → 303 で詳細へ。作成時は必ず `open`。issue 番号（ID）は workspace 横断でグローバル・再利用されない。
- 詳細 `GET /w/{ws}/issues/{id}`: 本文（Markdown 描画）、ラベル、コメント一覧（時系列）、コメントフォーム、status 切替・編集・削除ボタン。
- 編集 `GET/POST /w/{ws}/issues/{id}/edit`（title/body/labels 差し替え、`updated_at` 更新）。
- status 切替 `POST /w/{ws}/issues/{id}/status`（hidden `status` フィールドで open/closed 指定）。**変化がない POST（closed を closed に等）は `updated_at` を更新しない**。
- 削除 `GET/POST /w/{ws}/issues/{id}/delete`（GET は安全な確認ページ、POST でコメント・ラベル紐付けごと削除）→ 303 で一覧へ。
- Markdown エクスポート `GET /w/{ws}/issues/{id}/markdown`。
- API: list/create/get（issue + labels + comments を 1 コールで）/PATCH（渡したフィールドのみ部分更新）/DELETE（204）/status POST/comments POST/markdown GET。`per_page` 上限 100。

### 3.3 コメント（issue comments）

出典: `internal/store/issues.go`、`internal/store/moderation.go`、`internal/web/comments.go`。

- issue 詳細の下部フォーム `POST /w/{ws}/issues/{id}/comments` → 303 で `#comment-N` へ。時系列（created_at, id 昇順）で表示。
- 新規コメント投稿は親 issue の `updated_at` を bump する（一覧の「活動」として扱う）。
- 編集 `GET/POST .../comments/{cid}/edit`: 本文が実際に変わったときだけ `edited_at` を記録し「edited」バッジ表示。
- 削除 `GET/POST .../comments/{cid}/delete`: GET 確認ページ + POST で物理削除。
- hide/unhide `POST .../comments/{cid}/hide|unhide`: 理由必須の minimize（本文は残る）。理由は固定リスト（3.10 参照）。
- 各コメントにパーマリンクコピーボタン（Invoker Command `--copy-permalink`）。
- 編集/削除/hide は親の `updated_at` を bump しない（モデレーションは「活動」ではないルール）。
- バリデーション: body 必須・10,000 文字以内 → 失敗時は入力保持で 422 再描画。

### 3.4 Labels

出典: `internal/store/labels.go`、`internal/web/labels.go`、`internal/api/labels.go`、`internal/store/store.go`（`defaultLabelsDDL`）。

- ラベルは「kind（`issue` / `discussion`）× スコープ（共有マスタ `workspace_id NULL` / workspace 固有）」の 2 軸。一覧表示は共有 + 自 workspace の和集合。同名でも別スコープなら共存（上書きしない）。
- issue への付与は「共有 or 自 workspace・kind=issue・archived=0」のラベルのみ可能。存在しない ID・別 kind・別 workspace の ID は**エラーにせず黙ってスキップ**（`setIssueLabels`）。discussion 側も同様（kind=discussion）。
- アーカイブされたラベルの既存付与は残る（編集による置き換えでも保持）。ピッカー・フィルタからは消える。
- 管理画面 `GET /w/{ws}/labels`（kind 別一覧 + アーカイブ済みセクション）、作成 `POST /w/{ws}/labels`（kind と scope も指定、作成後は不変）、編集 `GET/POST .../labels/{id}/edit`（name/color のみ）、アーカイブ/復元 `POST .../labels/{id}/archive`（削除=アーカイブ）。
- 同名で archived な行があれば**作成は新規 INSERT せず色を更新して復元**。生きている同名は `ErrDuplicateLabel`。アーカイブ解除で同名衝突する場合も `ErrDuplicateLabel`。
- デフォルトマスタラベル（スキーマが全 DB に投入、issue kind のみ）: `Bug #d73a4a` / `Documentation #1f6feb` / `Enhancement #a2eeef` / `Feature #0075ca`。discussion kind のデフォルトはなし。
- API: `GET/POST /api/v1/w/{ws}/labels`、`PATCH/DELETE /api/v1/w/{ws}/labels/{id}`（DELETE はアーカイブ）。
- issue/discussion 一覧ではラベルバッジをクリックで絞り込み。

### 3.5 Discussions（threads / replies / answers / categories）

出典: `internal/store/discussions.go`、`internal/web/discussions.go`、`internal/api/discussions.go`。

- GitHub Discussions 相当。**discussion は close しない** — Q&A カテゴリでは answer マークが「解決」の役割。
- 構造: discussion → threads（トップレベル投稿）→ replies（1 段だけ）。スレッド自体が最初の投稿（body/author/is_answer は thread 側）。reply を answer にすることは不可（構造上 1 段ネスト固定）。
- 一覧 `GET /w/{ws}/discussions`: `category`（カテゴリ slug）/`answered`（`yes`|`no`）/`q`/`sort`/`page` でフィルタ。answered は「is_answer=1 の thread が存在する」。
- 作成 `GET/POST /w/{ws}/discussions/new` + `POST /w/{ws}/discussions`（category_id 必須、存在しないカテゴリは `ErrInvalidCategory`）。編集・削除は issue と同形。
- 詳細 `GET /w/{ws}/discussions/{id}`: カテゴリ表示、threads + replies。
- コメント投稿 `POST /w/{ws}/discussions/{id}/comments`: `thread_id` なしなら新規 thread、ありならその thread への reply → 303 で `#thread-N` / `#comment-N`。別 discussion の thread への reply は `ErrInvalidThread`。
- thread/reply ともに編集・削除・hide/unhide が可能（`/threads/{tid}/...`、`/threads/{tid}/comments/{cid}/...`）。thread 削除は replies ごと CASCADE。hidden thread の replies は表示されたまま。
- **answer マーク** `POST /w/{ws}/discussions/{id}/comments/{cid}/answer`: qa カテゴリの thread のみ。1 discussion に最大 1 件（部分ユニークインデックス）。別 thread をマークすると印が移る。unmark は no-op も可。**discussion を非 qa カテゴリに編集すると answer マークは全て消える**。非 qa への mark は `ErrNotAnswerable`。thread/reply 投稿・answer は discussion の `updated_at` を bump。
- カテゴリは seed 固定（作成 UI なし）: `announcements` / `general` / `ideas` / `qa`（format は `open`/`open`/`open`/`qa`）。全件 `GET /api/v1/discussion-categories`。
- API: list/create/get（threads に replies ネスト済み）/PATCH/DELETE、threads POST、threads/{tid}/comments POST、threads/{tid}/answer POST `{answer: bool}`、markdown GET、thread・reply の PATCH/DELETE/hide/unhide。

### 3.6 Pull Requests（git バック）

出典: `internal/store/pulls.go`、`internal/gitrepo/`、`internal/web/pulls.go`、`internal/api/pulls.go`、`README.md`。

- **機能自体が repo ゲーティング**: `-repo`/`ZATUDAN_REPO` で明示、未指定なら `-db` のあるディレクトリから `.git` を親方向に探索して自動検出。`off` で無効。repo が無ければ `/pulls*`・`/api/v1/pulls*` は全て 404 で nav リンクも非表示。明示指定の失敗のみ起動エラー、自動検出失敗は黙って機能なし。
- PR は DB 行（base_ref, head_ref, title, body, status, 各種 SHA）+ 実 git リポジトリ。open 中の commits/diff/merge 可否は git から都度計算。
- **workspace 非依存**で URL は `/pulls`（`/w/{ws}` プレフィックスなし）。
- 一覧 `GET /pulls`: `status`（open デフォルト / closed / merged / all。未知値は all）/`q`/`sort`/`page`。
- 作成 `GET/POST /pulls/new` + `POST /pulls`: `{base, head, title, body}`。base==head は `ErrSameRef`。**同一 (base, head) の open PR は 1 件まで**（部分ユニークインデックス → `ErrDuplicatePR`）。作成時に head tip を `refs/zatudan/pr/<id>/head` に pin（pin 失敗で insert ごとロールバック）。作成時に base_sha/head_sha を記録。
- 詳細 `GET /pulls/{id}`: status、merge 可否、commits 一覧、コメントタイムライン。
- 編集 `GET/POST /pulls/{id}/edit`: title/body のみ（**refs は作成後不変**）。
- Files タブ `GET /pulls/{id}/files`: merge-base→head の unified diff、行ごとにコメントフォーム。
- status `POST /pulls/{id}/status`: open→closed（head_sha/merge_base_sha を記録し pin 更新、closed_at 記録）、closed→open。**merged は終端**で、open 以外からの遷移は `ErrInvalidPRTransition`。
- マージ `GET/POST /pulls/{id}/merge`: GET は method 選択の確認ページ、POST で実行。method は `merge`/`squash`/`rebase`（任意 `message`）。base はローカルブランチ（refs/heads/*）必須、既 merge は `ErrNotMergeable`。コンフリクトは `git merge-tree` で事前検出してファイル名列挙。worktree があるときは porcelain（merge --no-ff / --squash / cherry-pick --no-merges）、なければ plumbing（merge-tree --write-tree）。merge 成功で `merge_sha`/`merge_method`/`head_sha`/`merge_base_sha`/`closed_at` を記録。
- 削除: PR + コメントを CASCADE 削除（API のみ DELETE。web UI はなし）。
- コメント `POST /pulls/{id}/comments`: 通常コメント or **レビューコメント**（`path`+`line` 必須セット、片方だけは `ErrInvalidAnchor`。`side` は `right` デフォルト/`left`、anchor_sha と diff_snippet も保持）。一覧は時系列で 1 本のタイムライン。
- コメントの編集/削除/hide/unhide は他コメント種と同形（`/pulls/{id}/comments/{cid}/...`）。hide は理由必須。
- シードはなし。`tools/demo-pr-history.sh` がフィクスチャ repo + `zatu pr` で履歴をリプレイ。
- API: `/api/v1/pulls`（repo があるときのみマウント）、`/{id}` GET/PATCH/DELETE、`/comments` GET/POST、`/comments/{cid}` PATCH/DELETE + hide/unhide、`/status` POST、`/merge` POST、`/diff` GET（unified diff、text/plain）、`/markdown` GET。

### 3.7 アカウント & セッション

出典: `internal/store/accounts.go`、`internal/web/auth.go`、`internal/auth/`。

- `GET/POST /register`: name + password で登録 → 即ログイン（セッションクッキー発行）→ `?next=` へ 303。
- `GET/POST /login`: 認証 → セッション発行 → `next` へ 303。`next` は同一サイトのパスのみ許可（`/issues` にフォールバック）。
- `POST /logout`（POST のみ、GET はなし）: セッション行削除 + クッキー失効 → 303。
- ログイン済みユーザーが `/register` `/login` に GET すると `next` へリダイレクト。
- 認証は識別のみ。未ログインでも全機能利用可、投稿は anonymous として記録。ログイン中は author_id が投稿に付き名前表示。
- パスワード: argon2id PHC 文字列で保存。ユーザー名は大小無視で一意（`COLLATE NOCASE`、重複は `ErrDuplicateName`）。
- ログイン失敗は「name か password か」を曖昧にした一律エラー + 200ms 遅延（`ErrInvalidCredentials` + `failedLoginDelay`）。
- セッション: opaque トークン、DB には SHA-256 のみ。サーバー側 TTL 30 日・**スライディング更新**（残り半分未満で延長）。クッキーは `session` 名・HttpOnly・SameSite=Lax・ブラウザセッションクッキー（Secure は HTTP localhost 運用のため未設定）。
- API トークン: ユーザー別 `api_tokens`（SHA-256 のみ保存、平文は発行時一度だけ表示）。失効は行削除。`zatu auth token` で発行（ローカルモードのみ）。

### 3.8 モデレーション（hide/unhide・edited マーカー）

出典: `internal/store/moderation.go`。

- 対象は 4 種の投稿: issue comment / discussion thread / discussion reply / PR comment。全て `(edited_at, hidden_at, hidden_reason)` の共通カラム 3 本で管理。
- **編集**: 本文が実際に変わったときだけ `edited_at` を記録（無変更保存はマーカーを立てない）。UI に「edited」バッジ。
- **hide**: 理由必須の minimize。`hidden_at` + `hidden_reason` を記録、本文は保持。理由は固定 7 種: `spam` / `abuse` / `off-topic` / `outdated` / `duplicate` / `resolved` / `low-quality`。リスト外は `ErrInvalidHideReason`（400/422）。unhide（理由 ""）で両方クリア。
- **削除は物理 DELETE**（復元なし）。thread 削除は replies も CASCADE。
- モデレーション操作は親リソースの `updated_at` を bump しない。

### 3.9 ライブ更新（SSE）とフラグメント

出典: `internal/events/`、`internal/web/events.go`、`internal/web/web.go`、`cmd/zatudan/main.go`。

- 詳細ページは `EventSource` で SSE 購読: `GET /w/{ws}/issues/{id}/events`、`GET /w/{ws}/discussions/{id}/events`、`GET /pulls/{id}/events`。
- 変更ヒントは `changes` テーブルに mutation と同一トランザクションで記録され、`WatchChanges` が新行を hub へ再配信 → **別プロセス（ローカル CLI）からの書き込みも SSE に届く**。通知は「変わったから再取得」のヒントのみで内容は含まない。
- kind: `comment`/`thread`/`reply`/`answer`/`status`/`edit`/`new`/`delete`/`remove`/`resync`。subscriber のバックログが溢れたら `resync` 1 件に圧縮。
- フロントは `live-updates.js` + フルメント取得エンドポイント（`/issues/{id}/fragments/comments|article`、`/discussions/{id}/fragments/threads|replies|article`、`/pulls/{id}/fragments/comments|article`）で差分再描画。
- **部分更新**: コメント・answer・status フォームは `data-partial` 属性付き。JS ありでは `partial-submit.js` が `fetch` + `X-Partial` ヘッダで POST し返却 HTML fragment を差し替え（view transition 内）。JS なしでは通常の PRG ラウンドトリップにフォールバック。

### 3.10 i18n

出典: `internal/web/i18n.go`、`README.md`。

- UI 文字列は `en`（デフォルト/フォールバック）と `ja` の 2 ロケール。ユーザー投稿は未翻訳。
- 解決順: `lang` クッキー > `Accept-Language`（日本語優先度判定）> デフォルト `en`。
- `?lang=<code>` でクッキーに保存して同 URL へリダイレクト。言語スイッチャは `GET /locale?lang=<code>` + `next` パラメータ。
- レスポンスに `Vary: Accept-Language` を付与（共有キャッシュ対策）。
- ローカライズ対象はサーバーテンプレートと JS 用メッセージ（`<html data-i18n>` → `/static/i18n.js`）の両方。

### 3.11 JSON API（`/api/v1`）と CLI（`zatu`）

出典: `docs/api.md`、`internal/api/api.go`、`internal/cli/`、`README.md`。

- **fail-closed マウント**: `-api-token`/`ZATUDAN_API_TOKEN` または `api_tokens` 行が 1 件以上あるときだけ `/api/v1` をマウント。どちらも無ければ 404。**起動時に一度だけ評価**（稼働中のトークン発行は再起動まで反映されない）。
- 認証: `Authorization: Bearer <token>` 必須。共有トークンは定時間比較（その書き込みは anonymous 扱い）。ユーザー別トークンはそのユーザーの `author_id` で書き込む。
- エラー形式: `{"error":{"message": ..., "fields": {...}}}`（`fields` は 422 のフィールド単位エラーのみ）。既知パス + 誤メソッドは 405 + `Allow` ヘッダ、未知パスは JSON 404。
- タイムスタンプは RFC 3339。status 変更は PATCH ではなく POST アクションエンドポイント。
- `/api/v1/` 以下のリソース構造: workspaces と discussion-categories はグローバル、issues/labels/discussions は `/api/v1/w/{ws}` スコープ、pulls は repo があるときのみ。
- 詳細は `docs/api.md` のエンドポイント表（issues/discussions/labels/workspaces/pulls の CRUD + status/merge/answer/hide アクション + markdown/diff エクスポート）。
- **CLI `zatu`**（`internal/cli/cli.go`）: `issue list|view|create|edit|close|reopen|delete`、`comment create|edit|delete|hide|unhide`、`label list|create|delete`、`discussion list|view|create|edit|delete` + `comment …` + `answer` + `categories`、`pr list|view|diff|create|edit|close|reopen|merge|delete` + `pr comment …`、`workspace list|create|delete`、`auth token [list|revoke]`、`api <path>`（gh 風 `-X`/`-f`/`-F`、リモートのみ）、`completion bash|zsh|fish`。`view --markdown` でエクスポート文書取得。全階層 `-h/--help` 有効、exit code 0/1/2。
- バックエンド 2 系統: ローカル（デフォルト、`-db`/`ZATUDAN_DB` で SQLite 直叩き、サーバー不要）とリモート（`--server`/`--token` または `ZATUDAN_SERVER`/`ZATUDAN_TOKEN` で `/api/v1`）。`pr` 系はローカル時に git repo が必要（`--repo`/`ZATUDAN_REPO` or `-db` 近傍の `.git` 自動検出）。`--workspace`/`ZATUDAN_WORKSPACE` で workspace 指定（デフォルト `default`）。

### 3.12 Markdown エクスポート

出典: `internal/export/`、`internal/web/markdown.go`。

- 1 リソース = 1 markdown 文書。issue + comments、discussion + threads + replies、PR + commits + review comments。serializer は web/API/CLI 全 surface で `internal/export` を共有。
- メタデータブロック（番号・workspace・status/category・author・labels・作成/更新日時）付き。qa カテゴリのみ `Answered:` 行。answer thread の見出しに `· answer` サフィックス。hide/編集済み投稿は `moderationSuffix` が付く。
- `GET /w/{ws}/issues|discussions/{id}/markdown`、`GET /pulls/{id}/markdown`（web）と API 対応エンドポイント。`text/markdown; charset=utf-8` + `Content-Disposition: inline`（ダウンロードではなくクリップボード用途）。
- 詳細ページの "Copy as markdown" ボタンがこのエンドポイントを fetch。

### 3.13 Markdown 描画・セキュリティ・その他横断仕様

出典: `docs/ui.md`、`internal/web/web.go`。

- 本文・コメントは生テキストで保存し、表示時に Markdown（goldmark GFM + GitHub 風 `> [!NOTE|TIP|IMPORTANT|WARNING|CAUTION]` アラート）描画。**生 HTML は出力されず、`javascript:`/`data:` 等の不安全リンクスキームは無力化**（stored XSS 不可）。
- PRG: 全 mutation は成功時 `303 See Other`。
- バリデーション失敗は同フォームを **422** で再描画（入力保持 + `aria-invalid`/`aria-describedby` のフィールドエラー）。
- CSRF: `http.NewCrossOriginProtection()`（`Sec-Fetch-Site`/`Origin` でクロスサイト非 GET を拒否）。POST ボディは `http.MaxBytesReader` で 1 MiB 上限（超過は 413）。
- セキュリティヘッダ: CSP（インライン script 禁止）、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: same-origin`、COOP/CORP、Permissions-Policy。
- 存在しない issue/discussion は装飾付き 404。ServeMux のメソッドパターンで 405 自動。
- フィルタ状態は全て URL クエリに乗る（リンク共有可能・ページネーションで条件保持）。

## 4. データ・バリデーション

出典: `internal/store/validate.go`（web フォームと API が共通関数を呼ぶ設計）、`internal/web/auth.go`、`internal/store/moderation.go`。

| フィールド | ルール | エラーコード |
|---|---|---|
| issue/discussion/PR title | 必須、200 文字（rune）以内 | `required` / `too_long` |
| issue/discussion/PR body | 任意、10,000 文字以内 | `too_long` |
| コメント body（全種） | 必須、10,000 文字以内 | `required` / `too_long` |
| discussion category | 必須（存在するカテゴリ ID。不正は `ErrInvalidCategory`） | `required` |
| label name | 必須、64 文字以内 | `required` / `too_long` |
| label color | `^#[0-9a-fA-F]{6}$` のみ | `hex_color` |
| workspace slug | 必須、`^[a-z0-9][a-z0-9_-]{0,62}$`（一意、重複 `ErrDuplicateSlug`） | `required` / `pattern_slug` |
| workspace name | 任意（空なら slug が表示名）、200 文字以内 | `too_long` |
| workspace description | 10,000 文字以内 | `too_long` |
| PR base/head ref | 両方必須、同一は `ErrSameRef` | `required` |
| アカウント name | 必須、100 文字以内、大小無視で一意 | フォームエラー / `ErrDuplicateName` |
| パスワード | 必須、8〜256 文字（rune） | フォームエラー |
| hide reason | `spam`/`abuse`/`off-topic`/`outdated`/`duplicate`/`resolved`/`low-quality` のいずれか必須 | `ErrInvalidHideReason` |
| PR コメント anchor | `path` と `line` は両方セット or 両方空 | `ErrInvalidAnchor` |
| issue status | `open` / `closed` のみ | フォームエラー / store エラー |
| PR status 遷移 | open→closed/merged、closed→open。merged は終端 | `ErrInvalidPRTransition` |
| merge method | `merge` / `squash` / `rebase` | エラー |
| status（issue 作成） | 常に `open` で作成（作成時に指定不可） | — |

store 層の代表的 sentinel エラー: `ErrNotFound`、`ErrDuplicateSlug`、`ErrDuplicateLabel`、`ErrDuplicateName`、`ErrInvalidCredentials`、`ErrInvalidCategory`、`ErrInvalidThread`、`ErrNotAnswerable`、`ErrInvalidHideReason`、`ErrDuplicatePR`、`ErrInvalidPRTransition`、`ErrSameRef`、`ErrInvalidAnchor`、`ErrLastWorkspace`。

## 5. シードデータ

出典: `internal/store/seed.go`、`internal/store/store.go`（`defaultLabelsDDL`）。

**マスターデータ**（`-seed` なしでも毎回投入・冪等）:

- workspace `default`（workspace が 0 個のときのみ作成。削除された default を他 workspace がある状態で復活させない）
- discussion カテゴリ 4 件: `announcements` / `general` / `ideas` / `qa`（format は open/open/open/qa。`init` でも投入。テーブルが空のときのみ）
- issue kind マスタラベル 4 件: `Bug` / `Documentation` / `Enhancement` / `Feature`（スキーマ DDL 側で常時確保）

**デモデータ**（`-seed`/`ZATUDAN_SEED` 時のみ。issues テーブル・discussions テーブルがそれぞれ空のときだけ投入）:

- issue 7 件（open 5 / closed 2、ラベル・コメント付き。コメント計 11 件）、投入先は `default`（なければ最初の workspace）
- discussion 4 件（announcements/general/ideas/qa 各 1 件、threads + replies 付き。qa の 1 件は answer マーク + reply あり）
- デモユーザー `demo` / パスワード `demo`（users に無ければ常に投入 — issues が既存でもログイン可能にするため）

## 6. 受け入れ観点（SPA 移植チェックリスト）

### 全体・横断

- [ ] issue/discussion/PR の一覧・詳細・作成・編集・削除・status 切替ができる
- [ ] 状態は URL（パス + クエリ）に反映され、フィルタ済みビューがリンク共有可能
- [ ] バリデーション失敗時にフィールド単位のエラーと入力保持が出る（MPA の 422 再描画相当）
- [ ] 削除に確認ステップがある（MPA の確認ページ/`<dialog>` 相当）
- [ ] 存在しないリソース・未知 workspace は 404 相当の表示
- [ ] issue/discussion 本文と各種コメントが Markdown（GFM + アラート）で描画され、生 HTML / 不安全リンクが無力化される
- [ ] workspace スコープ: `/w/{slug}` 相当のコンテキストがあり、workspace なしアクセスは default/先頭へ誘導、別 workspace のリソースは正しいスコープへ誘導（mutation は拒否）
- [ ] ログイン中は投稿に投稿者名が付き、未ログインでも全操作可能（anonymous 表記）

### Issues

- [ ] 一覧で status（open/closed/all）・`q`（title/body 部分一致）・label・sort（newest/oldest）・page でフィルタできる（デフォルト open・newest・20 件/ページ）
- [ ] ラベルの付け替えができ、archived ラベルはピッカーに出ないが既存付与は残る
- [ ] open/closed トグルができる（無変更トグルで updated_at を汚さない）
- [ ] issue + コメントを 1 markdown 文書としてエクスポート/コピーできる

### コメント・モデレーション

- [ ] コメント投稿・編集（実変更で edited マーカー）・削除ができる
- [ ] hide で理由（7 種固定）を必須選択し最小化できる。unhide で復帰
- [ ] コメントのパーマリンクが取れる（`#comment-N` 相当）

### Labels

- [ ] kind（issue/discussion）とスコープ（共有/workspace）の概念があり、一覧は共有 + 自 workspace の和集合
- [ ] 作成（name + hex color + kind + scope）・編集（name/color のみ）・アーカイブ/復元ができる
- [ ] 同名重複はエラー、archived 同名は復元になる
- [ ] 付与可能ラベルは「共有 or 自 workspace・対応 kind・非 archived」に限定される

### Discussions

- [ ] カテゴリ 4 種から選んで作成できる（必須）、カテゴリでフィルタできる
- [ ] thread 投稿・thread への reply（1 段ネスト）ができる
- [ ] qa カテゴリでのみ answer マーク/解除ができ、1 discussion 最大 1 件で印が移る
- [ ] answered yes/no でフィルタできる
- [ ] thread/reply の編集・削除（thread は replies ごと）・hide/unhide ができる
- [ ] 非 qa カテゴリへの編集で answer マークが消える

### Pull Requests（repo が結合されている場合）

- [ ] repo 未検出時は PR 機能ごと非表示/404
- [ ] base/head を指定して作成できる（同一 ref・重複 open PR はエラー）
- [ ] 一覧で status（open/closed/merged/all）・q・sort・page フィルタ
- [ ] 詳細で commits・merge 可否（コンフリクト表示）が見える
- [ ] diff（Files changed）を見て、diff 行アンカー付きレビューコメントを付けられる（path+line セット必須、side left/right）
- [ ] close/reopen、merge（merge/squash/rebase + 任意メッセージ）ができる。merged は終端
- [ ] PR + コメントの markdown / diff エクスポート

### アカウント

- [ ] register（8〜256 文字パスワード）→ 即ログイン、login/logout ができる
- [ ] セッションはクッキー（HttpOnly・SameSite=Lax）で持続・スライディング更新

### API・CLI・その他

- [ ] `/api/v1` 相当の JSON API が Bearer 認証付きで提供され、credential なしでは fail-closed
- [ ] API エラーが `{"error":{"message","fields"}}` 形式
- [ ] `zatu` 相当の CLI がローカル/リモート両バックエンドで動く
- [ ] en/ja の UI 言語切替がある
- [ ] SSE 等による他クライアント変更のライブ反映がある（MPA の EventSource + fragment 相当）
