# CSS方針

## 決定: CSS Modules + 素のモダンCSS (2026-11-25)

`*.module.css` をcomponentと同居させ、グローバルなもの
(design token, reset, レイアウト共通部品) は `src/app.css` に置く。

## 検討した選択肢

| 方式 | 評価 |
| --- | --- |
| **CSS Modules** | **採用**。Vite標準機能で依存ゼロ・codegenなし・HMR完動。スコープが自動で閉じる |
| Tailwind CSS | 不採用。v4はCSS-firstで十分モダンだが、utility-firstは元repoの手書きCSS路線と相性が悪い(ユーザー所感「古い」も考慮) |
| StyleX | 見送り。型安全なatomic CSSだがBabel plugin等のビルド統合が重い。厳密なデザインシステムが必要な規模ではない |
| Panda CSS | 見送り。codegenベースの型安全styleだが、こちらも導入コストに見合う課題が今はない。再検討候補 |
| 素のCSS (グローバル) | Modulesに一本化するため部分採用(app.cssのみ) |

## 理由

1. **modern-web-guidanceとの整合**: ガイドの出力はvanilla HTML/CSS。
   `.module.css` にそのまま落とせる。元repoがmodern CSSの実験場
   (nesting, `light-dark()`, anchor positioning, `field-sizing`等) だった
   路線を引き継ぐ
2. **依存ゼロ**: このrepoはuseState禁止・最小依存の方針。
   ビルドplugin/codegenを増やさない
3. **十分性**: zatudan移植のUIは一覧・詳細・フォーム中心で、
   variant駆動の複雑なコンポーネントは当面少ない

## 運用ルール

- componentのスタイルは `<Name>.module.css` に書き、
  `import styles from './Name.module.css'` で使う
- グローバルは `src/index.css` のみ (tokens, reset, body/navなど)
- インライン `style={{}}` は禁止せず緊急回避のみとし、原則module.cssへ
- variantが増えて管理が辛くなったら Panda CSS を再検討する

## 移行メモ

現状のcomponentは仮実装でinline styleを使っている箇所がある
(`IssueList.tsx`, `IssueDetail.tsx`, `routes.tsx` のnav等)。
modern-web-guidance適用のタイミングでmodule.cssへ移す。

### 2026-11-25 更新: issues UI は MPA 移植のグローバル CSS を採用

MPA parity の作業 (`docs/sketch/ja/ui-parity.md`) では、component を
個別の `*.module.css` に分ける代わりに、参照実装の
`internal/web/static/app.css` をほぼそのまま移植した **単一の
グローバル stylesheet (`src/index.css`)** を使う方針を取った。
理由は class 語彙を MPA テンプレと一致させるのが parity 確認
(スクショ比較・テスト) の要点であり、module 化すると class 名が
変わって照合が難しくなるため。component 単位の module.css 方針は
issues UI 以外の新規 component には引き続き使ってよい。
