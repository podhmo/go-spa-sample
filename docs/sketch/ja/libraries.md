# ライブラリ用法の整理

issue tracker のSPA実装で使うライブラリ・スキルの使いどころ。
内容は context7 MCP で取得した最新ドキュメントが元 (取得日: 2026-11-24)。

## ドキュメントの引き方

- **context7 MCP** (`.devin/mcp_config.json`): `resolve-library-id` → `query-docs` の2段階。
  ライブラリ名の曖昧さを解決してから、1トピック1クエリで引く
- **modern-web-guidance** (`.agents/skills/`): Web標準 (HTML/CSS/JS) の実装前に
  `npx -y modern-web-guidance@latest search "<2-5語のクエリ>"` → `retrieve "<id>"`。
  フレームワーク名はクエリに入れない

## TanStack Router (`@tanstack/react-router`)

採用済み。code-based routing (`createRootRoute` + `createRoute`) を使用中。
出典: https://tanstack.com/router/latest/docs/

- **データ取得は loader**: route に `loader: () => fetchJSON(...)` を書き、
  component では `Route.useLoaderData()` で読む。useEffect+useStateでの
  fetchはやらない (useState禁止とも整合)
- **更新後は `router.invalidate()`**: mutation成功後に呼ぶと、コミット済み・
  キャッシュ済み・in-flight のloader generationを全てinvalidateする。
  古いデータは裏で再検証され、表示は新しいデータが来るまで維持される。
  完了を待つなら `router.invalidate({ sync: true })`
  (ref: https://tanstack.com/router/latest/docs/guide/data-mutations)
- **エラー時の再試行も `router.invalidate()`**: errorComponent の retry でも使う
  (error boundaryのresetも兼ねる)
- **短期キャッシュ内蔵**: loaderデータはmatchがunmountされてもキャッシュに残る。
  手動キャッシュ管理は基本不要
- **検討中**: file-based routing (`createFileRoute`) は route 数が増えたら移行候補

## TanStack Query (`@tanstack/react-query`) — 未導入・要判断

出典: https://tanstack.com/query/latest/docs/framework/react/quick-start

- `QueryClientProvider` で `QueryClient` を提供 → `useQuery({queryKey, queryFn})`、
  mutation は `useMutation({mutationFn, onSuccess: () => queryClient.invalidateQueries(...)})`
- Router loader と役割が被る。導入する動機になりうるもの:
  キャッシュの細かい制御 (staleTime)、楽観的更新、ポーリング、
  invalidateのkey粒度。zatudanの規模なら loader+invalidate で足りる可能性が高い
- **判断ポイント**: 「loaderだけだと物足りない場面 (再取得タイミング・楽観更新) が
  出たら導入」を既定方針とする。issues UIを書き始めてから決める

## TanStack Form (`@tanstack/react-form`) — 未導入・フォーム実装時に検討

出典: https://github.com/tanstack/form (docs/overview.md)

- `useForm({defaultValues, onSubmit})` + `<form.Field>` render-prop +
  `<form.Subscribe>` の構成。`defaultValues` から型推論される
- バリデーションは field の `validators` (`onChange`, `onChangeAsync` +
  `onChangeAsyncDebounceMs`) または form レベル。`onSubmitAsync` で
  サーバーの422エラーを `{form, fields: {...}}` 形で各fieldに割り振れる
  — zatudan APIの `{"error":{"fields":{...}}}` と形が合うので相性は良さそう
- issue作成・編集フォームを書く段になったら採用を判断
  (素の `<form>` + useReducer でも十分小さい可能性はある)

## React の非同期パターン (Async React)

出典: https://react.dev/reference/react/use, /useTransition, /Suspense

- **`use()` + `<Suspense>`**: Promiseをcomponent内で直接読める。
  ただし公式も「キャッシュはフレームワーク側が持つのが普通」と明記
  → Router loaderがあるなら素直にloaderを使い、Suspenseは
  pending UIの制御が必要な場面で検討
- **`useTransition`**: `startTransition(async () => ...)` で
  `isPending` が取れる。actionをawaitできるのでmutation中の
  pending表示に使える (useStateなしでisPendingが持てるのは嬉しい)
- **`useReducer`**: ローカル状態の正 (idea.mdのuseState禁止の受け皿)。
  dispatchを子に渡す形で設計する

## UIの確認方法: Storybook vs Vitest browser mode

検討の結果、**Vitest browser mode を主軸**とする (2026-11-24、実際に動作確認済み)。

比較:

- **Vitest browser mode**: componentを実ブラウザでrenderしてassertできる
  = 「表示確認」と「テスト」が一本化される。providerにplaywrightを使い、
  playwright-cliと同じsystem Chromeを `channel: 'chrome'` で共有
  (追加DLなし)。ARIA snapshot・visual regression・trace viewの機能もある。
  `server.proxy` が効くので `/api` 依存componentのテストもやりやすい
- **Storybook**: バリアントのカタログ閲覧・デザイン反復に強いが、
  依存が重く設定がもう一系統増える。元repoの自作storybookは
  Goテンプレートをブラウザに出すためのもので、SPAでは役割が違う

判断:

- component単位の見た目・挙動確認 → Vitest browser mode (第一選択)
- アプリ全体の画面遷移・E2E → playwright-cli + dev server
- Storybookは「バリアント一覧を作りたくなった」時点で再検討

## まとめ: 役割分担のたたき台

| 関心事 | 第一候補 | 代替 |
| --- | --- | --- |
| UI確認 (component単位) | Vitest browser mode | Storybook (将来) |
| UI確認 (アプリ全体) | playwright-cli | — |
| サーバーデータの読み込み | Router loader | TanStack Query (評価済み・見送り: `docs/sketch/ja/tanstack-query.md`) |
| 更新後の再取得 | `router.invalidate()` | `invalidateQueries()` (Query採用時) |
| ローカルUI状態 | `useReducer` | (useStateは禁止) |
| pending表示 | `useTransition` / loader保留中のUI | `useMutation.isPending` |
| フォーム | 素のform+useReducer → 複雑化したら TanStack Form | — |
| Markdown描画 | react-markdown + remark-gfm + remark-github-blockquote-alert (構造型サニタイズ、innerHTML不使用) | marked+DOMPurify (評価済み・乗り換え: `docs/sketch/ja/markdown.md`) |
| CSS | CSS Modules + app.css (`docs/sketch/ja/css.md`) | Panda CSS (将来) |
| Web標準の実装指針 | modern-web-guidance | — |
| ライブラリ仕様の確認 | context7 | 公式docs直接 |

## modern-web-guidance 適用パターン (実装済み)

`npx -y modern-web-guidance@latest search/retrieve` で取得したガイドを
SPA に適用したもの:

- **削除確認はネイティブ `<dialog>`** — `components/ConfirmDialog.tsx`。
  `commandfor` + `command="show-modal"` の宣言的オープン (Invoker Commands;
  React の型にまだないので attribute spread で渡す)、閉じる側は
  `<form method="dialog">` + `returnValue` で判定。安全側の cancel を先に
  置き初期フォーカスを受けさせる。`closedby="any"` は ref 経由で付与
  (未対応ブラウザでも Esc/ジェスチャで閉じるため graceful)
- **フォームエラーの aria 同期** — `src/a11y.ts`。
  `:user-invalid` の視覚状態と `aria-invalid`/`aria-describedby` を
  document 直下の delegated listener (blur/input/change/invalid/reset) で
  同期。エラー表示はフィールド横の `.error-msg` (`data-error` で id 参照、
  CSS の `:user-invalid ~ .error-msg` で表示制御) + ページ単位の
  `aria-live="polite"` announcer。サーバー由来のエラーは React 側が
  `aria-invalid="true"` + `aria-describedby` を直接出す (script は触らない)
- **`<search>` で絞り込みフォームを囲む**、`type="search"`、
  visually-hidden label で全コントロールに名前を持たせる
- **フォーム群に `aria-label`** — ページに複数フォームがあるため
- **required は `required` 属性 + ラベルの `*` (aria-hidden)**、
  select の auto-submit はしない (Apply ボタン明示)
- **CSS**: `font: inherit`/`font-size: max(1rem,1em)`、タップターゲット
  `min-height: 2.5rem`、`dialog:modal`/`::backdrop` の
  `overscroll-behavior: contain` (scroll chaining 防止、未対応は無害)

### parity実装で追加適用したもの (features.md parity対応時)

- **hide理由 select は `required` + disabled placeholder** — 先頭値 (spam)
  の暗黙選択を防ぎ、理由の明示選択を強制 (`select-menu-interaction` /
  `required-field-feedback` ガイド)
- **コピーボタンのフィードバックはボタン外の `role="status"` bubble**
  (`copy-button` ガイド) — ボタン内のライブリージョンは確実に読まれない。
  `opacity` で表示制御 (a11y tree に残す)、timer は `useReducer` の
  state に持ち clearTimeout で重複防止、feedback中もボタンは無効化しない
- **コメントの編集/削除/hide は `<dialog>` + `form method="dialog"` +
  `returnValue` パターンを再利用** (ConfirmDialog と同型)

### MPA parity 実装で追加適用したもの (`docs/sketch/ja/ui-parity.md`)

- **Ctrl+Enter / Cmd+Enter でフォーム送信** — MPA の `enter-submit.js`
  (ime-safe-enter-submit ガイド) の React 版 `submitOnCtrlEnter`
  (`src/enter-submit.ts`)。modifier combo は IME composition と衝突
  しないので isComposing チェック不要
- **textarea の `field-sizing: content`** + `lh` 単位の上下限 —
  `rows=` をフォールバックに残す MPA 方式
- **hover-reveal は `opacity` のみ** (`.copy-btn`) — `display:none`/
  `visibility:hidden` は a11y tree から消えるため、常時 focus可能に
- **navbar ガイド**: sticky header + skip-link + `aria-current` な
  nav を `.site-header`/`.site-nav` 語彙で再現
