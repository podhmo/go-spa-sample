// Read-only git queries the pull-requests pages are built from. Open PRs
// always compute against live refs — "git is the truth" for anything
// code-derived — while closed/merged PRs read the SHAs recorded at close
// time (the pinned ref keeps them reachable).
package gitrepo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Commit is one entry of `git log base..head`.
type Commit struct {
	SHA      string
	ShortSHA string
	Subject  string
	Author   string
	Date     time.Time
}

// FileChange is one row of `git diff --name-status`: Status is the letter
// ("A", "M", "D", "R100", ...); OldPath differs from NewPath only on
// renames/copies.
type FileChange struct {
	Status  string
	OldPath string
	NewPath string
}

// RefName is one ref for the create form's select lists: Name is the
// canonical full form (refs/heads/x, refs/remotes/o/y), Short its
// display alias.
type RefName struct {
	Name  string
	Short string
}

// checkRevArg rejects refs that are empty or could be read as an option
// by a git subcommand. Everything else is proven by the git call itself.
func checkRevArg(ref string) error {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return ErrBadRef
	}
	return nil
}

// ValidatePath enforces the review-comment path rule: a plain
// repo-relative path that cannot be read as a flag or pathspec magic
// (docs/sketch/ja/pull-requests.md §9). Callers additionally check the
// path against the changed-files set.
func ValidatePath(p string) error {
	switch {
	case p == "":
		return ErrBadPath
	case strings.HasPrefix(p, "/"), strings.HasPrefix(p, "-"), strings.HasPrefix(p, ":"):
		return ErrBadPath
	case strings.HasPrefix(p, "./"), strings.Contains(p, ".."), strings.Contains(p, "::"):
		return ErrBadPath
	}
	return nil
}

// isHex reports whether s looks like an object id.
func isHex(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// RevParse resolves ref to a commit SHA. ErrBadRef when it does not
// resolve (missing branch, never-pushed head).
func (r *Repo) RevParse(ctx context.Context, ref string) (string, error) {
	if err := checkRevArg(ref); err != nil {
		return "", err
	}
	res, err := r.run(ctx, nil, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", ErrBadRef
	}
	return strings.TrimSpace(res.stdout), nil
}

// NormalizeRef maps any accepted ref spelling — "main", "refs/heads/main",
// "origin/main", "HEAD", a sha — to its canonical stored form: the
// symbolic-full-name for refs, the sha itself for commits. Every
// comparison (duplicate check, checked-out detection, merge gating) runs
// on this form. ErrBadRef when nothing resolves.
func (r *Repo) NormalizeRef(ctx context.Context, ref string) (string, error) {
	if err := checkRevArg(ref); err != nil {
		return "", err
	}
	// --symbolic-full-name resolves and rewrites a ref spelling in one
	// step: it fails for unknown revisions and prints nothing for a raw
	// sha (which stays a sha in the canonical form).
	res, err := r.run(ctx, nil, "rev-parse", "--symbolic-full-name", ref)
	if err != nil {
		return "", ErrBadRef
	}
	name := strings.TrimSpace(res.stdout)
	if name == "" {
		return r.RevParse(ctx, ref)
	}
	return name, nil
}

// Refs lists branch and remote-tracking refs for the create form.
func (r *Repo) Refs(ctx context.Context) ([]RefName, error) {
	res, err := r.run(ctx, nil, "for-each-ref", "--format=%(refname)%09%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var out []RefName
	for line := range strings.Lines(res.stdout) {
		line = strings.TrimRight(line, "\n")
		name, short, _ := strings.Cut(line, "\t")
		if name != "" {
			out = append(out, RefName{Name: name, Short: short})
		}
	}
	return out, nil
}

// CurrentBranch reports the checked-out branch's full name, or the HEAD
// sha when detached — the default for `zatu pr create --head`.
func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	res, err := r.run(ctx, nil, "symbolic-ref", "--quiet", "HEAD")
	if err == nil {
		return strings.TrimSpace(res.stdout), nil
	}
	return r.RevParse(ctx, "HEAD")
}

// MergeBase returns the merge-base commit of base and head — the point a
// three-dot diff starts from. ErrUnrelated when the histories do not meet.
func (r *Repo) MergeBase(ctx context.Context, base, head string) (string, error) {
	if err := checkRevArg(base); err != nil {
		return "", err
	}
	if err := checkRevArg(head); err != nil {
		return "", err
	}
	res, err := r.run(ctx, nil, "merge-base", base, head)
	if err != nil {
		return "", ErrUnrelated
	}
	return strings.TrimSpace(res.stdout), nil
}

// IsAncestor reports whether anc is an ancestor of desc — the cheap
// approximation behind the "outdated" review-comment badge.
func (r *Repo) IsAncestor(ctx context.Context, anc, desc string) (bool, error) {
	if err := checkRevArg(anc); err != nil {
		return false, err
	}
	_, err := r.run(ctx, nil, "merge-base", "--is-ancestor", anc, desc)
	if err != nil {
		// Exit 1 = not an ancestor; exit 128 = a vanished sha — for
		// display purposes both mean "not reachable".
		if e, ok := err.(*Error); ok && (e.ExitCode == 1 || e.ExitCode == 128) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Log lists commits in base..head (newest first).
func (r *Repo) Log(ctx context.Context, base, head string) ([]Commit, error) {
	if err := checkRevArg(base); err != nil {
		return nil, err
	}
	if err := checkRevArg(head); err != nil {
		return nil, err
	}
	res, err := r.run(ctx, nil, "log",
		"--format=%H%x00%h%x00%s%x00%an%x00%aI", base+".."+head)
	if err != nil {
		return nil, err
	}
	var out []Commit
	for line := range strings.Lines(res.stdout) {
		line = strings.TrimRight(line, "\n")
		f := strings.Split(line, "\x00")
		if len(f) != 5 {
			continue
		}
		c := Commit{SHA: f[0], ShortSHA: f[1], Subject: f[2], Author: f[3]}
		c.Date, _ = time.Parse(time.RFC3339, f[4])
		out = append(out, c)
	}
	return out, nil
}

// ChangedFiles lists the files differing between from and to — callers
// pass a merge-base as `from` for the three-dot semantics GitHub's
// "Files changed" uses.
func (r *Repo) ChangedFiles(ctx context.Context, from, to string) ([]FileChange, error) {
	// -z: NUL-separated, unquoted — without it core.quotepath mangles
	// non-ASCII paths into C escapes that name no real file.
	res, err := r.run(ctx, nil, "diff", "--name-status", "-z", from, to)
	if err != nil {
		return nil, err
	}
	var out []FileChange
	fields := strings.Split(res.stdout, "\x00")
	for i := 0; i+1 < len(fields); {
		status := fields[i]
		if status == "" {
			i++
			continue
		}
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+2 >= len(fields) {
				break
			}
			out = append(out, FileChange{Status: status, OldPath: fields[i+1], NewPath: fields[i+2]})
			i += 3
			continue
		}
		out = append(out, FileChange{Status: status, OldPath: fields[i+1], NewPath: fields[i+1]})
		i += 2
	}
	return out, nil
}

// Diff returns the unified diff of path ("" for the whole patch) between
// from and to. The bool reports truncation at the output cap.
func (r *Repo) Diff(ctx context.Context, from, to string, paths ...string) (string, bool, error) {
	args := []string{"diff", "-U3", from, to}
	for _, path := range paths {
		if path == "" {
			continue
		}
		// :(literal): ValidatePath allows glob metacharacters like [] in
		// filenames; without the magic prefix git would glob other files
		// into this file's diff. Renames pass both old and new paths so
		// the file's whole entry (old-side content included) is selected.
		args = append(args, "--", ":(literal)"+path)
	}
	res, err := r.run(ctx, nil, args...)
	if err != nil {
		return "", false, err
	}
	return res.stdout, res.truncated, nil
}

// MergeStatus is the conflict probe's verdict.
type MergeStatus int

const (
	// MergeUnknown means the check could not run — git < 2.38 or a
	// shallow clone — never "clean".
	MergeUnknown MergeStatus = iota
	// MergeClean means merge-tree produced a result tree.
	MergeClean
	// MergeConflicting means merge-tree reported conflicts; Files names them.
	MergeConflicting
)

// MergeCheck is the mergeability probe shown next to an open PR. The SHAs
// are the live resolution used for the check (and reused by callers
// rendering the diff the same request).
type MergeCheck struct {
	Status       MergeStatus
	Files        []string
	BaseSHA      string
	HeadSHA      string
	MergeBaseSHA string
}

// CheckMerge resolves base/head, finds the merge-base, and asks
// merge-tree --write-tree whether a merge would be clean. Old gits and
// shallow clones report MergeUnknown. ErrBadRef when either side is gone;
// ErrUnrelated when no merge base exists.
func (r *Repo) CheckMerge(ctx context.Context, base, head string) (*MergeCheck, error) {
	c := &MergeCheck{Status: MergeUnknown}
	var err error
	if c.BaseSHA, err = r.RevParse(ctx, base); err != nil {
		return nil, err
	}
	if c.HeadSHA, err = r.RevParse(ctx, head); err != nil {
		return nil, err
	}
	if c.MergeBaseSHA, err = r.MergeBase(ctx, base, head); err != nil {
		return nil, err
	}
	if !r.HasMergeTree() || r.Shallow {
		return c, nil
	}
	res, err := r.run(ctx, nil, "merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", base, head)
	if err == nil {
		c.Status = MergeClean
		return c, nil
	}
	e, ok := err.(*Error)
	if !ok || e.ExitCode != 1 {
		return nil, err
	}
	// Exit 1: the first field must be the result tree's OID — anything
	// else is a different failure (bad input) wearing the same code.
	files, ok2 := parseMergeTreeNames(res.stdout)
	if !ok2 {
		return nil, err
	}
	c.Status = MergeConflicting
	c.Files = files
	return c, nil
}

// ShowFile returns the blob content of path at sha (`git show sha:path`).
func (r *Repo) ShowFile(ctx context.Context, sha, path string) (string, error) {
	if err := checkRevArg(sha); err != nil {
		return "", err
	}
	if err := ValidatePath(path); err != nil {
		return "", err
	}
	res, err := r.run(ctx, nil, "show", sha+":"+path)
	if err != nil {
		return "", err
	}
	return res.stdout, nil
}

// Snippet extracts `context` lines around line (1-based) of path's blob at
// sha — the diff_hunk-style record stored on review comments at creation
// time. It returns the snippet text and its first line number.
func (r *Repo) Snippet(ctx context.Context, sha, path string, line, context int) (string, int, error) {
	content, err := r.ShowFile(ctx, sha, path)
	if err != nil {
		return "", 0, err
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if line < 1 || line > len(lines) {
		return "", 0, fmt.Errorf("gitrepo: %s has no line %d", path, line)
	}
	start := max(line-context, 1)
	end := min(line+context, len(lines))
	return strings.Join(lines[start-1:end], "\n"), start, nil
}

// PinRef names the ref that pins a PR's head commit against gc — the same
// trick as GitHub's refs/pull/N/head, but under an app-owned namespace
// (refs/zatudan/) that remote fetches can never collide with.
func PinRef(id int64) string {
	return "refs/zatudan/pr/" + strconv.FormatInt(id, 10) + "/head"
}

// PinHead points the PR's pin ref at sha unconditionally (used at create
// time, when nothing should exist yet).
func (r *Repo) PinHead(ctx context.Context, id int64, sha string) error {
	if err := checkRevArg(sha); err != nil {
		return err
	}
	_, err := r.run(ctx, nil, "update-ref", PinRef(id), sha)
	return err
}

// UpdatePin moves the pin ref compare-and-swap style when it already
// exists (close/merge time), so a concurrent pin update cannot silently
// lose. A missing pin is created.
func (r *Repo) UpdatePin(ctx context.Context, id int64, sha string) error {
	if err := checkRevArg(sha); err != nil {
		return err
	}
	old, err := r.RevParse(ctx, PinRef(id))
	if err != nil {
		// No pin yet (created before this feature or a failed create):
		// plain create.
		return r.PinHead(ctx, id, sha)
	}
	_, err = r.run(ctx, nil, "update-ref", PinRef(id), sha, old)
	if err != nil {
		return ErrConcurrentUpdate
	}
	return nil
}

// Unpin drops the pin ref; used if a pin must follow a comment/PR delete.
func (r *Repo) Unpin(ctx context.Context, id int64) error {
	// update-ref -d exits 0 on a missing ref, so there is no existence
	// case to swallow — real failures (locks, permissions) must surface.
	_, err := r.run(ctx, nil, "update-ref", "-d", PinRef(id))
	return err
}

// Worktrees maps each checked-out branch's full ref name to its worktree
// dir — the merge path uses it to decide between the in-worktree and
// plumbing routes (a detached worktree contributes no entry).
func (r *Repo) Worktrees(ctx context.Context) (map[string]string, error) {
	res, err := r.run(ctx, nil, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	var dir string
	for line := range strings.Lines(res.stdout) {
		line = strings.TrimRight(line, "\n")
		if w, ok := strings.CutPrefix(line, "worktree "); ok {
			dir = w
		} else if b, ok := strings.CutPrefix(line, "branch "); ok && dir != "" {
			out[b] = dir
		}
	}
	return out, nil
}

// StatusPorcelain returns the worktree's `status --porcelain` output;
// empty means clean enough for an in-worktree merge.
func (r *Repo) StatusPorcelain(ctx context.Context, dir string) (string, error) {
	res, err := runGit(ctx, dir, nil, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	return res.stdout, nil
}
