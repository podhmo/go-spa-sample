// Command zatu is the gh-like CLI client for the issue tracker —
// a separate binary from the server (docs/sketch/ja/cli-and-web-api.md,
// 案2: cmd/zatu 別バイナリ). Distributing a gh-style client must not
// require shipping the server with it, so unlike `init` these commands
// are not dispatched through the server binary's argv.
//
//	zatu issue list|view|create|edit|close|reopen|delete
//	zatu comment create N --body B
//	zatu label list|create|delete
//	zatu discussion list|view|create|edit|delete
//	zatu discussion comment create N --body B [--thread T]
//	zatu discussion answer N --thread T [--unmark]
//	zatu discussion categories
//	zatu workspace list|create SLUG|delete SLUG
//	zatu auth token --user NAME | token list | token revoke  (local mode only)
//	zatu api <path> [-X METHOD] [-f k=v]... [-F k=v]...       (remote mode only)
//	zatu completion bash|zsh|fish
//
// Every level accepts -h/--help — "zatu --help" for the command surface,
// "zatu <group> --help" for a group's actions, "<action> --help" for flags.
//
// Backend selection: --server/--token (or ZATUDAN_SERVER/ZATUDAN_TOKEN) for
// remote mode over /api/v1; otherwise -db/ZATUDAN_DB opens SQLite directly.
package main

import (
	"context"
	"os"

	"github.com/podhmo/go-spa-sample/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), cli.Options{Args: os.Args[1:]}))
}
