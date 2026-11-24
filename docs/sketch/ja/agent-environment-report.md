# エージェント環境レポート (2026-11-25)

探索プロンプトの設計知見、ツール動作確認結果、不足しているスキル/CLI/MCP の棚卸し。

## 1. 「今回の探索」を自動で行わせるためのプロンプト設計

今回のセッションで実際に機能した探索タスク (tsconfig 厳格化、markdown ライブラリ比較、TanStack Query 評価、UI parity) の指示から、再現可能なパターンを抽出する。

### 効いた指定

| 要素 | 実例 | 効果 |
| --- | --- | --- |
| **判定基準となる一次資料を渡す** | TS7 記事 URL、`docs/ja/features.md`、元 repo の `app.css`/`ui-rules.md` | 「正」が外部に固定され、エージェントの裁量ブレを防ぐ |
| **裏取りを必須化する** | 「記事の主張を `tsc --help --all` と repro で実測せよ」 | 記事の不正確な点 (baseUrl が help に残存等) を発見できた。一次資料を盲信させない最も重要な指定 |
| **採否 + 理由の記録を成果物にする** | 「`docs/sketch/ja/tsconfig.md` に各オプションの意味と採否理由」 | 却下した選択肢と理由が残り、再検討の無駄が消える |
| **触って良い範囲を明示** | 「`web/tsconfig*.json` と docs のみ。src は最小限」 | 並行作業する他エージェント/ユーザーとの衝突防止 |
| **検証ゲートを固定** | 「`npm run format && lint && build && test` 全通過」 | 完了定義が曖昧にならない。失敗時の報告にもなる |
| **worktree 分離 (破壊的検証時)** | `sketch/tanstack-query`、`sketch/react-markdown` | 採用/不採用どちらでも master を汚さない。doc だけ cherry-pick 可能 |
| **完了報告フォーマット** | 「検証結果・記事の誤り・変更内容を報告」 | 親エージェントが差分レビューせず要点を把握できる |

### 「現在の環境設定にたどり着く」ために必要な指定

探索をゼロからやり直させて同じ結論に至らせるには、プロンプトに次の情報が必要だった:

1. **入力**: 元アプリの場所 (`../go-classic-mpa-sample`)、仕様の正 (`docs/ja/features.md`)、移植スコープ (issues のみ)、禁則 (useState 禁止、MPA 向け API は捨てる)
2. **評価軸**: 「候補を列挙 → 一次資料で裏取り → 実装コストと bundle サイズ等で採否 → 理由を記録」という手順そのもの
3. **制約**: 既存パターン維持 (dialog/a11y/loader)、依存は 7 日ルール、granular commit、docs は `docs/sketch/ja`
4. **検証**: component-first workflow (vitest browser → playwright-cli → 実サーバー)

つまり `docs/sketch/ja/resume.md` + `AGENTS.md` + TODO.md の組が、既に「再探索プロンプト」の 8 割を構成している。足りないのは評価軸の明文化で、上記テーブルがそれに相当する。

### 再利用可能なプロンプト雛形

```
[対象] X の設定/ライブラリ選定を見直す
[基準] 一次資料 <URL or file> の推奨と比較する。ただし記事の主張は
       必ず実機 (コマンド/repro) で裏取りすること
[制約] 触って良いのは <files> のみ。既存パターン <patterns> は維持
[検証] <commands> が全通過すること
[成果物] docs/sketch/ja/<name>.md に各選択肢の意味と採否理由を記録
[報告] 検証結果・基準との齟齬・変更内容を要約して報告
```

## 2. ツール動作確認

| ツール | 状態 | 確認内容 |
| --- | --- | --- |
| **context7 MCP** | ✅ 稼働 | `resolve-library-id` / `query-docs` が応答 (react-markdown で実測)。OAuth 認証済み |
| **playwright-cli** | ✅ 稼働 | `npx playwright-cli` v0.1.24。本セッションでも E2E 確認・スクショで使用中 |
| **modern-web-guidance skill** | ✅ 稼働 | search/retrieve 経由で dialog/a11y/copy-button 等を適用済み |
| **todo-md skill** | ✅ 稼働 | TODO.md 運用中 |
| **esa MCP** | ✅ ツール一覧取得可 | 本プロジェクトでは未使用 (ドキュメント共有が要る場面で使える) |
| **`servers` MCP** | ❌ 一覧取得失敗 | ローカル設定 (`~/.config/devin/mcp_config.json` = context7 のみ) に未定義。グローバル/組織側の壊れたエントリと思われる。未使用なので実害なしだが、エラー表示の原因になる |

## 3. 不足しているスキル / CLI / MCP

| 不足 | 現状の代替 | 補うと嬉しい理由 |
| --- | --- | --- |
| **visual regression / screenshot diff** | playwright-cli で手動スクショ → 目視 | UI parity タスクで MPA(:8090) vs SPA(:5173) の同時スクショ比較を頻繁に行う。`zatudan` 側起動+両方撮る CLI があると diff が償却できる |
| **a11y 監査 (axe-core 等)** | modern-web-guidance の手動適用 + テスト内で role/name 確認 | 自動スキャンがないと dialog/aria のregress を見落とす。vitest browser + axe-playwright の導入余地あり |
| **API クライアント CLI (`zatu` 相当)** | curl / REST 直叩き | 元 repo には `zatu` CLI があり、zatudan API の手動検証に有用。SPA API 用の薄い CLI または OpenAPI 生成が欲しい (TODO の zatu 移植判断と連動) |
| **シード/リセット CLI** | `go run ./cmd/zatudan -seed` | dev server との併用で「DB を決定論的な状態に戻す」操作が手順化されていない |
| **依存更新 bot (dependabot/renovate)** | なし | 7 日ルールを手動で守っている。dependabot + minimumReleaseAge 相当の設定があれば自動化できる |
| **GitHub MCP** | `gh` CLI で代替中 | issue/PR 操作は gh で足りている。必須ではない |
| **Chrome 起動 flake 対策** | vitest browser で稀に起動失敗 → 手動再実行 | `browser.instances` の retry 設定 or CI での安定化策が未整備 (`.github/workflows/ci.yml` で実際に発生するか要観察) |

### 優先度の所感

- **即効性**: a11y 監査 (UI parity 完了後のregress 防止)、seed リセット手順のドキュメント化
- **中期的**: visual regression (UI parity が継続的な作業になるなら)、zatu 移植 (API surface が増えると curl が辛い)
- **様子見**: `servers` MCP エントリの除去、dependabot
