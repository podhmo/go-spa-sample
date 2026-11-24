package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// seedCategories gives a fresh DB the standard category rows (and the
// rest of the demo data — SeedDemoData is all-or-nothing).
func seedCategories(t *testing.T, dbPath string) {
	t.Helper()
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SeedDemoData(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// createDiscussion runs `discussion create` and returns the new id.
func createDiscussion(t *testing.T, o Options, title, category string) int64 {
	t.Helper()
	code, out, errOut := run(t, o, "discussion", "create",
		"--title", title, "--category", category, "--body", "body of "+title)
	var id int64
	if code != ExitOK {
		t.Fatalf("discussion create: exit %d\n%s", code, errOut)
	}
	if _, err := fmt.Sscanf(out, "created discussion #%d", &id); err != nil || id == 0 {
		t.Fatalf("create output: %q", out)
	}
	return id
}

// createDThread runs `discussion comment create` and returns the new
// thread id (a top-level post is a thread).
func createDThread(t *testing.T, o Options, id int64, body string) int64 {
	t.Helper()
	code, out, _ := run(t, o, "discussion", "comment", "create", itoa(id), "--body", body)
	var cid int64
	if code != ExitOK {
		t.Fatalf("comment create: exit %d\n%s", code, out)
	}
	if _, err := fmt.Sscanf(out, "created thread #%d", &cid); err != nil || cid == 0 {
		t.Fatalf("comment output: %q", out)
	}
	return cid
}

func TestDiscussionLifecycleLocal(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedCategories(t, db)
	o := localOptions(t, db)

	id := createDiscussion(t, o, "cli question", "qa")

	code, out, _ := run(t, o, "discussion", "list", "--search", "cli question")
	if code != ExitOK || !strings.Contains(out, "cli question") || !strings.Contains(out, "qa") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "discussion", "view", itoa(id))
	if code != ExitOK || !strings.Contains(out, "title:\tcli question") ||
		!strings.Contains(out, "category:\tQ&A (qa)") ||
		!strings.Contains(out, "answered:\tno") {
		t.Fatalf("view: exit %d\n%s", code, out)
	}

	cid := createDThread(t, o, id, "the answer")
	code, out, _ = run(t, o, "discussion", "comment", "create", itoa(id),
		"--thread", itoa(cid), "--body", "a reply")
	if code != ExitOK || !strings.Contains(out, "created reply #") {
		t.Fatalf("reply create: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "discussion", "view", itoa(id), "--comments")
	if code != ExitOK || !strings.Contains(out, "the answer") || !strings.Contains(out, "a reply") {
		t.Fatalf("view --comments: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "discussion", "answer", itoa(id), "--thread", itoa(cid))
	if code != ExitOK || !strings.Contains(out, "marked thread #") {
		t.Fatalf("answer: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "view", itoa(id))
	if !strings.Contains(out, "answered:\tyes") {
		t.Fatalf("view after answer:\n%s", out)
	}
	code, out, _ = run(t, o, "discussion", "list", "--answered", "yes", "--search", "cli question")
	if code != ExitOK || !strings.Contains(out, "cli question") {
		t.Fatalf("list --answered yes: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "answer", itoa(id), "--thread", itoa(cid), "--unmark")
	if code != ExitOK || !strings.Contains(out, "unmarked") {
		t.Fatalf("unmark: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "view", itoa(id))
	if !strings.Contains(out, "answered:\tno") {
		t.Fatalf("view after unmark:\n%s", out)
	}
}

// --markdown prints the same document the /markdown endpoints serve,
// threads and replies included.
func TestDiscussionViewMarkdown(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedCategories(t, db)
	o := localOptions(t, db)

	id := createDiscussion(t, o, "cli question", "qa")
	cid := createDThread(t, o, id, "the answer")
	run(t, o, "discussion", "comment", "create", itoa(id), "--thread", itoa(cid), "--body", "a reply")
	run(t, o, "discussion", "answer", itoa(id), "--thread", itoa(cid))

	code, out, _ := run(t, o, "discussion", "view", itoa(id), "--markdown")
	if code != ExitOK {
		t.Fatalf("view --markdown: exit %d", code)
	}
	for _, want := range []string{
		"# cli question\n", "- **Category**: Q&A\n", "- **Answered**: yes\n",
		"## Threads", "· answer\n", "the answer", "#### Reply by", "a reply",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--markdown missing %q:\n%s", want, out)
		}
	}

	code, _, _ = run(t, o, "discussion", "view", itoa(id), "--markdown", "--template", "{{.title}}")
	if code != ExitUsage {
		t.Fatalf("--markdown --template: exit %d, want %d", code, ExitUsage)
	}
	code, _, _ = run(t, o, "discussion", "view", itoa(id), "--markdown", "--comments")
	if code != ExitUsage {
		t.Fatalf("--markdown --comments: exit %d, want %d", code, ExitUsage)
	}
}

func TestDiscussionEditDeleteAndJSON(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedCategories(t, db)
	o := localOptions(t, db)

	id := createDiscussion(t, o, "to rename", "general")

	code, _, _ := run(t, o, "discussion", "edit", itoa(id), "--title", "renamed", "--category", "ideas")
	if code != ExitOK {
		t.Fatalf("edit: exit %d", code)
	}
	code, out, _ := run(t, o, "discussion", "view", itoa(id))
	if code != ExitOK || !strings.Contains(out, "title:\trenamed") ||
		!strings.Contains(out, "category:\tIdeas (ideas)") {
		t.Fatalf("view after edit: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "discussion", "list", "--category", "ideas", "--json", "id,title,category")
	if code != ExitOK || !strings.Contains(out, `"title": "renamed"`) || !strings.Contains(out, `"slug": "ideas"`) {
		t.Fatalf("list --json: exit %d\n%s", code, out)
	}

	code, out, _ = run(t, o, "discussion", "delete", itoa(id))
	if code != ExitOK || !strings.Contains(out, "deleted discussion #") {
		t.Fatalf("delete: exit %d\n%s", code, out)
	}
	code, _, _ = run(t, o, "discussion", "view", itoa(id))
	if code != ExitError {
		t.Fatalf("view after delete: exit %d, want %d", code, ExitError)
	}
}

// `discussion categories` enumerates the slugs --category takes. The
// rows are global master data, so the verb opens the backend unscoped —
// --workspace is ignored like the workspace commands.
func TestDiscussionCategories(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedCategories(t, db)
	o := localOptions(t, db)

	code, out, _ := run(t, o, "discussion", "categories")
	if code != ExitOK {
		t.Fatalf("categories: exit %d", code)
	}
	for _, want := range []string{"announcements", "general", "ideas", "qa"} {
		if !strings.Contains(out, want) {
			t.Fatalf("categories missing %q:\n%s", want, out)
		}
	}
	// rows are slug\tname\tformat\tdescription
	if !strings.Contains(out, "qa\tQ&A\tqa\t") {
		t.Fatalf("categories row shape:\n%s", out)
	}

	code, out, _ = run(t, o, "discussion", "categories", "--json", "name,slug")
	if code != ExitOK || !strings.Contains(out, `"slug": "qa"`) || !strings.Contains(out, `"name": "Ideas"`) {
		t.Fatalf("categories --json: exit %d\n%s", code, out)
	}
	if strings.Contains(out, `"description"`) {
		t.Fatalf("--json leaked unrequested field:\n%s", out)
	}

	code, out, _ = run(t, o, "discussion", "categories", "--workspace", "nope")
	if code != ExitOK || !strings.Contains(out, "qa") {
		t.Fatalf("categories --workspace nope: exit %d\n%s", code, out)
	}
}

func TestDiscussionUsageAndErrors(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedCategories(t, db)
	o := localOptions(t, db)

	for _, args := range [][]string{
		{"discussion", "view"},                                             // missing number
		{"discussion", "view", "abc"},                                      // bad number
		{"discussion", "create", "--title", "x"},                           // missing --category
		{"discussion", "create", "--category", "qa"},                       // missing --title
		{"discussion", "edit", "1"},                                        // nothing to update
		{"discussion", "comment"},                                          // missing action
		{"discussion", "answer", "1"},                                      // missing --thread
		{"discussion", "view", "1", "-w"},                                  // -w without --server
		{"discussion", "create", "--title", "x", "--category", "qa", "-w"}, // -w without --server
	} {
		code, _, _ := run(t, o, args...)
		if code != ExitUsage {
			t.Fatalf("args %v: want exit %d, got %d", args, ExitUsage, code)
		}
	}

	code, _, errOut := run(t, o, "discussion", "create",
		"--title", "x", "--category", "nope")
	if code != ExitError || !strings.Contains(errOut, "qa") {
		t.Fatalf("unknown category: exit %d\n%s", code, errOut)
	}

	id := createDiscussion(t, o, "not qa", "general")
	cid := createDThread(t, o, id, "c")
	code, _, _ = run(t, o, "discussion", "answer", itoa(id), "--thread", itoa(cid))
	if code != ExitError {
		t.Fatalf("answer on open category: exit %d, want %d", code, ExitError)
	}
}

// Thread-vs-reply branching on the moderation verbs: --thread alone
// targets the opening post, --thread + --comment targets the reply.
func TestDiscussionCommentModerationLocal(t *testing.T) {
	db := t.TempDir() + "/test.db"
	seedCategories(t, db)
	o := localOptions(t, db)

	id := createDiscussion(t, o, "mod question", "qa")
	tid := createDThread(t, o, id, "opening")
	var rid int64
	code, out, _ := run(t, o, "discussion", "comment", "create", itoa(id),
		"--thread", itoa(tid), "--body", "a reply")
	if code != ExitOK {
		t.Fatalf("reply create: exit %d", code)
	}
	if _, err := fmt.Sscanf(out, "created reply #%d", &rid); err != nil || rid == 0 {
		t.Fatalf("reply output: %q", out)
	}

	// Non-positive ids are usage errors, not backend 404s.
	for _, args := range [][]string{
		{"discussion", "comment", "edit", itoa(id), "--thread", itoa(tid), "--comment", "-3", "--body", "x"},
		{"discussion", "comment", "create", itoa(id), "--thread", "-1", "--body", "x"},
		{"discussion", "comment", "hide", itoa(id), "--thread", itoa(tid), "--reason", "bogus"},
		{"discussion", "comment", "unhide", itoa(id), "--thread", itoa(tid), "--reason", "spam"},
	} {
		if code, _, _ := run(t, o, args...); code != ExitUsage {
			t.Fatalf("args %v: want exit %d, got %d", args, ExitUsage, code)
		}
	}

	// Reply edit + hide + delete.
	code, out, _ = run(t, o, "discussion", "comment", "edit", itoa(id),
		"--thread", itoa(tid), "--comment", itoa(rid), "--body", "better reply")
	if code != ExitOK || !strings.Contains(out, "updated reply #") {
		t.Fatalf("reply edit: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "comment", "hide", itoa(id),
		"--thread", itoa(tid), "--comment", itoa(rid), "--reason", "off-topic")
	if code != ExitOK || !strings.Contains(out, "hidden reply #") {
		t.Fatalf("reply hide: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "view", itoa(id), "--comments")
	if code != ExitOK || !strings.Contains(out, "[hidden: off-topic]") || !strings.Contains(out, "[edited]") {
		t.Fatalf("view after reply moderation:\n%s", out)
	}
	code, out, _ = run(t, o, "discussion", "comment", "delete", itoa(id),
		"--thread", itoa(tid), "--comment", itoa(rid))
	if code != ExitOK || !strings.Contains(out, "deleted reply #") {
		t.Fatalf("reply delete: exit %d\n%s", code, out)
	}

	// Thread edit + hide + unhide + delete (no --comment → the thread).
	code, out, _ = run(t, o, "discussion", "comment", "edit", itoa(id),
		"--thread", itoa(tid), "--body", "edited opening")
	if code != ExitOK || !strings.Contains(out, "updated thread #") {
		t.Fatalf("thread edit: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "comment", "hide", itoa(id),
		"--thread", itoa(tid), "--reason", "duplicate")
	if code != ExitOK || !strings.Contains(out, "hidden thread #") {
		t.Fatalf("thread hide: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "comment", "unhide", itoa(id), "--thread", itoa(tid))
	if code != ExitOK || !strings.Contains(out, "unhidden thread #") {
		t.Fatalf("thread unhide: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "discussion", "comment", "delete", itoa(id), "--thread", itoa(tid))
	if code != ExitOK || !strings.Contains(out, "deleted thread #") {
		t.Fatalf("thread delete: exit %d\n%s", code, out)
	}
}
