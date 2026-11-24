# 環境設定

SPA開発のためのエージェント・ツール環境のセットアップ手順と構成のメモ。

## 構成の方針

- スキルはすべてプロジェクト内 (`.agents/skills/`) に置き、gitで共有する
- MCPサーバーの設定はプロジェクトスコープ (`.devin/mcp_config.json`) に置く
- 個人のAPIキー・認証情報は `.devin/mcp_config.local.json` (gitignore対象) に置く

## MCPサーバー

### context7

ライブラリの最新ドキュメントを検索するためのMCPサーバー (Upstash製)。

- 設定: `.devin/mcp_config.json`
- エンドポイント: `https://mcp.context7.com/mcp` (Streamable HTTP)

追加は以下のコマンドで行った:

```sh
devin mcp add -s project context7 https://mcp.context7.com/mcp
```

#### 認証

初回利用時にOAuth認証が必要。ブラウザで認証する:

```sh
devin mcp login context7
```

無料アカウントでサインインすればよい。APIキー (`ctx7sk...`) を使う場合は
gitignoreされる `.devin/mcp_config.local.json` に以下を書く:

```json
{
  "mcpServers": {
    "context7": {
      "url": "https://mcp.context7.com/mcp",
      "headers": {
        "Authorization": "Bearer ctx7sk-..."
      }
    }
  }
}
```

#### 確認

```sh
devin mcp list          # 登録済みサーバーの一覧
devin mcp get context7  # 詳細表示
curl https://mcp.context7.com/ping  # 疎通確認 (pongが返る)
```

## スキル

`.agents/skills/` 以下に配置済み。

### modern-web-guidance

モダンなWeb標準 (HTML/CSS/JS) のベストプラクティスを検索するスキル。
UI実装の前に `npx -y modern-web-guidance@latest search "<query>"` でガイドを引く。

導入済み (`.agents/skills/modern-web-guidance/`)。インストール作業は不要で、
実行はnpx経由のため常に最新版が使われる。

### playwright-cli

ブラウザ操作・動作確認のためのCLI (`@playwright/cli`)。トークン効率のよい
コマンド形式でブラウザを操作できる。Playwright MCPよりエージェント向け。

```sh
npm install -D @playwright/cli
npx playwright-cli install --skills=agents
```

- `install` で `.playwright-cli/` ワークスペースが作られ `.gitignore` に追加される
- `--skills=agents` でスキルが `.agents/skills/playwright-cli/` にコピーされる
- ローカルのdevDependencyとして入れているので `npx playwright-cli` で実行

主要コマンド: `open`, `snapshot`, `click`, `fill`, `screenshot` など。
詳細は `.agents/skills/playwright-cli/SKILL.md` と `references/` を参照。

### todo-md

タスクトラッカー (TODO.md) 管理用スキル。`.agents/skills/todo-md/` に導入済み。

## 今後の検討: ライブラリ構成 (idea.mdより)

- React + TanStack Router + useReducer (useState禁止)
- テストはVitestか
- ビルドはViteか
- ts-next? (typescript@nextのことか要確認)
