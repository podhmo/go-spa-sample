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
| サーバーデータの読み込み | Router loader | TanStack Query |
| 更新後の再取得 | `router.invalidate()` | `queryClient.invalidateQueries()` |
| ローカルUI状態 | `useReducer` | (useStateは禁止) |
| pending表示 | `useTransition` / loader保留中のUI | `useMutation.isPending` |
| フォーム | 素のform+useReducer → 複雑化したら TanStack Form | — |
| CSS | CSS Modules + app.css (`docs/sketch/ja/css.md`) | Panda CSS (将来) |
| Web標準の実装指針 | modern-web-guidance | — |
| ライブラリ仕様の確認 | context7 | 公式docs直接 |
