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

引数なし (`npx -y chrome-devtools-cli`) は対話モードだが、stdin からの
一括投入は readline が閉じて失敗する — スクリプトからは常に
1コマンドずつの呼び出しにする。

## 前提: CDP 付き Chrome

接続先の Chrome を `--remote-debugging-port=9222` 付きで別起動しておく
(メインのブラウザとは別プロファイルにする):

```sh
google-chrome --headless=new --remote-debugging-port=9222 \
  --user-data-dir=/tmp/chrome-cdt about:blank &
```

## パフォーマンス計測の流れ

各 `npx chrome-devtools-cli <cmd>` は**毎回新しい DevTools セッション**
を張る。trace 状態は呼び出しを跨いで残らないので、
`performance_start_trace` → 操作 → `performance_stop_trace` を
別々の呼び出しで実行しても stop 時には "no active trace" になる。
`performance_analyze_insight` も同様に前の trace を参照できない。

そのため reload 計測は 1 コマンドで完結させる — `reload`+`autoStop`
を指定すると、その呼び出し内で reload → 自動停止 → 要約を返す:

```sh
npx -y chrome-devtools-cli list_pages '{}'            # pageId 確認
npx -y chrome-devtools-cli select_page '{"pageId":2}'
npx -y chrome-devtools-cli performance_start_trace \
  '{"pageId":2,"reload":true,"autoStop":true}'        # 要約が直接返る
```

返り値に LCP/CLS・insight 一覧 (LCPBreakdown, NetworkDependencyTree 等)
が含まれる。`performance_analyze_insight` は同じセッション内でしか
使えないため、この CLI からは実質呼べない — 詳細が欲しい場合は
`evaluate_script` で `performance.getEntriesByType` や DOM 集計
(`getElementsByTagName("*").length`, 深さ, ノード数) を取る:

```sh
npx -y chrome-devtools-cli evaluate_script \
  '{"pageId":2,"function":"() => document.getElementsByTagName(\"*\").length"}'
```

- 遷移・操作を挟んだ trace を取りたい場合はこの CLI ではなく、
  playwright-cli や `evaluate_script` の `performance.mark()` 計測を使う
- trace 要約は大きいので `grep`/`jq` で必要部分だけ拾う

## 注意

- dev server 等は別途起動しておく (この CLI はブラウザ操作のみ)
- `take_snapshot` の a11y ツリーは playwright-cli と同様に ref 指定で
  `click`/`fill` できる
- ネットワークは `list_network_requests` / `get_network_request`、
  コンソールは `list_console_messages` / `get_console_message`
- 引数 JSON は多くのコマンドで `pageId` 必須 (`list_pages` で調べる)
- 長時間の trace は重い。計測したい操作だけを挟む
