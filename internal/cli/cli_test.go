package cli

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/auth"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// run exercises the CLI against a real store in a temp DB via the Options
// seams — the same path main takes minus the os.Args dance.
func run(t *testing.T, o Options, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	o.Args = args
	o.Stdout = &out
	o.Stderr = &errBuf
	if o.Stdin == nil {
		o.Stdin = strings.NewReader("")
	}
	code = Run(context.Background(), o)
	return code, out.String(), errBuf.String()
}

// localOptions points every command at dbPath's temp database.
func localOptions(t *testing.T, dbPath string) Options {
	t.Helper()
	return Options{
		OpenLocal: func(ctx context.Context, _ string, ws string) (Backend, io.Closer, error) {
			st, err := store.Open(dbPath)
			if err != nil {
				return nil, nil, err
			}
			be, err := st.In(ctx, ws)
			if err != nil {
				st.Close()
				return nil, nil, err
			}
			return be, st, nil
		},
		OpenTokenStore: func(string) (TokenStore, error) {
			return store.Open(dbPath)
		},
		OpenRemote: func(string, string, string) (Backend, error) {
			t.Fatal("remote backend used in local-mode test")
			return nil, nil
		},
		OpenBrowser: func(string) error {
			t.Fatal("browser used without -w")
			return nil
		},
	}
}

func seedIssue(t *testing.T, dbPath string, title string) int64 {
	t.Helper()
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateIssue(context.Background(), 1,
		store.IssueInput{Title: title, Body: "body of " + title}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIssueListAndViewLocal(t *testing.T) {
	db := t.TempDir() + "/test.db"
	id := seedIssue(t, db, "first issue")
	o := localOptions(t, db)

	code, out, _ := run(t, o, "issue", "list")
	if code != ExitOK {
		t.Fatalf("issue list: exit %d", code)
	}
	if !strings.Contains(out, "first issue") {
		t.Fatalf("issue list missing title:\n%s", out)
	}

	code, out, _ = run(t, o, "issue", "view", itoa(id))
	if code != ExitOK || !strings.Contains(out, "title:\tfirst issue") {
		t.Fatalf("issue view: exit %d\n%s", code, out)
	}
}

// --markdown prints the same document the /markdown endpoints serve —
// the export package shared by all three surfaces.
func TestIssueViewMarkdown(t *testing.T) {
	db := t.TempDir() + "/test.db"
	id := seedIssue(t, db, "first issue")
	o := localOptions(t, db)

	code, out, _ := run(t, o, "comment", "create", itoa(id), "--body", "me too")
	if code != ExitOK {
		t.Fatalf("comment create: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "issue", "view", itoa(id), "--markdown")
	if code != ExitOK {
		t.Fatalf("view --markdown: exit %d", code)
	}
	for _, want := range []string{"# first issue\n", "- **Status**: open\n", "body of first issue", "## Comments", "me too"} {
		if !strings.Contains(out, want) {
			t.Errorf("--markdown missing %q:\n%s", want, out)
		}
	}

	code, _, _ = run(t, o, "issue", "view", itoa(id), "--markdown", "--json", "title")
	if code != ExitUsage {
		t.Fatalf("--markdown --json: exit %d, want %d", code, ExitUsage)
	}
	code, _, _ = run(t, o, "issue", "view", itoa(id), "--markdown", "--comments")
	if code != ExitUsage {
		t.Fatalf("--markdown --comments: exit %d, want %d", code, ExitUsage)
	}
}

func TestIssueCreateJSONAndLifecycle(t *testing.T) {
	db := t.TempDir() + "/test.db"
	o := localOptions(t, db)

	code, out, _ := run(t, o, "issue", "create", "--title", "second", "--body", "b2")
	if code != ExitOK || !strings.Contains(out, "created issue #1") {
		t.Fatalf("create: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "issue", "list", "--state", "all", "--json", "id,title")
	if code != ExitOK {
		t.Fatalf("list --json: exit %d", code)
	}
	if !strings.Contains(out, `"id": 1`) || !strings.Contains(out, `"title": "second"`) {
		t.Fatalf("--json output missing fields:\n%s", out)
	}
	if strings.Contains(out, `"body"`) {
		t.Fatalf("--json leaked unrequested field:\n%s", out)
	}

	for _, tc := range []struct {
		action, want string
	}{
		{"close", "closed issue #1"},
		{"reopen", "reopened issue #1"},
		{"delete", "deleted issue #1"},
	} {
		code, out, _ = run(t, o, "issue", tc.action, "1")
		if code != ExitOK || !strings.Contains(out, tc.want) {
			t.Fatalf("issue %s: exit %d want %q got\n%s", tc.action, code, tc.want, out)
		}
	}
}

func TestIssueEditAndComment(t *testing.T) {
	db := t.TempDir() + "/test.db"
	id := seedIssue(t, db, "edit me")
	o := localOptions(t, db)

	code, _, _ := run(t, o, "issue", "edit", itoa(id), "--title", "edited")
	if code != ExitOK {
		t.Fatalf("edit: exit %d", code)
	}
	code, out, _ := run(t, o, "issue", "view", itoa(id))
	if code != ExitOK || !strings.Contains(out, "title:\tedited") {
		t.Fatalf("view after edit:\n%s", out)
	}

	code, out, _ = run(t, o, "comment", "create", itoa(id), "--body", "looks good")
	if code != ExitOK || !strings.Contains(out, "created comment #1") {
		t.Fatalf("comment create: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "issue", "view", itoa(id), "--comments")
	if code != ExitOK || !strings.Contains(out, "looks good") {
		t.Fatalf("view --comments:\n%s", out)
	}
}

func TestBodyFileStdin(t *testing.T) {
	db := t.TempDir() + "/test.db"
	o := localOptions(t, db)
	o.Stdin = strings.NewReader("stdin body\n")

	code, _, _ := run(t, o, "issue", "create", "--title", "piped", "--body-file", "-")
	if code != ExitOK {
		t.Fatalf("create --body-file -: exit %d", code)
	}
	code, out, _ := run(t, o, "issue", "view", "1")
	if code != ExitOK || !strings.Contains(out, "stdin body") {
		t.Fatalf("view piped body:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	db := t.TempDir() + "/test.db"
	o := localOptions(t, db)
	for _, args := range [][]string{
		{"issue"},                    // missing action
		{"issue", "bogus"},           // unknown action
		{"issue", "view"},            // missing number
		{"issue", "view", "abc"},     // bad number
		{"issue", "create"},          // missing --title
		{"issue", "view", "1", "-w"}, // -w without --server
		{"api", "/issues"},           // remote-only
		{"auth", "token"},            // missing --user
		{"discussion"},               // missing action
		{"discussion", "bogus"},      // unknown action
		{"wat"},                      // unknown command
	} {
		code, _, errOut := run(t, o, args...)
		if code != ExitUsage {
			t.Fatalf("args %v: want exit %d, got %d (stderr %q)", args, ExitUsage, code, errOut)
		}
	}
}

// -h/--help works at every level — top, group and leaf — and always
// exits 0 with the help text on stdout (stderr stays clean for errors).
func TestHelp(t *testing.T) {
	db := t.TempDir() + "/test.db"
	o := localOptions(t, db)

	// Top level: bare invocation, "help" and the flag forms.
	for _, args := range [][]string{
		{}, {"help"}, {"--help"}, {"-h"},
	} {
		code, out, errOut := run(t, o, args...)
		if code != ExitOK || !strings.Contains(out, "issue list|view|create") {
			t.Fatalf("args %v: exit %d, want usage on stdout:\n%s\nstderr: %s", args, code, out, errOut)
		}
	}

	// Group level prints the group's actions.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"issue", "--help"}, "usage: zatu issue <action>"},
		{[]string{"issue", "help"}, "usage: zatu issue <action>"},
		{[]string{"discussion", "-h"}, "usage: zatu discussion <action>"},
		{[]string{"comment", "--help"}, "usage: zatu comment <action>"},
		{[]string{"label", "--help"}, "usage: zatu label <action>"},
		{[]string{"auth", "--help"}, "usage: zatu auth token"},
		{[]string{"workspace", "--help"}, "usage: zatu workspace <action>"},
		{[]string{"discussion", "comment", "--help"}, "usage: zatu discussion comment <action>"},
	} {
		code, out, _ := run(t, o, tc.args...)
		if code != ExitOK || !strings.Contains(out, tc.want) {
			t.Fatalf("args %v: exit %d, want %q:\n%s", tc.args, code, tc.want, out)
		}
	}

	// Leaf level prints the usage line plus the flag list (the flag
	// definitions themselves, e.g. -state).
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"issue", "list", "--help"}, []string{"usage: zatu issue list [flags]", "-state"}},
		{[]string{"issue", "view", "-h"}, []string{"usage: zatu issue view N [flags]", "-comments"}},
		{[]string{"issue", "create", "--help"}, []string{"usage: zatu issue create --title T", "-body-file"}},
		{[]string{"comment", "create", "--help"}, []string{"usage: zatu comment create N --body B", "-body-file"}},
		{[]string{"discussion", "answer", "--help"}, []string{"usage: zatu discussion answer N --thread T", "-unmark"}},
		{[]string{"discussion", "categories", "--help"}, []string{"usage: zatu discussion categories [flags]", "-json"}},
		{[]string{"auth", "token", "--help"}, []string{"usage: zatu auth token --user NAME", "-user"}},
		{[]string{"api", "--help"}, []string{"usage: zatu api <path>", "-method"}},
		{[]string{"completion", "--help"}, []string{"usage: zatu completion bash|zsh|fish"}},
	} {
		code, out, _ := run(t, o, tc.args...)
		if code != ExitOK {
			t.Fatalf("args %v: exit %d, want %d", tc.args, code, ExitOK)
		}
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Fatalf("args %v: help missing %q:\n%s", tc.args, w, out)
			}
		}
	}

	// --help after a positional still resolves (flag re-parse in parseLeaf).
	code, out, _ := run(t, o, "issue", "view", "1", "--help")
	if code != ExitOK || !strings.Contains(out, "usage: zatu issue view N") {
		t.Fatalf("view 1 --help: exit %d\n%s", code, out)
	}
}

func TestAuthTokenLocal(t *testing.T) {
	db := t.TempDir() + "/test.db"
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := st.CreateUser(context.Background(), "carol", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UserByName(context.Background(), "carol"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	o := localOptions(t, db)
	code, out, errOut := run(t, o, "auth", "token", "--user", "carol")
	if code != ExitOK {
		t.Fatalf("auth token: exit %d\n%s", code, errOut)
	}
	token := strings.TrimSpace(out)
	if token == "" || !strings.Contains(errOut, `"carol"`) {
		t.Fatalf("auth token output: stdout %q stderr %q", out, errOut)
	}

	st, err = store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	u, err := st.APITokenUser(context.Background(), token)
	if err != nil {
		t.Fatalf("issued token does not resolve: %v", err)
	}
	if u.ID != uid {
		t.Fatalf("token resolves to user %d, want %d", u.ID, uid)
	}

	code, _, _ = run(t, o, "auth", "token", "--user", "nobody")
	if code != ExitError {
		t.Fatalf("auth token --user nobody: want exit %d got %d", ExitError, code)
	}
}

// TestAuthTokenListRevoke exercises the management side: the list shows
// hash prefixes (never plaintext), revoke deletes by prefix id or by an
// explicit --token, and --server is refused everywhere.
func TestAuthTokenListRevoke(t *testing.T) {
	ctx := context.Background()
	db := t.TempDir() + "/test.db"
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, "dave", "hashed"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	o := localOptions(t, db)

	var tokens []string
	for i := 0; i < 2; i++ {
		code, out, _ := run(t, o, "auth", "token", "--user", "dave")
		if code != ExitOK {
			t.Fatalf("auth token #%d: exit %d", i, code)
		}
		tokens = append(tokens, strings.TrimSpace(out))
	}

	code, out, _ := run(t, o, "auth", "token", "list")
	if code != ExitOK {
		t.Fatalf("token list: exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("token list shows %d rows, want 2:\n%s", len(lines), out)
	}
	// Rows print a hash prefix + owner — never the plaintext secret.
	for _, tok := range tokens {
		if strings.Contains(out, tok) {
			t.Fatalf("token list leaked plaintext token")
		}
	}
	if !strings.Contains(out, "dave") {
		t.Fatalf("token list missing user:\n%s", out)
	}

	code, out, _ = run(t, o, "auth", "token", "list", "--json", "id,user")
	if code != ExitOK || !strings.Contains(out, `"id"`) || !strings.Contains(out, `"dave"`) {
		t.Fatalf("token list --json: exit %d\n%s", code, out)
	}

	// Revoke the first listed row by its prefix id — and figure out which
	// plaintext it was, so the --token path revokes the *other* token.
	prefix := strings.Fields(lines[0])[0]
	remaining := tokens[0]
	for _, tok := range tokens {
		if strings.HasPrefix(auth.HashToken(tok), prefix) {
			remaining = tokens[1]
			if tok == tokens[1] {
				remaining = tokens[0]
			}
		}
	}
	code, _, errOut := run(t, o, "auth", "token", "revoke", prefix)
	if code != ExitOK {
		t.Fatalf("token revoke %s: exit %d\n%s", prefix, code, errOut)
	}
	code, out, _ = run(t, o, "auth", "token", "list")
	if code != ExitOK || len(strings.Split(strings.TrimSpace(out), "\n")) != 1 {
		t.Fatalf("after revoke: %d / %q", code, out)
	}

	code, _, _ = run(t, o, "auth", "token", "revoke", "zzzz-nope")
	if code != ExitError {
		t.Fatalf("revoke unknown id: want %d got %d", ExitError, code)
	}
	code, _, _ = run(t, o, "auth", "token", "revoke", "--token", remaining)
	if code != ExitOK {
		t.Fatalf("revoke --token: exit %d", code)
	}
	code, out, _ = run(t, o, "auth", "token", "list")
	if code != ExitOK || strings.TrimSpace(out) != "" {
		t.Fatalf("after --token revoke: %d / %q", code, out)
	}

	for _, args := range [][]string{
		{"auth", "token", "--server", "http://x", "--user", "dave"},
		{"auth", "token", "list", "--server", "http://x"},
		{"auth", "token", "revoke", "abc", "--server", "http://x"},
	} {
		if code, _, _ := run(t, o, args...); code != ExitUsage {
			t.Fatalf("%v: want usage exit %d got %d", args, ExitUsage, code)
		}
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// Regression: Devin Review findings — validation, -w ordering, --json shape.
func TestReviewRegressions(t *testing.T) {
	db := t.TempDir() + "/test.db"
	id := seedIssue(t, db, "target")
	o := localOptions(t, db)

	code, _, _ := run(t, o, "issue", "create", "--title", "no server", "-w")
	if code != ExitUsage {
		t.Fatalf("create -w: exit %d, want %d", code, ExitUsage)
	}
	code, out, _ := run(t, o, "issue", "list", "--state", "all")
	if strings.Contains(out, "no server") {
		t.Fatalf("create -w still wrote an issue:\n%s", out)
	}

	code, _, _ = run(t, o, "issue", "edit", itoa(id), "--title", "   ")
	if code != ExitError {
		t.Fatalf("edit --title '   ': exit %d, want %d", code, ExitError)
	}
	code, out, _ = run(t, o, "issue", "view", itoa(id))
	if !strings.Contains(out, "title:\ttarget") {
		t.Fatalf("bad edit was saved:\n%s", out)
	}
	code, _, _ = run(t, o, "comment", "create", itoa(id),
		"--body", strings.Repeat("x", 10001))
	if code != ExitError {
		t.Fatalf("oversized comment: exit %d, want %d", code, ExitError)
	}

	code, out, _ = run(t, o, "issue", "view", itoa(id), "--comments",
		"--json", "title,comments")
	if code != ExitOK {
		t.Fatalf("view --comments --json: exit %d", code)
	}
	if !strings.Contains(out, `"title": "target"`) || !strings.Contains(out, `"comments"`) {
		t.Fatalf("--json output:\n%s", out)
	}
}

// --template renders views through a Go template (decision: gh's --jq is
// not re-implemented; Go templates are the stdlib answer), and a non-TTY
// stdout never emits ANSI escapes.
func TestTemplateAndNoColor(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedIssue(t, db, "tmpl issue")
	o := localOptions(t, db)

	code, out, _ := run(t, o, "issue", "list", "--state", "all",
		"--template", "{{range .}}{{.title}}\n{{end}}")
	if code != ExitOK || !strings.Contains(out, "tmpl issue") {
		t.Fatalf("issue list --template: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "issue", "list", "--state", "all")
	if code != ExitOK {
		t.Fatalf("issue list: exit %d", code)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("non-TTY output must not contain ANSI escapes:\n%q", out)
	}

	if code, _, _ := run(t, o, "issue", "list", "--json", "id", "--template", "x"); code != ExitUsage {
		t.Fatalf("--json + --template: want %d got %d", ExitUsage, code)
	}
	if code, _, _ := run(t, o, "issue", "list", "--template", "{{."); code != ExitUsage {
		t.Fatalf("bad template: want %d got %d", ExitUsage, code)
	}
}

// Comment moderation verbs on issue comments: edit/delete/hide/unhide
// with reason validation and the unhide --reason guard.
func TestCommentModerationLocal(t *testing.T) {
	db := t.TempDir() + "/test.db"
	id := seedIssue(t, db, "moderate me")
	o := localOptions(t, db)

	code, out, _ := run(t, o, "comment", "create", itoa(id), "--body", "first")
	if code != ExitOK || !strings.Contains(out, "created comment #1") {
		t.Fatalf("create: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "comment", "edit", itoa(id), "1", "--body", "revised")
	if code != ExitOK || !strings.Contains(out, "updated comment #1") {
		t.Fatalf("edit: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "issue", "view", itoa(id), "--comments")
	if code != ExitOK || !strings.Contains(out, "revised") || !strings.Contains(out, "[edited]") {
		t.Fatalf("view after edit:\n%s", out)
	}

	// Reason validation: missing and unknown reasons are usage errors.
	for _, args := range [][]string{
		{"comment", "hide", itoa(id), "1"},
		{"comment", "hide", itoa(id), "1", "--reason", "bogus"},
		{"comment", "unhide", itoa(id), "1", "--reason", "spam"},
	} {
		if code, _, _ := run(t, o, args...); code != ExitUsage {
			t.Fatalf("args %v: want exit %d, got %d", args, ExitUsage, code)
		}
	}

	code, out, _ = run(t, o, "comment", "hide", itoa(id), "1", "--reason", "spam")
	if code != ExitOK || !strings.Contains(out, "hidden comment #1") {
		t.Fatalf("hide: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "issue", "view", itoa(id), "--comments")
	if code != ExitOK || !strings.Contains(out, "[hidden: spam]") {
		t.Fatalf("view after hide:\n%s", out)
	}

	code, out, _ = run(t, o, "comment", "unhide", itoa(id), "1")
	if code != ExitOK || !strings.Contains(out, "unhidden comment #1") {
		t.Fatalf("unhide: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "comment", "delete", itoa(id), "1")
	if code != ExitOK || !strings.Contains(out, "deleted comment #1") {
		t.Fatalf("delete: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "issue", "view", itoa(id), "--comments")
	if strings.Contains(out, "revised") {
		t.Fatalf("deleted comment still shows:\n%s", out)
	}
}
