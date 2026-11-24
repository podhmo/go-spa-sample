// Merge execution — the only repository-writing operation the app
// performs. Two routes, decided by where the base branch is checked out
// (docs/sketch/ja/pull-requests.md §5):
//
//   - checked out in some worktree → run porcelain there (git merge
//     --no-ff / --squash + commit / cherry-pick --no-merges), which
//     updates files, index and hooks — refused on a dirty worktree;
//   - not checked out anywhere → plumbing only (merge-tree --write-tree,
//     commit-tree, update-ref CAS): files and hooks untouched. update-ref
//     would happily move a checked-out branch leaving a phantom diff,
//     so this route is never taken for checked-out bases.
package gitrepo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MergeMethod selects the merge mechanics; the values are part of the
// CLI/API surface ("merge"|"squash"|"rebase") and stored on the PR row.
type MergeMethod string

const (
	MethodMerge  MergeMethod = "merge"
	MethodSquash MergeMethod = "squash"
	MethodRebase MergeMethod = "rebase"
)

// Valid reports whether m is a supported method.
func (m MergeMethod) Valid() bool {
	switch m {
	case MethodMerge, MethodSquash, MethodRebase:
		return true
	}
	return false
}

// MergeResult records what a successful merge produced — everything the
// PR row needs to reproduce the diff after branches move or vanish.
type MergeResult struct {
	Method       MergeMethod
	MergeSHA     string // result commit; for rebase, the replayed tip
	HeadSHA      string // head tip at merge time
	MergeBaseSHA string // merge-base at merge time
}

// Merge merges head into base. base must resolve to refs/heads/* —
// merge advances that ref, so remote-tracking refs and detached commits
// are rejected — and `base..head` must be non-empty. msg is the commit
// message; empty falls back to git's default (merge) or the PR-derived
// text callers pass for squash.
//
// Everything that can fail is checked before any write: merge-tree
// probes the conflict first, and the plumbing path builds commits before
// moving the ref. In-worktree failures run the matching --abort/reset to
// leave the worktree as found.
func (r *Repo) Merge(ctx context.Context, base, head string, method MergeMethod, msg string) (*MergeResult, error) {
	if !method.Valid() {
		return nil, fmt.Errorf("gitrepo: unknown merge method %q", method)
	}
	if err := checkRevArg(base); err != nil {
		return nil, err
	}
	if err := checkRevArg(head); err != nil {
		return nil, err
	}

	baseRef, err := r.NormalizeRef(ctx, base)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(baseRef, "refs/heads/") {
		return nil, fmt.Errorf("%w: merge target %q is not a local branch", ErrNotMergeable, base)
	}
	res := &MergeResult{Method: method}
	if res.MergeBaseSHA, err = r.MergeBase(ctx, base, head); err != nil {
		return nil, err
	}
	baseSHA, err := r.RevParse(ctx, baseRef)
	if err != nil {
		return nil, err
	}
	if res.HeadSHA, err = r.RevParse(ctx, head); err != nil {
		return nil, err
	}
	empty, err := r.revListCount(ctx, res.MergeBaseSHA, res.HeadSHA)
	if err != nil {
		return nil, err
	}
	if empty == 0 {
		return nil, fmt.Errorf("%w: %s is already merged into %s", ErrNotMergeable, head, base)
	}

	// Pre-flight the conflict before touching anything (git >= 2.38; on
	// older gits the porcelain commands surface the conflict instead).
	if r.HasMergeTree() && !r.Shallow {
		if err := r.checkConflict(ctx, baseSHA, res.HeadSHA); err != nil {
			return nil, err
		}
	}

	worktrees, err := r.Worktrees(ctx)
	if err != nil {
		return nil, err
	}
	if dir, checkedOut := worktrees[baseRef]; checkedOut {
		mergeSHA, err := r.mergeInWorktree(ctx, dir, baseRef, res, msg)
		if err != nil {
			return nil, err
		}
		res.MergeSHA = mergeSHA
		return res, nil
	}
	mergeSHA, err := r.mergePlumbing(ctx, baseRef, baseSHA, res, msg)
	if err != nil {
		return nil, err
	}
	res.MergeSHA = mergeSHA
	return res, nil
}

// checkConflict asks merge-tree whether base+head would merge cleanly and
// reports a *ConflictError when not.
func (r *Repo) checkConflict(ctx context.Context, baseSHA, headSHA string) error {
	res, err := r.run(ctx, nil, "merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", baseSHA, headSHA)
	if err == nil {
		return nil
	}
	e, ok := err.(*Error)
	if !ok || e.ExitCode != 1 {
		return err
	}
	files, ok2 := parseMergeTreeNames(res.stdout)
	if !ok2 {
		return err
	}
	return &ConflictError{Files: files}
}

// parseMergeTreeNames reads the NUL-separated output of merge-tree
// --name-only -z: first field is the result tree's OID, the rest are
// conflicted paths (unquoted thanks to -z). ok=false means the output
// was not in that shape — a different failure wearing exit 1.
func parseMergeTreeNames(stdout string) (files []string, ok bool) {
	fields := strings.Split(stdout, "\x00")
	if len(fields) == 0 || !isHex(fields[0]) {
		return nil, false
	}
	for _, f := range fields[1:] {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, true
}

// revListCount counts commits in from..to.
func (r *Repo) revListCount(ctx context.Context, from, to string) (int, error) {
	res, err := r.run(ctx, nil, "rev-list", "--count", from+".."+to)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(res.stdout))
	return n, nil
}

// mergeInWorktree merges inside the worktree where base is checked out —
// the user's visible copy — refusing when it carries uncommitted changes.
func (r *Repo) mergeInWorktree(ctx context.Context, dir, baseRef string, res *MergeResult, msg string) (string, error) {
	status, err := r.StatusPorcelain(ctx, dir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) != "" {
		return "", &DirtyWorktreeError{Dir: dir, Status: status}
	}
	// The Worktrees map was a snapshot: another process may have switched
	// the worktree's branch since — merging then would advance the wrong
	// branch while the requested base stays put.
	cur, err := runGit(ctx, dir, nil, "symbolic-ref", "-q", "HEAD")
	if err != nil || strings.TrimSpace(cur.stdout) != baseRef {
		return "", fmt.Errorf("%w: %s no longer checks out %s", ErrConcurrentUpdate, dir, baseRef)
	}
	env := r.identityEnv(ctx)
	// Cleanup must outlive a caller cancellation: an aborted merge must
	// still unwind the worktree, and a canceled ctx would kill the abort.
	cleanupCtx := context.WithoutCancel(ctx)

	var mergeSHA string
	switch res.Method {
	case MethodMerge:
		args := []string{"merge", "--no-ff"}
		if msg != "" {
			args = append(args, "-m", msg)
		} else {
			args = append(args, "--no-edit")
		}
		args = append(args, res.HeadSHA)
		if _, err := runGit(ctx, dir, env, args...); err != nil {
			runGit(cleanupCtx, dir, nil, "merge", "--abort") //nolint:errcheck
			return "", err
		}
	case MethodSquash:
		if _, err := runGit(ctx, dir, env, "merge", "--squash", res.HeadSHA); err != nil {
			runGit(cleanupCtx, dir, nil, "reset", "--hard", "HEAD") //nolint:errcheck
			return "", err
		}
		// --squash leaves a prepared SQUASH_MSG; --no-edit takes it as the
		// commit message when the caller did not supply one.
		commitArgs := []string{"commit"}
		if msg != "" {
			commitArgs = append(commitArgs, "-m", msg)
		} else {
			commitArgs = append(commitArgs, "--no-edit")
		}
		if _, err := runGit(ctx, dir, env, commitArgs...); err != nil {
			runGit(cleanupCtx, dir, nil, "reset", "--hard", "HEAD") //nolint:errcheck
			return "", err
		}
	case MethodRebase:
		// --no-merges keeps the replay linear (GitHub rebase-merge
		// semantics): merge commits cannot be cherry-picked without -m.
		if _, err := runGit(ctx, dir, env, "cherry-pick", "--no-merges", res.MergeBaseSHA+".."+res.HeadSHA); err != nil {
			runGit(cleanupCtx, dir, nil, "cherry-pick", "--abort") //nolint:errcheck
			return "", err
		}
	}
	out, err := runGit(ctx, dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	mergeSHA = strings.TrimSpace(out.stdout)
	return mergeSHA, nil
}

// mergePlumbing builds the merge result purely with plumbing — no
// worktree, index, or hooks — then moves refs/heads/base with a
// compare-and-swap update-ref.
func (r *Repo) mergePlumbing(ctx context.Context, baseRef, baseSHA string, res *MergeResult, msg string) (string, error) {
	if !r.HasMergeTree() {
		return "", ErrUnsupported
	}
	env := r.identityEnv(ctx)

	var mergeSHA string
	switch res.Method {
	case MethodMerge, MethodSquash:
		tree, err := r.mergeTree(ctx, baseSHA, res.HeadSHA)
		if err != nil {
			return "", err
		}
		parents := []string{"-p", baseSHA, "-p", res.HeadSHA}
		if res.Method == MethodSquash {
			parents = parents[:2]
			if msg == "" {
				msg = "Squashed commit"
			}
		}
		mergeSHA, err = r.commitTree(ctx, env, tree, parents, msg)
		if err != nil {
			return "", err
		}
	case MethodRebase:
		tip, err := r.rebasePlumbing(ctx, env, res.MergeBaseSHA, baseSHA, res.HeadSHA)
		if err != nil {
			return "", err
		}
		mergeSHA = tip
	}

	// CAS: the base ref must still sit where we resolved it — a
	// concurrent merge or push must not be silently overwritten.
	if _, err := r.run(ctx, nil, "update-ref", baseRef, mergeSHA, baseSHA); err != nil {
		return "", ErrConcurrentUpdate
	}
	return mergeSHA, nil
}

// mergeTree runs merge-tree --write-tree and returns the result tree OID,
// translating exit 1 into *ConflictError.
func (r *Repo) mergeTree(ctx context.Context, base, head string) (string, error) {
	args := []string{"merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", base, head}
	res, err := r.run(ctx, nil, args...)
	if err == nil {
		return strings.TrimRight(res.stdout, "\x00"), nil
	}
	e, ok := err.(*Error)
	if !ok || e.ExitCode != 1 {
		return "", err
	}
	files, ok2 := parseMergeTreeNames(res.stdout)
	if !ok2 {
		return "", err
	}
	return "", &ConflictError{Files: files}
}

// commitTree creates a commit object; env carries the identity (and for
// replays the original author).
func (r *Repo) commitTree(ctx context.Context, env []string, tree string, parents []string, msg string) (string, error) {
	args := append([]string{"commit-tree", tree}, parents...)
	args = append(args, "-m", msg)
	res, err := r.run(ctx, env, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.stdout), nil
}

// rebasePlumbing replays the --no-merges commits of base..head onto
// newBase one by one (merge-tree + commit-tree), preserving each original
// author via GIT_AUTHOR_* — commit-tree does not inherit it the way
// cherry-pick does. The first conflict aborts with the offending commit
// named; nothing has been written to any ref by then.
func (r *Repo) rebasePlumbing(ctx context.Context, env []string, mergeBase, newBase, head string) (string, error) {
	// --parents pairs each commit with its original parent: replaying c
	// must merge against c^ as the base — the rewritten tip shares no
	// ancestry with c, so merge-tree's inferred fork point would find
	// the original merge-base and re-apply already-replayed changes,
	// reporting phantom conflicts on consecutive same-line edits.
	res, err := r.run(ctx, nil, "rev-list", "--reverse", "--parents", "--no-merges", mergeBase+".."+head)
	if err != nil {
		return "", err
	}
	// emptyTree + a lazily-created root commit wrapping it: replaying a
	// root commit (e.g. unrelated history merged into head) needs both
	// merge sides grafted onto a shared empty root, because merge-tree
	// refuses unrelated histories outright.
	const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	var emptyRoot string
	treeOf := func(rev string) (string, error) {
		res, err := r.run(ctx, nil, "rev-parse", rev+"^{tree}")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(res.stdout), nil
	}
	tip := newBase
	for line := range strings.Lines(res.stdout) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		c := f[0]
		// merge-tree --merge-base=parent would name the replay base
		// directly, but it needs git >= 2.40 while --write-tree works on
		// 2.38. Grafting achieves the same on every supported version:
		// commit the current tip's tree with c's original parent as its
		// parent, and merge-tree infers exactly that parent as the fork
		// point. The graft commits stay dangling — nothing references them.
		tipTree, err := treeOf(tip)
		if err != nil {
			return "", err
		}
		var ours, theirs string
		if len(f) > 1 {
			// Normal case: graft the tip onto c's original parent.
			if ours, err = r.commitTree(ctx, env, tipTree, []string{"-p", f[1]}, "rebase graft"); err != nil {
				return "", err
			}
			theirs = c
		} else {
			// Root commit: graft both sides onto a synthetic empty root
			// so merge-tree's fork point is the empty tree — the replay
			// then applies c's whole tree, matching cherry-pick.
			if emptyRoot == "" {
				if emptyRoot, err = r.commitTree(ctx, env, emptyTree, nil, "rebase empty root"); err != nil {
					return "", err
				}
			}
			if ours, err = r.commitTree(ctx, env, tipTree, []string{"-p", emptyRoot}, "rebase graft"); err != nil {
				return "", err
			}
			cTree, err := treeOf(c)
			if err != nil {
				return "", err
			}
			if theirs, err = r.commitTree(ctx, env, cTree, []string{"-p", emptyRoot}, "rebase graft"); err != nil {
				return "", err
			}
		}
		tree, err := r.mergeTree(ctx, ours, theirs)
		if err != nil {
			if ce, ok := err.(*ConflictError); ok {
				return "", fmt.Errorf("gitrepo: replaying %s: %w", c, ce)
			}
			return "", err
		}
		meta, err := r.run(ctx, nil, "show", "-s", "--format=%an%x00%ae%x00%aI%x00%B", c)
		if err != nil {
			return "", err
		}
		mf := strings.SplitN(meta.stdout, "\x00", 4)
		if len(mf) != 4 {
			return "", fmt.Errorf("gitrepo: unreadable commit %s", c)
		}
		authorEnv := append([]string{}, env...)
		authorEnv = append(authorEnv,
			"GIT_AUTHOR_NAME="+strings.TrimSpace(mf[0]),
			"GIT_AUTHOR_EMAIL="+strings.TrimSpace(mf[1]),
			"GIT_AUTHOR_DATE="+strings.TrimSpace(mf[2]),
		)
		tip, err = r.commitTree(ctx, authorEnv, tree, []string{"-p", tip}, strings.TrimRight(mf[3], "\n"))
		if err != nil {
			return "", err
		}
	}
	return tip, nil
}
