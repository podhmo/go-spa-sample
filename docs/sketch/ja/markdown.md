# Markdown レンダリング方式 — 評価記録

2026-11-24。`sketch/react-markdown` ブランチで `src/components/Markdown.tsx`
の実装を marked+DOMPurify (文字列HTML → `dangerouslySetInnerHTML`) から
react-markdown (hast → React elements) に置き換え、採否判断の材料を集めた。
結論として **react-markdown 方式を採用** した。

## 比較した 3 案

| | 現行: marked + DOMPurify | react-markdown | remark 直組み (DIY) |
| --- | --- | --- | --- |
| 構成 | `marked` + `marked-alert` → HTML 文字列 → `DOMPurify` → `dangerouslySetInnerHTML` | `react-markdown` + `remark-gfm` + `remark-github-blockquote-alert` | `unified` + `remark-parse` + `remark-gfm` + `remark-rehype` + `rehype-react` を自前で配線 |
| バンドル (raw / gzip) | 401.71 kB / 129.13 kB | 485.64 kB / 151.56 kB (+83.9 / +22.4 kB) | 482.90 kB / 149.99 kB (+81.2 / +20.9 kB) |
| サニタイズモデル | **事後型**: 生成済み HTML 文字列を whitelist で洗う。生 HTML はカスタム renderer で事前エスケープ、`javascript:` は DOMPurify が潰す。2 層 | **構造型**: mdast → hast → React elements で innerHTML を経由しない。`raw` ノードは text に潰され、`defaultUrlTransform` が安全でない scheme の href を空にする | 同じく構造型だが、`raw` ノード処理と URL サニタイズは**自分で実装する必要がある** |
| GFM + alert | GFM 内蔵 + `marked-alert` | `remark-gfm` + `remark-github-blockquote-alert` | `remark-gfm` + `remark-github-blockquote-alert` |
| カスタマイズ | renderer/tokenizer の文字列生成フック | `components` prop でタグ名 → React コンポーネントの写像 (vdom 流儀) | 同じ (react-markdown の内部実装そのもの) |
| 判定 | ベースライン | **採用** | 見送り |

## 決定: react-markdown を採用

理由は「全部 vdom に寄せると綺麗」という当初の直感が、調べるほど
正確だったこと。

1. **サニタイズが「出力後の洗浄」から「構造上存在しない」に変わる。**
   marked+DOMPurify は「とりあえず HTML を吐いて後から洗う」モデルで、
   安全の根拠が「PURIFY の whitelist が網羅的か」の維持にかかっていた。
   react-markdown では HTML 文字列自体が生成されず、DOM は
   React element のみから作られる。`javascript:` リンクは
   `defaultUrlTransform` が href を `''` に潰し、生 HTML は
   `raw` ノードとして text に変換される (MPA が生 HTML を出力しない
   振る舞いと同じ)。防御が多層ではなく単層で、しかも仕組み的。
2. **DOMPurify の whitelist 保守が消える。** `ALLOWED_TAGS`/`ALLOWED_ATTR`
   の網羅性を考え続ける責務が無くなった。必要なら `allowedElements` で
   構造的に絞れるが、そもそも攻撃面が文字列パーサ経由で存在しない。
3. **`components` prop がスタイリング・拡張の正規の口になる。**
   リンクに `target="_blank"` を付けたい、`<img>` を差し替える、
   などの変更が「HTML 文字列を正規表現で弄る」ではなく素の React
   コンポーネント差し替えとして書ける。
4. **alert プラグインも hast に SVG を挿入するので vdom で完結**。
   `remark-github-blockquote-alert` が octicon を hast ノードとして
   埋め込むため、`hast-util-to-jsx-runtime` 経由で素の `<svg>` 要素に
   なる。既存の `.markdown-alert` / `.markdown-alert-title` /
   `.markdown-alert-note` CSS (marked-alert と同名) がそのまま効いた。

## remark 直組み (DIY) を見送った理由

`react-markdown` の内部は正にこの配線 (`unified().use(remarkParse)
.use(remarkGfm).use(remarkRehype).use(rehypeReact)` 相当) で、
自前実装でも依存木はほぼ同一、節約できるのはラッパー分の
**約 2.7 kB / 1.6 kB gzip** のみ。対価として:

- `raw` ノード (生 HTML) を text に潰す変換を自分で書く必要がある
  (`mdast-util-to-hast` は `allowDangerousHtml` 無しだと html ノードを
  **黙って落とす**。MPA の「エスケープして見せる」振る舞いに合わせるには
  raw 化 → text 化の手順が要る = react-markdown がやっている処理)
- `hast-util-to-jsx-runtime` は URL サニタイズを**しない**。
  `javascript:` 対策の tree walk を自前で持つことになる

つまり「react-markdown が既に実装している安全層を 1.6 kB gzip 削るために
再実装する」話であり、メリットが見つからなかった。

## 採用した実装

```tsx
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { remarkAlert } from 'remark-github-blockquote-alert'

// GFM (tables, task lists, strikethrough, autolinks) + GitHub-style
// [!NOTE] alerts, matching the MPA's goldmark setup. Rendering stays in
// the vdom: the markdown is compiled to hast and rendered as React
// elements, so no dangerouslySetInnerHTML is involved. Raw HTML in the
// source is rendered as escaped text (react-markdown's default —
// remark-rehype's allowDangerousHtml produces `raw` hast nodes that
// react-markdown flattens to text). Unsafe link schemes such as
// javascript: are neutralized structurally by the default urlTransform,
// which drops the href of any non-safe protocol.
const remarkPlugins = [remarkGfm, remarkAlert]

export function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown-body">
      <ReactMarkdown remarkPlugins={remarkPlugins}>{text}</ReactMarkdown>
    </div>
  )
}
```

`Markdown` コンポーネントの API (`text` prop) と外側の
`className="markdown-body"` wrapper は不変。呼び出し側
(`IssueDetail.tsx` の issue 本文・コメント) は無修正。

## 正直なトレードオフ

- **+22.4 kB gzip (+83.9 kB raw)**。unified/remark エコシステムは
  細かいパッケージの集合 (micromark, mdast-util-*, hast-util-* …) で、
  transitive deps の数も増える。SPA の初期 JS が 129 → 152 kB gzip。
- marked+DOMPurify 側が持っていた `useMemo` による「text 不変なら
  再パースしない」は react-markdown 内部では走らない (render 毎に
  processSync される)。issue 本文・コメント規模では実害なしと判断。
  巨大文書が問題になったら `memo` で包むか `allowedElements` で
  枝刈りする余地はある。
- `remark-github-blockquote-alert` は個人メンテナ (jaywcjlove) の
  パッケージで、marked-alert と同程度のエコシステム規模。
  GitHub alerts 自体は remark エコシステムでは事実上これが定番。
- 依存バージョンはすべて公開から 7 日以上経過したもの:
  `react-markdown@10.1.0` (2025-03-07)、`remark-gfm@4.0.1`
  (2025-02-10)、`remark-github-blockquote-alert@2.1.0` (2026-03-14)。
  最新 `2.1.1` (2026-11-19) は 7 日ルールに抵触したため 1 つ落とした。

## 検証結果

- `npm run format` / `npm run lint` / `npm run build` / `npm test`: 全て pass
- 既存テストは無修正で通過。markdown テストに alert の assert
  (`article .markdown-alert-note` の存在) を追加して全 17 件 green
- 生 HTML は text 化 (`<script>` は要素として存在しない)、
  `javascript:` href は空文字に潰れる、task list / `**bold**` /
  GFM 要素は既存テストが担保
