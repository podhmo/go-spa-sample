// Discussion subcommands — the same shapes as the issue commands, plus
// category and answer-mark handling (docs/sketch/ja/cli-and-web-api.md).
package cli

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/export"
	"github.com/podhmo/go-spa-sample/internal/store"
)

const discussionUsage = `usage: zatu discussion <action> [flags]

actions:
  list       list discussions (--category, --answered, --search, --sort, --limit, --page)
  view       show a discussion: N [--comments|--markdown] [-w]
  create     start a discussion: --title T --category SLUG [--body B|--body-file -] [-w]
  edit       update a discussion: N [--title T] [--body B] [--category SLUG] [--label L]...
  delete     delete a discussion: N
  comment    thread/reply commands: create|edit|delete|hide|unhide
  answer     mark the answer thread: answer N --thread T [--unmark]
  categories list discussion categories (slugs for --category)

"zatu discussion <action> --help" shows an action's flags.
`

// discussionCmd dispatches `zatu discussion <action>`.
func discussionCmd(ctx context.Context, o Options, args []string) error {
	if len(args) == 0 {
		return usageError{msg: "usage: zatu discussion list|view|create|edit|delete|comment|answer|categories"}
	}
	if isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, discussionUsage)
		return nil
	}
	switch args[0] {
	case "list":
		return discussionList(ctx, o, args[1:])
	case "view":
		return discussionView(ctx, o, args[1:])
	case "create":
		return discussionCreate(ctx, o, args[1:])
	case "edit":
		return discussionEdit(ctx, o, args[1:])
	case "delete":
		return discussionDelete(ctx, o, args[1:])
	case "comment":
		return discussionCommentCmd(ctx, o, args[1:])
	case "answer":
		return discussionAnswer(ctx, o, args[1:])
	case "categories":
		return discussionCategories(ctx, o, args[1:])
	default:
		return usageError{msg: fmt.Sprintf("unknown discussion command %q", args[0])}
	}
}

// zatu discussion list [--category SLUG] [--answered yes|no]
// [--search Q] [--sort newest|oldest] [--limit N] [--page N] [--json F,...]
func discussionList(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion list", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	category := fs.String("category", "", "filter by category slug")
	answered := fs.String("answered", "", "filter by answer mark: yes|no")
	query := fs.String("search", "", "search title and body")
	sortFlag := fs.String("sort", "newest", "sort order: newest|oldest")
	limit := fs.Int("limit", 30, "maximum discussions to list")
	page := fs.Int("page", 1, "page number")
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	if _, err := o.parseLeaf(fs, "", args, 0); err != nil {
		return err
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}
	switch *answered {
	case "", "yes", "no":
	default:
		return usageError{msg: fmt.Sprintf("--answered must be yes|no, got %q", *answered)}
	}
	if *sortFlag != "newest" && *sortFlag != "oldest" {
		return usageError{msg: fmt.Sprintf("--sort must be newest|oldest, got %q", *sortFlag)}
	}

	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	f := store.DiscussionListFilter{
		CategorySlug: *category, Answered: *answered,
		Query: strings.TrimSpace(*query),
		Sort:  *sortFlag, Page: *page, PerPage: *limit,
	}
	discussions, _, err := b.ListDiscussions(ctx, f)
	if err != nil {
		return err
	}
	if *jsonFields != "" {
		return writeJSONFields(o.Stdout, discussionViews(discussions), *jsonFields)
	}
	if *tmpl != "" {
		return writeTemplate(o.Stdout, discussionViews(discussions), *tmpl)
	}
	for _, d := range discussions {
		fmt.Fprintf(o.Stdout, "%d\t%s\t%s\t%s\t%s\n",
			d.ID, d.Category.Slug, o.answeredMark(d.Answered), d.Title, fmtTimeShort(d.UpdatedAt))
	}
	return nil
}

// zatu discussion view N [--comments|--markdown] [--json F,...] [-w]
func discussionView(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion view", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	comments := fs.Bool("comments", false, "include threads and their replies")
	markdown := fs.Bool("markdown", false, "print the discussion with threads as a markdown document")
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	web := fs.Bool("web", false, "open the discussion page in a browser (requires --server)")
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
		return usageError{msg: "--markdown is mutually exclusive with --comments (markdown always includes threads)"}
	}
	id, err := argNumber(rest, 0, "discussion")
	if err != nil {
		return err
	}

	if *web {
		return o.openPage(bf, "/discussions/", id)
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	d, err := b.GetDiscussion(ctx, id)
	if err != nil {
		return err
	}
	var ts []*store.DiscussionThread
	if *comments || *markdown {
		ts, err = b.ListDiscussionThreads(ctx, id)
		if err != nil {
			return err
		}
	}
	if *markdown {
		fmt.Fprint(o.Stdout, export.DiscussionMarkdown(d, ts, bf.wsSlug()))
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
			m, err := jsonObject(discussionViewJSON(d))
			if err != nil {
				return err
			}
			m["threads"] = discussionThreadViews(ts)
			return out(m)
		}
		return out(discussionViewJSON(d))
	}

	fmt.Fprintf(o.Stdout, "title:\t%s\n", d.Title)
	fmt.Fprintf(o.Stdout, "category:\t%s (%s)\n", d.Category.Name, d.Category.Slug)
	fmt.Fprintf(o.Stdout, "answered:\t%s\n", map[bool]string{true: "yes", false: "no"}[d.Answered])
	fmt.Fprintf(o.Stdout, "labels:\t%s\n", labelNames(d.Labels))
	fmt.Fprintf(o.Stdout, "author:\t%s\n", authorName(d.Author))
	fmt.Fprintf(o.Stdout, "updated:\t%s\n", fmtTimeShort(d.UpdatedAt))
	fmt.Fprintf(o.Stdout, "--\n%s\n", d.Body)
	if *comments {
		fmt.Fprintln(o.Stdout, "\ncomments:")
		for _, t := range ts {
			printDiscussionThread(o, t, "  ")
			for _, c := range t.Comments {
				printDiscussionComment(o, c, "    ")
			}
		}
	}
	return nil
}

func printDiscussionThread(o Options, t *store.DiscussionThread, indent string) {
	answer := ""
	if t.IsAnswer {
		answer = " " + o.paint("32", "[answer]")
	}
	fmt.Fprintf(o.Stdout, "%s#%d %s (%s)%s%s\n%s  %s\n",
		indent, t.ID, authorName(t.Author), fmtTimeShort(t.CreatedAt), answer, moderationMarks(t.Moderation),
		indent, strings.ReplaceAll(t.Body, "\n", "\n"+indent+"  "))
}

func printDiscussionComment(o Options, c *store.DiscussionComment, indent string) {
	fmt.Fprintf(o.Stdout, "%s#%d %s (%s)%s\n%s  %s\n",
		indent, c.ID, authorName(c.Author), fmtTimeShort(c.CreatedAt), moderationMarks(c.Moderation),
		indent, strings.ReplaceAll(c.Body, "\n", "\n"+indent+"  "))
}

// zatu discussion create --title T --category SLUG
// [--body B|--body-file -] [-w]
func discussionCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion create", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	title := fs.String("title", "", "discussion title (required)")
	category := fs.String("category", "", "category slug (required)")
	body := fs.String("body", "", "discussion body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	var labels nameFlag
	fs.Var(&labels, "label", "attach a label by name (repeatable)")
	web := fs.Bool("web", false, "open the new discussion in a browser (requires --server)")
	fs.BoolVar(web, "w", false, "shorthand for --web")
	if _, err := o.parseLeaf(fs, "--title T --category SLUG", args, 0); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return usageError{msg: "--title is required"}
	}
	if *category == "" {
		return usageError{msg: "--category is required"}
	}
	if *web && bf.server == "" {
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

	categoryID, err := resolveCategoryID(ctx, b, *category)
	if err != nil {
		return err
	}
	labelIDs, err := resolveLabelIDs(ctx, b, store.KindLabelDiscussion, labels)
	if err != nil {
		return err
	}
	in := store.DiscussionInput{
		Title: strings.TrimSpace(*title), Body: strings.TrimSpace(bodyText), CategoryID: categoryID,
		LabelIDs: labelIDs,
	}
	if err := validateDiscussionInput(in); err != nil {
		return err
	}
	id, err := b.CreateDiscussion(ctx, in, 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "created discussion #%d\n", id)
	if *web {
		return o.openPage(bf, "/discussions/", id)
	}
	return nil
}

// zatu discussion edit N [--title T] [--body B] [--category SLUG] [--label NAME]...
func discussionEdit(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion edit", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	title := fs.String("title", "", "new title")
	body := fs.String("body", "", "new body")
	category := fs.String("category", "", "new category slug")
	var labels nameFlag
	fs.Var(&labels, "label", "replace labels with this name set (repeatable)")
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	id, err := argNumber(rest, 0, "discussion")
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	d, err := b.GetDiscussion(ctx, id)
	if err != nil {
		return err
	}
	in := store.DiscussionInput{Title: d.Title, Body: d.Body, CategoryID: d.Category.ID}
	for _, l := range d.Labels {
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
	if *category != "" {
		in.CategoryID, err = resolveCategoryID(ctx, b, *category)
		if err != nil {
			return err
		}
		changed = true
	}
	if len(labels) > 0 {
		in.LabelIDs, err = resolveLabelIDs(ctx, b, store.KindLabelDiscussion, labels)
		if err != nil {
			return err
		}
		changed = true
	}
	if !changed {
		return usageError{msg: "nothing to update: pass --title, --body, --category or --label"}
	}
	if err := validateDiscussionInput(in); err != nil {
		return err
	}
	if err := b.UpdateDiscussion(ctx, id, in); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "updated discussion #%d\n", id)
	return nil
}

// zatu discussion delete N
func discussionDelete(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion delete", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	rest, err := o.parseLeaf(fs, "N", args, 1)
	if err != nil {
		return err
	}
	id, err := argNumber(rest, 0, "discussion")
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if err := b.DeleteDiscussion(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "deleted discussion #%d\n", id)
	return nil
}

const discussionCommentUsage = `usage: zatu discussion comment <action> [flags]

actions:
  create   open a thread or reply: create N --body B [--thread T]
  edit     edit a thread or reply: edit N --thread T [--comment C] --body B
  delete   delete a thread or reply: delete N --thread T [--comment C]
  hide     hide a thread or reply: hide N --thread T [--comment C] --reason R
  unhide   restore a hidden thread or reply: unhide N --thread T [--comment C]

--thread picks the thread; --comment picks a reply inside it (a thread's
"comments" are its replies — the opening post is the thread itself).

"zatu discussion comment <action> --help" shows an action's flags.
`

// zatu discussion comment create|edit|delete|hide|unhide
func discussionCommentCmd(ctx context.Context, o Options, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, discussionCommentUsage)
		return nil
	}
	if len(args) == 0 {
		return usageError{msg: "usage: zatu discussion comment create|edit|delete|hide|unhide"}
	}
	switch args[0] {
	case "create":
		return discussionCommentCreate(ctx, o, args[1:])
	case "edit":
		return discussionCommentEdit(ctx, o, args[1:])
	case "delete":
		return discussionCommentDelete(ctx, o, args[1:])
	case "hide":
		return discussionCommentHide(ctx, o, args[1:], true)
	case "unhide":
		return discussionCommentHide(ctx, o, args[1:], false)
	default:
		return usageError{msg: fmt.Sprintf("unknown discussion comment command %q", args[0])}
	}
}

// zatu discussion comment create N --body B [--thread T]
func discussionCommentCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion comment create", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	body := fs.String("body", "", "comment body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	thread := fs.Int64("thread", 0, "reply to this thread")
	rest, err := o.parseLeaf(fs, "N --body B", args, 1)
	if err != nil {
		return err
	}
	id, err := argNumber(rest, 0, "discussion")
	if err != nil {
		return err
	}
	if *thread < 0 {
		return usageError{msg: "--thread must be a positive id"}
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

	var cid int64
	if *thread != 0 {
		cid, err = b.CreateDiscussionComment(ctx, id, *thread, bodyText, 0)
	} else {
		cid, err = b.CreateDiscussionThread(ctx, id, bodyText, 0)
	}
	if err != nil {
		return err
	}
	if *thread != 0 {
		fmt.Fprintf(o.Stdout, "created reply #%d on thread #%d of discussion #%d\n", cid, *thread, id)
	} else {
		fmt.Fprintf(o.Stdout, "created thread #%d on discussion #%d\n", cid, id)
	}
	return nil
}

// discussionPostTarget parses the flags every thread/reply leaf shares:
// the discussion number N, --thread T (required), and --comment C which
// switches the target from the thread itself to one of its replies.
func discussionPostTarget(fs *flag.FlagSet, o Options, hint string, args []string) (id, tid, cid int64, bf *backendFlags, err error) {
	bf = o.addBackendFlags(fs)
	tidf := fs.Int64("thread", 0, "thread id (required)")
	cidf := fs.Int64("comment", 0, "reply id under --thread (omit to target the thread itself)")
	rest, err := o.parseLeaf(fs, hint, args, 1)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	id, err = argNumber(rest, 0, "discussion")
	if err != nil {
		return 0, 0, 0, nil, err
	}
	if *tidf < 1 {
		return 0, 0, 0, nil, usageError{msg: "--thread is required"}
	}
	if *cidf < 0 {
		return 0, 0, 0, nil, usageError{msg: "--comment must be a positive id"}
	}
	return id, *tidf, *cidf, bf, nil
}

// zatu discussion comment edit N --thread T [--comment C] --body B
func discussionCommentEdit(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion comment edit", flag.ContinueOnError)
	body := fs.String("body", "", "new body")
	bodyFile := fs.String("body-file", "", "read body from file (- for stdin)")
	id, tid, cid, bf, err := discussionPostTarget(fs, o, "N --thread T [--comment C] --body B", args)
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

	if cid != 0 {
		if err := b.UpdateDiscussionComment(ctx, id, tid, cid, bodyText); err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "updated reply #%d on thread #%d of discussion #%d\n", cid, tid, id)
	} else {
		if err := b.UpdateDiscussionThread(ctx, id, tid, bodyText); err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "updated thread #%d on discussion #%d\n", tid, id)
	}
	return nil
}

// zatu discussion comment delete N --thread T [--comment C]
func discussionCommentDelete(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion comment delete", flag.ContinueOnError)
	id, tid, cid, bf, err := discussionPostTarget(fs, o, "N --thread T [--comment C]", args)
	if err != nil {
		return err
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if cid != 0 {
		if err := b.DeleteDiscussionComment(ctx, id, tid, cid); err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "deleted reply #%d on thread #%d of discussion #%d\n", cid, tid, id)
	} else {
		if err := b.DeleteDiscussionThread(ctx, id, tid); err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "deleted thread #%d on discussion #%d (replies removed)\n", tid, id)
	}
	return nil
}

// zatu discussion comment hide|unhide N --thread T [--comment C] [--reason R]
func discussionCommentHide(ctx context.Context, o Options, args []string, hide bool) error {
	name := "discussion comment hide"
	if !hide {
		name = "discussion comment unhide"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	reason := ""
	hint := "N --thread T [--comment C]"
	if hide {
		fs.StringVar(&reason, "reason", "", "hide reason: "+strings.Join(store.HideReasons, "|"))
		hint = "N --thread T [--comment C] --reason R"
	}
	id, tid, cid, bf, err := discussionPostTarget(fs, o, hint, args)
	if err != nil {
		return err
	}
	if hide && !store.ValidHideReason(reason) {
		return usageError{msg: "--reason is required: " + strings.Join(store.HideReasons, ", ")}
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if cid != 0 {
		if err := b.SetDiscussionCommentHidden(ctx, id, tid, cid, reason); err != nil {
			return err
		}
		if hide {
			fmt.Fprintf(o.Stdout, "hidden reply #%d on thread #%d of discussion #%d (%s)\n", cid, tid, id, reason)
		} else {
			fmt.Fprintf(o.Stdout, "unhidden reply #%d on thread #%d of discussion #%d\n", cid, tid, id)
		}
	} else {
		if err := b.SetDiscussionThreadHidden(ctx, id, tid, reason); err != nil {
			return err
		}
		if hide {
			fmt.Fprintf(o.Stdout, "hidden thread #%d on discussion #%d (%s)\n", tid, id, reason)
		} else {
			fmt.Fprintf(o.Stdout, "unhidden thread #%d on discussion #%d\n", tid, id)
		}
	}
	return nil
}

// zatu discussion answer N --thread T [--unmark]
func discussionAnswer(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion answer", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	thread := fs.Int64("thread", 0, "thread to mark as the answer (required)")
	unmark := fs.Bool("unmark", false, "clear the answer mark instead of setting it")
	rest, err := o.parseLeaf(fs, "N --thread T", args, 1)
	if err != nil {
		return err
	}
	id, err := argNumber(rest, 0, "discussion")
	if err != nil {
		return err
	}
	if *thread < 1 {
		return usageError{msg: "--thread is required"}
	}
	b, closer, err := o.backend(ctx, bf)
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	if err := b.SetThreadAnswer(ctx, id, *thread, !*unmark); err != nil {
		return err
	}
	if *unmark {
		fmt.Fprintf(o.Stdout, "unmarked thread #%d as answer on discussion #%d\n", *thread, id)
	} else {
		fmt.Fprintf(o.Stdout, "marked thread #%d as answer on discussion #%d\n", *thread, id)
	}
	return nil
}

// zatu discussion categories [--json F,...] [--template T]
// Categories are global master data (not per-workspace), so the backend
// opens unscoped like the workspace verbs — --workspace is ignored and
// remote mode hits /discussion-categories either way.
func discussionCategories(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("discussion categories", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	if _, err := o.parseLeaf(fs, "", args, 0); err != nil {
		return err
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}
	b, closer, err := o.backendIn(ctx, bf, "")
	if err != nil {
		return err
	}
	defer closeQuietly(closer)

	cats, err := b.ListDiscussionCategories(ctx)
	if err != nil {
		return err
	}
	if *jsonFields != "" {
		return writeJSONFields(o.Stdout, categoryViews(cats), *jsonFields)
	}
	if *tmpl != "" {
		return writeTemplate(o.Stdout, categoryViews(cats), *tmpl)
	}
	for _, c := range cats {
		fmt.Fprintf(o.Stdout, "%s\t%s\t%s\t%s\n", c.Slug, c.Name, c.Format, c.Description)
	}
	return nil
}

// resolveCategoryID maps a category slug to its id through the backend's
// category list — the CLI takes slugs like the ?category= URL filter.
func resolveCategoryID(ctx context.Context, b Backend, slug string) (int64, error) {
	cats, err := b.ListDiscussionCategories(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range cats {
		if c.Slug == slug {
			return c.ID, nil
		}
	}
	valid := make([]string, 0, len(cats))
	for _, c := range cats {
		valid = append(valid, c.Slug)
	}
	sort.Strings(valid)
	return 0, fmt.Errorf("unknown category %q (valid: %s)", slug, strings.Join(valid, ", "))
}

// validateDiscussionInput applies the shared validation rules — the local
// store backend does not validate on its own.
func validateDiscussionInput(in store.DiscussionInput) error {
	fe := store.ValidateDiscussionInput(in)
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
