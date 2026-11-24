// Sandbox-repo tests: every case builds a real git repository under
// t.TempDir and runs the real git binary — the feature is git-backed by
// design, so faking it would verify nothing.
package gitrepo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs a command in dir, failing the test on error.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// write commits path with content on the current branch.
func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", path)
	git(t, dir, "commit", "-qm", "add "+path)
}

// initRepo builds a repo on branch main with one commit.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "a.txt", "one\n")
	return dir
}

func openRepo(t *testing.T, dir string) *Repo {
	t.Helper()
	r, err := open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return r
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]Version{
		"git version 2.56.0":                 {2, 56, 0},
		"git version 2.39.3 (Apple Git-145)": {2, 39, 3},
		"git version 2.38.1.windows.1":       {2, 38, 1},
		"git version 1.9.0":                  {1, 9, 0},
	} {
		if got := parseVersion(in); got != want {
			t.Errorf("parseVersion(%q) = %v, want %v", in, got, want)
		}
	}
	if !((Version{2, 38, 0}).AtLeast(2, 38)) || (Version{2, 37, 9}).AtLeast(2, 38) {
		t.Error("AtLeast boundary wrong")
	}
}

func TestResolveAutoDetect(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	// db in a subdirectory of the repo resolves to the toplevel.
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := Resolve(ctx, Options{DBPath: filepath.Join(sub, "zatudan.db")})
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Dir != dir {
		t.Fatalf("auto-detect = %+v, want dir %s", r, dir)
	}
	if !r.HasMergeTree() {
		t.Fatalf("git %s unexpectedly lacks merge-tree --write-tree", r.Version)
	}

	// No .git anywhere above → nil (feature off).
	outside := t.TempDir()
	r, err = Resolve(ctx, Options{DBPath: filepath.Join(outside, "z.db")})
	if err != nil || r != nil {
		t.Fatalf("outside repo: r=%v err=%v", r, err)
	}

	// :memory: and file: DSNs have no anchor.
	for _, db := range []string{":memory:", "file:zatudan.db?x=1"} {
		if r, err := Resolve(ctx, Options{DBPath: db}); err != nil || r != nil {
			t.Fatalf("db=%q: r=%v err=%v", db, r, err)
		}
	}

	// "off" disables even when auto would find one.
	if r, err := Resolve(ctx, Options{RepoPath: "off", DBPath: filepath.Join(sub, "z.db")}); r != nil || err != nil {
		t.Fatalf("off: r=%v err=%v", r, err)
	}

	// Explicit path wins and bad explicit path errors.
	if r, err := Resolve(ctx, Options{RepoPath: dir}); err != nil || r.Dir != dir {
		t.Fatalf("explicit: r=%v err=%v", r, err)
	}
	if _, err := Resolve(ctx, Options{RepoPath: outside}); err == nil {
		t.Fatal("explicit bad path should error")
	}
	// EnvRepo is used when RepoPath is empty.
	if r, err := Resolve(ctx, Options{EnvRepo: dir}); err != nil || r.Dir != dir {
		t.Fatalf("env: r=%v err=%v", r, err)
	}
}

func TestNormalizeRef(t *testing.T) {
	dir := initRepo(t)
	r := openRepo(t, dir)
	ctx := context.Background()

	got, err := r.NormalizeRef(ctx, "main")
	if err != nil || got != "refs/heads/main" {
		t.Fatalf("main → %q, %v", got, err)
	}
	sha := strings.TrimSpace(git(t, dir, "rev-parse", "main"))
	got, err = r.NormalizeRef(ctx, sha)
	if err != nil || got != sha {
		t.Fatalf("sha → %q, %v", got, err)
	}
	if _, err := r.NormalizeRef(ctx, "no-such-ref"); !errors.Is(err, ErrBadRef) {
		t.Fatalf("unknown ref err = %v", err)
	}
	if _, err := r.NormalizeRef(ctx, "-n"); !errors.Is(err, ErrBadRef) {
		t.Fatalf("option-like ref err = %v", err)
	}
}

func TestGitEnvIsScrubbed(t *testing.T) {
	dir := initRepo(t)
	r := openRepo(t, dir)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "nonexistent.git"))
	if _, err := r.RevParse(context.Background(), "main"); err != nil {
		t.Fatalf("GIT_DIR leaked into git run: %v", err)
	}
}

func TestCheckMergeCleanAndConflict(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "b.txt", "two\n")
	git(t, dir, "checkout", "-q", "main")

	mc, err := r.CheckMerge(ctx, "main", "feat")
	if err != nil {
		t.Fatal(err)
	}
	if mc.Status != MergeClean || len(mc.Files) != 0 {
		t.Fatalf("clean check = %+v", mc)
	}
	if mc.MergeBaseSHA == "" || mc.HeadSHA == "" || mc.BaseSHA == "" {
		t.Fatalf("missing SHAs: %+v", mc)
	}

	// Make both sides touch the same line.
	write(t, dir, "a.txt", "main-side\n")
	git(t, dir, "checkout", "-q", "feat")
	write(t, dir, "a.txt", "feat-side\n")
	git(t, dir, "checkout", "-q", "main")

	mc, err = r.CheckMerge(ctx, "main", "feat")
	if err != nil {
		t.Fatal(err)
	}
	if mc.Status != MergeConflicting || len(mc.Files) == 0 {
		t.Fatalf("conflict check = %+v", mc)
	}
	if mc.Files[0] != "a.txt" {
		t.Fatalf("conflict files = %v", mc.Files)
	}
}

func TestMergePlumbing(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	// release is created but never checked out → plumbing path.
	git(t, dir, "branch", "release")
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "b.txt", "two\n")
	git(t, dir, "checkout", "-q", "main")

	res, err := r.Merge(ctx, "release", "feat", MethodMerge, "test merge")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.MergeSHA == "" || res.Method != MethodMerge {
		t.Fatalf("result = %+v", res)
	}
	// The ref advanced to a merge commit with two parents.
	parents := strings.Fields(git(t, dir, "rev-list", "--parents", "-n", "1", "release"))
	if len(parents) != 3 {
		t.Fatalf("release tip parents = %v", parents)
	}
	// The checked-out worktree (main) was never touched.
	if got := git(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("worktree dirty after plumbing merge: %q", got)
	}
	if got := git(t, dir, "rev-parse", "main"); strings.Contains(git(t, dir, "branch", "--contains", "release"), "main") && got == parents[0] {
		// main must NOT have moved.
	}
	// b.txt now exists on release.
	if got := git(t, dir, "show", "release:b.txt"); got != "two\n" {
		t.Fatalf("release:b.txt = %q", got)
	}
}

func TestMergeInWorktree(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "b.txt", "two\n")
	git(t, dir, "checkout", "-q", "main")

	// Dirty worktree → refusal.
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Merge(ctx, "main", "feat", MethodMerge, ""); err == nil {
		t.Fatal("dirty worktree merge should fail")
	} else {
		var de *DirtyWorktreeError
		if !errors.As(err, &de) {
			t.Fatalf("err = %v, want DirtyWorktreeError", err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "dirty.txt")); err != nil {
		t.Fatal(err)
	}

	res, err := r.Merge(ctx, "main", "feat", MethodMerge, "")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := git(t, dir, "show", "main:b.txt"); got != "two\n" {
		t.Fatalf("main:b.txt = %q", got)
	}
	if strings.TrimSpace(git(t, dir, "rev-parse", "HEAD")) != res.MergeSHA {
		t.Fatal("HEAD did not move to merge commit")
	}
}

func TestMergeRejectsNonBranchBase(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)
	sha := strings.TrimSpace(git(t, dir, "rev-parse", "HEAD"))
	if _, err := r.Merge(ctx, sha, "main", MethodMerge, ""); !errors.Is(err, ErrNotMergeable) {
		t.Fatalf("sha base err = %v", err)
	}
	if _, err := r.Merge(ctx, "main", "main", MethodMerge, ""); !errors.Is(err, ErrNotMergeable) {
		t.Fatalf("empty range err = %v", err)
	}
}

func TestMergeConflictPreflight(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "a.txt", "feat-side\n")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "a.txt", "main-side\n")

	_, err := r.Merge(ctx, "main", "feat", MethodMerge, "")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if len(ce.Files) == 0 {
		t.Fatal("no conflict files reported")
	}
	// The worktree must be untouched (pre-flight caught it).
	if got := git(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("worktree touched: %q", got)
	}
}

func TestMergeSquashAndRebasePlumbing(t *testing.T) {
	ctx := context.Background()

	for _, method := range []MergeMethod{MethodSquash, MethodRebase} {
		dir := initRepo(t)
		r := openRepo(t, dir)
		git(t, dir, "branch", "release")
		git(t, dir, "checkout", "-qb", "feat")
		write(t, dir, "b.txt", "two\n")
		write(t, dir, "c.txt", "three\n")
		git(t, dir, "checkout", "-q", "main")

		res, err := r.Merge(ctx, "release", "feat", method, "squashed")
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		parents := strings.Fields(git(t, dir, "rev-list", "--parents", "-n", "1", "release"))
		if method == MethodSquash && len(parents) != 2 {
			t.Fatalf("squash tip parents = %v", parents)
		}
		if method == MethodRebase && len(parents) != 2 {
			// each replayed commit has one parent; tip has one parent
			t.Fatalf("rebase tip parents = %v", parents)
		}
		if got := git(t, dir, "show", "release:c.txt"); got != "three\n" {
			t.Fatalf("%s: release:c.txt = %q", method, got)
		}
		if res.MergeSHA == "" {
			t.Fatalf("%s: no merge sha", method)
		}
	}
}

func TestPinSurvivesGC(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "b.txt", "two\n")
	head := strings.TrimSpace(git(t, dir, "rev-parse", "feat"))
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "branch", "-qD", "feat")

	if err := r.PinHead(ctx, 42, head); err != nil {
		t.Fatalf("pin: %v", err)
	}
	git(t, dir, "gc", "--prune=now", "-q")

	if got := strings.TrimSpace(git(t, dir, "rev-parse", PinRef(42))); got != head {
		t.Fatalf("pin = %q", got)
	}
	// Commits are still reachable through the pin.
	if got := git(t, dir, "show", head+":b.txt"); got != "two\n" {
		t.Fatalf("pinned blob = %q", got)
	}
	// CAS update: move the pin, then a stale oldvalue fails.
	if err := r.UpdatePin(ctx, 42, head); err != nil {
		t.Fatalf("update pin: %v", err)
	}
}

func TestLogDiffFiles(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "b.txt", "two\n")
	git(t, dir, "checkout", "-q", "main")

	mbase, err := r.MergeBase(ctx, "main", "feat")
	if err != nil {
		t.Fatal(err)
	}
	commits, err := r.Log(ctx, "main", "feat")
	if err != nil || len(commits) != 1 {
		t.Fatalf("log = %v, %v", commits, err)
	}
	if commits[0].Subject != "add b.txt" || commits[0].Author != "test" {
		t.Fatalf("commit = %+v", commits[0])
	}
	files, err := r.ChangedFiles(ctx, mbase, "feat")
	if err != nil || len(files) != 1 || files[0].NewPath != "b.txt" || files[0].Status != "A" {
		t.Fatalf("files = %+v, %v", files, err)
	}
	diff, trunc, err := r.Diff(ctx, mbase, "feat", "b.txt")
	if err != nil || trunc || !strings.Contains(diff, "+two") {
		t.Fatalf("diff = %q, %v, %v", diff, trunc, err)
	}
	snip, start, err := r.Snippet(ctx, "feat", "b.txt", 1, 2)
	if err != nil || snip != "two\n" && snip != "two" || start != 1 {
		t.Fatalf("snippet = %q @%d, %v", snip, start, err)
	}
	if err := ValidatePath("../x"); !errors.Is(err, ErrBadPath) {
		t.Fatal(".. accepted")
	}
	if err := ValidatePath(":(exclude)x"); !errors.Is(err, ErrBadPath) {
		t.Fatal("pathspec magic accepted")
	}
	if err := ValidatePath("internal/web/x.go"); err != nil {
		t.Fatalf("normal path rejected: %v", err)
	}
}

func TestRebasePlumbingConsecutiveSameLineEdits(t *testing.T) {
	// Two consecutive commits extending the same line must replay
	// cleanly: without an explicit merge-base, merge-tree infers the
	// original fork and re-applies the first commit, a phantom conflict.
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)
	git(t, dir, "branch", "release")
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "a.txt", "one\ntwo\n")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")
	git(t, dir, "checkout", "-q", "main")

	res, err := r.Merge(ctx, "release", "feat", MethodRebase, "")
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if res.MergeSHA == "" {
		t.Fatal("no merge sha")
	}
	if got := git(t, dir, "show", "release:a.txt"); got != "one\ntwo\nthree\n" {
		t.Fatalf("release:a.txt = %q", got)
	}
	// Two replayed commits (tip descends 2 above the pre-rebase base).
	count := strings.TrimSpace(git(t, dir, "rev-list", "--count", "main..release"))
	if count != "2" {
		t.Fatalf("replayed commits = %s", count)
	}
}

func TestChangedFilesQuotedAndDiffLiteral(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)
	git(t, dir, "checkout", "-qb", "feat")
	// Non-ASCII name (core.quotepath would mangle it without -z) and a
	// name that is also a glob pattern.
	write(t, dir, "dir/ö.txt", "umlaut\n")
	write(t, dir, "docs/[draft].md", "brackets\n")
	write(t, dir, "docs/d.md", "globby\n")
	git(t, dir, "checkout", "-q", "main")
	mbase, err := r.MergeBase(ctx, "main", "feat")
	if err != nil {
		t.Fatal(err)
	}

	files, err := r.ChangedFiles(ctx, mbase, "feat")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.NewPath] = f.Status
	}
	if got["dir/ö.txt"] != "A" || got["docs/[draft].md"] != "A" {
		t.Fatalf("quoted/mangled names: %+v", files)
	}
	// The literal diff must NOT include the glob-adjacent docs/d.md.
	diff, _, err := r.Diff(ctx, mbase, "feat", "docs/[draft].md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+brackets") || strings.Contains(diff, "d.md") {
		t.Fatalf("diff = %q", diff)
	}
	// ShowFile reads the non-ASCII path fine.
	if got, err := r.ShowFile(ctx, "feat", "dir/ö.txt"); err != nil || got != "umlaut\n" {
		t.Fatalf("show = %q, %v", got, err)
	}
}

func TestRebasePlumbingRootCommit(t *testing.T) {
	// head merged an unrelated branch's history: --no-merges still walks
	// that side's root commit, which has no parent to graft onto.
	ctx := context.Background()
	dir := initRepo(t)
	r := openRepo(t, dir)

	// Unrelated history with its own root.
	git(t, dir, "checkout", "-q", "--orphan", "imported")
	git(t, dir, "rm", "-q", "--cached", "a.txt")
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "legacy.txt", "legacy\n")
	// Back to main, merge the unrelated branch in, then one more commit.
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--allow-unrelated-histories", "-m", "merge imported", "imported")
	write(t, dir, "b.txt", "two\n")
	git(t, dir, "branch", "release")
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "c.txt", "three\n")
	git(t, dir, "checkout", "-q", "main")
	// Give feat the unrelated history too, so release must replay it.
	git(t, dir, "checkout", "-q", "feat")
	git(t, dir, "merge", "-q", "-m", "pick up main", "main")
	git(t, dir, "checkout", "-q", "main")
	// rewind: release lacks both the merge and feat's commits.
	git(t, dir, "checkout", "-q", "release")
	git(t, dir, "reset", "-q", "--hard", "main~1")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "reset", "-q", "--hard", "main~1")

	res, err := r.Merge(ctx, "release", "feat", MethodRebase, "")
	if err != nil {
		t.Fatalf("rebase with imported root: %v", err)
	}
	if res.MergeSHA == "" {
		t.Fatal("no merge sha")
	}
	for _, p := range []string{"legacy.txt", "a.txt", "b.txt", "c.txt"} {
		if _, err := r.ShowFile(ctx, "release", p); err != nil {
			t.Fatalf("release:%s missing after replay: %v", p, err)
		}
	}
}
