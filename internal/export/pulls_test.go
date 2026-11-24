package export

import (
	"strings"
	"testing"
	"time"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// PullRequestMarkdown: metadata block, commit list, plain comments and
// review comments in the sketch's `path:L<line>` format with an
// unnumbered snippet fence.
func TestPullRequestMarkdown(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pr := &store.PullRequest{
		ID: 7, Title: "add b", Body: "body text",
		Status: store.PRStatusOpen, BaseRef: "refs/heads/main", HeadRef: "refs/heads/feat/x",
		CreatedAt: now, UpdatedAt: now,
		Author: &store.User{Name: "alice"},
	}
	md := PullRequestMarkdown(PRMarkdownData{
		PR: pr,
		Commits: []PRCommit{
			{SHA: "abc1234567890def", Subject: "add b", Author: "alice", Date: now},
		},
		Comments: []*store.PRComment{
			{ID: 1, Body: "lgtm", CreatedAt: now, Author: &store.User{Name: "bob"}},
			{ID: 2, Body: "fix this", Path: "b.txt", Line: 3, Side: "right",
				AnchorSHA: "abc", DiffSnippet: "two\nthree\nfour", CreatedAt: now,
				Author: &store.User{Name: "carol"}},
		},
	})
	for _, want := range []string{
		"# add b", "**Pull request**: #7", "**Status**: open",
		"### Comment by @bob", "### Review comment on `b.txt:L3`",
		"```\ntwo\nthree\nfour\n```",
		"`abc123456789` add b — alice",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	// No line-number prefixes inside the snippet fence — agents miscount.
	if strings.Contains(md, "3: two") || strings.Contains(md, "3 two") {
		t.Fatalf("snippet must not carry line numbers:\n%s", md)
	}
}
