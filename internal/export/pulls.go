package export

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// PRCommit is one head-side commit summary, collected by the caller —
// export stays a pure function over plain values and never runs git.
type PRCommit struct {
	SHA     string
	Subject string
	Author  string
	Date    time.Time
}

// PRMarkdownData is everything PullRequestMarkdown renders: the store row
// plus the commit list and comments the caller gathered.
type PRMarkdownData struct {
	PR       *store.PullRequest
	Commits  []PRCommit
	Comments []*store.PRComment
}

// PullRequestMarkdown renders a pull request as one markdown document —
// the export behind GET /pulls/{id}/markdown and `zatu pr view
// --markdown` (docs/sketch/ja/pull-requests.md §7). Review comments carry
// their anchor (`path:L<line>` (side)) and the diff snippet snapshotted
// at post time, so the document stays readable after the anchor commit
// is garbage-collected.
func PullRequestMarkdown(d PRMarkdownData) string {
	pr := d.PR
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(pr.Title))
	fmt.Fprintf(&b, "- **Pull request**: #%d\n", pr.ID)
	fmt.Fprintf(&b, "- **Status**: %s\n", pr.Status)
	fmt.Fprintf(&b, "- **Base**: %s\n", pr.BaseRef)
	fmt.Fprintf(&b, "- **Head**: %s\n", pr.HeadRef)
	fmt.Fprintf(&b, "- **Author**: %s\n", author(pr.Author))
	fmt.Fprintf(&b, "- **Created**: %s\n", rfc3339(pr.CreatedAt))
	fmt.Fprintf(&b, "- **Updated**: %s\n", rfc3339(pr.UpdatedAt))
	if pr.Status == store.PRStatusMerged {
		fmt.Fprintf(&b, "- **Merged**: %s (%s)\n", pr.MergeSHA, pr.MergeMethod)
	}
	if pr.ClosedAt != nil {
		fmt.Fprintf(&b, "- **Closed**: %s\n", rfc3339(*pr.ClosedAt))
	}
	body(&b, pr.Body)
	if len(d.Commits) > 0 {
		b.WriteString("\n## Commits\n")
		for _, c := range d.Commits {
			sha := c.SHA
			if len(sha) > 12 {
				sha = sha[:12]
			}
			fmt.Fprintf(&b, "\n- `%s` %s — %s · %s", sha, oneLine(c.Subject), oneLine(c.Author), rfc3339(c.Date))
		}
		b.WriteString("\n")
	}
	if len(d.Comments) > 0 {
		b.WriteString("\n## Comments\n")
		for _, c := range d.Comments {
			if c.Anchored() {
				writeReviewComment(&b, c)
			} else {
				fmt.Fprintf(&b, "\n### Comment by %s · %s%s\n", author(c.Author), rfc3339(c.CreatedAt), moderationSuffix(c.Moderation))
				body(&b, c.Body)
			}
		}
	}
	return b.String()
}

// writeReviewComment renders one anchored comment in the machine-readable
// shape the sketch fixes: a `path:L<anchor>` (side) heading, a
// `path` L<start>-<end> locator, the plain snippet (no line-number
// prefixes — agents miscount them), then the body.
func writeReviewComment(b *strings.Builder, c *store.PRComment) {
	fmt.Fprintf(b, "\n### Review comment on `%s:L%d` (%s) · %s · %s%s\n",
		c.Path, c.Line, c.Side, author(c.Author), rfc3339(c.CreatedAt), moderationSuffix(c.Moderation))
	if c.DiffSnippet != "" {
		lines := strings.Split(strings.TrimSuffix(c.DiffSnippet, "\n"), "\n")
		start := c.Line - 2 // the capture context, clamped at the blob's top
		if start < 1 {
			start = 1
		}
		fmt.Fprintf(b, "\n`%s` L%d-L%d:\n\n", c.Path, start, start+len(lines)-1)
		fmt.Fprintf(b, "```%s\n%s\n```\n", fenceLang(c.Path), strings.Join(lines, "\n"))
	}
	body(b, c.Body)
}

// fenceLang picks a fenced-code language hint from the file extension;
// unknown extensions degrade to a plain fence.
func fenceLang(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".sql":
		return "sql"
	case ".yml", ".yaml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".sh":
		return "bash"
	case ".rs":
		return "rust"
	default:
		return ""
	}
}
