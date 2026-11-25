# web ディレクトリ構造の再編案

issue: [podhmo/go-spa-sample#20](https://github.com/podhmo/go-spa-sample/issues/20)

調査日: 2026-11-25。issue 本文は「co-location」「pagesと合う感じ」。
ここでは実装前の検討と移行計画だけを記録する。コードの移動自体は
この文書の方針を確認してから行う。

## 結論

**`web/src/routes/` を切り、TanStack Router の file-based routing に移行する案を推す。**
`pages/` は使わない。

理由:

1. TanStack Router 公式が file-based routing を preferred/recommended とし、
   デフォルトの routes directory も `./src/routes` である。
2. route には page component だけでなく redirect, layout, loader,
   `validateSearch` が含まれる。`pages/` という名前より `routes/` の方が
   実態に合う。
3. ファイルシステムが URL 構造をそのまま表すため、
   「ブラウザで見えているパス → 編集する場所」の対応が強い。
4. `-` prefix のファイル/ディレクトリを route tree から除外できるので、
   route の近くに component, form, test, data helper を co-location できる。
5. generated route tree による型付けと automatic code splitting の恩恵がある。

参考:

- [TanStack Router: File-Based Routing](https://tanstack.com/router/latest/docs/routing/file-based-routing)
- [TanStack Router: File Naming Conventions](https://tanstack.com/router/latest/docs/routing/file-naming-conventions)
- [TanStack Router: Installation with Vite](https://tanstack.com/router/latest/docs/installation/with-vite)
- [TanStack Router FAQ: `routeTree.gen.ts` は commit する](https://tanstack.com/router/latest/docs/framework/react/faq)

## 現状の問題

### 1. `src/routes.tsx` が巨大で責務が混ざっている

`web/src/routes.tsx` は 2,035 行。root layout を含めて 31 件の route definition、
redirect、search validation、loader、mutation、ページ JSX が 1 ファイルに入っている。

主な混在:

- site chrome (`SiteNav`, `LogoutButton`, not-found/error component)
- workspace なし URL から canonical URL への redirect
- `/workspaces`, `/login`, `/register`
- `/w/$ws/issues*`
- `/w/$ws/discussions*`
- `/w/$ws/labels`
- `/pulls*`
- `scoped`, `homeSlug`, `apiFieldErrors`, `str`, `num` などの helper

結果として、1 ページを直すだけでも diff が巨大ファイルに集中し、
コンフリクトしやすく、URL と編集場所の対応も読み取りにくい。

### 2. `src/components/` がフラットで、「誰が使うか」が分からない

現在 13 ファイル。`IssueList`, `DiscussionDetail`, `PullFiles` のような
route 専用 component と、`ConfirmDialog`, `CopyButton`, `Markdown` のような
横断 primitive が同居している。

さらに依存が斜めに入っている:

- `PullDetail.tsx` が `HideDialog` を `DiscussionDetail.tsx` から import
- `LabelsAdmin.tsx` と `forms.tsx` が `LabelBadge` を `IssueList.tsx` から import
- `IssueDetail.tsx` が `StatusBadge` を `IssueList.tsx` から import

これらは「たまたま最初に作られたファイル」への依存で、所有関係を表していない。

### 3. `forms.tsx` がドメイン横断の寄せ集めになっている

`web/src/components/forms.tsx` は 906 行で 6 種類の form を export している。

| form | 主な利用先 | 置き場所の性質 |
| --- | --- | --- |
| `IssueForm` | `/w/$ws/issues/new`, `/w/$ws/issues/$issueId/edit` | issues route に局所 |
| `DiscussionForm` | `/w/$ws/discussions/new`, `/w/$ws/discussions/$discussionId/edit` | discussions route に局所 |
| `PullForm` | `/pulls/new`, `/pulls/$pullId/edit` | pulls route に局所 |
| `WorkspaceForm` | `/workspaces/new` | workspaces route に局所 |
| `AuthForm` | `/login`, `/register` | auth routes に局所 |
| `CommentForm` | issue/discussion/pull detail | 横断 shared component |

`CommentForm` 以外は feature route の近くに置ける。
`CommentForm` は cross-feature なので、無理に co-location せず shared に置く方が自然。

### 4. 「query 的なもの」の置き場所が暗黙的

このアプリは TanStack Query を採用していない
(`docs/sketch/ja/tanstack-query.md`)。そのため query object は存在しないが、
実質的な data fetch/mutation は次に分散している。

- route loader: `api.listIssues`, `api.getIssue`, `api.listDiscussions` など
- search validation: `issueSearch`, `discussionSearch`
- mutation wiring: `api.createIssue`, `api.updateComment`, `router.invalidate()`
- low-level endpoint wrapper: `src/api.ts`

これらが 1 つの `routes.tsx` と 1 つの `api.ts` に集まっているため、
component と data 取得の距離が遠い。

## 選択肢の比較

| 案 | 良い点 | 悪い点 | 評価 |
| --- | --- | --- | --- |
| `src/pages/` | 画面単位として直感的 | redirect/layout/loader は page ではない。TanStack の標準用語ともずれる | 不採用 |
| `src/features/<feature>/` | feature 単位で component/data を閉じやすい | URL 構造が見えない。workspace あり/なしの重複 route とどう対応するか曖昧 | 単独では不採用 |
| `src/routes/` + 現在の code-based routing を分割 | 新しい build plugin 不要。段階移行しやすい | route tree の手動編集が残る。ファイル構成と実際の route tree が一致する保証が弱い | fallback |
| **`src/routes/` + file-based routing** | 公式推奨。URL と一致。型安全性・code splitting・co-location 規約がある | `@tanstack/router-plugin` 追加、generated file、移行時の一括変更が必要 | **推奨** |

「ベストプラクティス」は React 一般には唯一の正解がないが、
この repo は TanStack Router を採用済みなので、framework が公式に提供する
構造を優先するのが最も一貫している。

## 推奨ディレクトリ構成

### 全体

```txt
web/src/
  main.tsx
  routeTree.gen.ts          # TanStack Router が生成。commit する
  api.ts                    # 初期移行では残す
  i18n/
  components/               # cross-feature な UI primitive だけ
    ConfirmDialog.tsx
    CopyButton.tsx
    Markdown.tsx
    HideDialog.tsx
    CommentForm.tsx
    LabelBadge.tsx
  routes/
    __root.tsx
    index.tsx

    login.tsx
    register.tsx
    -auth/
      AuthPage.tsx
      AuthForm.tsx

    -lib/
      scoped.ts             # scoped(), homeSlug()
      search.ts             # str(), num()
      mutation.ts           # apiFieldErrors など共通 mutation glue
    -tests/                 # route tree 横断の integration test
      navigation.test.tsx
      workspace-routing.test.tsx

    workspaces/
      route.tsx             # Outlet のみ
      index.tsx             # /workspaces
      new.tsx               # /workspaces/new
      -components/
        WorkspaceList.tsx
        WorkspaceForm.tsx

    # workspace なし compatibility routes。
    # 各 leaf が canonical /w/$ws/... へ redirect する。
    issues/
      route.tsx             # Outlet のみ。redirect logic は置かない
      index.tsx             # /issues → /w/$ws/issues
      new.tsx               # /issues/new → /w/$ws/issues/new
      $issueId/
        index.tsx           # /issues/$issueId → canonical
        edit.tsx            # /issues/$issueId/edit → canonical
    discussions/
      route.tsx
      index.tsx
      new.tsx
      $discussionId/
        index.tsx
        edit.tsx
    labels.tsx              # /labels → /w/$ws/labels

    pulls/
      route.tsx             # Outlet のみ
      index.tsx             # /pulls
      new.tsx               # /pulls/new
      -components/
        PullList.tsx
        PullForm.tsx
        PullStatusBadge.tsx
      -lib/
        pull-or-not-found.ts
      $pullId/
        index.tsx           # /pulls/$pullId
        edit.tsx            # /pulls/$pullId/edit
        files.tsx           # /pulls/$pullId/files
        -components/
          PullDetail.tsx
          PRCommentList.tsx
          PullFiles.tsx

    w/
      $ws/
        route.tsx           # /w/$ws layout。現在の wsRoute 相当で Outlet のみ
        index.tsx           # /w/$ws → /w/$ws/issues
        issues/
          route.tsx         # Outlet のみ
          index.tsx         # /w/$ws/issues
          new.tsx           # /w/$ws/issues/new
          -components/
            IssueList.tsx
            IssueForm.tsx
            StatusBadge.tsx
          -data.ts          # 必要な shared loader/search helper のみ
          $issueId/
            index.tsx       # /w/$ws/issues/$issueId
            edit.tsx        # /w/$ws/issues/$issueId/edit
            -components/
              IssueDetail.tsx
              CommentList.tsx
        discussions/
          route.tsx
          index.tsx
          new.tsx
          -components/
            DiscussionList.tsx
            DiscussionForm.tsx
            badges.tsx      # AnsweredBadge, CategoryBadge
          $discussionId/
            index.tsx
            edit.tsx
            -components/
              DiscussionDetail.tsx
              ThreadList.tsx
        labels/
          route.tsx
          index.tsx         # /w/$ws/labels
          -components/
            LabelsAdmin.tsx
```

### `route.tsx` を置く意図

directory route では、同じ path prefix の配下は親 route にネストする。
例えば `/w/$ws/issues/new` を `w/$ws/issues/new.tsx` に置くなら、
`w/$ws/issues/route.tsx` を親 layout として明示するのが分かりやすい。

ここでの `route.tsx` は基本的に `<Outlet />` だけを返す薄い layout にする。
一覧ページの loader や redirect を親に移さない。特に legacy compatibility
routes では、親に `/issues` の redirect を置くと `/issues/$issueId` まで
一覧へ redirect されてしまうので、redirect logic は必ず leaf route に残す。

ネストを避けたい場合は `issues_` のような suffix escaping もあるが、
今回は feature 境界がはっきりするため、Outlet-only layout を使う方が読みやすい。

## co-location のルール

### route file に置くもの

- `export const Route = createFileRoute(...)`
- `validateSearch`
- `loader` / `beforeLoad`
- mutation を component callback に配線する薄い page component

route file は「その URL で何が起きるか」の索引にする。
大きな JSX や複雑な編集 UI は component へ出す。

### `-components/` に置くもの

その route 群でだけ使う React component/form/badge。

例:

- `IssueForm` → `routes/w/$ws/issues/-components/IssueForm.tsx`
- `IssueDetail` → `routes/w/$ws/issues/$issueId/-components/IssueDetail.tsx`
- `PullForm` → `routes/pulls/-components/PullForm.tsx`
- `WorkspaceForm` → `routes/workspaces/-components/WorkspaceForm.tsx`

component test も原則同じ場所に `*.test.tsx` として置く。

### `-data.ts` に置くもの

2 か所以上の route から使う loader/mutation composition だけを置く。

例:

- issue new/edit の label 一覧取得
- discussion new/edit の category + label 一覧取得
- pull detail/files の `getPull` 系の組み立て

single-use の loader をわざわざ `-data.ts` に移す必要はない。
loader は route file にある方が `deps` や search との型対応が読みやすい。

### `src/api.ts` は最初は分割しない

`api.ts` は 534 行で大きいが、low-level endpoint wrapper であり、
今回の主因である route/component 混在とは別問題。
最初の移行で一緒に分割すると diff が読めなくなるので、まず残す。

将来さらに co-location を強めるなら、次のどちらかを別途検討する。

- `src/api/client.ts` + `src/api/issues.ts` など domain 別に分割
- feature 所有が明確な endpoint wrapper だけ route の `-data.ts` 側へ寄せる

ただし labels は issue/discussion/labels admin から参照され、comments は
issue/discussion/pull にまたがるため、安易に feature 内へ移すと逆方向依存を作る。
今回は中央の API client を残すのが妥当。

### shared component の判定

2 つ以上の feature route から使うものは、深い route 同士で import し合わず
`src/components/` に置く。

移動/抽出対象:

- `HideDialog` を `DiscussionDetail.tsx` から抽出 → `src/components/HideDialog.tsx`
- `CommentForm` を `forms.tsx` から抽出 → `src/components/CommentForm.tsx`
- `LabelBadge` を `IssueList.tsx` から抽出 → `src/components/LabelBadge.tsx`
- 既存の `ConfirmDialog`, `CopyButton`, `Markdown` は残る

`LabelBadge` は workspace 配下でしか使っていないので `routes/w/$ws/-components/`
に置く選択肢もあるが、label は独立した共通 entity なので shared でよい。

## URL と target file の対応

| URL | target file | 備考 |
| --- | --- | --- |
| `/` | `routes/index.tsx` | home workspace へ redirect |
| `/login` | `routes/login.tsx` | `-auth` を共有 |
| `/register` | `routes/register.tsx` | `-auth` を共有 |
| `/workspaces` | `routes/workspaces/index.tsx` | list/delete |
| `/workspaces/new` | `routes/workspaces/new.tsx` | create |
| `/issues` | `routes/issues/index.tsx` | canonical `/w/$ws/issues` へ redirect |
| `/issues/new` | `routes/issues/new.tsx` | redirect |
| `/issues/$issueId` | `routes/issues/$issueId/index.tsx` | hash を保って redirect |
| `/issues/$issueId/edit` | `routes/issues/$issueId/edit.tsx` | redirect |
| `/discussions` | `routes/discussions/index.tsx` | search を保って redirect |
| `/discussions/new` | `routes/discussions/new.tsx` | redirect |
| `/discussions/$discussionId` | `routes/discussions/$discussionId/index.tsx` | hash を保って redirect |
| `/discussions/$discussionId/edit` | `routes/discussions/$discussionId/edit.tsx` | redirect |
| `/labels` | `routes/labels.tsx` | redirect |
| `/pulls` | `routes/pulls/index.tsx` | list |
| `/pulls/new` | `routes/pulls/new.tsx` | create |
| `/pulls/$pullId` | `routes/pulls/$pullId/index.tsx` | detail/comments/merge/status |
| `/pulls/$pullId/edit` | `routes/pulls/$pullId/edit.tsx` | edit |
| `/pulls/$pullId/files` | `routes/pulls/$pullId/files.tsx` | diff/inline comments |
| `/w/$ws` | `routes/w/$ws/index.tsx` | issues へ redirect |
| `/w/$ws/issues` | `routes/w/$ws/issues/index.tsx` | list |
| `/w/$ws/issues/new` | `routes/w/$ws/issues/new.tsx` | create |
| `/w/$ws/issues/$issueId` | `routes/w/$ws/issues/$issueId/index.tsx` | detail/comments/status |
| `/w/$ws/issues/$issueId/edit` | `routes/w/$ws/issues/$issueId/edit.tsx` | edit |
| `/w/$ws/discussions` | `routes/w/$ws/discussions/index.tsx` | list |
| `/w/$ws/discussions/new` | `routes/w/$ws/discussions/new.tsx` | create |
| `/w/$ws/discussions/$discussionId` | `routes/w/$ws/discussions/$discussionId/index.tsx` | detail/threads/replies |
| `/w/$ws/discussions/$discussionId/edit` | `routes/w/$ws/discussions/$discussionId/edit.tsx` | edit |
| `/w/$ws/labels` | `routes/w/$ws/labels/index.tsx` | admin |

## 現行ファイルの移動先

| 現行 | 移動先/扱い |
| --- | --- |
| `routes.tsx` | 廃止。責務を `routes/**` と `routes/-lib/` へ分配 |
| `components/forms.tsx` | 廃止。feature form に分割。`CommentForm` は shared へ |
| `components/IssueList.tsx` | `routes/w/$ws/issues/-components/`。`StatusBadge` もここ |
| `components/IssueDetail.tsx` | `routes/w/$ws/issues/$issueId/-components/` |
| `components/DiscussionList.tsx` | `routes/w/$ws/discussions/-components/` |
| `components/DiscussionDetail.tsx` | `routes/w/$ws/discussions/$discussionId/-components/` |
| `components/PullList.tsx` | `routes/pulls/-components/`。`PullStatusBadge` もここ |
| `components/PullDetail.tsx` | `routes/pulls/$pullId/-components/` |
| `components/PullFiles.tsx` | `routes/pulls/$pullId/-components/` |
| `components/LabelsAdmin.tsx` | `routes/w/$ws/labels/-components/` |
| `components/Workspaces.tsx` | `routes/workspaces/-components/` |
| `components/ConfirmDialog.tsx` | `src/components/` に残す |
| `components/CopyButton.tsx` | `src/components/` に残す |
| `components/Markdown.tsx` | `src/components/` に残す |
| `src/issues.test.tsx` など route test | component 単体は各 `-components/` へ、横断遷移は `routes/-tests/` へ |
| `src/test-utils.tsx` | `routeTree.gen.ts` を import するよう更新 |

## 移行計画

関連変更なので stacked PRs にする。各 PR は behavior preserving を基本とし、
URL・表示・API 呼び出し順を変えない。

### PR 1: 移動の下地を作る

1. 現在の `src/routes.tsx` を一時的に `src/router.tsx` へ rename し、
   `main.tsx` と `test-utils.tsx` の import を更新する。
   - これは `src/routes.tsx` と `src/routes/` を同時に作れない問題を避けるため。
2. `forms.tsx` を feature 単位に分割する。
3. `HideDialog`, `LabelBadge` を shared component として抽出する。
4. component の import 整理だけを行い、route はまだ code-based のまま保つ。

確認:

- `npm run format`
- `npm run lint`
- `npm run build`
- `npm test`

### PR 2: file-based routing へ移行する

1. `@tanstack/router-plugin` を devDependency に追加する。
2. `vite.config.ts` で `tanstackRouter({ target: 'react', autoCodeSplitting: true })`
   を `react()` より前に追加する。
3. `@tanstack/router-cli` と `tsr.config.json` を追加し、
   `generate-routes` script (`tsr generate`) を用意する。`build` は
   `npm run generate-routes && tsc -b && vite build` の順にし、
   typecheck が常に最新の route tree を見るようにする。
4. `src/routes/` に URL 構造どおりの route files を作る。
5. `createRoute` + `getParentRoute` を `createFileRoute` へ機械的に移す。
6. `main.tsx` / `test-utils.tsx` を generated `routeTree.gen.ts` に切り替える。
7. generated `routeTree.gen.ts` を commit する。
8. CI の web job で `npm run generate-routes` 直後に
   `test -z "$(git status --porcelain -- src/routeTree.gen.ts)"` を実行し、
   stale/untracked な generated file が commit された場合を fail にする。
9. `src/router.tsx` を消す。

注意:

- `tanstackRouter` plugin は `react()` より前に置く。
- legacy redirect の logic は leaf route に置き、親 layout に移さない。
- `useLoaderData`/`useSearch`/`useParams` は `Route.use...` 形式へ寄せる。
- route のネスト構造が変わるため、nav active state と focus/scroll テストを重点確認する。

### PR 3: co-location を仕上げる

1. component test を各 `-components/` へ移す。
2. route tree 横断 test だけ `routes/-tests/` に残す。
3. 2 か所以上で使う loader composition を `-data.ts` に抽出する。
4. `PullFiles` 内の diff parser のような pure logic は `-lib/` へ出し、
   単体 test を近くに置く。
5. `docs/sketch/ja/tsconfig.md` など `routes.tsx` 前提の記述を更新する。

## 移行時の検証観点

最低限、既存 test がカバーしている次の挙動を落とさない。

- `/issues` など workspace なし URL が home workspace 配下へ redirect される
- home workspace が 1 件もない場合は `/` や legacy redirect が `/workspaces` へ redirect される
- `?q=...` など search params と `#comment-N` hash が redirect 後も残る
- `/w/$ws` が `/w/$ws/issues` へ redirect される
- login/register の `next` が安全な same-site path のときだけ使われる
- pull routes は repo 未設定時に 404 になる
- mutation 後に `router.invalidate()` で loader が再実行される
- route navigation 後に main landmark へ focus が移る
- leave guard が comment/thread/reply draft で動く

## 懸念と判断

### generated file を増やすのか

`routeTree.gen.ts` は generated だが runtime/typecheck に必要な application
source なので、公式 FAQ に従って commit する。formatter/linter からは除外する。

### 依存を増やしたくない方針との関係

この repo は最小依存寄りなので、`@tanstack/router-plugin` と
`@tanstack/router-cli` の追加はコスト。ただし routing は中心機能で、
すでに `@tanstack/react-router` に依存している。公式 plugin による構造・型・
code splitting の利益に加え、CLI による generation-first check で stale な
generated file を CI で検出できるため、devDependency 2 個に見合うと判断する。

それでも generated code/plugin を避けたい場合は fallback として、
`src/routes/` に分割しつつ hand-written route tree を `src/route-tree.ts`
に残す方法がある。この場合でも URL 構造と co-location は改善できるが、
「ファイルシステムが router を定義する」状態にはならない。

### `pages/` ではなく `routes/`

`pages/` は画面だけの置き場所に見える。今回置くのは画面だけでなく、
redirect route、layout route、loader、search validation なので `routes/` が自然。
また TanStack Router のデフォルトと一致し、学習コストも低い。

## 提案する最終判断

1. **第一選択**: `src/routes/` + TanStack file-based routing
2. **co-location**: `-components/`, `-data.ts`, `-lib/`, `-tests/` を route の近くに置く
3. **shared は shared に置く**: cross-feature component と low-level API client を無理に route 内へ押し込まない
4. **移行は stacked PRs**: component 分割 → routing 機構移行 → test/data helper 整理
