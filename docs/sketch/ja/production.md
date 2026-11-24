# 本番構成 (未決定 — 比較メモ)

TODO.md「本番構成の決定」の検討メモ。現状のdev構成と2案の比較。

## 現状

- SPA は vite dev server (:5173) が `server.proxy` で `/api` →
  `127.0.0.1:8080` に流し、proxy が `Authorization: Bearer dev-token`
  を注入する**開発専用**構成
- zatudan は静的ファイルを serve しない。`/api/v1` のみ + `/healthz`
- 認証: `/auth/*` の cookie セッション (SPA が login/logout で使う)
  + 任意で共有 Bearer (`-api-token`/`ZATUDAN_API_TOKEN`)
  + `api_tokens` 行 (CLI 等の非ブラウザクライアント向け)

## 案A: embed 配信 + セッション認証 (単一バイナリ)

`go:embed web/dist` で SPA を zatudan に同梱し、同一オリジンで配信。

- 変更不要な部分: fetch は相対 `/api/v1/...` のまま動く、
  cookie セッションは SameSite=Lax のまま成立、`-api-token` は不要
- 必要な実装:
  - `embed.FS` + `http.FileServerFS`、非 `/api` パスは
    index.html へフォールバック (history routing)
  - `vite.config.ts` の asset path 確認 (base `/` ならそのまま)
  - ビルド手順: `npm --prefix web run build` → `go build`
    (embed 対象に `web/dist` が必要。Makefile or CI job)
  - 任意で CSP/security headers
- 運用: バイナリ1個 + DB ファイル + (pulls 用) git repo
- MPA版 (go-classic-mpa-sample) と同じ単一バイナリ構成になり、
  移植元との思想が一致する
- 未ログインの着地に注意: dev token なしだと index route の
  `homeSlug` が保護された workspace 一覧を叩いて 401 → エラーページ
  に落ち、login へ誘導しない (A/B 共通の既存課題。ログイン誘導は
  別途必要)

## 案B: 別配備 + Bearer token (SPA は別オリジン)

SPA を静的ホスト/CDN に置き、API は別オリジン。

- 必要な実装 (Aより大きい):
  - API 側に CORS 対応 (origin allowlist)
  - cookie セッション運用なら: fetchJSON/fetchText に
    `credentials: 'include'` が必須 (現在はデフォルト same-origin)。
    SameSite=None + Secure が要るのは**別サイト** (eTLD+1 が違う)
    場合のみ — 別オリジンでも同一サイトなら不要
  - SameSite=None 運用には CSRF 対策 (token or Origin 検証) が要る
  - Bearer のみ運用なら: ブラウザ向けの token 発行経路がない。
    login/register は cookie を返すだけで、api_tokens の発行は
    ローカル CLI 経路のみ → 発行エンドポイントか管理UIが要る。
    加えて token を JS に保持する設計は XSS 露出が増える
    (localStorage かメモリか)
  - SPA 側に API base URL 設定 (環境ごとのビルド or runtime config)
- 運用: デプロイが2系統 (静的配信 + API)。CDN に乗せられるのは利点
- 単一ユーザー/小規模用途では複雑さが見合わない

## 判断基準

| 観点 | A (embed) | B (別配備) |
| --- | --- | --- |
| デプロイ | 1バイナリ | 2系統 |
| 認証 | cookie そのまま | CORS+CSRF or Bearer 管理 |
| フロント変更 | asset path 程度 | base URL 設定 + 認証持ち回り |
| CDN/スケール | 不要なら問題なし | 静的資産を CDN 化できる |
| MPA版との整合 | 同じ構成 | 異なる |

今のところ CDN 化の需要はないので A 寄りだが、保留。
決定したら embed/配信実装は別PR、TODO.md の当該項目を更新する。
