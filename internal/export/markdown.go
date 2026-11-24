// Package export renders store domain objects as standalone markdown
// documents — the thread export served by GET .../{id}/markdown on both
// the HTML app (the "Copy as markdown" fetch target) and /api/v1, and
// reused by the CLI's `view --markdown`.
package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// IssueMarkdown renders an issue and its comments as one markdown
// document. wsSlug identifies the owning workspace in the metadata block.
// Bodies are embedded verbatim — they are already markdown, so escaping
// would corrupt them; a body carrying headings simply takes its place in
// the document outline.
func IssueMarkdown(i *store.Issue, comments []*store.Comment, wsSlug string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(i.Title))
	fmt.Fprintf(&b, "- **Issue**: #%d\n", i.ID)
	fmt.Fprintf(&b, "- **Workspace**: %s\n", wsSlug)
	fmt.Fprintf(&b, "- **Status**: %s\n", i.Status)
	fmt.Fprintf(&b, "- **Author**: %s\n", author(i.Author))
	if len(i.Labels) > 0 {
		fmt.Fprintf(&b, "- **Labels**: %s\n", labelNames(i.Labels))
	}
	fmt.Fprintf(&b, "- **Created**: %s\n", rfc3339(i.CreatedAt))
	fmt.Fprintf(&b, "- **Updated**: %s\n", rfc3339(i.UpdatedAt))
	body(&b, i.Body)
	if len(comments) > 0 {
		b.WriteString("\n## Comments\n")
		for _, c := range comments {
			fmt.Fprintf(&b, "\n### Comment by %s · %s%s\n", author(c.Author), rfc3339(c.CreatedAt), moderationSuffix(c.Moderation))
			body(&b, c.Body)
		}
	}
	return b.String()
}

// DiscussionMarkdown renders a discussion with its threads and replies as
// one markdown document — the issue counterpart with one nesting level
// more. Threads carry "###", their replies "####", and the answer mark
// rides the thread heading as a "· answer" suffix. The Answered metadata
// line only appears for qa categories, where the mark is meaningful.
func DiscussionMarkdown(d *store.Discussion, threads []*store.DiscussionThread, wsSlug string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(d.Title))
	fmt.Fprintf(&b, "- **Discussion**: #%d\n", d.ID)
	fmt.Fprintf(&b, "- **Workspace**: %s\n", wsSlug)
	fmt.Fprintf(&b, "- **Category**: %s\n", oneLine(d.Category.Name))
	fmt.Fprintf(&b, "- **Author**: %s\n", author(d.Author))
	if len(d.Labels) > 0 {
		fmt.Fprintf(&b, "- **Labels**: %s\n", labelNames(d.Labels))
	}
	if d.Category.Format == store.CategoryFormatQA {
		fmt.Fprintf(&b, "- **Answered**: %s\n", yesNo(d.Answered))
	}
	fmt.Fprintf(&b, "- **Created**: %s\n", rfc3339(d.CreatedAt))
	fmt.Fprintf(&b, "- **Updated**: %s\n", rfc3339(d.UpdatedAt))
	body(&b, d.Body)
	if len(threads) > 0 {
		b.WriteString("\n## Threads\n")
		for _, t := range threads {
			answer := ""
			if t.IsAnswer {
				answer = " · answer"
			}
			fmt.Fprintf(&b, "\n### Thread by %s · %s%s%s\n", author(t.Author), rfc3339(t.CreatedAt), answer, moderationSuffix(t.Moderation))
			body(&b, t.Body)
			for _, c := range t.Comments {
				fmt.Fprintf(&b, "\n#### Reply by %s · %s%s\n", author(c.Author), rfc3339(c.CreatedAt), moderationSuffix(c.Moderation))
				body(&b, c.Body)
			}
		}
	}
	return b.String()
}

// moderationSuffix renders the edited/hidden markers on a post heading
// — "· edited" and "· hidden (spam)". A hidden post keeps its body in
// the export (it is still the record); the marker says it is minimized.
func moderationSuffix(m store.Moderation) string {
	var s string
	if m.Edited() {
		s += " · edited"
	}
	if m.Hidden {
		s += " · hidden (" + m.HiddenReason + ")"
	}
	return s
}

// body writes a verbatim markdown block separated by one blank line; an
// empty body adds nothing.
func body(b *strings.Builder, s string) {
	if s == "" {
		return
	}
	b.WriteString("\n")
	b.WriteString(s)
	if !strings.HasSuffix(s, "\n") {
		b.WriteString("\n")
	}
}

// oneLine keeps the title a single H1 line even when it contains line
// breaks (they would inject stray paragraphs into the header).
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

func author(u *store.User) string {
	if u == nil {
		return "anonymous"
	}
	// Names are only length-validated — an interior newline would inject
	// a fake line into the metadata block, same as a multiline title.
	return "@" + oneLine(u.Name)
}

func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func labelNames(labels []store.Label) string {
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		names = append(names, oneLine(l.Name))
	}
	return strings.Join(names, ", ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
