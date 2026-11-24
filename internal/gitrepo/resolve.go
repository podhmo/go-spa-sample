// Repository resolution: the pull-requests feature exists only when a git
// repository can be found, so Resolve returns (*Repo, nil) on success and
// (nil, nil) when the feature should silently disappear (auto-detect found
// nothing, or it was switched off). A failure of an EXPLICIT -repo value
// is the only error path — asking for a repo and getting none is a
// startup error, the same philosophy as an empty -db.
package gitrepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options carries the resolution inputs; the flag value wins over the
// environment, matching every other ZATUDAN_* setting.
type Options struct {
	// RepoPath is the -repo flag value. "" means auto-detect from DBPath,
	// "off" disables the feature outright.
	RepoPath string
	// EnvRepo is ZATUDAN_REPO, used when RepoPath is "".
	EnvRepo string
	// DBPath is the -db value: auto-detection searches for a .git entry
	// (dir or file — linked worktrees and submodules) walking UP from the
	// database file's directory. A "file:" DSN or ":memory:" has no
	// anchor, so auto-detect is skipped.
	DBPath string
}

// Resolve locates the repository per Options. The result is resolved once
// at startup: a repository appearing later is invisible until restart,
// the same policy the -api-token evaluation follows.
func Resolve(ctx context.Context, opt Options) (*Repo, error) {
	path := opt.RepoPath
	if path == "" {
		path = opt.EnvRepo
	}
	switch {
	case path == "off":
		return nil, nil
	case path != "":
		return openExplicit(ctx, path)
	default:
		return autoDetect(ctx, opt.DBPath)
	}
}

// autoDetect walks upward from the database file's directory looking for
// a .git entry. Everything is best-effort: any failure — no .git, a bare
// repo, a missing git binary — yields (nil, nil) so the feature just
// isn't there.
func autoDetect(ctx context.Context, dbPath string) (*Repo, error) {
	if dbPath == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return nil, nil
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, nil
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			// open is allowed to fail silently in auto mode — the repo
			// might be bare or git might be missing entirely.
			repo, err := open(ctx, dir)
			if err != nil {
				return nil, nil
			}
			return repo, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// openExplicit resolves a user-given path; failures are real errors.
func openExplicit(ctx context.Context, path string) (*Repo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: resolve %q: %w", path, err)
	}
	repo, err := open(ctx, abs)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: -repo %q: %w", path, err)
	}
	return repo, nil
}

// open validates that dir sits inside a git worktree, then probes the
// git version and shallowness once — both gate later behavior, so they
// are decided at resolve time and never re-probed.
func open(ctx context.Context, dir string) (*Repo, error) {
	res, err := runGit(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git worktree: %w", err)
	}
	top := strings.TrimSpace(res.stdout)
	if top == "" {
		return nil, fmt.Errorf("not a git worktree")
	}
	r := &Repo{Dir: top}

	ver, err := runGit(ctx, top, nil, "--version")
	if err != nil {
		return nil, fmt.Errorf("git --version: %w", err)
	}
	r.Version = parseVersion(ver.stdout)
	if r.Version.Major == 0 {
		return nil, fmt.Errorf("unparsable git version %q", strings.TrimSpace(ver.stdout))
	}

	shallow, err := runGit(ctx, top, nil, "rev-parse", "--is-shallow-repository")
	if err == nil && strings.TrimSpace(shallow.stdout) == "true" {
		r.Shallow = true
	}
	return r, nil
}
