---
name: chrome-devtools-cli
description: Drive a real Chrome via the Chrome DevTools CLI (npx chrome-devtools-cli) — page navigation, DOM snapshots, console/network inspection, and performance traces + insight analysis. Use when measuring or tuning front-end performance (DOM size, layout, rendering) or when you need DevTools-level inspection that playwright-cli doesn't expose.
---

# chrome-devtools-cli

Chrome DevTools MCP サーバーの CLI ラッパー (npm: `chrome-devtools-cli`,
bin: `chrome-mcp-cli`)。パフォーマンス計測と DevTools 的な検査を行う。
スナップショット/クリック等の基本操作は playwright-cli と被るが、
`performance_start_trace` / `performance_stop_trace` /
`performance_analyze_insight` はこちら固有。

## 使い方

```sh
npx -y chrome-devtools-cli --commands          # 全コマンド一覧
npx -y chrome-devtools-cli <command> -h        # コマンド個別の引数ヘルプ
npx -y chrome-devtools-cli <command> '<json>'  # 1コマンド実行 (headless)
```

コマンドは対話モード (`npx -y chrome-devtools-cli`) でも逐次実行できる。

## パフォーマンス計測の流れ

```sh
npx -y chrome-devtools-cli new_page '{"url":"http://localhost:5173/w/default/issues"}'
npx -y chrome-devtools-cli performance_start_trace '{}'
npx -y chrome-devtools-cli navigate_page '{"url":"http://localhost:5173/w/default/issues/1"}'  # または click 等
npx -y chrome-devtools-cli performance_stop_trace '{}'
npx -y chrome-devtools-cli performance_analyze_insight '{}'   # insight名は -h で確認
```

trace は JSON で要約 (DOM size、style/layout 時間、各種 insight) が
返るので、`jq` や read で必要部分だけ拾う (全文は大きい)。

## 注意

- dev server 等は別途起動しておく (この CLI はブラウザ操作のみ)
- `take_snapshot` の a11y ツリーは playwright-cli と同様に ref 指定で
  `click`/`fill` できる
- ネットワークは `list_network_requests` / `get_network_request`、
  コンソールは `list_console_messages` / `get_console_message`
- 長時間の trace は重い。計測したい操作だけを挟む
