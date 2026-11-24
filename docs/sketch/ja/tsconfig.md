# web/ の TypeScript 設定 — 評価記録

2026-11-25。`web/` (Vite 8 + React 19 SPA、TypeScript 7.0.2 = native port) の
tsconfig を見直し、厳格化した。起点は1ファイル統合案のドラフトだったが、
検討の結果 **app/node の2プロジェクト分割を維持したまま、両方に厳格系
フラグを追加** する形にした。

## 背景

- `npm run build` = `tsc -b && vite build`。tsc は型検査専用
  (`noEmit`)、実際のトランスパイルは Vite (rolldown) が行う。
  この分業が「Bundler mode」系オプション群の存在理由。
- 既存の `tsconfig.app.json` / `tsconfig.node.json` は create-vite
  (vite@main, template-react-ts) と一字一句同じで、**足りていないのは
  `strict` を含む厳格系フラグ** だった (現在の create-vite テンプレートは
  `strict: true` を出力しない)。
- TypeScript 7 (tsgo) は compilerOptions の互換性を持つため、ここで
  議論するフラグは全てサポートされる。`noEmit` 運用なので emit 系の
  未対応差分も関係ない。

## 構成の選択: 分割を維持

| | tsconfig.app.json | tsconfig.node.json |
| --- | --- | --- |
| 対象 | `src/` (ブラウザで動くコード+テスト) | `vite.config.ts` のみ |
| lib | ES2023 + DOM | ES2023 (DOM なし) |
| types | `["vite/client"]` | `["node"]` |
| module | esnext + moduleResolution: bundler | nodenext |

1ファイルに統合しなかった理由:

- `vite.config.ts` は `vitest/config` と `@vitest/browser-playwright` を
  import し、Node 側の型 (`types: ["node"]`) を必要とする。
- `src/` に `types: ["node"]` を混ぜると Node のグローバル
  (`process`, `setTimeout` 等) がブラウザコードから見えてしまう。
  実害として `ReturnType<typeof setTimeout>` が DOM lib では `number`、
  node types では `Timeout` に変わり、`CopyButton` の timer 型が壊れる。
- DOM lib を `vite.config.ts` に混ぜる逆方向も同様に不純。
- ルートの `tsconfig.json` は `references` だけの solution ファイルで、
  `tsc -b` が両プロジェクトをまとめてビルドする。`include` を分ける
  境界はこのままが自然。

## 最終設定と各オプションの意味

両ファイル共通で、差分は上表の lib/types/module だけ。

### 基本

- `tsBuildInfoFile`: `tsc -b` の incremental 情報の出力先。
  `noEmit` でも buildinfo は生成されるので node_modules/.tmp に逃がす。
- `target: es2023`: 型上の言語レベル。Vite 8 の build target も現代
  ブラウザ前提なので過剰に下げる理由がない。
- `lib`: 実行環境の組み込み型。app は DOM、node は ES のみ。
- `module: esnext` (app) / `nodenext` (node): app は Vite がバンドル
  するので esnext で十分。node 側は `package.json` の `"type": "module"`
  と合わせて Node の ESM 解決ルール (exports フィールド等) をモデル化
  する nodenext が正確。どちらも scaffold 由来。
- `types`: 暗黙に読み込む ambient types のホワイトリスト。app は
  `vite/client` (CSS/asset import の `declare module` と
  `import.meta.env`)。これが無いと `import './index.css'` が
  unresolved になる。node は `node`。
- `allowArbitraryExtensions`: `foo.css` に対する `foo.css.d.ts` を
  型定義として拾えるようにする scaffold 標準フラグ。
- `skipLibCheck`: 依存パッケージの .d.ts 内部の型矛盾を検査しない。
  node_modules の d.ts まで厳密検査すると依存側の問題で build が
  止まるため事実上必須。

### Bundler mode

- `moduleResolution: bundler`: exports 条件 + 拡張子省略を許す
  バンドラ向け解決規則。
- `allowImportingTsExtensions`: `import { routeTree } from './routes.tsx'`
  のような `.tsx` 拡張子付き import を許す (`noEmit` 前提のため両立可)。
- `verbatimModuleSyntax`: `import type` と値 import の区別を保持し、
  型だけの import を値 import と書いたらエラーにする。トランスパイラが
  1ファイルずつ処理する前提で型の import/export が残って壊れる事故を防ぐ。
- `moduleDetection: force`: import/export が無いファイルもグローバル
  script ではなく module として扱う。`declare module` やトップレベル名の
  暗黙グローバル化を防ぐ。
- `noEmit`: 型検査専用宣言 (emit は Vite の仕事)。
- `jsx: react-jsx` (app のみ): 自動ランタイムの JSX 変換。
- `isolatedModules`: **今回追加**。1ファイル単位のトランスパイルで
  安全に処理できる構文だけに制限する (const enum・型の再エクスポート
  等を禁止)。`verbatimModuleSyntax` + `moduleDetection: force` と
  `erasableSyntaxOnly` で実質的にカバー済みだが、意図を明文化するため
  明示した。

### Linting (厳格系)

- `strict`: **今回追加**。strictNullChecks・noImplicitAny 等の
  ファミリー一括。既存コードはエラー 0 件で通った (書き方が既に
  strict 前提だった)。
- `noUncheckedIndexedAccess`: **今回追加**。`arr[i]` や
  `Record<string, T>` の添字アクセスに `| undefined` を付ける。
  実害として `active.splice(i, 1)[0]` (テスト内) が
  `Label | undefined` になり、分割代入 + 存在チェックに直した。
- `exactOptionalPropertyTypes`: **今回追加**。`prop?: T` を
  「省略可能」として厳密化し、`undefined` の明示代入を型エラーにする。
  渡し得る側は `prop?: T | undefined` と宣言するのが正。この1件で
  19 箇所直した (下記「修正内容」)。
- `noUnusedLocals` / `noUnusedParameters`: 未使用の変数・引数を禁止。
- `erasableSyntaxOnly`: enum・namespace・parameter properties 等の
  「型消去だけでは変換できない構文」を禁止。esbuild/rolldown の
  単純消去と整合させる。
- `noFallthroughCasesInSwitch`: case の fallthrough を禁止。
  reducer (`issueFormReducer`, `commentUIReducer`) が switch なので
  実効性あり。既に有効だった。
- `noUncheckedSideEffectImports`: **今回追加**。副作用だけの import
  (`import './index.css'`) も解決できなければエラーにする。
  `vite/client` の `declare module '*.css'` で解決されるため導入可能。
- `forceConsistentCasingInFileNames`: **今回追加**。import 記述と
  実ファイルの大文字小文字が一致しないとエラー。case-insensitive
  FS (macOS) で書かれたコードが Linux CI で壊れる事故の予防。

## ドラフト → 最終の差分と理由

| ドラフト | 最終 | 理由 |
| --- | --- | --- |
| 1ファイル統合 (`include: ["src/**/*"]`) | app/node 2分割維持 | vite.config.ts に node types + nodenext が必要なため (上記) |
| `module: "Preserve"` | `esnext` (scaffold のまま) | `Preserve` は「書いたままの module 構文を保持」する emit 向けで、`noEmit` + bundler 運用では esnext と等価。scaffold 標準を維持 |
| `lib: [..., "DOM.Iterable"]` | `["ES2023", "DOM"]` | TS7 の `lib.dom.d.ts` は `HTMLCollectionBase`/`NodeList`/`FormData` 等に `[Symbol.iterator]` を内蔵済み (`[...form.elements]` が DOM.Iterable 無しで通ることを確認)。DOM.Iterable は後方互換用で実質不要 |
| `rootDir`/`outDir` | 不採用 | `noEmit` (型検査専用) では emit 先もソース境界も意味を持たない |
| `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `verbatimModuleSyntax`, `isolatedModules` | 採用 | verbatimModuleSyntax は scaffold 済み。残りは今回追加 |
| `skipLibCheck`, `types: ["vite/client"]`, `target: ES2022` | 採用 (target は scaffold の es2023) | scaffold の es2023 で十分新しく、下げる理由なし |

## 検討して採用しなかったオプション

- `useDefineForClassFields`: target es2022+ ではデフォルト true で
  あるため明示しても no-op。書くなら「将来 target を下げたときの
  保険」だが、下げる予定がないため見送り。
- `noPropertyAccessFromIndexSignature`: `errors?.title` のような
  Record のドットアクセスを全て `errors?.['title']` に直させる。
  `Record<string, string>` 用途で安全性の差はほぼ無く、差分の
  ノイズが大きいため見送り。
- `noImplicitOverride`: `RequestError extends Error` 以外に継承が
  無く効果ゼロ。継承が増えたら再検討。
- `resolveJsonModule`: JSON import が無いので不要。
- `checkJs` / `allowJs`: ソースは全て ts/tsx。
- `tsconfig.node.json` 側の `moduleResolution: bundler`:
  nodenext で動作しており、Vite が config を bundle する実態に
  合わせて bundler に変える利点も薄い (むしろ exports 準拠の
  nodenext の方が厳密)。scaffold 維持。

## exactOptionalPropertyTypes で直した内容

「オプショナルな prop/type に呼び出し側が undefined を明示的に
渡し得る」箇所を `T | undefined` 宣言に修正 (型を緩めたのではなく、
実態に合わせて正確にした):

- `IssueSearch` (routes.tsx) / `IssueListFilter` (api.ts):
  `validateSearch` が全キーを `undefined` 可で埋めるので各フィールドを
  `?: T | undefined` に。`qs()` が undefined を取り除く設計はそのまま。
- `CommentHandlers` の全メンバ、`pending`/`onArchive` props
  (IssueDetail/LabelsAdmin/ConfirmDialog/CopyButton/HideCommentDialog):
  `{...handlers}` スプレッドや `pending={pending}` の転送で
  `boolean | undefined` が流れるため。
- `CopyButton` の reducer state を `CopyUIState` として切り出し、
  `timer?: ... | undefined` に (timer 無し dispatch が undefined を
  書き込むため)。
- テスト: `requests: { url; init?: RequestInit | undefined }[]`、
  `splice(i,1)[0]` を `const [removed] = ...; if (removed)` に
  (noUncheckedIndexedAccess 対応でもある)。

## 検証結果

- `npm run format` (oxfmt) / `npm run lint` (oxlint, 0 warnings) /
  `npm run build` (`tsc -b` + `vite build`) / `npm test` (vitest
  browser mode, 17/17): 全て pass。
- 新フラグ導入時のエラー件数: `exactOptionalPropertyTypes` が 19 件、
  `noUncheckedIndexedAccess` が 1 件、`strict` 以下他は 0 件。
