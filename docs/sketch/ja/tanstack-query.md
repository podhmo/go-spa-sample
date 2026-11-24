# TanStack Query 導入スケッチ — 評価記録

2026-11-24。`sketch/tanstack-query` ブランチで issues feature を
`@tanstack/react-query@5.107.1` (公開: 2026-11-06、7日ルール適合) に
移植し、採否判断の材料を集めた。**本稿は評価メモであり、master への
マージ可否はここでの結論を参照して別途決める。**

## 採用した構成 (公式推奨の Router + Query 統合)

- `src/queries.ts` (新規): query key の階層ファクトリ `issueKeys`、
  `queryOptions` で包んだ `issueListQuery` / `issueQuery`、
  `createQueryClient()` (staleTime: 30s, retry: 1)
- Router context に `queryClient` を載せる
  (`createRootRouteWithContext<{ queryClient: QueryClient }>`)。
  loader は `context.queryClient.ensureQueryData(issueListQuery(...))` を呼ぶ
- component は `useSuspenseQuery(同じ queryOptions)` でキャッシュから読む。
  `useLoaderData` は使わない
- mutation は `useMutation` + `onSuccess: invalidateQueries(issueKeys.all(WS))`。
  `useTransition` / `router.invalidate()` は全廃
- `main.tsx` / `test-utils.tsx`: `QueryClientProvider` で wrap。
  テストは **render 毎に新しい QueryClient** (キャッシュ汚染防止)

### Before / After

読み込み側 (一覧):

```tsx
// before: loader が直接 fetch、component は loader data を読む
loader: ({ deps }) => api.listIssues(WS, deps),
component: function IssuesPage() {
  const data = issuesRoute.useLoaderData()
  ...
}

// after: loader は QueryClient に「保証」させ、component は cache 読み
loader: ({ context, deps }) =>
  context.queryClient.ensureQueryData(issueListQuery(WS, deps)),
component: function IssuesPage() {
  const search = issuesRoute.useSearch()
  const { data } = useSuspenseQuery(issueListQuery(WS, search))
  ...
}
```

更新側 (詳細ページの status 変更):

```tsx
// before: useTransition + 手書き try + router.invalidate() で全loader再実行
const [isPending, startTransition] = useTransition()
onSetStatus={(status) => {
  startTransition(async () => {
    await api.setIssueStatus(WS, Number(issueId), status)
    await router.invalidate()
  })
}}

// after: useMutation + キー指向の invalidate (list/detail 双方が対象)
const statusMutation = useMutation({
  mutationFn: (s) => api.setIssueStatus(WS, id, s),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: issueKeys.all(WS) }),
})
onSetStatus={(status) => statusMutation.mutate(status)}
```

作成フォームの 422 フィールドエラーは「`useReducer` + try/catch」から
「`createMutation.error` から導出する純粋関数」に変わり、
ローカル状態がひとつ消えた (useReducer-only 規約との相性も良い)。

## DX 比較

| 観点 | loader のみ (before) | + TanStack Query (after) |
| --- | --- | --- |
| pending | `useTransition` の `isPending` (mutation 単位で区別不可) | `mutation.isPending` が mutation 毎に独立。合成も容易 |
| mutation エラー | try/catch + useReducer で手動保持 | `mutation.error` に自動保持、導出で済む |
| 再取得 | `router.invalidate()` = 全 loader 再実行 (粗い) | `invalidateQueries({queryKey})` で prefix 単位に絞れる |
| キャッシュ | router 内蔵の短期キャッシュのみ | staleTime/gcTime/条件付き無効化まで制御可能 |
| search params 連動 | loaderDeps がそのまま fetch 引数 | 同じ deps が queryKey に入るので key 設計と一致させやすい |
| 二重管理 | loader が単一の取得経路 | loader (ensure) と component (useQuery) の2層。`queryOptions` 共有で drift は防げるが層は増える |
| 新規ボイラープレート | なし | key ファクトリ + context 配線 + Provider (≈+50行) |

### `useQuery` vs `useSuspenseQuery`

loader が `ensureQueryData` で先に詰める前提なら **useSuspenseQuery が正**。
`useQuery` だと `data` が `T | undefined` になり「loader済みなのに
undefined チェック」という不自然な分岐が残る。suspense 版は `data: T`
で確定し、loader 未経由の直接 mount でも suspense fallback に倒れる
(今回 defaultPendingComponent 未設定だが router が担保)。

## ハマりどころ / 注意点

1. **staleTime: 0 (default) だと二重 fetch になる。**
   loader の `ensureQueryData` で入れたデータは即 stale 扱い →
   mount 時の `useSuspenseQuery` が background refetch を発火する。
   「loader+Query」統合では staleTime を明示 (今回 30s) するのが実質必須。
   一方で staleTime を上げると「invalidate 漏れで古い画面」リスクが増す。
   **キャッシュ戦略という新しい設計論点が生まれる**のが最大のコスト。
2. **QueryClient の寿命。** Provider の client は module レベルで1つ
   (SPA) だが、テストでは `renderAt` 毎に `createQueryClient()` しないと
   前テストのキャッシュが残る。router context 経由にしたおかげで
   「client を差し替える口」が型付きで用意できた。
3. **key の同期。** loader が `deps` (loaderDeps の戻り値)、
   component が `useSearch()` の結果を key に使う。両者が同じ shape
   (validateSearch が常に `{status, q}` を返す) なので一致したが、
   **deps と key の構成がずれると静かに別エントリができる**。
   `queryOptions` に集約したのが防衛策。
4. **invalidate の粒度設計。** `issueKeys.all(WS)` で list+detail を
   一括 invalidate する形にした。key 階層を先に決めないと
   「detail は直ったのに一覧が古い」が起きる。削除時は
   `removeQueries(detail)` + `invalidate(lists)` + navigate の3段。
5. `mutate` の第2引数 `onSuccess` (作成→詳細へ navigate) と
   hook 定義側の `onSuccess` (invalidate) は両方呼ばれる。
   直感的だが「どちらに書くべき処理か」の規約は必要。

## バンドルサイズ

`vite build` 出力 (この worktree で HEAD と同一条件で計測):

| | raw | gzip |
| --- | --- | --- |
| before (546fa67) | 304.44 kB | 97.48 kB |
| after | 340.55 kB | 107.71 kB |
| **差分** | **+36.1 kB** | **+10.2 kB** |

## 検証結果

- `npm run format` / `npm run lint` / `npm run build` / `npm test`: 全て pass
- 既存 5 テストは test-utils 変更のみで無修正で通過

## 結論: **現時点では見送り寄り (ただし条件付きで採用価値あり)**

正直なところ **五分五分より少し「見送り」側**。理由:

- **得られたものは本物**: `useMutation` の pending/error 内蔵で
  useTransition+try/catch の定型が消え、422 エラーの `useReducer` まで
  不要になった。invalidate が key prefix で狙えるのも
  `router.invalidate()` の全再実行より洗練されている。
- **しかし現規模では router loader で不足していなかった。**
  list/detail + 4 mutation 程度なら before のコードで欠点は
  「pendingが1本化される」「invalidate が粗い」だけで、どちらも
  実害は出ていない。対価として +10KB gzip、key 階層設計、
  staleTime という新設計論点、2層の取得経路が増える。
- **採用に傾くトリガー** (これらが出たらこのブランチを土台にする):
  - リソース種別が増えて `issueKeys` 的な key 管理がむしろ整理に寄与する
  - 楽観的更新・ポーリング・無限スクロール等 Query 固有機能が欲しい
  - 楽観 UI や prefetch-on-hover など「キャッシュを能動的に使う」要求

アーキテクチャ上の懸念だった「loader と Query の二重ソース」は、
`queryOptions` を loader と hook で共有する公式パターンで十分に
軽減できることが確認できた (drift の余地は key 構成のずれのみ)。
つまり技術的な不整合ではなく、**コスト対効果の問題**として
判断できる状態になった、というのがこのスケッチの成果。
