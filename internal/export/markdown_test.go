package export

import (
	"testing"
	"time"

	"github.com/podhmo/go-spa-sample/internal/store"
)

func TestIssueMarkdown(t *testing.T) {
	at := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	i := &store.Issue{
		ID: 3, Title: "Layout broken\non mobile", Body: "Steps:\n\n1. open\n2. look",
		Status: store.StatusOpen, CreatedAt: at, UpdatedAt: at.Add(time.Hour),
		Labels: []store.Label{{Name: "Bug"}, {Name: "Feature"}},
		Author: &store.User{Name: "alice"},
	}
	comments := []*store.Comment{
		{ID: 1, Body: "confirmed", CreatedAt: at.Add(2 * time.Hour), Author: &store.User{Name: "bob"}},
		{ID: 2, Body: "no repro", CreatedAt: at.Add(3 * time.Hour)},
	}
	got := IssueMarkdown(i, comments, "default")
	want := `# Layout broken on mobile

- **Issue**: #3
- **Workspace**: default
- **Status**: open
- **Author**: @alice
- **Labels**: Bug, Feature
- **Created**: 2026-11-18T10:00:00Z
- **Updated**: 2026-11-18T11:00:00Z

Steps:

1. open
2. look

## Comments

### Comment by @bob · 2026-11-18T12:00:00Z

confirmed

### Comment by anonymous · 2026-11-18T13:00:00Z

no repro
`
	if got != want {
		t.Errorf("IssueMarkdown mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestIssueMarkdownMinimal(t *testing.T) {
	i := &store.Issue{ID: 1, Title: "x", Status: store.StatusClosed}
	got := IssueMarkdown(i, nil, "default")
	want := `# x

- **Issue**: #1
- **Workspace**: default
- **Status**: closed
- **Author**: anonymous
- **Created**: 0001-01-01T00:00:00Z
- **Updated**: 0001-01-01T00:00:00Z
`
	if got != want {
		t.Errorf("IssueMarkdown mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDiscussionMarkdown(t *testing.T) {
	at := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	d := &store.Discussion{
		ID: 2, Title: "How to X?", Body: "context",
		Category:  store.DiscussionCategory{Name: "Q&A", Format: store.CategoryFormatQA},
		Answered:  true,
		CreatedAt: at, UpdatedAt: at.Add(time.Hour),
		Author: &store.User{Name: "alice"},
	}
	threads := []*store.DiscussionThread{
		{ID: 1, Body: "try Y", IsAnswer: true, CreatedAt: at.Add(time.Hour),
			Author: &store.User{Name: "bob"},
			Comments: []*store.DiscussionComment{
				{ID: 9, Body: "worked!", CreatedAt: at.Add(2 * time.Hour), Author: &store.User{Name: "alice"}},
			}},
		{ID: 2, Body: "or Z", CreatedAt: at.Add(3 * time.Hour)},
	}
	got := DiscussionMarkdown(d, threads, "default")
	want := `# How to X?

- **Discussion**: #2
- **Workspace**: default
- **Category**: Q&A
- **Author**: @alice
- **Answered**: yes
- **Created**: 2026-11-18T10:00:00Z
- **Updated**: 2026-11-18T11:00:00Z

context

## Threads

### Thread by @bob · 2026-11-18T11:00:00Z · answer

try Y

#### Reply by @alice · 2026-11-18T12:00:00Z

worked!

### Thread by anonymous · 2026-11-18T13:00:00Z

or Z
`
	if got != want {
		t.Errorf("DiscussionMarkdown mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDiscussionMarkdownOpenCategoryHasNoAnsweredLine(t *testing.T) {
	d := &store.Discussion{
		ID: 1, Title: "idea",
		Category: store.DiscussionCategory{Name: "Ideas", Format: store.CategoryFormatOpen},
	}
	got := DiscussionMarkdown(d, nil, "default")
	want := `# idea

- **Discussion**: #1
- **Workspace**: default
- **Category**: Ideas
- **Author**: anonymous
- **Created**: 0001-01-01T00:00:00Z
- **Updated**: 0001-01-01T00:00:00Z
`
	if got != want {
		t.Errorf("DiscussionMarkdown mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
