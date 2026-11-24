// `zatu pr` — pull request verbs (docs/sketch/ja/pull-requests.md §7).
// PRs are workspace-independent: the workspace flag is ignored and PR
// paths never carry a slug. Local mode resolves the git repo itself
// (--repo / ZATUDAN_REPO / auto-detect from --db); remote mode drives
// /api/v1/pulls* — a --repo flag there is meaningless and warned about.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/podhmo/go-spa-sample/internal/api"

	"github.com/podhmo/go-spa-sample/internal/client"
	"github.com/podhmo/go-spa-sample/internal/export"
	"github.com/podhmo/go-spa-sample/internal/gitrepo"
	"github.com/podhmo/go-spa-sample/internal/store"
)

const prUsage = `zatu pr — pull requests (git-backed)

  list     --state open|closed|merged|all [--search Q] [--sort newest|oldest]
           [--limit N] [--page N] [--json F,...] [--template T]
  view     N [--markdown] [--json F,...] [--template T] [-w]
  diff     N                       unified diff (merge-base..head)
  create   --base B [--head H] --title T [--body B|--body-file F]
  edit     N [--title T] [--body B|--body-file F]
  close    N / reopen N / delete N
  merge    N --method merge|squash|rebase [-m MSG]
  comment create|edit|delete|hide|unhide N ...

Local mode needs a git repository: --repo PATH, ZATUDAN_REPO, or
auto-detect upward from --db. Remote mode uses /api/v1/pulls* and
ignores --repo.
`

func prCmd(ctx context.Context, o Options, args []string) error {
	if len(args) == 0 {
		return usageError{msg: "usage: zatu pr list|view|diff|create|edit|close|reopen|merge|delete|comment"}
	}
	if isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, prUsage)
		return nil
	}
	switch args[0] {
	case "list":
		return prList(ctx, o, args[1:])
	case "view":
		return prView(ctx, o, args[1:])
	case "diff":
		return prDiff(ctx, o, args[1:])
	case "create":
		return prCreate(ctx, o, args[1:])
	case "edit":
		return prEdit(ctx, o, args[1:])
	case "close", "reopen", "delete":
		return prStateChange(ctx, o, args[0], args[1:])
	case "merge":
		return prMerge(ctx, o, args[1:])
	case "comment":
		return prCommentCmd(ctx, o, args[1:])
	default:
		return usageError{msg: fmt.Sprintf("unknown pr command %q", args[0])}
	}
}

// prFlags adds --repo on top of the shared backend flags.
func (o Options) addPRFlags(fs *flag.FlagSet) (*backendFlags, *string) {
	bf := o.addBackendFlags(fs)
	repo := fs.String("repo", "", "git repository path for local mode (env ZATUDAN_REPO); remote mode ignores it")
	return bf, repo
}

// prEnv opens the backend and, in local mode, resolves the git repo.
// Remote mode needs no repo and ignores --repo with a warning.
func (o Options) prEnv(ctx context.Context, bf *backendFlags, repoFlag string) (Backend, io.Closer, *gitrepo.Repo, *client.Client, error) {
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if bf.server != "" {
		if repoFlag != "" {
			fmt.Fprintln(o.Stderr, "warning: --repo is ignored in remote mode")
		}
		rc, ok := b.(*client.Client)
		if !ok {
			closeQuietly(closer)
			return nil, nil, nil, nil, errors.New("cli: remote pr commands need the HTTP client backend")
		}
		return b, closer, nil, rc, nil
	}
	repo, err := o.ResolveRepo(ctx, bf.dbPath(), repoFlag)
	if err != nil {
		closeQuietly(closer)
		return nil, nil, nil, nil, err
	}
	if repo == nil {
		closeQuietly(closer)
		return nil, nil, nil, nil, errors.New("cli: not a git repository — pass --repo or run where a repo resolves from --db")
	}
	return b, closer, repo, nil, nil
}

// prViewBundle is the assembled detail for view/markdown — the store row
// plus live git data (local) or the API bundle (remote).
type prViewBundle struct {
	PR        *store.PullRequest
	Comments  []*store.PRComment
	Commits   []export.PRCommit
	Mergeable string // "clean" | "conflicting" | "unknown" | "" (closed/unknown)
	Conflicts []string
}

func (o Options) prViewData(ctx context.Context, b Backend, repo *gitrepo.Repo, rc *client.Client, id int64) (*prViewBundle, error) {
	out := &prViewBundle{}
	if rc != nil {
		v, err := rc.PullView(ctx, id)
		if err != nil {
			return nil, err
		}
		out.PR, out.Comments = v.PR, v.Comments
		for _, c := range v.Commits {
			d, _ := time.Parse(time.RFC3339, c.Date)
			out.Commits = append(out.Commits, export.PRCommit{SHA: c.SHA, Subject: c.Subject, Author: c.Author, Date: d})
		}
		if v.Mergeable != nil {
			out.Mergeable = v.Mergeable.Status
			out.Conflicts = v.Mergeable.Files
		}
		return out, nil
	}
	pr, err := b.GetPullRequest(ctx, id)
	if err != nil {
		return nil, err
	}
	out.PR = pr
	if out.Comments, err = b.ListPRComments(ctx, id); err != nil {
		return nil, err
	}
	from, to := pr.BaseRef, pr.HeadRef
	if !pr.Open() {
		from, to = pr.MergeBaseSHA, pr.HeadSHA
	}
	if from != "" && to != "" {
		if cs, err := repo.Log(ctx, from, to); err == nil {
			for _, c := range cs {
				out.Commits = append(out.Commits, export.PRCommit{SHA: c.SHA, Subject: c.Subject, Author: c.Author, Date: c.Date})
			}
		}
	}
	if pr.Open() {
		out.Mergeable = "unknown"
		if check, err := repo.CheckMerge(ctx, pr.BaseRef, pr.HeadRef); err == nil && check != nil {
			switch check.Status {
			case gitrepo.MergeClean:
				out.Mergeable = "clean"
			case gitrepo.MergeConflicting:
				out.Mergeable = "conflicting"
				out.Conflicts = check.Files
			}
		}
	}
	return out, nil
}

// zatu pr list [--state open|closed|merged|all] [--search Q] [--sort] [--limit N] [--page N] [--json|--template]
func prList(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr list", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	state := fs.String("state", "open", "filter by state: open|closed|merged|all")
	query := fs.String("search", "", "search title and body")
	sort := fs.String("sort", "newest", "sort order: newest|oldest")
	limit := fs.Int("limit", 30, "maximum pull requests to list")
	page := fs.Int("page", 1, "page number")
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	if _, err := o.parseLeaf(fs, "", args, 0); err != nil {
		return err
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}
	switch *state {
	case "open", "closed", "merged", "all":
	default:
		return usageError{msg: fmt.Sprintf("--state must be open|closed|merged|all, got %q", *state)}
	}
	if *sort != "newest" && *sort != "oldest" {
		return usageError{msg: fmt.Sprintf("--sort must be newest|oldest, got %q", *sort)}
	}
	b, closer, _, _, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	pulls, _, err := b.ListPullRequests(ctx, store.PRListFilter{
		Status: *state, Query: strings.TrimSpace(*query), Sort: *sort,
		Page: *page, PerPage: *limit,
	})
	if err != nil {
		return err
	}
	if *jsonFields != "" {
		return writeJSONFields(o.Stdout, prViews(pulls), *jsonFields)
	}
	if *tmpl != "" {
		return writeTemplate(o.Stdout, prViews(pulls), *tmpl)
	}
	for _, p := range pulls {
		fmt.Fprintf(o.Stdout, "%d\t%s\t%s\t%s\n", p.ID, o.statusText(p.Status), p.Title, fmtTimeShort(p.UpdatedAt))
	}
	return nil
}

// zatu pr view N [--markdown|--json F,...|--template T] [-w]
func prView(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr view", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	markdown := fs.Bool("markdown", false, "render the thread as markdown")
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	web := fs.Bool("web", false, "open the pull request in a browser (requires --server)")
	fs.BoolVar(web, "w", false, "shorthand for --web")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "N is required"}
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}
	if *markdown && (*jsonFields != "" || *tmpl != "" || *web) {
		return usageError{msg: "--markdown is mutually exclusive with --json, --template and --web"}
	}
	if *web && bf.server == "" {
		return usageError{msg: "-w/--web requires --server (or ZATUDAN_SERVER)"}
	}
	id, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || id < 1 {
		return usageError{msg: fmt.Sprintf("invalid pull request id %q", rest[0])}
	}
	b, closer, repo, rc, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	if *web {
		return openInBrowser(o, fmt.Sprintf("%s/pulls/%d", strings.TrimRight(bf.server, "/"), id))
	}
	if rc != nil && *markdown {
		doc, err := rc.PullMarkdown(ctx, id)
		if err != nil {
			return err
		}
		fmt.Fprint(o.Stdout, doc)
		return nil
	}
	v, err := o.prViewData(ctx, b, repo, rc, id)
	if err != nil {
		return err
	}
	if *markdown {
		fmt.Fprint(o.Stdout, export.PullRequestMarkdown(export.PRMarkdownData{
			PR: v.PR, Commits: v.Commits, Comments: v.Comments}))
		return nil
	}
	out := func(x any) error {
		if *tmpl != "" {
			return writeTemplate(o.Stdout, x, *tmpl)
		}
		return writeJSONFields(o.Stdout, x, *jsonFields)
	}
	if *jsonFields != "" || *tmpl != "" {
		m, err := jsonObject(prViewJSON(v.PR))
		if err != nil {
			return err
		}
		m["comments"] = prCommentViews(v.Comments)
		if len(v.Commits) > 0 {
			cs := make([]api.PRCommit, 0, len(v.Commits))
			for _, c := range v.Commits {
				cs = append(cs, api.PRCommit{SHA: c.SHA, Subject: c.Subject,
					Author: c.Author, Date: c.Date.UTC().Format("2006-01-02T15:04:05Z07:00")})
			}
			m["commits"] = cs
		}
		if v.Mergeable != "" {
			m["mergeable"] = v.Mergeable
		}
		return out(m)
	}
	fmt.Fprintf(o.Stdout, "title:\t%s\n", v.PR.Title)
	fmt.Fprintf(o.Stdout, "status:\t%s\n", o.statusText(v.PR.Status))
	fmt.Fprintf(o.Stdout, "base:\t%s\n", v.PR.BaseRef)
	fmt.Fprintf(o.Stdout, "head:\t%s\n", v.PR.HeadRef)
	fmt.Fprintf(o.Stdout, "author:\t%s\n", authorName(v.PR.Author))
	if v.Mergeable != "" {
		fmt.Fprintf(o.Stdout, "mergeable:\t%s\n", v.Mergeable)
		for _, f := range v.Conflicts {
			fmt.Fprintf(o.Stdout, "  conflict: %s\n", f)
		}
	}
	if v.PR.Status == store.PRStatusMerged {
		fmt.Fprintf(o.Stdout, "merged:\t%s (%s)\n", v.PR.MergeSHA, v.PR.MergeMethod)
	}
	fmt.Fprintf(o.Stdout, "updated:\t%s\n", fmtTimeShort(v.PR.UpdatedAt))
	fmt.Fprintf(o.Stdout, "--\n%s\n", v.PR.Body)
	if len(v.Commits) > 0 {
		fmt.Fprintln(o.Stdout, "\ncommits:")
		for _, c := range v.Commits {
			sha := c.SHA
			if len(sha) > 8 {
				sha = sha[:8]
			}
			fmt.Fprintf(o.Stdout, "  %s %s — %s\n", sha, c.Subject, c.Author)
		}
	}
	if len(v.Comments) > 0 {
		fmt.Fprintln(o.Stdout, "\ncomments:")
		for _, c := range v.Comments {
			anchor := ""
			if c.Anchored() {
				anchor = fmt.Sprintf(" on %s:L%d (%s)", c.Path, c.Line, c.Side)
			}
			fmt.Fprintf(o.Stdout, "  #%d %s (%s)%s%s\n    %s\n",
				c.ID, authorName(c.Author), fmtTimeShort(c.CreatedAt), moderationMarks(c.Moderation), anchor,
				strings.ReplaceAll(c.Body, "\n", "\n    "))
		}
	}
	return nil
}

// zatu pr diff N — unified diff merge-base..head.
func prDiff(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr diff", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "N is required"}
	}
	id, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || id < 1 {
		return usageError{msg: fmt.Sprintf("invalid pull request id %q", rest[0])}
	}
	b, closer, repo, rc, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	if rc != nil {
		diff, truncated, err := rc.PullDiffTruncated(ctx, id)
		if err != nil {
			return err
		}
		if truncated {
			fmt.Fprintln(o.Stderr, "warning: diff truncated by the server's output cap")
		}
		fmt.Fprint(o.Stdout, diff)
		return nil
	}
	pr, err := b.GetPullRequest(ctx, id)
	if err != nil {
		return err
	}
	from, to := pr.BaseRef, pr.HeadRef
	if pr.Open() {
		if mb, err := repo.MergeBase(ctx, pr.BaseRef, pr.HeadRef); err == nil && mb != "" {
			from = mb
		}
	} else {
		from, to = pr.MergeBaseSHA, pr.HeadSHA
		if from == "" {
			from = pr.BaseRef
		}
		if to == "" {
			to = gitrepo.PinRef(pr.ID)
		}
	}
	diff, truncated, err := repo.Diff(ctx, from, to)
	if err != nil {
		return err
	}
	if truncated {
		fmt.Fprintln(o.Stderr, "warning: diff truncated by the output cap")
	}
	fmt.Fprint(o.Stdout, diff)
	return nil
}

// zatu pr create --base B [--head H] --title T [--body B|--body-file F]
// --head defaults to the repo's current branch (local only; remote
// requires it — the server has no checkout of the caller's branch).
func prCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr create", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	base := fs.String("base", "", "base ref (required)")
	head := fs.String("head", "", "head ref (default: current branch in local mode)")
	title := fs.String("title", "", "pull request title (required)")
	body := fs.String("body", "", "pull request body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	if _, err := o.parseLeaf(fs, "", args, 0); err != nil {
		return err
	}
	if strings.TrimSpace(*base) == "" {
		return usageError{msg: "--base is required"}
	}
	if strings.TrimSpace(*title) == "" {
		return usageError{msg: "--title is required"}
	}
	b, closer, repo, rc, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	headRef := strings.TrimSpace(*head)
	if headRef == "" {
		if rc != nil {
			return usageError{msg: "--head is required in remote mode"}
		}
		if headRef, err = repo.CurrentBranch(ctx); err != nil {
			return fmt.Errorf("resolve current branch: %w", err)
		}
	}
	bodyText, err := o.readBody(*body, *bodyFile)
	if err != nil {
		return err
	}
	var id int64
	if rc != nil {
		id, err = rc.CreatePullRequest(ctx, store.PRInput{
			BaseRef: *base, HeadRef: headRef, Title: *title, Body: bodyText,
		}, 0, nil)
	} else {
		in := store.PRInput{Title: *title, Body: bodyText}
		var normErr error
		if in.BaseRef, normErr = repo.NormalizeRef(ctx, *base); normErr != nil {
			return fmt.Errorf("base: %w", normErr)
		}
		// Merge must be able to update the base ref, so it must be a
		// local branch — a tag, sha, or remote-tracking ref is accepted
		// by NormalizeRef but can never merge.
		if !strings.HasPrefix(in.BaseRef, "refs/heads/") {
			return usageError{msg: fmt.Sprintf("--base must be a local branch, got %q", *base)}
		}
		if in.HeadRef, normErr = repo.NormalizeRef(ctx, headRef); normErr != nil {
			return fmt.Errorf("head: %w", normErr)
		}
		if fe := store.ValidatePullRequestInput(in); fe.Any() {
			return usageError{msg: fmt.Sprintf("%v", fe)}
		}
		if in.BaseSHA, err = repo.RevParse(ctx, in.BaseRef); err != nil {
			return err
		}
		if in.HeadSHA, err = repo.RevParse(ctx, in.HeadRef); err != nil {
			return err
		}
		headSHA := in.HeadSHA
		id, err = b.CreatePullRequest(ctx, in, 0, func(prID int64) error {
			return repo.PinHead(ctx, prID, headSHA)
		})
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "pull request #%d created\n", id)
	return nil
}

// zatu pr edit N [--title T] [--body B|--body-file F]
func prEdit(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr edit", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	title := fs.String("title", "", "new title")
	body := fs.String("body", "", "new body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "N is required"}
	}
	id, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || id < 1 {
		return usageError{msg: fmt.Sprintf("invalid pull request id %q", rest[0])}
	}
	b, closer, _, _, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	pr, err := b.GetPullRequest(ctx, id)
	if err != nil {
		return err
	}
	in := store.PRInput{BaseRef: pr.BaseRef, HeadRef: pr.HeadRef,
		Title: pr.Title, Body: pr.Body}
	if *title != "" {
		in.Title = *title
	}
	bodySet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "body" || f.Name == "body-file" {
			bodySet = true
		}
	})
	if bodySet {
		if in.Body, err = o.readBody(*body, *bodyFile); err != nil {
			return err
		}
	}
	// Same validation the API applies — local mode must not write
	// titles/bodies the server would reject on the same data.
	if fe := store.ValidatePullRequestInput(in); fe.Any() {
		return usageError{msg: fmt.Sprintf("%v", fe)}
	}
	if err := b.UpdatePullRequest(ctx, id, in); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "pull request #%d updated\n", id)
	return nil
}

// zatu pr close|reopen|delete N
func prStateChange(ctx context.Context, o Options, action string, args []string) error {
	fs := flag.NewFlagSet("pr "+action, flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "N is required"}
	}
	id, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || id < 1 {
		return usageError{msg: fmt.Sprintf("invalid pull request id %q", rest[0])}
	}
	b, closer, repo, rc, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	switch action {
	case "close":
		if rc != nil {
			err = rc.SetPullStatus(ctx, id, "closed")
		} else {
			pr, gerr := b.GetPullRequest(ctx, id)
			if gerr != nil {
				return gerr
			}
			if !pr.Open() {
				return errors.New("cli: only an open pull request can be closed")
			}
			headSHA, _ := repo.RevParse(ctx, pr.HeadRef) // deleted head closes anyway
			if headSHA == "" {
				// The head branch is already gone — keep the recorded
				// shas instead of overwriting them with empty values,
				// or view/diff lose the PR's history.
				headSHA = pr.HeadSHA
			}
			if headSHA != "" {
				if err := repo.UpdatePin(ctx, id, headSHA); err != nil {
					return err
				}
			}
			mergeBase, _ := repo.MergeBase(ctx, pr.BaseRef, pr.HeadRef)
			if mergeBase == "" {
				mergeBase = pr.MergeBaseSHA
			}
			err = b.ClosePullRequest(ctx, id, headSHA, mergeBase)
		}
	case "reopen":
		err = b.ReopenPullRequest(ctx, id)
	case "delete":
		err = b.DeletePullRequest(ctx, id)
		if err == nil && repo != nil {
			_ = repo.Unpin(ctx, id)
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "pull request #%d %sd\n", id, action)
	return nil
}

// zatu pr merge N --method merge|squash|rebase [-m MSG]
func prMerge(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr merge", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	method := fs.String("method", "", "merge method: merge|squash|rebase (required)")
	msg := fs.String("m", "", "commit message (squash)")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if *method == "" {
		return usageError{msg: "--method is required"}
	}
	if !gitrepo.MergeMethod(*method).Valid() {
		return usageError{msg: fmt.Sprintf("--method must be merge|squash|rebase, got %q", *method)}
	}
	id, err := argNumber(rest, 0, "pull request")
	if err != nil {
		return err
	}
	b, closer, repo, rc, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	if rc != nil {
		pr, err := rc.MergePullRequest(ctx, id, *method, *msg)
		if err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "pull request #%d merged as %s (%s)\n", pr.ID, pr.MergeSHA, pr.MergeMethod)
		return nil
	}
	pr, err := b.GetPullRequest(ctx, id)
	if err != nil {
		return err
	}
	if !pr.Open() {
		return errors.New("cli: only an open pull request can be merged")
	}
	m := gitrepo.MergeMethod(*method)
	message := *msg
	if message == "" && m == gitrepo.MethodSquash {
		message = fmt.Sprintf("%s (#%d)", pr.Title, pr.ID)
	}
	res, err := repo.Merge(ctx, pr.BaseRef, pr.HeadRef, m, message)
	if err != nil {
		return err
	}
	_ = repo.UpdatePin(ctx, id, res.HeadSHA)
	if err := b.MarkMerged(ctx, id, store.PRMergeInfo{
		MergeSHA:     res.MergeSHA,
		MergeMethod:  string(res.Method),
		HeadSHA:      res.HeadSHA,
		MergeBaseSHA: res.MergeBaseSHA,
	}); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "pull request #%d merged as %s (%s)\n", id, res.MergeSHA, res.Method)
	return nil
}

// zatu pr comment create|edit|delete|hide|unhide
func openInBrowser(o Options, url string) error {
	fmt.Fprintln(o.Stdout, "Opening "+url)
	return o.OpenBrowser(url)
}

func prCommentCmd(ctx context.Context, o Options, args []string) error {
	if len(args) == 0 {
		return usageError{msg: "usage: zatu pr comment create|edit|delete|hide|unhide"}
	}
	if isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, `zatu pr comment — comments and review comments on a pull request

  create  N --body B|--body-file F [--path P --line L [--side left]]
  edit    N COMMENT_ID --body B|--body-file F
  delete  N COMMENT_ID
  hide    N COMMENT_ID --reason spam|abuse|off-topic|outdated|duplicate|resolved|low-quality
  unhide  N COMMENT_ID
`)
		return nil
	}
	switch args[0] {
	case "create":
		return prCommentCreate(ctx, o, args[1:])
	case "edit":
		return prCommentEdit(ctx, o, args[1:])
	case "delete":
		return prCommentDelete(ctx, o, args[1:])
	case "hide", "unhide":
		return prCommentHide(ctx, o, args[0], args[1:])
	default:
		return usageError{msg: fmt.Sprintf("unknown pr comment command %q", args[0])}
	}
}

// zatu pr comment create N --body B [--path P --line L [--side left]]
func prCommentCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr comment create", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	body := fs.String("body", "", "comment body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	path := fs.String("path", "", "file path for a review comment")
	line := fs.Int("line", 0, "line number for a review comment")
	side := fs.String("side", "right", "anchor side: right|left")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "N is required"}
	}
	id, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || id < 1 {
		return usageError{msg: fmt.Sprintf("invalid pull request id %q", rest[0])}
	}
	if (*path == "") != (*line == 0) {
		return usageError{msg: "--path and --line go together"}
	}
	if *side != "right" && *side != "left" {
		return usageError{msg: "--side must be right|left"}
	}
	text, err := o.readBody(*body, *bodyFile)
	if err != nil {
		return err
	}
	if fe := store.ValidateCommentBody(strings.TrimSpace(text)); fe.Code != "" {
		return usageError{msg: "body: " + fe.Message}
	}
	b, closer, repo, rc, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	in := store.PRCommentInput{Body: text, Path: *path, Line: *line, Side: *side}
	if rc != nil {
		cid, err := rc.CreatePRComment(ctx, id, in, 0)
		if err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "comment #%d created\n", cid)
		return nil
	}
	pr, err := b.GetPullRequest(ctx, id)
	if err != nil {
		return err
	}
	if in.Path != "" {
		anchorSHA := pr.HeadSHA
		if pr.Open() {
			if sha, err := repo.RevParse(ctx, pr.HeadRef); err == nil {
				anchorSHA = sha
			}
		}
		in.AnchorSHA = anchorSHA
		snippetSHA := anchorSHA
		if in.Side == "left" {
			snippetSHA = pr.MergeBaseSHA
			if pr.Open() {
				if mb, err := repo.MergeBase(ctx, pr.BaseRef, pr.HeadRef); err == nil {
					snippetSHA = mb
				}
			}
		}
		if snippetSHA != "" {
			if snippet, _, err := repo.Snippet(ctx, snippetSHA, in.Path, in.Line, 2); err == nil {
				in.DiffSnippet = snippet
			}
		}
	}
	cid, err := b.CreatePRComment(ctx, id, in, 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "comment #%d created\n", cid)
	return nil
}

// zatu pr comment edit PR_ID COMMENT_ID --body B
func prCommentEdit(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr comment edit", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	body := fs.String("body", "", "new body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	rest, err := o.parseLeaf(fs, "PR_ID COMMENT_ID", args, 2)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return usageError{msg: "PR_ID and COMMENT_ID are required"}
	}
	prID, cid, err := twoIDs(rest)
	if err != nil {
		return err
	}
	text, err := o.readBody(*body, *bodyFile)
	if err != nil {
		return err
	}
	if fe := store.ValidateCommentBody(strings.TrimSpace(text)); fe.Code != "" {
		return usageError{msg: "body: " + fe.Message}
	}
	b, closer, _, _, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	if err := b.UpdatePRComment(ctx, prID, cid, text); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "comment #%d updated\n", cid)
	return nil
}

func prCommentDelete(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("pr comment delete", flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	rest, err := o.parseLeaf(fs, "PR_ID COMMENT_ID", args, 2)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return usageError{msg: "PR_ID and COMMENT_ID are required"}
	}
	prID, cid, err := twoIDs(rest)
	if err != nil {
		return err
	}
	b, closer, _, _, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	if err := b.DeletePRComment(ctx, prID, cid); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "comment #%d deleted\n", cid)
	return nil
}

// zatu pr comment hide PR_ID COMMENT_ID --reason R / unhide PR_ID COMMENT_ID
func prCommentHide(ctx context.Context, o Options, action string, args []string) error {
	fs := flag.NewFlagSet("pr comment "+action, flag.ContinueOnError)
	bf, repoF := o.addPRFlags(fs)
	reason := fs.String("reason", "", "hide reason: "+strings.Join(store.HideReasons, "|"))
	rest, err := o.parseLeaf(fs, "PR_ID COMMENT_ID", args, 2)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return usageError{msg: "PR_ID and COMMENT_ID are required"}
	}
	prID, cid, err := twoIDs(rest)
	if err != nil {
		return err
	}
	if action == "hide" {
		if !slices.Contains(store.HideReasons, *reason) {
			return usageError{msg: fmt.Sprintf("--reason must be one of %s", strings.Join(store.HideReasons, ", "))}
		}
	} else {
		*reason = ""
	}
	b, closer, _, _, err := o.prEnv(ctx, bf, *repoF)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	if err := b.SetPRCommentHidden(ctx, prID, cid, *reason); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "comment #%d %sd\n", cid, action)
	return nil
}

func twoIDs(rest []string) (int64, int64, error) {
	a, err1 := strconv.ParseInt(rest[0], 10, 64)
	b, err2 := strconv.ParseInt(rest[1], 10, 64)
	if err1 != nil || err2 != nil || a < 1 || b < 1 {
		return 0, 0, usageError{msg: fmt.Sprintf("invalid ids %q %q", rest[0], rest[1])}
	}
	return a, b, nil
}

// --- JSON view helpers ---

func prViewJSON(p *store.PullRequest) api.PullRequest {
	return api.PullRequest{
		ID: p.ID, Title: p.Title, Body: p.Body, Status: p.Status,
		Base: p.BaseRef, Head: p.HeadRef, HeadSHA: p.HeadSHA,
		MergeSHA: p.MergeSHA, MergeMethod: p.MergeMethod,
		Author:    userView(p.Author),
		CreatedAt: p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func prViews(ps []*store.PullRequest) []api.PullRequest {
	out := make([]api.PullRequest, 0, len(ps))
	for _, p := range ps {
		out = append(out, prViewJSON(p))
	}
	return out
}

func prCommentViews(cs []*store.PRComment) []api.PRComment {
	out := make([]api.PRComment, 0, len(cs))
	for _, c := range cs {
		out = append(out, api.PRComment{
			ID: c.ID, Body: c.Body,
			Path: c.Path, Line: c.Line, Side: c.Side,
			AnchorSHA: c.AnchorSHA, DiffSnippet: c.DiffSnippet,
			Author:     userView(c.Author),
			CreatedAt:  c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Moderation: moderationView(c.Moderation),
		})
	}
	return out
}
