# features.md 動作チェック記録

2026-11-25 に `docs/ja/features.md` §6 の受け入れチェックリストを、実起動した SPA + Go API で確認した記録。

## 確認方法

- Go API: `go run ./cmd/zatudan -seed -api-token dev-token -db <一時SQLite>` (`:8080`)
- SPA: `npm --prefix web run dev` (`:5173`)
- Chrome 上の実 UI を主軸に操作。cookie の属性・sliding renewal、password 境界値、CLI local/remote backend、一部 export/API エラー形式は API/CLI で補助確認した
- Pull Requests は git repo あり / `-repo off` の両方で確認した

## 結果

`docs/ja/features.md` §6 の全項目を `[x]` に更新した。主な確認範囲:

- workspace 作成、`/w/{slug}` スコープ、bare URL の誘導、cross-scope rebase と mutation 拒否
- anonymous / login ユーザーの投稿者表示、register/login/logout、cookie session persistence と sliding renewal
- issue の作成・編集・close/reopen・削除、コメント、label lifecycle、archived label の picker/既存付与、21件での pagination
- discussion の category/thread/reply/answer/answered filter/編集/削除/hide/unhide、非 Q&A 化での answer 消去
- PR の repo 未検出時 404、作成エラー、detail、conflict 表示、Files diff、left/right review comment、close/reopen、merge/squash/rebase、markdown/diff export、q/sort/page
- `/api/v1` Bearer fail-closed、`{"error":{"message","fields"}}` 形式、`zatu` CLI local/remote、en/ja 切替、SSE live update
- Markdown の生 HTML 非描画、`javascript:` link の無力化、GFM alert 描画

## 見つかった不足と対応

| 不足 | 再現 | 対応 | 状態 |
| --- | --- | --- | --- |
| コメント投稿成功後に、空になったフォームへ `Body is required.` が表示される | issue/discussion/PR の詳細でコメント投稿 | `CommentForm` が mutation 結果を待ち、成功時だけ native `form.reset()` を呼ぶようにした。controlled value を空にするだけでは `:user-invalid` の interaction state が残るため。投稿中は textarea も disabled にし、成功後の reset が後続入力を消さないようにした | 修正済み |
| backend 停止中の投稿失敗で route error boundary に落ち、draft が見えなくなる | PR detail で本文入力後、backend を止めて投稿 | mutation 失敗時は `router.invalidate()` しないようにした。成功時だけ再取得する。Files tab の行コメントだけ、stale anchor/path エラー時は新しい diff が必要なので例外的に再取得する。backend 停止中も PR detail と draft が残り、復帰後に同じ本文を投稿できることを確認 | 修正済み |
| issue comment の API validation error がフォームの textarea と accessibility 上関連付いていない | 10,001 文字のコメント投稿を 422 で拒否 | `CommentForm` に error を渡し、textarea の `aria-invalid` / `aria-describedby` から alert を参照するようにした | 修正済み |
| comment 編集の保存失敗で editor が閉じ、失敗理由が表示されない | comment 編集 API が 422/network error を返す | mutation の戻り値が失敗のときは editor を開いたままにし、保存中は textarea を disabled にした。編集エラーは対象 textarea の `aria-invalid` / `aria-describedby` に関連付け、status/delete/hide/unhide の失敗は `role="alert"` で表示するようにした | 修正済み |

422 validation failure（10,001 文字のコメント）では、入力本文が保持され、issue 側でも field error がフォーム近くに表示されることを確認した。

## 仕様確認メモ

- rebase merge の dialog に commit message 欄がないのは不足ではなく意図的。rebase は各コミット本文を replay するため、単一 custom message は適用できない。merge/squash では任意メッセージを確認した
- i18n で `Issues` / `Discussions` / `Pulls` / `Files` が英語のままなのは、MPA 語彙に合わせた意図的な表記

## 残件

受け入れチェックリスト上の未対応項目はない。`TODO.md` の「本番構成の決定」は運用構成の残件であり、今回の機能パリティ確認の対象外。
