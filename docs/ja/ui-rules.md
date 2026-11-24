# UI ルール（ui-rules）— SPA 版

本アプリ（SPA + JSON API 構成）の UI 規約を 1 箇所に集約した文書。
[go-classic-mpa-sample の docs/ja/ui-rules.md](https://github.com/podhmo/go-classic-mpa-sample/blob/main/docs/ja/ui-rules.md)
（外部プリミティブ・デザインシステム チェックリスト全 41 項目との監査済み）
を元に、SPA 構成での「正」・採用した代案・変更予定・N/A を分けて記載する。

MPA との最大の差分: サーバ描画 + PRG (POST→303→GET) が、
loader + mutation + `router.invalidate()` + `navigate()` に置き換わる。
画面遷移・フォーカス・タイトル・スクロールは SPA 側で管理する
（`usePageTitle` / `onResolved` で `#main-content` へ focus /
`scrollRestoration` / `route-progress`）。規約の意図は MPA 版を継承し、
実装手段だけ SPA 向きに読み替える。

## 凡例

- **正** — 現行実装がそのまま規約。新規 UI もこれに従う。
- **正（代案）** — MPA 版の指定形とは異なるが、独自のやり方として採用したもの。
  代案・独自判断の項目には「元の規定:」として MPA 版が求めていた形を付記する。
- **変更予定** — 違反・不統一として認識しているもの。対応タスクは `TODO.md` にある。
- **N/A（対象外）** — 該当する UI・機能が存在しない項目。将来機能を足すときの参考として残す。

## 1. ボタンとアクション配置

- [正] **確定系は右、取り消し系は左、右寄せ配置**。`.form-actions` が
  `justify-content: flex-end` を持ち、作成/編集フォーム・`<dialog>`・
  返信フォームのすべてで「Cancel → 確定（`<button type="submit">`）」
  の順に統一する。Cancel は `<a>`、`<button type="button">`、または
  `<dialog>` 内の `method="dialog"` フォームの `value="cancel"` submit
  （`formmethod="dialog"` は送信せず dialog を閉じるだけの submit なので
  例外的に submit 型のまま使う。`ConfirmDialog` がこの形）。
- [正] **1 画面 1 プライマリ**（ページ内アクションに限る）。
- [正] **グローバルヘッダの "New issue" / "New discussion" は枠線 `.btn`**。
  Primary はページ内アクション 1 つのみ。
- [正] **破壊的アクションは `btn-danger`（枠線型・hover で塗り）で抑制し、
  必ず確認を挟む** — SPA では native `<dialog>`（`ConfirmDialog.tsx`、
  `command`/`commandfor` または `showModal()` 経由）を使う。GET 確認ページは
  SPA では持たない（代案としてモーダルに一本化）。
- [正] **削除モーダルは Cancel に `autoFocus`**。Esc・Cancel・背景クリックで閉じる。
- [正] **ボタンラベルは具体動詞**（"Delete issue" "Close issue" "Start thread"）。
  "OK" "はい" 等の抽象語は使わない。i18n 後も en/ja 両方でこの方針を守る。
- [正（代案）] **entity の編集は専用 `/edit` ページ**（issue/discussion/pull の
  タイトル・本文）。**コメント・返信の編集はカード内インライン**
  （`editReducer` の `editing` + ドラフト。MPA では専用ページリンクだったが、
  SPA では画面遷移コストが小さいためカード内フォームを採用）。
  （元の規定: インライン編集は持たず専用 /edit ページで代替）

## 2. ダイアログ・通知・アラート

- [正] **モーダルはユーザー判断が必須のときのみ**（現状は削除・マージ確認のみ）。
  Cancel・Esc・背景クリックで閉じられる。`ConfirmDialog` を使い回す。
- [正（代案）] **成功フィードバックは遷移・再描画そのもの**。mutation 成功時は
  `navigate()` + `router.invalidate()` で結果画面へ移すため、MPA の toast
  （成功通知）は持たない。小さなフィードバックは `role="status"`（セクション 13）。
  （元の規定: トーストは投稿成功の確認のみ、8 秒自動消去）
- [正] **エラーや要対処の情報は常駐するインライン表示**（`role="alert"` の
  `<em>` を対象フィールド直下に、`aria-invalid` と同期。`a11y.ts` の
  `:user-invalid` 連携を使う）。

## 3. フォーム・入力

- [正] **ラベルは入力欄の直上**。プレースホルダーをラベル代わりにしない。
- [正] **必須は `*` + "Fields marked * are required." の表記で統一**。
- [正] **エラーは `:user-invalid`（blur 相当）または送信時**に入力欄の直下へ
  赤文言で出す。タイピング中は出さない。
- [正] **Ctrl/Cmd+Enter で送信**（`enter-submit.ts` の `submitOnCtrlEnter` を
  textarea の `onKeyDown` に付ける）。`aria-keyshortcuts` と `.field-hint` で示唆。
- [正] **二重送信防止**: mutation は pending フラグで submit を非活性化
  （`disabled={pending}`）。フラグの取り方は画面によって
  `useTransition` の `isPending` か reducer の `pending` — invalidate を
  伴う mutation は reducer 側（transition 内で supersedes される
  loader suspend を避けるため。AGENTS.md の "Pending UI via
  `useTransition`" と併記するが、本項が詳細側）。失敗時は入力を消さず
  `role="alert"` のインラインエラーを出す（state が残る＝ドラフト保持）。
- [正（代案）] **入力中の離脱ガードは `useLeaveGuard`**（`web/src/leave-guard.ts`）。
  MPA は `beforeunload`（leave-guard.js）で全画面離脱をガードしていたが、
  SPA はアプリ内遷移もあるため `useBlocker`（in-app 遷移を `window.confirm`
  で止める）+ `beforeunload`（ブラウザ離脱）の両対応とした。dirty 判定は
  各フォームが行い、submit した時点で reducer の `submitted` flag を立てて
  ガードを解除する（§13a の送信後遷移をブロックしないため。失敗時は次の
  編集で再武装する）。複数ガードが同時に dirty のときは confirm は一度だけ
  （ページ共通の latch が最初の accept を共有し、遷移完了で再武装）。
  ページ自身を削除した後の遷移は `navigate({ ignoreBlocker: true })` で
  ガードを迂回する（画面が消えるのでドラフトを問い直す意味がない）。

## 4. 一覧（リスト画面）

- [正（代案）] **行構成は「タイトル行 + メタ行」の 2 段**で統一。
  左端ステータスアイコン・担当者アバター・コメント数は持たない
  （メタ行のバッジが状態表示を兼ねる）。
  （元の規定: 最左ステータスアイコン → 中央タイトル+ラベル/#ID/日時/著者 →
  最右アバター・コメント数）
- [正] **詳細へのリンクはタイトルのみ**。行全体はクリック不可。ラベルは
  フィルタ適用の独立リンク。
- [正] **ソート・フィルタはリスト直上**（`.filter-bar`、`<search>` 要素）。
  適用中は Clear リンクを常時表示。フィルタ状態は URL の search param に
  乗せる（SPA: 共有・戻る/進むで状態が復元される）。
- [正] **長い一覧は `content-visibility: auto` + `contain-intrinsic-size`**
  （`.issue-row`、`.comment`）でレイアウトコストを抑制する。

## 5. 詳細（スレッド画面）

- [正（代案）] **単一カラム**。サイドバーに載せるべきメタ情報が機能上存在
  しないため、ラベル等はタイトル下にインライン表示。
- [正] **本文・コメントはカード**で囲み、左上に「名前・日時・copy link」。
- [正] **ネスト返信は親カード内・左縦線インデント・最大 1 段**
  （store 層でも 2 段目の返信を拒否）。
- [正] **時系列は上→下（古い順）**、新規入力欄は一番下。
- [正] **編集済みの投稿はメタ行に `· edited` マーカー**を付ける
  （編集履歴・diff は持たない）。
- [正] **hide された投稿は本文を `<details class="comment-hidden-body">` で畳む**。
  カード自体は消さず `.comment-hidden` で視覚を落とし、理由の注記と
  「Show comment」でアクセス可能に保つ（ページ内検索・スクリーンリーダーから
  見えるよう `display:none` ではなく `<details>`）。

## 6. コメント追加 UI

- [正] **トップレベル入力欄は画面最下部に常時展開**、キャンセルボタンなし。
- [正] **コメント欄が空のとき送信ボタンを Disabled に**
  （MPA: comment-submit-disabled.js 相当）。トップレベル・返信・インライン編集
  とも、本文が空/空白のみのときは `disabled`。Ctrl+Enter（`requestSubmit`）は
  `disabled` を迂回するため `enter-submit.ts` 側でも submit ボタンの disabled を
  見て弾いている。
- [正（代案）] **返信欄は "Reply" ボタンでトグルするインラインフォーム**
  （`aria-expanded` を持つ `.comment-action` ボタン + `replying` reducer
  state。MPA は `<details>` 畳みの擬似入力バーだったが、SPA は state 駆動の
  toggle を採用 — 畳んだ状態でドラフトを保持する点は同じ）。
  （元の規定: `<details>` で畳んだ擬似入力バー）
- [正] **展開時は autofocus、`field-sizing: content` で自動伸縮**、
  右下に「Cancel（左）/ Comment（右）」。
- [正] **複数返信欄の排他制御は設けない**。同時に複数開いたままでよい。
- [正（代案）] **返信キャンセルは入力有無を問わず即座に畳む**。破棄確認は
  出さないが、畳んでもドラフトは reducer state に残る（再展開で戻る）。
- [正] **返信・コメント投稿後は入力欄を初期状態に戻す**（reducer dispatch で
  `replying:false` + draft クリア）。

## 7. メニュー（`...`）と破壊的操作

- [正] **`...` メニューは持たない**。copy link は `CopyButton`（`role="status"`
  のフィードバック付き）、"Copy as markdown" もアクション行の `.btn`。
- [正（代案）] アクション行で Edit と Delete は隣接するが、削除は必ず
  確認モーダル経由のため誤操作防止は確認側で担保する。
  （元の規定: 編集と削除は隣接させない）
- [正] **コメント系のモデレーション（Edit / Delete / Hide / Unhide）は
  投稿カードの `.comment-actions` 行に並べる**。繰り返し出るボタンの
  `aria-label` は対象 ID 込みで一意化する（セクション 10 参照）。
- [正] **hide の理由選択は必須 `<select>`**（7 値: spam / abuse / off-topic /
  outdated / duplicate / resolved / low-quality、i18n は
  `m.comments.hideReasons` マップ）。

## 8. 画面状態・メタデータ

- [正] **Empty**（0件案内+次アクション）と **Error**（ルートの
  `errorElement`/`notFoundComponent` + インライン `role="alert"`）。
  **Loading** はルート遷移中に `<progress class="route-progress">`
  （`useRouterState` の `status === 'pending'`）を出す。mutation 中は
  `disabled` + pending ラベル。
- [正（代案）] **ステータスバッジは「色+アイコン（::before）+テキスト」の
  3点セットで、タイトル行の左横**。（元の規定: タイトルのすぐ下・左寄せ）
- [正] **日時は絶対時刻のみ**（`format.ts`、locale 対応済み）。
  相対時刻・ホバーツールチップは採用しない。
- [正] **アバター機能は持たず、ユーザー名テキストのみ**を一貫して表示。
- [正] **遷移後の document.title / フォーカス / スクロールは SPA 側で管理**:
  `usePageTitle`（`page-title.ts`）で `— zatudan` 付きタイトル、
  `router.subscribe('onResolved')` で pathname 変化時に `#main-content` へ
  focus（`preventScroll`）、`scrollRestoration` で戻る/進むのスクロール復元。

## 9. 視覚的階層・スペーシング・表記

- [正] **余白は偶数 px 基準 + 近接の原則**。4 の倍数グリッドは採用しない。
- [正] **色は semantic token**（`--accent` / `--danger` / `--warn` /
  `--open` / `--closed` / `--text` / `--text-muted`、light/dark 両対応）。
- [正] **タッチ端末は `@media (pointer: coarse)` でボタン・入力を 44px
  以上**に引き上げ。
- [正] **ボタンは動詞、見出し・ラベルは名詞形で統一**。i18n 後も
  en/ja それぞれの辞書でこの構造を守る（`en.ts` が型の正、`ja.ts` が
  全キーを実装する）。
- [正] **リンクは本文テキストと見分けがつく表現にする** — `--accent` 色、
  `.btn` 化、badge/tab 等の affordance、現在地の下線。
- [正] **`.comment-permalink` は常時下線つき**（muted 色 + hover で accent）。

## 10. アクセシビリティ・キーボード操作

- [正] **リンクとボタンの使い分け**: 画面遷移は `<Link>`（`<a>`）、状態変更は
  `<form>` + `<button>` または mutation を呼ぶ `<button>`（Cancel が
  `<a>`/`<button type="button">`、送信が `<button type="submit">` の構造を
  全フォームで維持する）。
- [正] **フォーカスリング**: `:focus-visible` で全対話要素に 2px
  アウトライン。スキップリンクをレイアウト先頭に置く（`.skip-link` →
  `#main-content`）。SPA 遷移で `#main-content` に focus を戻す（セクション 8）。
- [正] **アイコン・装飾は alt 空 or `aria-hidden`**。繰り返し出るボタンの
  accessible name は対象 ID 込みで一意化（"Post comment on thread N"
  パターン、i18n 文字列は `fmt()` で ID を埋める）。
- [正] **現在地表示は色だけに頼らない**: `aria-current="page"` + underline・
  太字の複合（サイトナビ・ステータスタブ・パンくずで共通）。

## 11. 色・テーマ・コントラスト

- [正] **ライト/ダーク両対応**: `light-dark()` + `prefers-color-scheme`
  フォールバックの 2 系統。単一スキームのハードコード色は使わない。
- [正] **塗り要素の文字コントラストは 4.5:1 以上**（`--accent-emphasis` /
  `--danger-emphasis` / `--on-emphasis` 系。`--accent` に直接白文字を載せない）。
- [正] **状態色は形も変える**（Open=中抜きリング、Closed=塗り円の `::before`）。

## 12. タイポグラフィ・モーション

- [正] **フォントは rem 基準**（入力は `font-size: max(1rem, 1em)` で
  iOS ズーム対策）。
- [正] **アニメーションは `prefers-reduced-motion` で無効化できる**
  （`.route-progress` を含む全 transition/animation）。
- [正] **長文の扱い**: `overflow-wrap: anywhere`、一覧タイトルは 2 行
  クランプ、ラベルバッジは ellipsis + hover/focus 展開。

## 13. 操作フィードバック・その他

- [正] **破壊的でない状態変更は `btn-quiet`**（Mark as answer 等）—
  Primary でも Danger でもない第三の階層。
- [正] **コピー等の小さなフィードバックは `role="status"` のインライン
  表示 + 数秒で消去**（`CopyButton` の `.copy-tip`）。
- [正（代案）] **状態変更は mutation の後、画面の動きに応じて
  `router.invalidate()` と `navigate()` を使い分ける**。MPA の
  POST→303→GET（PRG）に相当する。実際には3パターンがある:
  (a) 作成→詳細へ移るものは `navigate()` のみ（遷移先の loader が新規取得
  するので invalidate 不要） (b) 同画面内の状態変更（コメント投稿・
  ステータス切替等）は `router.invalidate()` のみ (c) 編集・削除のように
  mutation→navigate を両方行うものは navigate の前に必ず
  `router.invalidate()` を呼ぶ（`_preloads` 済みの古い preload データが
  出るのを防ぐ）。
  （元の規定: POST → 303 → GET（PRG）、部分更新のアナウンスは 1 系統）
- [正] **ページネーションは Previous/Next + "Page X of Y"**、端では非リンク表記。
  ページ番号は URL の search param に乗せる。
- [正] **エラーページはコード表示 + 戻り先リンク 1 本**。
- [正] **高コストな詳細系リンクは `preload={false}`**（pulls 詳細・files 等の
  loader が重い画面への hover prefetch を抑止。`defaultPreload: 'intent'`
  と併用）。

## 14. モバイル・タッチ

- [正] **タップターゲットは `@media (pointer: coarse)` で 44px 以上**
  （checkbox/radio は `<label>` 行全体のクリックでカバー）。
- [正] **hover 前提の UI を作らない**: hover で初めて現れる操作は
  `@media (hover: hover)` でゲートし、タッチ環境では常時表示にする
  （copy-btn のパターン）。
- [正] **狭幅はコンテナクエリ / `auto-fit` で自然に積む**（filter-bar・
  issue-row）。
- [正] **safe-area-inset 考慮**: 固定要素のオフセットに
  `env(safe-area-inset-*)` を加算、viewport meta に `viewport-fit=cover`。
- [正] **sticky hover 対策**: 装飾的な `:hover` はすべて
  `@media (hover: hover)` でガード。`:is(:hover, :focus-visible)` 複合は
  focus 側を無条件・hover 側をガードに分離。

## 15. ルールの検証方法

- [正（代案）] **見た目・挙動の検証は vitest browser mode + playwright-cli**。
  MPA の自作 storybook に相当するものは持たない。コンポーネント単位は
  `web/src/*.test.tsx`（vitest browser mode、stub API）で、実サーバを
  跨いだ挙動は playwright-cli + `cmd/zatudan -seed` で実機確認する
  （`dev-workflow.md` の component-first + verification loop）。
  （元の規定: 静的な見た目は storybook で、JS 挙動は実アプリで）
- [正] **新しい UI 状態を実装したら対応するテストを追加する**。
  ルールが「守れているか」を追えるようにするのが目的。
- [正] **JS が絡む挙動は実アプリで検証する**。disabled 制御・`<details>` の
  畳み・dialog の開閉は vitest browser mode または playwright-cli で確認する。

## 16. Pull requests（/pulls 系）

- [正] **一覧・詳細・files・merge は issues/discussions と同じ SPA 構造**。
  mutation→invalidate→navigate・`.form-actions` の「左:キャンセル / 右:確定」・
  確認モーダル経由の状態変更、`...` ではなくアクション行の動詞ボタン、は
  共通ルールのまま。
- [正] **files タブのファイル集合は merge-base→head（three-dot）相当**。
  open PR は都度 `git merge-base` で解決し、closed/merged 後は記録済みの
  merge_base/head で固定する。
- [正] **行アンカーコメントはインラインの折りたたみフォーム**（`PullFiles.tsx`、
  anchor path は files 表示の Old/New パス集合のみ受付、範囲外は 422）。
- [正] **hidden コメントは `<details class="comment-hidden-body">
  <summary>Show comment</summary>`**（セクション 5-6 のコメント規約と同型）。
- [正] **merge 確認は conflict（unmergeable）時にフォームを出さない**。
  衝突案内と files への戻りリンクのみ表示し、解消は git 側で行う前提。
- [正] **詳細・files へのリンクは `preload={false}`**（loader が重いため
  hover prefetch を抑止、セクション 13）。

## N/A（対象外）一覧

該当 UI・機能が存在しないため監査対象外とした項目。将来機能を追加する際の
指針として残す。

| チェック項目 | 内容 | 理由 |
| :--- | :--- | :--- |
| 1-6 | コメント＋状態変更ボタンの並び | 「クローズしてコメント」型の併設 UI なし（status toggle は独立したアクション行） |
| 3-5 | Write / Preview エディタタブ | プレビュー機能なし |
| 5-3 | システムイベントの差別化表示 | ラベル変更・クローズ履歴等のイベント表示機能なし |
| 7-1 | メニュー項目の並び順 | `...` メニュー自体が存在しない |
| 8-4 | アバターとユーザー名 | アバター機能・ユーザー詳細/フィルタ導線が未実装 |
| 2-x | トースト通知 | 成功フィードバックは遷移+再描画で代替（セクション 2 参照） |
