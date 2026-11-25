# CSS方針

## 決定: CSS Modules + 素のモダンCSS (2026-11-25 再確認)

`*.module.css` を component / route と同居させ、グローバルなもの
(design token, minimal reset, base typography) だけを `src/index.css` に残す。
2026-11-25時点でもこの方針を維持する。新規ライブラリは導入しない。

issue: <https://github.com/podhmo/go-spa-sample/issues/22>

## 現状監査 (2026-11-25)

以前は「MPA parity のために class 名を残す」ことを理由に、参照実装の
`internal/web/static/app.css` をほぼそのまま `src/index.css` へ移植した。
その後 component 側は file-based routing に合わせて整理され、
route-local component (`routes/**/-components/*.tsx`) が増えた。
一方でCSSだけが単一の global file に残っている。

- `web/src/index.css`: 約2,000行、130種類のclass selector
- production TSX 62文件中38文件がglobal classを参照している
- 特に共有度が高いもの: `btn` 30文件、`btn-quiet` 16文件、
  `btn-primary` 15文件、`page-head` 14文件、`form-actions` 13文件、
  `field` / `error-msg` 12文件
- feature固有の塊も同じfileに混在している:
  workspace list、labels admin、pull request badge/form、diff table、
  markdown alert、comment thread、app shell など

つまり、今の問題は「CSSがcomponentから遠い」ことに加えて、
「真に共有なprimitive」と「feature固有のstyle」が同じ名前空間に
 flatten されていることである。

### MPA parity を理由にglobal classを残す必要はもう薄い

parity確認の要点は見た目とDOMの意味構造であって、class名そのものではない。
Vitest内に `.issue-row .badge` や `.markdown-alert-note` を見るテストはあるが、
これは移行時にtest selectorを更新すればよい。screenshot比較もclass名が
hash化されても成立する。したがって、以前の移行メモに書いた
「class語彙をMPAと一致させる」制約は、現在は重視しなくてよい。

## ライブラリ再評価

「他のlibraryを使うこと」も前提に再評価したが、今回はCSS Modulesの
優位性が変わらなかった。決定的な理由は、既存CSSがmodern CSSそのものの
実験場であり、move主体で移行できることが重要だからである。

| 方式 | 現在の評価 | 判定 |
| --- | --- | --- |
| **CSS Modules** | Vite標準。追加依存なし。既存CSSをほぼ素のまま分割できる。nesting, `@media`, `@supports`, container/scroll-state query, `light-dark()` などをそのまま保持できる。class名だけlocal化される | **採用** |
| vanilla-extract | zero-runtimeで型安全。`*.css.ts` co-locationも自然。ただし2,000行の既存CSSをTS object記法へ書き換える必要があり、moveではなくrewriteになる。複雑なdescendant selectorや生成HTML向けruleの移植コストが高い | 第2候補。全面rewriteするなら有力 |
| Panda CSS | token / recipe / variant管理は強力。Vite pluginとcodegenもある。ただし現状はvariant駆動design systemというより、semantic class + modern CSSのapp。`styled-system`生成物と抽出ルールを抱えてでも欲しい課題がまだ小さい | 見送り。variant管理が辛くなったら再評価 |
| StyleX | `@stylexjs/unplugin` によりVite統合は以前より改善している。atomic CSSで型安全。ただしstyleをTS objectへ全面rewriteする必要があり、markdown生成HTMLやdetail内の複雑なdescendant stylingとは相性が悪い | 見送り |
| Tailwind CSS v4 | zero-runtimeでVite pluginも簡単。global CSSは減るが、styleはclass文字列としてJSXへ移るだけで、今回欲しい「component近傍のCSS file」ではない。既存のmodern CSS表現をutilityへ翻訳するコストも高い | 不採用 |
| native `@scope` | proximityを表現するのに有用だが、class名の衝突回避やbundler-levelのownershipは解決しない。CSS Modulesの代替ではない | 補助的に使う |

参照:
[Panda CSS Vite](https://panda-css.com/docs/get-started/vite),
[vanilla-extract Vite](https://vanilla-extract.style/documentation/integrations/vite/),
[StyleX Vite](https://stylexjs.com/docs/learn/installation/vite/),
[Tailwind CSS Vite](https://tailwindcss.com/docs/installation/using-vite)。

## 目標構造

global CSSをゼロにする必要はない。`html` / `body` / design tokenのように
本質的にglobalなものだけを残し、component/featureのstyleはexplicit importにする。

```text
web/src/index.css                         # tokens, minimal reset, base elements only
web/src/styles/button.module.css          # cross-cutting primitive (explicit import)
web/src/styles/form.module.css            # field/error/form-actionsなど
web/src/styles/list.module.css            # issue/discussion/pull共通のrow list
web/src/components/Markdown.module.css
web/src/components/CopyButton.module.css
web/src/components/ConfirmDialog.module.css
web/src/routes/-components/SiteChrome.module.css  # header/nav/footer/progress/error page
web/src/routes/w/$ws/issues/-components/IssueList.module.css
web/src/routes/w/$ws/issues/$issueId/-components/IssueDetail.module.css
web/src/routes/pulls/$pullId/-components/PullFiles.module.css
```

TanStack Routerのfile-based routingではrouteではないfileを`-`始まりの
directory/fileに置く慣例がある。route直下に `index.module.css` を置くより、
既存の `-components/` にcomponentとCSSを同居させるか、
必要なら `-styles/` を使う方が安全である。

### `src/index.css` に残してよいもの

- design token (`--bg`, `--surface`, `--accent` など)
- `color-scheme` / light-dark token解決
- minimal reset (`box-sizing` など。modern-web-guidanceの観点では
  universal selectorへ過剰に寄せすぎない)
- `html`, `body`, link, headingのbase typography
- app全体で使うaccessibility utility (例: `.visually-hidden`)

### globalから出すもの

- `site-header`, `site-nav`, `route-progress`, `error-page` → site chrome module
- `btn`, `card`, `page-head`, `badge`, `tabs`, `filter-*`, `pagination`,
  `breadcrumb`, `empty-state` → shared primitive module
- `field`, `form-*`, `error-msg`, `label-picker` → form primitive module
- `issue-*`, `workspace-*`, `label-admin-*`, `badge-pr-*`, `pull-*`,
  `diff-*` → feature/route module
- `comment-*`, `comments` → comment shared module または専用component群
- `markdown-*` → `Markdown` component module
- `copy-*`, `confirm-dialog` → 対応component module

## 共有とco-locationの境界

「componentに閉じる」は「全てのCSSを重複させる」ことではない。
ownershipごとに分ける。

1. **1 componentだけが使う** → そのcomponentの`*.module.css`
2. **1 route配下だけが使う** → そのrouteの`-components/`または`-styles/`
3. **複数featureが使う primitive** → `web/src/styles/*.module.css`
4. **environment全体の前提** → `src/index.css`

ここで重要なのは、3もglobal CSS fileではなくmodule CSSにすることである。
`btn`のような共有primitiveはglobal名前空間に置く必要はない。
必要なcomponentが明示的にimportすればよい。

## 移行上の注意

### descendant selectorはroot classで閉じる

CSS Modulesでも `.field input` のようなselectorは使える。
`.field`だけlocal化され、`input`自体はglobal element selectorだが、
実際のmatchはlocal class配下に限定される。したがって全要素へclassを
付け直す必要はない。

```css
.field input:user-invalid {
  border-color: var(--danger);
}
```

### markdown生成HTMLは `:global()` をroot配下に限定する

`remark-github-blockquote-alert` が生成するclassはTSX側で制御できない。
完全なlocal化はできないが、`Markdown` componentのroot配下に限定すれば
実害は避けられる。

```css
.body :global(.markdown-alert-note) {
  /* ... */
}
```

同様に、動的に組み立てている `badge-open` / `badge-closed` や
`diff-add` / `diff-del` は、文字列生成ではなくmodule objectのkey参照へ
寄せる。

### 単純な行移動で済まないselectorを先に洗う

現状のglobal CSSにはclass以外のselectorも混ざっている。特に
`#comment-body` / `#issue-body` のようなid selectorは、style用途には
classへ置き換える (id自体はlabelやariaのために残してよい)。
また `:is(input, select, textarea):user-invalid ~ .error-msg` のように
root classを持たないruleは、moduleへ移しただけではglobal element selectorが
残る。`.field` などのlocal class配下へ組み替える。
`textarea` / `select` / `h1` / `h2` などのelement selectorは、
base typographyとして `src/index.css` に残すものとform primitiveへ移すものを
移行前に分ける。

### module間で順序依存のoverrideを作らない

global CSSでは「後に書いたruleが勝つ」ことに依存したoverrideが残っている
可能性がある。module化後はimport順に露出しやすいので、
別moduleのclassを上書きする形は避ける。variantは同じmodule内に持つか、
cascade layerを明示する。移行直後は見た目を変えないことを優先し、
layer整理は別途行う。

### 共有markupの不足はCSSだけで解決しない

`issue-list`はdiscussions/pullsでも使われ、`comment-*`もissues/discussions/pulls
にまたがっている。これはCSS以前に「list row」「comment item」のReact componentが
十分に共有されていない表れでもある。無理にrouteごとへ複製するとCSS重複が増える。
必要なら先に小さいshared componentを抽出し、そのcomponentにCSSをco-locateする。

## 移行順

一度に全量を動かさず、影響範囲の小さいものからstrangler fig的に移す。

1. **component単位で閉じるもの**: `Markdown`、`ConfirmDialog`+`HideDialog`+
   pull merge dialog (共有するdialog styleは同じmoduleへ)
2. **app shell**: `__root.tsx`のheader/nav/footer/progress/error page
3. **単一featureで閉じるもの**: workspace list、labels admin、pulls diff、
   PR badge/form
4. **form/buttonなどの共有primitive**: `btn`, `field`, `form-actions`,
   `card`, `page-head`, `filter`, `pagination`, `breadcrumb`
5. **list/comment/detail**: issue/discussion/pull跨ぎの共有関係を整理しつつ分割。
   `CopyButton`はcomment hover/focusで表示するruleと同じstepで動かし、
   `LabelBadge`もlist/detail側の利用箇所と同じstepで動かす
6. **dead CSS削除**: dynamic classやMPA由来の未使用selectorを消す

各stepはvisual変更なしを基本とし、必要なtest selector更新だけを伴う。
全量移行後の目安は、`src/index.css`を300行以内、feature固有selectorを
global fileからゼロにすることである。

## 再検討条件

- variant/recipe管理が増え、`btn`や`badge`の組合せがCSS Modulesでは読みにくい
- design tokenの型安全性が欲しい
- component library化してpublic APIとしてstyleを整理したい

この場合はCSS Modulesではなくvanilla-extractを第一候補、Panda CSSを第二候補に
再評価する。StyleXとTailwindは、既存のmodern CSS資産をrewriteする方向なので、
このrepoの実験方針を変えない限り優先しない。
