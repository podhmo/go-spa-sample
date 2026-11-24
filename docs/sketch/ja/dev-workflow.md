# 開発ワークフロー

疎通スパイク (Go API ↔ SPA) で確立した開発・検証の手順。
エージェントが「コードを書く → ブラウザで動作確認する」ループを回すための前提。

## 構成

```
cmd/fakeapi/       捨てる前提のfake API (zatudanとは無関係の独自signature)
web/               SPA (Vite + React + TS + TanStack Router)
```

- fakeapi: `127.0.0.1:8080` で待ち受け (`GET /api/ping`, `GET/POST /api/counter*`)
- vite dev: `127.0.0.1:5173`。`server.proxy` で `/api` → `http://127.0.0.1:8080`
  - 同一オリジンになるので CORS 不要。本番も同じ形 (Go側で静的ファイルをserveするか別ドメイン+認証) を想定

## 起動

```sh
go run ./cmd/fakeapi          # terminal 1 (API :8080)
npm --prefix web run dev      # terminal 2 (vite :5173)
```

## 検証コマンド

```sh
cd web
npm run build    # tsc -b + vite build (型チェック兼ビルド)
npm test         # vitest run — browser mode (実Chromeで実行)
npm run lint     # oxlint
go vet ./...     # Go側 (repo rootで)
```

### Vitest browser mode

テストは jsdom ではなく実ブラウザで走る (`test.browser` 設定)。
provider は playwright、`launchOptions.channel: 'chrome'` で
システムのChromeを使うためブラウザのDLは不要。

- component testは `vitest-browser-react` の `render` +
  `page.getByRole` などのlocator、`expect.element` でassert
  (`src/browser.test.tsx` 参照)
- browser mode でも vite dev server が使われるため、
  `server.proxy` の `/api` 設定がそのまま効く
  (fetchするcomponentのテストは fakeapi 起動で済む)
- 見た目の確認は `npx vitest --browser.headless=false` や
  `npx vitest --ui` でGUIを出しながら行える

## ブラウザでの動作確認 (playwright-cli)

プロジェクトのdevDependency `npx playwright-cli` を使う
(スキル: `.agents/skills/playwright-cli/SKILL.md`)。

```sh
npx playwright-cli open http://localhost:5173/
npx playwright-cli snapshot        # a11yツリー + ref (e5など) を取得
npx playwright-cli click e5        # ref / role locator / css selector で操作
npx playwright-cli screenshot --filename=x.png
npx playwright-cli console         # コンソールエラー確認
npx playwright-cli requests        # ネットワークリクエスト確認
npx playwright-cli close
```

スパイクで確認したフロー: `/` のloaderが `/api/ping` を表示 →
`/counter` で increment → POST `/api/counter/increment` →
`router.invalidate()` で再fetch → 画面が `count: 1` に更新
(`spike-counter.png`)。

## ルール

- ローカル状態は `useState` ではなく `useReducer` (idea.mdの方針)。
  サーバー由来の状態は TanStack Router の loader + `router.invalidate()` で扱う
- UI実装前に `npx -y modern-web-guidance@latest search "<query>"` で
  モダンなベストプラクティスを確認 (`.agents/skills/modern-web-guidance/`)
- ライブラリの使い方は context7 MCP で最新ドキュメントを引く
  (`docs/sketch/ja/libraries.md` 参照)
