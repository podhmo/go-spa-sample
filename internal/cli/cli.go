// Package cli implements the gh-like command surface of the cmd/zatu
// binary (docs/sketch/ja/cli-and-web-api.md, Phase 1 and 3). The commands
// only know the Backend interface: locally it is *store.Store over -db,
// remotely it is *client.Client over /api/v1 (--server/--token).
//
// Command grammar (gh-style):
//
//	zatu issue list|view|create|edit|close|reopen|delete
//	zatu comment create|edit|delete|hide|unhide
//	zatu label list
//	zatu discussion list|view|create|edit|delete
//	zatu discussion comment create|edit|delete|hide|unhide
//	zatu discussion answer N --thread T [--unmark]
//	zatu discussion categories
//	zatu workspace list|create SLUG [--name N] [--description D]|delete SLUG
//	zatu auth token --user NAME | token list | token revoke  (local mode only)
//	zatu api <path> [-X METHOD] [-f k=v]... [-F k=v]...       (remote mode only)
//	zatu completion bash|zsh|fish
//
// Every level accepts -h/--help: it prints that level's usage to stdout
// and exits 0 (a leaf's flag list comes from flag.PrintDefaults).
//
// Exit codes: 0 success / 1 runtime error / 2 usage error.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/client"
	"github.com/podhmo/go-spa-sample/internal/gitrepo"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// Backend is the persistence contract the CLI needs — the same "service
// contract" web.Store and api.Store already are (the design memo's 同一契約
// の 2 実装). *store.Store satisfies it directly; *client.Client satisfies
// it over HTTP. Commands never learn which one they are talking to.
type Backend interface {
	ListIssues(ctx context.Context, f store.ListFilter) ([]*store.Issue, int, error)
	GetIssue(ctx context.Context, id int64) (*store.Issue, error)
	CreateIssue(ctx context.Context, in store.IssueInput, authorID int64) (int64, error)
	UpdateIssue(ctx context.Context, id int64, in store.IssueInput) error
	SetIssueStatus(ctx context.Context, id int64, status string) error
	DeleteIssue(ctx context.Context, id int64) error
	ListComments(ctx context.Context, issueID int64) ([]*store.Comment, error)
	CreateComment(ctx context.Context, issueID int64, body string, authorID int64) (int64, error)
	UpdateComment(ctx context.Context, issueID, commentID int64, body string) error
	DeleteComment(ctx context.Context, issueID, commentID int64) error
	SetCommentHidden(ctx context.Context, issueID, commentID int64, reason string) error
	ListLabels(ctx context.Context, kind string) ([]*store.Label, error)
	CreateLabel(ctx context.Context, kind string, in store.LabelInput, master bool) (int64, error)
	ArchiveLabel(ctx context.Context, id int64, archived bool) error

	ListDiscussions(ctx context.Context, f store.DiscussionListFilter) ([]*store.Discussion, int, error)
	GetDiscussion(ctx context.Context, id int64) (*store.Discussion, error)
	CreateDiscussion(ctx context.Context, in store.DiscussionInput, authorID int64) (int64, error)
	UpdateDiscussion(ctx context.Context, id int64, in store.DiscussionInput) error
	DeleteDiscussion(ctx context.Context, id int64) error
	ListDiscussionThreads(ctx context.Context, discussionID int64) ([]*store.DiscussionThread, error)
	CreateDiscussionThread(ctx context.Context, discussionID int64, body string, authorID int64) (int64, error)
	CreateDiscussionComment(ctx context.Context, discussionID, threadID int64, body string, authorID int64) (int64, error)
	SetThreadAnswer(ctx context.Context, discussionID, threadID int64, answer bool) error
	UpdateDiscussionThread(ctx context.Context, discussionID, threadID int64, body string) error
	DeleteDiscussionThread(ctx context.Context, discussionID, threadID int64) error
	SetDiscussionThreadHidden(ctx context.Context, discussionID, threadID int64, reason string) error
	UpdateDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64, body string) error
	DeleteDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64) error
	SetDiscussionCommentHidden(ctx context.Context, discussionID, threadID, commentID int64, reason string) error
	ListDiscussionCategories(ctx context.Context) ([]*store.DiscussionCategory, error)

	// Pull requests — workspace-independent (docs/sketch/ja/
	// pull-requests.md §7). `zatu pr` drives these; local mode does the
	// git work itself and calls the store-level verbs, remote mode maps
	// them onto /api/v1/pulls*.
	ListPullRequests(ctx context.Context, f store.PRListFilter) ([]*store.PullRequest, int, error)
	GetPullRequest(ctx context.Context, id int64) (*store.PullRequest, error)
	CreatePullRequest(ctx context.Context, in store.PRInput, authorID int64, pin func(int64) error) (int64, error)
	UpdatePullRequest(ctx context.Context, id int64, in store.PRInput) error
	DeletePullRequest(ctx context.Context, id int64) error
	ClosePullRequest(ctx context.Context, id int64, headSHA, mergeBaseSHA string) error
	ReopenPullRequest(ctx context.Context, id int64) error
	MarkMerged(ctx context.Context, id int64, mi store.PRMergeInfo) error
	ListPRComments(ctx context.Context, prID int64) ([]*store.PRComment, error)
	CreatePRComment(ctx context.Context, prID int64, in store.PRCommentInput, authorID int64) (int64, error)
	UpdatePRComment(ctx context.Context, prID, commentID int64, body string) error
	DeletePRComment(ctx context.Context, prID, commentID int64) error
	SetPRCommentHidden(ctx context.Context, prID, commentID int64, reason string) error

	// Workspace management is unscoped — the same backend serves them
	// regardless of which workspace it was opened for.
	ListWorkspaces(ctx context.Context) ([]*store.Workspace, error)
	CreateWorkspace(ctx context.Context, in store.WorkspaceInput) (int64, error)
	DeleteWorkspace(ctx context.Context, slug string) error
}

// TokenStore is the extra contract "auth token" needs. Token management
// is a local-mode operation: it reads and writes the api_tokens table
// directly instead of going through HTTP, so it needs the concrete
// store, not just Backend.
type TokenStore interface {
	UserByName(ctx context.Context, name string) (*store.User, error)
	CreateAPIToken(ctx context.Context, userID int64) (string, error)
	ListAPITokens(ctx context.Context) ([]*store.APIToken, error)
	DeleteAPIToken(ctx context.Context, token string) error
	DeleteAPITokenByID(ctx context.Context, hash string) error
	Close() error
}

// Options carries the invocation environment. The zero value is usable:
// nil fields fall back to os.Stdout/os.Stderr/os.Stdin, os.Getenv, the real
// store/client constructors, and the platform's browser opener.
type Options struct {
	Args   []string // argv after the program name; Args[0] is the resource group
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Getenv func(string) string

	// Factory seams for tests (and for keeping package cli independent of
	// *store.Store's concrete type). Defaults hit the real implementations.
	// ResolveRepo opens the git repo for local `pr` commands; nil (repo
	// undetected) is the "not a git repository" exit path, an error only
	// for an explicit --repo. Unused in remote mode.
	ResolveRepo    func(ctx context.Context, dbPath, repoPath string) (*gitrepo.Repo, error)
	OpenLocal      func(ctx context.Context, dbPath, ws string) (Backend, io.Closer, error)
	OpenRemote     func(server, token, ws string) (Backend, error)
	OpenTokenStore func(dbPath string) (TokenStore, error)
	OpenBrowser    func(url string) error
}

// Exit codes returned by Run.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// Run executes the CLI described by o and returns the process exit code.
func Run(ctx context.Context, o Options) int {
	o = o.withDefaults()
	args := o.Args
	if len(args) == 0 || isHelpArg(args[0]) {
		o.printUsage(o.Stdout)
		return ExitOK
	}
	var err error
	switch args[0] {
	case "issue":
		err = issueCmd(ctx, o, args[1:])
	case "label":
		err = labelCmd(ctx, o, args[1:])
	case "comment":
		err = commentCmd(ctx, o, args[1:])
	case "auth":
		err = authCmd(ctx, o, args[1:])
	case "api":
		err = apiCmd(ctx, o, args[1:])
	case "completion":
		err = completionCmd(ctx, o, args[1:])
	case "pr":
		err = prCmd(ctx, o, args[1:])
	case "discussion":
		err = discussionCmd(ctx, o, args[1:])
	case "workspace":
		err = workspaceCmd(ctx, o, args[1:])
	default:
		err = usageError{msg: fmt.Sprintf("unknown command %q", args[0])}
	}
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, errHelp) {
		return ExitOK
	}
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintf(o.Stderr, "Error: %v\n\n", ue.msg)
		o.printUsage(o.Stderr)
		return ExitUsage
	}
	fmt.Fprintf(o.Stderr, "Error: %v\n", err)
	return ExitError
}

// usageError marks misuse (bad flags, missing args) — exit 2 with usage.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// errHelp means -h/--help was passed and the relevant help text was
// already printed; Run maps it to a clean exit rather than an error.
var errHelp = errors.New("cli: help requested")

// isHelpArg reports whether s asks for help at any command level.
func isHelpArg(s string) bool {
	return s == "help" || s == "--help" || s == "-h"
}

func (o Options) withDefaults() Options {
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.OpenLocal == nil {
		o.OpenLocal = func(ctx context.Context, dbPath, ws string) (Backend, io.Closer, error) {
			st, err := store.Open(dbPath)
			if err != nil {
				return nil, nil, err
			}
			// Match the server's startup: ensure master data (default
			// workspace, discussion categories) so a fresh database is
			// usable without going through the server first.
			if err := st.SeedMasterData(ctx); err != nil {
				st.Close()
				return nil, nil, err
			}
			be, err := st.In(ctx, ws)
			if err != nil {
				st.Close()
				return nil, nil, err
			}
			return be, st, nil
		}
	}
	if o.ResolveRepo == nil {
		o.ResolveRepo = func(ctx context.Context, dbPath, repoPath string) (*gitrepo.Repo, error) {
			return gitrepo.Resolve(ctx, gitrepo.Options{
				RepoPath: repoPath,
				EnvRepo:  o.Getenv("ZATUDAN_REPO"),
				DBPath:   dbPath,
			})
		}
	}
	if o.OpenRemote == nil {
		o.OpenRemote = func(server, token, ws string) (Backend, error) {
			return client.New(server, token).In(ws), nil
		}
	}
	if o.OpenTokenStore == nil {
		o.OpenTokenStore = func(dbPath string) (TokenStore, error) {
			return store.Open(dbPath)
		}
	}
	if o.OpenBrowser == nil {
		o.OpenBrowser = openBrowser
	}
	return o
}

func (o Options) printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: zatu <command> [<flags>]

Commands:
  issue list|view|create|edit|close|reopen|delete
  pr list|view|diff|create|edit|close|reopen|merge|delete
  pr comment create|edit|delete|hide|unhide
  comment create|edit|delete|hide|unhide
  label list [--kind issue|discussion]
  label create NAME --color HEX [--kind K] [--shared]
  label delete NAME [--kind K]   archives the label (still shown on existing issues)
  discussion list|view|create|edit|delete
  discussion comment create|edit|delete|hide|unhide
  discussion answer N --thread T [--unmark]
  discussion categories          list categories (--category slugs)
  auth token --user NAME     issue a per-user API token (local mode only)
  auth token list [--user NAME]      list issued tokens (local mode only)
  auth token revoke (ID|--token T)   revoke a token (local mode only)
  workspace list             list workspaces
  workspace create SLUG      create a workspace [--name N] [--description D]
  workspace delete SLUG      delete a workspace and its contents
  api <path>                 raw call to /api/v1 (remote mode only)
                             [-X METHOD] [-f name=value]... [-F name=value]...
  completion bash|zsh|fish   print a shell completion script
  help                       show this text

Every level accepts -h/--help: "zatu issue --help" lists a group's actions,
"zatu issue list --help" prints one action's arguments and flags.

Backend selection:
  --server URL   remote mode via /api/v1 (env ZATUDAN_SERVER); requires --token
  --token T      Bearer token for remote mode (env ZATUDAN_TOKEN)
  --db PATH      local mode, direct SQLite access (env ZATUDAN_DB, default zatudan.db)
  --workspace S  workspace for issue/discussion commands (env ZATUDAN_WORKSPACE, default "default")

Exit codes: 0 ok, 1 error, 2 usage.
`)
}

// backendFlags are the flags every resource command accepts. They are
// defined per-leaf-command flagset because flag.Parse stops at the first
// positional argument — `zatu issue list --db x` must parse --db itself.
type backendFlags struct {
	db     string
	server string
	token  string
	webURL string
	ws     string
}

func (o Options) addBackendFlags(fs *flag.FlagSet) *backendFlags {
	b := &backendFlags{}
	fs.StringVar(&b.db, "db", o.Getenv("ZATUDAN_DB"), "SQLite database file for local mode (env ZATUDAN_DB, default zatudan.db)")
	fs.StringVar(&b.server, "server", o.Getenv("ZATUDAN_SERVER"), "server base URL for remote mode (env ZATUDAN_SERVER)")
	fs.StringVar(&b.token, "token", o.Getenv("ZATUDAN_TOKEN"), "Bearer token for remote mode (env ZATUDAN_TOKEN)")
	// --help prints every flag's default; the token is a credential, so
	// blank the recorded default — the flag keeps its value, the help
	// text keeps the env name as the discoverable way to set it.
	fs.Lookup("token").DefValue = ""
	fs.StringVar(&b.webURL, "web-url", o.Getenv("ZATUDAN_WEB"), "web UI base URL for -w/--web links (env ZATUDAN_WEB, default --server)")
	fs.StringVar(&b.ws, "workspace", o.Getenv("ZATUDAN_WORKSPACE"), `workspace slug for issue/discussion commands (env ZATUDAN_WORKSPACE, default "default")`)
	return b
}

// dbPath resolves the local database path, defaulting to zatudan.db like
// the server's -db flag.
// wsSlug resolves the workspace scope, defaulting to "default" — the one
// every fresh database starts with.
func (b *backendFlags) wsSlug() string {
	if b.ws == "" {
		return store.DefaultWorkspaceSlug
	}
	return b.ws
}

func (b *backendFlags) dbPath() string {
	if b.db == "" {
		return "zatudan.db"
	}
	return b.db
}

// backend opens the Backend for the given flags: --server/ZATUDAN_SERVER
// selects remote mode, anything else opens the local database. The
// returned io.Closer is nil for remote backends.
func (o Options) backend(ctx context.Context, b *backendFlags) (Backend, io.Closer, error) {
	ws := b.wsSlug()
	if b.server == "" && b.ws == "" {
		// "default" may have been deleted or the database may not have
		// it yet — fall back to the first workspace, like the SPA's
		// homeSlug rule. An explicit --workspace still fails loudly.
		if ub, ucloser, err := o.backendIn(ctx, b, ""); err == nil {
			if wss, err := ub.ListWorkspaces(ctx); err == nil && len(wss) > 0 {
				ws = wss[0].Slug
				for _, w := range wss {
					if w.Slug == store.DefaultWorkspaceSlug {
						ws = w.Slug
						break
					}
				}
			}
			closeQuietly(ucloser)
		}
	}
	return o.backendIn(ctx, b, ws)
}

// backendIn opens the Backend scoped to an explicit workspace slug; ws==""
// means unscoped (workspace management verbs must work even when no
// workspace exists — e.g. after deleting default).
func (o Options) backendIn(ctx context.Context, b *backendFlags, ws string) (Backend, io.Closer, error) {
	if b.server != "" {
		warnPlaintext(o.Stderr, b.server, b.token)
		be, err := o.OpenRemote(b.server, b.token, ws)
		return be, nil, err
	}
	return o.OpenLocal(ctx, b.dbPath(), ws)
}

// warnPlaintext warns when an API token is about to travel over
// unencrypted HTTP to a non-loopback host — anyone on the path could
// replay it. A warning rather than an error: http to loopback is the
// normal case, and a hard refusal would also block exotic-but-valid
// setups (a trusted LAN box, an SSH tunnel).
func warnPlaintext(w io.Writer, server, token string) {
	if token == "" {
		return
	}
	u, err := url.Parse(server)
	if err != nil || u.Scheme != "http" {
		return
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return
	}
	fmt.Fprintf(w, "warning: %s is plaintext http; the API token will be sent unencrypted\n", server)
}

// nameFlag is a repeatable string flag (--label a --label b).
type nameFlag []string

func (n *nameFlag) String() string { return strings.Join(*n, ",") }
func (n *nameFlag) Set(v string) error {
	*n = append(*n, v)
	return nil
}

// issueNumber parses a positional issue/comment id argument.
func issueNumber(args []string, i int) (int64, error) {
	return argNumber(args, i, "issue")
}

// argNumber parses a positional id argument; noun names the resource in
// error messages ("issue", "discussion").
func argNumber(args []string, i int, noun string) (int64, error) {
	if len(args) <= i {
		return 0, usageError{msg: "missing " + noun + " number"}
	}
	n, err := strconv.ParseInt(args[i], 10, 64)
	if err != nil || n < 1 {
		return 0, usageError{msg: fmt.Sprintf("invalid %s number %q", noun, args[i])}
	}
	return n, nil
}

// parseLeaf parses a leaf command's args, rejecting leftover positionals.
// argHint is the leaf's argument synopsis for the --help usage line
// ("N --body B"); every leaf implicitly accepts the backend flags, so
// "[flags]" is appended unconditionally. flag.Parse stops at the first
// positional, so it is re-run on the remainder — gh-style
// `issue view 1 --json title` parses the same as `issue view --json title 1`.
// -h/--help prints the leaf's usage and flag list, then bubbles up as
// errHelp.
func (o Options) parseLeaf(fs *flag.FlagSet, argHint string, args []string, allow int) ([]string, error) {
	fs.SetOutput(io.Discard)
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				o.printLeafUsage(fs, argHint)
				return nil, errHelp
			}
			return nil, usageError{msg: err.Error()}
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		if len(positional) > allow {
			return nil, usageError{msg: fmt.Sprintf("unexpected arguments: %s", strings.Join(positional[allow:], " "))}
		}
		args = rest[1:]
	}
	return positional, nil
}

// printLeafUsage writes one leaf command's usage line plus its flag set
// (the flag definitions are the source of truth — no duplicated flag
// lists in help strings). The flag package's own "Usage of ..." header
// stays on io.Discard; this replaces it.
func (o Options) printLeafUsage(fs *flag.FlagSet, argHint string) {
	fmt.Fprintf(o.Stdout, "usage: zatu %s", fs.Name())
	if argHint != "" {
		fmt.Fprintf(o.Stdout, " %s", argHint)
	}
	fmt.Fprint(o.Stdout, " [flags]\n\noptions:\n")
	fs.SetOutput(o.Stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

// readBody resolves --body / --body-file ("-" means stdin), exactly like
// gh's --body-file convention.
func (o Options) readBody(body, bodyFile string) (string, error) {
	if bodyFile == "" {
		return body, nil
	}
	var data []byte
	var err error
	if bodyFile == "-" {
		data, err = io.ReadAll(o.Stdin)
	} else {
		data, err = os.ReadFile(bodyFile)
	}
	if err != nil {
		return "", fmt.Errorf("read body file: %w", err)
	}
	return string(data), nil
}

// resolveLabelIDs maps label names to ids through the backend's label list
// (the CLI takes names like gh's --label, the store takes ids).
func resolveLabelIDs(ctx context.Context, b Backend, kind string, names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, nil
	}
	all, err := b.ListLabels(ctx, kind)
	if err != nil {
		return nil, err
	}
	byName := map[string]int64{}
	for _, l := range all {
		byName[l.Name] = l.ID
	}
	var ids []int64
	for _, n := range names {
		id, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown label: %s", n)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
