// Output helpers: --json field selection, views shared by commands, and
// the -w/--web browser opener.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/podhmo/go-spa-sample/internal/api"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// fmtTimeShort renders a timestamp for table output.
func fmtTimeShort(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// authorName renders a post's author; nil is an anonymous post.
func authorName(u *store.User) string {
	if u == nil {
		return "anonymous"
	}
	return u.Name
}

// --json output uses the api package's wire field names so `zatu issue
// list --json` and `zatu api /issues` speak the same JSON vocabulary.

func issueViewJSON(i *store.Issue) api.Issue {
	out := api.Issue{
		ID: i.ID, Title: i.Title, Body: i.Body, Status: i.Status,
		Author:    userView(i.Author),
		CreatedAt: i.CreatedAt.Format(time.RFC3339), UpdatedAt: i.UpdatedAt.Format(time.RFC3339),
		Labels: []api.Label{},
	}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, labelView(l))
	}
	return out
}

func issueViews(issues []*store.Issue) []api.Issue {
	out := make([]api.Issue, 0, len(issues))
	for _, i := range issues {
		out = append(out, issueViewJSON(i))
	}
	return out
}

func commentViews(cs []*store.Comment) []api.Comment {
	out := make([]api.Comment, 0, len(cs))
	for _, c := range cs {
		out = append(out, api.Comment{
			ID: c.ID, Body: c.Body, Author: userView(c.Author),
			CreatedAt:  c.CreatedAt.Format(time.RFC3339),
			Moderation: moderationView(c.Moderation),
		})
	}
	return out
}

// moderationView maps a store moderation triple onto the api wire
// shape so --json output matches /api/v1 verbatim.
func moderationView(m store.Moderation) api.Moderation {
	out := api.Moderation{Hidden: m.Hidden, HiddenReason: m.HiddenReason}
	if m.Edited() {
		s := m.EditedAt.Format(time.RFC3339)
		out.EditedAt = &s
	}
	return out
}

// moderationMarks renders the trailing "[edited]"/"[hidden: reason]"
// markers on a post's header line in text output.
func moderationMarks(m store.Moderation) string {
	var s string
	if m.Edited() {
		s += " [edited]"
	}
	if m.Hidden {
		s += " [hidden: " + m.HiddenReason + "]"
	}
	return s
}

func labelView(l store.Label) api.Label {
	out := api.Label{ID: l.ID, Name: l.Name, Color: l.Color, Kind: l.Kind, Archived: l.Archived}
	if l.WorkspaceID != 0 {
		wsID := l.WorkspaceID
		out.WorkspaceID = &wsID
	}
	return out
}

func labelViews(labels []*store.Label) []api.Label {
	out := make([]api.Label, 0, len(labels))
	for _, l := range labels {
		out = append(out, labelView(*l))
	}
	return out
}

func categoryView(c store.DiscussionCategory) api.DiscussionCategory {
	return api.DiscussionCategory{
		ID: c.ID, Name: c.Name, Slug: c.Slug,
		Description: c.Description, Format: c.Format,
	}
}

func categoryViews(cats []*store.DiscussionCategory) []api.DiscussionCategory {
	out := make([]api.DiscussionCategory, 0, len(cats))
	for _, c := range cats {
		out = append(out, categoryView(*c))
	}
	return out
}

func discussionViewJSON(d *store.Discussion) api.Discussion {
	out := api.Discussion{
		ID: d.ID, Title: d.Title, Body: d.Body,
		Category:  categoryView(d.Category),
		Labels:    []api.Label{},
		Author:    userView(d.Author),
		Answered:  d.Answered,
		CreatedAt: d.CreatedAt.Format(time.RFC3339), UpdatedAt: d.UpdatedAt.Format(time.RFC3339),
	}
	for _, l := range d.Labels {
		out.Labels = append(out.Labels, labelView(l))
	}
	return out
}

func discussionViews(ds []*store.Discussion) []api.Discussion {
	out := make([]api.Discussion, 0, len(ds))
	for _, d := range ds {
		out = append(out, discussionViewJSON(d))
	}
	return out
}

func discussionThreadViews(ts []*store.DiscussionThread) []api.DiscussionThread {
	out := make([]api.DiscussionThread, 0, len(ts))
	for _, t := range ts {
		out = append(out, discussionThreadView(t))
	}
	return out
}

func discussionThreadView(t *store.DiscussionThread) api.DiscussionThread {
	out := api.DiscussionThread{
		ID: t.ID, IsAnswer: t.IsAnswer, Body: t.Body,
		Author:     userView(t.Author),
		CreatedAt:  t.CreatedAt.Format(time.RFC3339),
		Moderation: moderationView(t.Moderation),
	}
	for _, c := range t.Comments {
		out.Comments = append(out.Comments, discussionCommentView(c))
	}
	return out
}

func discussionCommentView(c *store.DiscussionComment) api.DiscussionReply {
	return api.DiscussionReply{
		ID: c.ID, Body: c.Body,
		Author:     userView(c.Author),
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		Moderation: moderationView(c.Moderation),
	}
}

func userView(u *store.User) *api.User {
	if u == nil {
		return nil
	}
	return &api.User{ID: u.ID, Name: u.Name}
}

// apiTokenViews is the --json shape of `auth token list`. The id is the
// stored hash — the plaintext is never persisted, so there is nothing
// more sensitive to emit.
func apiTokenViews(tokens []*store.APIToken) []map[string]any {
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, map[string]any{
			"id":         t.ID,
			"user":       userView(t.User),
			"created_at": t.CreatedAt.Format(time.RFC3339),
		})
	}
	return out
}

// writeJSONFields writes v as JSON restricted to the comma-separated top-
// level field names, like gh's `--json field1,field2`. For a slice input
// the selection applies to each element.
func writeJSONFields(w io.Writer, v any, fields string) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, f := range strings.Split(fields, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			want[f] = true
		}
	}
	if len(want) == 0 {
		return usageError{msg: "--json requires at least one field name"}
	}
	pick := func(m map[string]any) map[string]any {
		out := map[string]any{}
		for k, val := range m {
			if want[k] {
				out[k] = val
			}
		}
		return out
	}
	var out any
	switch g := generic.(type) {
	case []any:
		list := make([]any, 0, len(g))
		for _, e := range g {
			if m, ok := e.(map[string]any); ok {
				list = append(list, pick(m))
			} else {
				list = append(list, e)
			}
		}
		out = list
	case map[string]any:
		out = pick(g)
	default:
		out = generic
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// decodeJSON re-marshals v through JSON so --template sees the same key
// names as --json output (api DTO field names, snake_case).
func decodeJSON(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return generic, nil
}

// writeTemplate renders v through a Go text/template, like gh's
// --template. The template sees the decoded JSON shape (maps and slices),
// so `{{.title}}` and `{{range .}}{{.id}} {{end}}` both work.
func writeTemplate(w io.Writer, v any, tmpl string) error {
	generic, err := decodeJSON(v)
	if err != nil {
		return err
	}
	t, err := template.New("output").Parse(tmpl)
	if err != nil {
		return usageError{msg: fmt.Sprintf("invalid --template: %v", err)}
	}
	return t.Execute(w, generic)
}

// colorEnabled reports whether stdout wants ANSI color: it must be a
// character device (terminal) and NO_COLOR must be unset
// (https://no-color.org). Non-TTY consumers (pipes, files, tests) always
// get plain output.
func (o Options) colorEnabled() bool {
	if o.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := o.Stdout.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// paint wraps s in an ANSI color when color output is enabled.
func (o Options) paint(code, s string) string {
	if s == "" || !o.colorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// statusText renders an issue status with gh-ish color hints: open
// green, closed red.
func (o Options) statusText(status string) string {
	switch status {
	case store.StatusOpen:
		return o.paint("32", status)
	case store.StatusClosed:
		return o.paint("31", status)
	}
	return status
}

// answeredMark renders the discussion answer marker, green when colored.
func (o Options) answeredMark(answered bool) string {
	if !answered {
		return ""
	}
	return o.paint("32", "answered")
}

func closeQuietly(c io.Closer) {
	if c != nil {
		c.Close()
	}
}

// openBrowser asks the OS to open url. Linux uses xdg-open, macOS `open`,
// Windows rundll32 — the same trio every stdlib-only CLI ships.
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	if err := exec.Command(name, args...).Start(); err != nil {
		return fmt.Errorf("open browser (%s): %w", name, err)
	}
	return nil
}
