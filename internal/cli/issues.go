package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/export"
	"github.com/podhmo/go-spa-sample/internal/store"
)

const issueUsage = `usage: zatu issue <action> [flags]

actions:
  list     list issues (--state, --label, --search, --sort, --limit, --page)
  view     show an issue: N [--comments|--markdown] [-w]
  create   file an issue: --title T [--body B|--body-file -] [--label L]... [-w]
  edit     update an issue: N [--title T] [--body B] [--label L]...
  close    close an issue: N
  reopen   reopen an issue: N
  delete   delete an issue: N

"zatu issue <action> --help" shows an action's flags.
`

// issueCmd dispatches `zatu issue <action>`.
func issueCmd(ctx context.Context, o Options, args []string) error {
	if len(args) == 0 {
		return usageError{msg: "usage: zatu issue list|view|create|edit|close|reopen|delete"}
	}
	if isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, issueUsage)
		return nil
	}
	switch args[0] {
	case "list":
		return issueList(ctx, o, args[1:])
	case "view":
		return issueView(ctx, o, args[1:])
	case "create":
		return issueCreate(ctx, o, args[1:])
	case "edit":
		return issueEdit(ctx, o, args[1:])
	case "close", "reopen", "delete":
		return issueStateChange(ctx, o, args[0], args[1:])
	default:
		return usageError{msg: fmt.Sprintf("unknown issue command %q", args[0])}
	}
}

// zatu issue list [--state open|closed|all] [--label NAME]
// [--search Q] [--sort newest|oldest] [--limit N] [--page N] [--json F,...]
func issueList(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("issue list", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	state := fs.String("state", "open", "filter by state: open|closed|all")
	label := fs.String("label", "", "filter by label name")
	query := fs.String("search", "", "search title and body")
	sort := fs.String("sort", "newest", "sort order: newest|oldest")
	limit := fs.Int("limit", 30, "maximum issues to list")
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
	case "open", "closed", "all":
	default:
		return usageError{msg: fmt.Sprintf("--state must be open|closed|all, got %q", *state)}
	}
	if *sort != "newest" && *sort != "oldest" {
		return usageError{msg: fmt.Sprintf("--sort must be newest|oldest, got %q", *sort)}
	}

	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	f := store.ListFilter{
		Status: *state, Query: strings.TrimSpace(*query),
		Sort: *sort, Page: *page, PerPage: *limit,
	}
	if *label != "" {
		ids, err := resolveLabelIDs(ctx, b, store.KindLabelIssue, []string{*label})
		if err != nil {
			return err
		}
		f.LabelID = ids[0]
	}
	issues, _, err := b.ListIssues(ctx, f)
	if err != nil {
		return err
	}
	if *jsonFields != "" {
		return writeJSONFields(o.Stdout, issueViews(issues), *jsonFields)
	}
	if *tmpl != "" {
		return writeTemplate(o.Stdout, issueViews(issues), *tmpl)
	}
	for _, i := range issues {
		fmt.Fprintf(o.Stdout, "%d\t%s\t%s\t%s\t%s\n",
			i.ID, o.statusText(i.Status), i.Title, labelNames(i.Labels), fmtTimeShort(i.UpdatedAt))
	}
	return nil
}

// zatu issue view N [--comments|--markdown] [--json F,...] [-w]
func issueView(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("issue view", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	comments := fs.Bool("comments", false, "include comments")
	markdown := fs.Bool("markdown", false, "print the issue with comments as a markdown document")
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	web := fs.Bool("web", false, "open the issue page in a browser (requires --server)")
	fs.BoolVar(web, "w", false, "shorthand for --web")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}
	if *markdown && (*jsonFields != "" || *tmpl != "" || *web) {
		return usageError{msg: "--markdown is mutually exclusive with --json, --template and --web"}
	}
	if *markdown && *comments {
		return usageError{msg: "--markdown is mutually exclusive with --comments (markdown always includes comments)"}
	}
	id, err := issueNumber(rest, 0)
	if err != nil {
		return err
	}

	if *web {
		return o.openPage(bf, "/issues/", id)
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	issue, err := b.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	var cs []*store.Comment
	if *comments || *markdown {
		cs, err = b.ListComments(ctx, id)
		if err != nil {
			return err
		}
	}
	if *markdown {
		fmt.Fprint(o.Stdout, export.IssueMarkdown(issue, cs, bf.wsSlug()))
		return nil
	}
	out := func(v any) error {
		if *tmpl != "" {
			return writeTemplate(o.Stdout, v, *tmpl)
		}
		return writeJSONFields(o.Stdout, v, *jsonFields)
	}
	if *jsonFields != "" || *tmpl != "" {
		if *comments {
			// Issue fields stay at the root with "comments" added —
			// nesting them under "issue" would make --json title
			// silently select nothing.
			m, err := jsonObject(issueViewJSON(issue))
			if err != nil {
				return err
			}
			m["comments"] = commentViews(cs)
			return out(m)
		}
		return out(issueViewJSON(issue))
	}

	fmt.Fprintf(o.Stdout, "title:\t%s\n", issue.Title)
	fmt.Fprintf(o.Stdout, "status:\t%s\n", o.statusText(issue.Status))
	fmt.Fprintf(o.Stdout, "author:\t%s\n", authorName(issue.Author))
	fmt.Fprintf(o.Stdout, "labels:\t%s\n", labelNames(issue.Labels))
	fmt.Fprintf(o.Stdout, "updated:\t%s\n", fmtTimeShort(issue.UpdatedAt))
	fmt.Fprintf(o.Stdout, "--\n%s\n", issue.Body)
	if *comments {
		fmt.Fprintln(o.Stdout, "\ncomments:")
		for _, c := range cs {
			fmt.Fprintf(o.Stdout, "  #%d %s (%s)%s\n    %s\n",
				c.ID, authorName(c.Author), fmtTimeShort(c.CreatedAt), moderationMarks(c.Moderation),
				strings.ReplaceAll(c.Body, "\n", "\n    "))
		}
	}
	return nil
}

// zatu issue create --title T [--body B|--body-file -] [--label NAME]... [-w]
func issueCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	title := fs.String("title", "", "issue title (required)")
	body := fs.String("body", "", "issue body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	var labels nameFlag
	fs.Var(&labels, "label", "attach a label by name (repeatable)")
	web := fs.Bool("web", false, "open the new issue in a browser (requires --server)")
	fs.BoolVar(web, "w", false, "shorthand for --web")
	if _, err := o.parseLeaf(fs, "--title T", args, 0); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return usageError{msg: "--title is required"}
	}
	if *web && bf.server == "" {
		// Check before touching the backend: creating the issue first
		// and only then failing means a retry creates a duplicate.
		return usageError{msg: "-w/--web requires --server (or ZATUDAN_SERVER)"}
	}
	bodyText, err := o.readBody(*body, *bodyFile)
	if err != nil {
		return err
	}

	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	labelIDs, err := resolveLabelIDs(ctx, b, store.KindLabelIssue, labels)
	if err != nil {
		return err
	}
	// The actor is anonymous locally; remotely it is whoever the API
	// token resolves to — the authorID arg is ignored over HTTP.
	in := store.IssueInput{
		Title: strings.TrimSpace(*title), Body: strings.TrimSpace(bodyText), LabelIDs: labelIDs,
	}
	// The local backend doesn't validate; apply the same rules the
	// web/API enforce so both modes reject the same input.
	if err := validateIssueInput(in); err != nil {
		return err
	}
	id, err := b.CreateIssue(ctx, in, 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "created issue #%d\n", id)
	if *web {
		return o.openPage(bf, "/issues/", id)
	}
	return nil
}

// zatu issue edit N [--title T] [--body B] [--label NAME]...
func issueEdit(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("issue edit", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	title := fs.String("title", "", "new title")
	body := fs.String("body", "", "new body")
	var labels nameFlag
	fs.Var(&labels, "label", "replace labels with this name set (repeatable)")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	id, err := issueNumber(rest, 0)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	issue, err := b.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	in := store.IssueInput{Title: issue.Title, Body: issue.Body}
	for _, l := range issue.Labels {
		in.LabelIDs = append(in.LabelIDs, l.ID)
	}
	changed := false
	if *title != "" {
		in.Title = strings.TrimSpace(*title)
		changed = true
	}
	fs.Visit(func(f *flag.Flag) { // --body counts even when empty
		if f.Name == "body" {
			in.Body = strings.TrimSpace(*body)
			changed = true
		}
	})
	if len(labels) > 0 {
		in.LabelIDs, err = resolveLabelIDs(ctx, b, store.KindLabelIssue, labels)
		if err != nil {
			return err
		}
		changed = true
	}
	if !changed {
		return usageError{msg: "nothing to update: pass --title, --body or --label"}
	}
	if err := validateIssueInput(in); err != nil {
		return err
	}
	if err := b.UpdateIssue(ctx, id, in); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "updated issue #%d\n", id)
	return nil
}

// zatu issue close|reopen|delete N
func issueStateChange(ctx context.Context, o Options, action string, args []string) error {
	fs := flag.NewFlagSet("issue "+action, flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	id, err := issueNumber(rest, 0)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	switch action {
	case "close", "reopen":
		status := store.StatusClosed
		if action == "reopen" {
			status = store.StatusOpen
		}
		if err := b.SetIssueStatus(ctx, id, status); err != nil {
			return err
		}
	case "delete":
		if err := b.DeleteIssue(ctx, id); err != nil {
			return err
		}
	}
	verb := map[string]string{"close": "closed", "reopen": "reopened", "delete": "deleted"}[action]
	fmt.Fprintf(o.Stdout, "%s issue #%d\n", verb, id)
	return nil
}

const commentUsage = `usage: zatu comment <action> [flags]

actions:
  create   comment on an issue: create N --body B | --body-file -
  edit     edit a comment: edit N C --body B | --body-file -
  delete   delete a comment: delete N C
  hide     hide a comment: hide N C --reason R
  unhide   restore a hidden comment: unhide N C

"zatu comment <action> --help" shows an action's flags.
`

// zatu comment create|edit|delete|hide|unhide
func commentCmd(ctx context.Context, o Options, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, commentUsage)
		return nil
	}
	if len(args) == 0 {
		return usageError{msg: "usage: zatu comment create|edit|delete|hide|unhide"}
	}
	switch args[0] {
	case "create":
		return commentCreate(ctx, o, args[1:])
	case "edit":
		return commentEdit(ctx, o, args[1:])
	case "delete":
		return commentDelete(ctx, o, args[1:])
	case "hide":
		return commentHide(ctx, o, args[1:], true)
	case "unhide":
		return commentHide(ctx, o, args[1:], false)
	default:
		return usageError{msg: fmt.Sprintf("unknown comment command %q", args[0])}
	}
}

// commentTarget parses the (issue, comment) positional pair the
// moderation leaves share: N is the issue number, C the comment id.
func commentTarget(rest []string) (issueID, commentID int64, err error) {
	issueID, err = issueNumber(rest, 0)
	if err != nil {
		return 0, 0, err
	}
	commentID, err = argNumber(rest, 1, "comment")
	if err != nil {
		return 0, 0, err
	}
	return issueID, commentID, nil
}

// readCommentBody resolves --body/--body-file and applies the shared
// body validation (empty maps to the usage error, the rest to exit 1).
func (o Options) readCommentBody(body, bodyFile string) (string, error) {
	bodyText, err := o.readBody(body, bodyFile)
	if err != nil {
		return "", err
	}
	bodyText = strings.TrimSpace(bodyText)
	if fe := store.ValidateCommentBody(bodyText); fe.Code != "" {
		if bodyText == "" {
			return "", usageError{msg: "--body is required (or --body-file)"}
		}
		return "", fmt.Errorf("invalid comment: %s", fe.Message)
	}
	return bodyText, nil
}

// zatu comment create N --body B | --body-file -
func commentCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("comment create", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	body := fs.String("body", "", "comment body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	rest, err := o.parseLeaf(fs, "N --body B", args, 1)
	if err != nil {
		return err
	}
	id, err := issueNumber(rest, 0)
	if err != nil {
		return err
	}
	bodyText, err := o.readCommentBody(*body, *bodyFile)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	cid, err := b.CreateComment(ctx, id, bodyText, 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "created comment #%d on issue #%d\n", cid, id)
	return nil
}

// zatu comment edit N C --body B | --body-file -
func commentEdit(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("comment edit", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	body := fs.String("body", "", "new comment body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	rest, err := o.parseLeaf(fs, "N C --body B", args, 2)
	if err != nil {
		return err
	}
	id, cid, err := commentTarget(rest)
	if err != nil {
		return err
	}
	bodyText, err := o.readCommentBody(*body, *bodyFile)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if err := b.UpdateComment(ctx, id, cid, bodyText); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "updated comment #%d on issue #%d\n", cid, id)
	return nil
}

// zatu comment delete N C
func commentDelete(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("comment delete", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	rest, err := o.parseLeaf(fs, "N C", args, 2)
	if err != nil {
		return err
	}
	id, cid, err := commentTarget(rest)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if err := b.DeleteComment(ctx, id, cid); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "deleted comment #%d on issue #%d\n", cid, id)
	return nil
}

// zatu comment hide|unhide N C [--reason R]
func commentHide(ctx context.Context, o Options, args []string, hide bool) error {
	name := "comment hide"
	if !hide {
		name = "comment unhide"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	reason := ""
	hint := "N C"
	if hide {
		fs.StringVar(&reason, "reason", "", "hide reason: "+strings.Join(store.HideReasons, "|"))
		hint = "N C --reason R"
	}
	rest, err := o.parseLeaf(fs, hint, args, 2)
	if err != nil {
		return err
	}
	if hide && !store.ValidHideReason(reason) {
		return usageError{msg: "--reason is required: " + strings.Join(store.HideReasons, ", ")}
	}
	id, cid, err := commentTarget(rest)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if err := b.SetCommentHidden(ctx, id, cid, reason); err != nil {
		return err
	}
	if hide {
		fmt.Fprintf(o.Stdout, "hidden comment #%d on issue #%d (%s)\n", cid, id, reason)
	} else {
		fmt.Fprintf(o.Stdout, "unhidden comment #%d on issue #%d\n", cid, id)
	}
	return nil
}

const labelUsage = `usage: zatu label <action> [flags]

actions:
  list     list labels [--kind issue|discussion]
  create   create a label: NAME --color HEX [--kind K] [--shared]
  delete   archive a label: NAME [--kind K]

"zatu label <action> --help" shows an action's flags.
`

// zatu label list|create|delete
func labelCmd(ctx context.Context, o Options, args []string) error {
	if len(args) == 0 {
		return usageError{msg: "usage: zatu label list|create|delete"}
	}
	if isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, labelUsage)
		return nil
	}
	switch args[0] {
	case "list":
		return labelList(ctx, o, args[1:])
	case "create":
		return labelCreate(ctx, o, args[1:])
	case "delete":
		return labelDelete(ctx, o, args[1:])
	default:
		return usageError{msg: "usage: zatu label list|create|delete"}
	}
}

// labelKindFlag validates the --kind flag shared by the label commands.
func labelKindFlag(fs *flag.FlagSet) *string {
	return fs.String("kind", "issue", "label kind: issue|discussion")
}

func checkKind(k string) error {
	if k != store.KindLabelIssue && k != store.KindLabelDiscussion {
		return usageError{msg: fmt.Sprintf("--kind must be issue|discussion, got %q", k)}
	}
	return nil
}

// zatu label list [--kind issue|discussion]
func labelList(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("label list", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	kind := labelKindFlag(fs)
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	if _, err := o.parseLeaf(fs, "", args, 0); err != nil {
		return err
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}
	if err := checkKind(*kind); err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	labels, err := b.ListLabels(ctx, *kind)
	if err != nil {
		return err
	}
	if *jsonFields != "" {
		return writeJSONFields(o.Stdout, labelViews(labels), *jsonFields)
	}
	if *tmpl != "" {
		return writeTemplate(o.Stdout, labelViews(labels), *tmpl)
	}
	for _, l := range labels {
		scope := "workspace"
		if l.WorkspaceID == 0 {
			scope = "shared"
		}
		fmt.Fprintf(o.Stdout, "%s\t%s\t%s\n", l.Name, l.Color, scope)
	}
	return nil
}

// zatu label create NAME --color HEX [--kind K] [--shared]
// Creating a name that matches an archived label restores it with the
// new color — that is the unarchive path (there is no `label restore`).
func labelCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("label create", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	kind := labelKindFlag(fs)
	color := fs.String("color", "", "label color as #RRGGBB (required)")
	shared := fs.Bool("shared", false, "create a shared label visible in every workspace")
	rest, err := o.parseLeaf(fs, "NAME --color HEX", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "missing label name"}
	}
	if *color == "" {
		return usageError{msg: "--color is required"}
	}
	if err := checkKind(*kind); err != nil {
		return err
	}
	in := store.LabelInput{Name: strings.TrimSpace(rest[0]), Color: strings.TrimSpace(*color)}
	if fe := store.ValidateLabelInput(in); len(fe) > 0 {
		keys := make([]string, 0, len(fe))
		for k := range fe {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+": "+fe[k].Message)
		}
		return fmt.Errorf("invalid input: %s", strings.Join(parts, "; "))
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	id, err := b.CreateLabel(ctx, *kind, in, *shared)
	if err != nil {
		return err
	}
	scope := "workspace"
	if *shared {
		scope = "shared"
	}
	fmt.Fprintf(o.Stdout, "created %s label #%d %q\n", scope, id, in.Name)
	return nil
}

// zatu label delete NAME [--kind K] — archives the label: it leaves
// pickers and filters but stays attached to issues/discussions that
// already carry it.
func labelDelete(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("label delete", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	kind := labelKindFlag(fs)
	rest, err := o.parseLeaf(fs, "NAME", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "missing label name"}
	}
	if err := checkKind(*kind); err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)
	labels, err := b.ListLabels(ctx, *kind)
	if err != nil {
		return err
	}
	name := rest[0]
	// Prefer this workspace's own label when the name exists in both
	// scopes — the union shows both, and archiving the shared row is the
	// less reversible, wider-reaching choice.
	var target *store.Label
	for _, l := range labels {
		if l.Name != name {
			continue
		}
		if target == nil || l.WorkspaceID != 0 {
			target = l
		}
	}
	if target == nil {
		return fmt.Errorf("unknown %s label: %s", *kind, name)
	}
	if err := b.ArchiveLabel(ctx, target.ID, true); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "archived label #%d %q\n", target.ID, name)
	return nil
}

// openPage builds the web page URL for a resource and opens it in the
// browser. It needs the server base URL, so remote mode is required.
func (o Options) openPage(bf *backendFlags, prefix string, id int64) error {
	// In the split SPA+API deployment the server origin only answers
	// /api/v1 — ZATUDAN_WEB/--web names the UI origin; --server stays
	// the fallback for a combined deployment.
	base := bf.webURL
	if base == "" {
		base = bf.server
	}
	if base == "" {
		return usageError{msg: "-w/--web requires --server (or ZATUDAN_SERVER)"}
	}
	return o.OpenBrowser(strings.TrimSuffix(base, "/") + prefix + fmt.Sprint(id))
}

// validateIssueInput applies the shared validation rules (the same
// ValidateIssueInput the web/API handlers use) — the local store backend
// does not validate on its own.
func validateIssueInput(in store.IssueInput) error {
	fe := store.ValidateIssueInput(in)
	if !fe.Any() {
		return nil
	}
	keys := make([]string, 0, len(fe))
	for k := range fe {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+fe[k].Message)
	}
	return fmt.Errorf("invalid input: %s", strings.Join(parts, "; "))
}

// jsonObject re-marshals a DTO into a map so extra keys (like comments)
// can be spliced in before field selection.
func jsonObject(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func labelNames(labels []store.Label) string {
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		names = append(names, l.Name)
	}
	return strings.Join(names, ", ")
}
