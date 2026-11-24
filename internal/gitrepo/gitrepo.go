// Package gitrepo adapts a local git repository to the pull-requests
// feature: repository resolution, read-only queries (log, diff,
// merge-base, conflict probing) and merge execution. Everything shells
// out to the git binary — there is no in-process git library — so every
// operation runs as `git -C <toplevel> ...` with the inherited GIT_*
// environment scrubbed (an exported GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE
// would silently redirect -C's discovery). Refs and paths always travel
// as argv elements, never through a shell.
//
// Design: docs/sketch/ja/pull-requests.md.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// cmdTimeout bounds every single git invocation; the plumbing merge path
// issues several commands, each with its own budget.
const cmdTimeout = 15 * time.Second

// maxOutput caps captured stdout per command; maxErrOutput caps stderr
// (kept smaller — it exists for diagnostics, not data).
const (
	maxOutput    = 8 << 20 // 8 MiB
	maxErrOutput = 256 << 10
)

// Sentinel errors surfaced to callers. Handlers map them to user-facing
// messages; the concrete *Error keeps stderr for the logs.
var (
	// ErrBadRef reports a ref argument that is empty, option-like, or does
	// not resolve to a commit in this repository.
	ErrBadRef = errors.New("gitrepo: bad or unknown ref")
	// ErrBadPath reports a repository-relative path that would be read as
	// a flag or pathspec magic by git (leading '-', '/', ':', or '..').
	ErrBadPath = errors.New("gitrepo: bad path")
	// ErrNotMergeable reports a merge that cannot happen by policy: the
	// base is not a local branch, or the range is already empty.
	ErrNotMergeable = errors.New("gitrepo: not mergeable")
	// ErrUnsupported reports an operation the detected git is too old
	// for (merge-tree --write-tree and the plumbing merge path need
	// git >= 2.38).
	ErrUnsupported = errors.New("gitrepo: operation needs git >= 2.38")
	// ErrConcurrentUpdate reports a lost compare-and-swap race on a ref:
	// another process moved it between our resolve and our update-ref.
	ErrConcurrentUpdate = errors.New("gitrepo: ref changed concurrently")
	// ErrUnrelated reports a base/head pair sharing no merge base
	// (unrelated histories) — like GitHub, such PRs cannot merge.
	ErrUnrelated = errors.New("gitrepo: unrelated histories")
)

// DirtyWorktreeError reports an in-worktree merge target whose index or
// files carry uncommitted changes; merging there would entangle them, so
// the caller must commit/stash first.
type DirtyWorktreeError struct {
	Dir    string
	Status string // `git status --porcelain` output, for the message
}

func (e *DirtyWorktreeError) Error() string {
	return fmt.Sprintf("gitrepo: worktree %s has uncommitted changes", e.Dir)
}

// ConflictError reports a merge that merge-tree already proved
// conflicting — detected before any worktree/index is touched.
type ConflictError struct {
	Files []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("gitrepo: merge conflicts in %s", strings.Join(e.Files, ", "))
}

// Error wraps a failed git invocation, keeping argv and a stderr tail
// for diagnosis. ExitCode is the process exit status, or -1 when the
// command never reached exit (spawn failure, timeout kill).
type Error struct {
	Args     []string
	Stderr   string
	ExitCode int
	Err      error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

func (e *Error) Unwrap() error { return e.Err }

// Version is a parsed `git --version` triple.
type Version struct {
	Major, Minor, Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// parseVersion extracts the triple from `git --version` output, tolerating
// platform suffixes ("git version 2.39.3 (Apple Git-145)").
func parseVersion(s string) Version {
	var v Version
	f := strings.Fields(s)
	// "git version X.Y.Z ..." — find the first token that starts with a digit.
	for _, tok := range f {
		if tok == "" || tok[0] < '0' || tok[0] > '9' {
			continue
		}
		parts := strings.SplitN(tok, ".", 3)
		if len(parts) == 3 {
			v.Major, _ = strconv.Atoi(parts[0])
			v.Minor, _ = strconv.Atoi(parts[1])
			// Patch may carry a suffix like "3 (Apple" is impossible —
			// Fields split already — but things like "2.56.0.windows.1"
			// leave a non-numeric tail; strconv fails → 0, acceptable.
			patch := parts[2]
			if i := strings.IndexAny(patch, ". -("); i >= 0 {
				patch = patch[:i]
			}
			v.Patch, _ = strconv.Atoi(patch)
			return v
		}
	}
	return v
}

// AtLeast reports whether the version is >= major.minor.
func (v Version) AtLeast(major, minor int) bool {
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// Repo is a resolved local repository: Dir is the toplevel of a (possibly
// linked) worktree, the only path the package runs git in.
type Repo struct {
	Dir     string
	Version Version
	// Shallow marks a clone with .git/shallow — merge-base lookups can
	// fail and the UI degrades to a "shallow repository" note.
	Shallow bool
}

// HasMergeTree reports whether this git supports `merge-tree --write-tree`
// (git >= 2.38), gating both the conflict badge and the plumbing merge path.
func (r *Repo) HasMergeTree() bool { return r.Version.AtLeast(2, 38) }

// cappedBuffer is an io.Writer that stores at most limit bytes, recording
// truncation instead of erroring so a huge diff can't exhaust memory.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit > 0 && b.buf.Len()+len(p) > b.limit {
		if room := b.limit - b.buf.Len(); room > 0 {
			b.buf.Write(p[:room])
		}
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

// scrubbedEnv returns the process environment minus every GIT_* variable
// (their values would override -C's repository discovery), plus the
// caller's extra assignments.
func scrubbedEnv(extra []string) []string {
	src := os.Environ()
	env := make([]string, 0, len(src)+len(extra))
	for _, kv := range src {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

// gitResult captures one invocation: exit status and whether stdout was
// truncated at maxOutput (stderr truncates silently — it is diagnostic).
type gitResult struct {
	stdout    string
	stderr    string
	exitCode  int
	truncated bool
}

// runGit executes `git -C dir <args...>` with a scrubbed environment and
// the per-command timeout. A nonzero exit is returned as *Error — callers
// that expect failures (merge-tree conflicts) inspect ExitCode.
func runGit(ctx context.Context, dir string, extraEnv []string, args ...string) (gitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()

	argv := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	// CommandContext kills git itself; a child that inherited stdout/stderr
	// (a hook) would otherwise keep cmd.Wait blocked on the pipes forever.
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = scrubbedEnv(extraEnv)
	var out, errb cappedBuffer
	out.limit, errb.limit = maxOutput, maxErrOutput
	cmd.Stdout, cmd.Stderr = &out, &errb

	res := gitResult{exitCode: -1}
	err := cmd.Run()
	res.stdout, res.stderr = out.buf.String(), errb.buf.String()
	res.truncated = out.truncated
	if err == nil {
		res.exitCode = 0
		return res, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.exitCode = ee.ExitCode()
	}
	return res, &Error{Args: argv, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
}

// run is the per-repo convenience form of runGit.
func (r *Repo) run(ctx context.Context, extraEnv []string, args ...string) (gitResult, error) {
	return runGit(ctx, r.Dir, extraEnv, args...)
}

// identityEnv resolves the committer/author identity for commits this app
// creates: repo config (user.name/user.email) first, then any caller-set
// GIT_*_NAME/EMAIL already in the environment, then the "zatudan"
// fallback — an unconfigured repo would otherwise fail every commit-tree
// with "unable to auto-detect email address".
func (r *Repo) identityEnv(ctx context.Context) []string {
	name := strings.TrimSpace(r.configGet(ctx, "user.name"))
	email := strings.TrimSpace(r.configGet(ctx, "user.email"))
	if name == "" {
		name = os.Getenv("GIT_COMMITTER_NAME")
	}
	if email == "" {
		email = os.Getenv("GIT_COMMITTER_EMAIL")
	}
	if name == "" {
		name = "zatudan"
	}
	if email == "" {
		email = "zatudan@localhost"
	}
	return []string{
		"GIT_AUTHOR_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name,
		"GIT_COMMITTER_EMAIL=" + email,
	}
}

// configGet reads one key from the repository's config; a miss is "".
func (r *Repo) configGet(ctx context.Context, key string) string {
	res, err := r.run(ctx, nil, "config", "--get", key)
	if err != nil {
		return ""
	}
	return res.stdout
}
