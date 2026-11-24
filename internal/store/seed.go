package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/podhmo/go-spa-sample/internal/auth"
)

type seedComment struct {
	body string
	age  time.Duration
}

type seedIssue struct {
	title    string
	body     string
	status   string
	age      time.Duration
	labels   []string
	comments []seedComment
}

var seedIssues = []seedIssue{
	{
		title:  "Crash when opening an empty project",
		body:   "The app panics with an index-out-of-range error when a project with no files is opened.\n\nSteps to reproduce:\n1. Create a new project\n2. Do not add any files\n3. Press Open",
		status: StatusOpen,
		age:    6*24*time.Hour + 12*time.Hour,
		labels: []string{"Bug"},
		comments: []seedComment{
			{body: "Reproduced on macOS and Linux. Windows seems unaffected.", age: 6*24*time.Hour + 6*time.Hour},
			{body: "Bisected to the changes in the file scanner last week.", age: 6 * 24 * time.Hour},
		},
	},
	{
		title:  "Add dark mode to the settings page",
		body:   "Several users asked for a dark theme. A `prefers-color-scheme` media query should cover most of it without any JavaScript.",
		status: StatusOpen,
		age:    5 * 24 * time.Hour,
		labels: []string{"Feature", "Enhancement"},
		comments: []seedComment{
			{body: "I can pick this up. Proposal: two CSS custom property sets toggled by the media query.", age: 4*24*time.Hour + 18*time.Hour},
			{body: "Sounds good. Please keep the contrast ratio at AA or better.", age: 4*24*time.Hour + 9*time.Hour},
			{body: "Draft palette posted; the dark surface color still needs work.", age: 4 * 24 * time.Hour},
		},
	},
	{
		title:  "Typo in the getting-started guide",
		body:   "\"Recieve\" should be \"Receive\" in the second paragraph.",
		status: StatusClosed,
		age:    4*24*time.Hour + 6*time.Hour,
		labels: []string{"Documentation"},
		comments: []seedComment{
			{body: "Fixed, thanks for the report.", age: 4 * 24 * time.Hour},
		},
	},
	{
		title: "Search slows down with 10,000+ records",
		body: `Search latency grows roughly linearly once the record count crosses about ten thousand. On a mid-range laptop a query that used to return instantly now takes around two seconds.

A quick profile shows almost all of the time is spent in the substring scan rather than in sorting or pagination, so an index alone probably will not fix it.

Possible directions: an FTS5 virtual table, a trigram index, or simply capping the scanned window when a filter already narrows the result set. Measurements for each option are welcome before we pick one.`,
		status: StatusOpen,
		age:    3 * 24 * time.Hour,
		labels: []string{"Bug", "Enhancement"},
		comments: []seedComment{
			{body: "Confirmed. At 50k records the same query takes ~9s.", age: 2*24*time.Hour + 20*time.Hour},
			{body: "FTS5 sounds promising; I will benchmark it against a trigram approach.", age: 2*24*time.Hour + 8*time.Hour},
		},
	},
	{
		title:  "Improve onboarding for first-time users",
		body:   "New users currently land on an empty list with no guidance. A short welcome tour or a set of sample items would help.\n\nIdeas: seed demo issues, add an inline hint on empty states, and link the guide from the header.",
		status: StatusOpen,
		age:    2 * 24 * time.Hour,
		labels: []string{"Feature", "Enhancement", "Documentation"},
		comments: []seedComment{
			{body: "Seeding a few demo issues on first launch seems like the cheapest win.", age: 24 * time.Hour},
		},
	},
	{
		title:  "Export issues as CSV",
		body:   "Needed for monthly reporting. Columns: id, title, status, labels, created_at, updated_at.",
		status: StatusOpen,
		age:    24*time.Hour + 3*time.Hour,
		labels: []string{"Feature"},
	},
	{
		title:  "Update screenshots in the user manual",
		body:   "The manual still shows the old two-column layout. Retake all screenshots with the current UI.",
		status: StatusClosed,
		age:    8 * time.Hour,
		labels: []string{"Documentation"},
		comments: []seedComment{
			{body: "Half of them are done; the list page ones remain.", age: 4 * time.Hour},
			{body: "All screenshots updated and merged.", age: 2 * time.Hour},
		},
	},
}

// SeedMasterData ensures the reference data a usable database needs on top
// of the schema: the default workspace and the fixed discussion
// categories. (The shared master labels are schema data — defaultLabelsDDL
// inserts them during migration.) Idempotent; the server runs this on
// every startup regardless of -seed.
func (s *Store) SeedMasterData(ctx context.Context) error {
	if err := s.EnsureDefaultWorkspace(ctx); err != nil {
		return err
	}
	return s.SeedCategories(ctx)
}

// SeedDemoData inserts demo issues/comments, but only when the issues
// table is empty; the issue part is a no-op otherwise. The demo account is
// seeded independently so a database that already has issues still gets a
// way to log in. The discussion side follows the same rule: categories are
// seeded when their table is empty, demo discussions when theirs is.
// (Labels are not seeded here — the four default master labels are schema
// data, inserted by defaultLabelsDDL on every database.)
//
// This is mock data, not master data: the server only calls it when the
// -seed flag (ZATUDAN_SEED) is set, so a plain start leaves the tables
// empty.
func (s *Store) SeedDemoData(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM issues").Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if err := seedIssuesAndLabels(ctx, tx); err != nil {
			return err
		}
	}

	if err := ensureCategories(ctx, tx); err != nil {
		return err
	}
	var dn int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM discussions").Scan(&dn); err != nil {
		return err
	}
	if dn == 0 {
		if err := seedDiscussions(ctx, tx); err != nil {
			return err
		}
	}

	// The demo account lets people try the login flow out of the box. The
	// argon2id hash is computed at runtime; no credential literal is stored
	// in the source.
	demoHash, err := auth.HashPassword("demo")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users(name, password_hash, created_at)
			SELECT 'demo', ?, ?
			WHERE NOT EXISTS (SELECT 1 FROM users WHERE name = 'demo')`,
		demoHash, time.Now().UTC().Unix()); err != nil {
		return err
	}

	return tx.Commit()
}

// seedIssuesAndLabels inserts the demo issues/comments inside tx,
// resolving label names against the master label rows the schema created.
func seedIssuesAndLabels(ctx context.Context, tx *sql.Tx) error {
	wsID, err := seedWorkspaceID(ctx, tx)
	if err != nil {
		return err
	}
	labelIDs := map[string]int64{}
	rows, err := tx.QueryContext(ctx,
		"SELECT id, name FROM labels WHERE kind = '"+KindLabelIssue+"' AND workspace_id IS NULL AND archived = 0")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		labelIDs[name] = id
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, si := range seedIssues {
		created := now.Add(-si.age).Unix()
		updated := created
		if si.status == StatusClosed {
			updated = now.Add(-si.age + 4*time.Hour).Unix()
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO issues(workspace_id, title, body, status, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)",
			wsID, si.title, si.body, si.status, created, updated)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, name := range si.labels {
			// A master label an operator renamed or deleted is skipped —
			// the demo issue still lands, just without that label.
			lid, ok := labelIDs[name]
			if !ok {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO issue_labels(issue_id, label_id) VALUES(?, ?)", id, lid); err != nil {
				return err
			}
		}
		for _, c := range si.comments {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO comments(issue_id, body, created_at) VALUES(?, ?, ?)",
				id, c.body, now.Add(-c.age).Unix()); err != nil {
				return err
			}
		}
	}
	return nil
}

type seedDComment struct {
	body string
	age  time.Duration
}

type seedDThread struct {
	body     string
	age      time.Duration
	isAnswer bool
	comments []seedDComment
}

type seedDiscussion struct {
	title    string
	body     string
	category string // slug
	age      time.Duration
	threads  []seedDThread
}

var seedCategoryRows = []DiscussionCategory{
	{Name: "Announcements", Slug: "announcements", Description: "Project news and updates.", Format: CategoryFormatOpen},
	{Name: "General", Slug: "general", Description: "Anything about this tracker.", Format: CategoryFormatOpen},
	{Name: "Ideas", Slug: "ideas", Description: "Share ideas for new features.", Format: CategoryFormatOpen},
	{Name: "Q&A", Slug: "qa", Description: "Ask the community — one comment can be marked as the answer.", Format: CategoryFormatQA},
}

var seedDiscussionRows = []seedDiscussion{
	{
		title:    "Welcome to the issue tracker",
		body:     "This project now has a **Discussions** tab alongside Issues.\n\nUse issues for trackable work and discussions for open-ended questions, ideas, and announcements.",
		category: "announcements",
		age:      7 * 24 * time.Hour,
	},
	{
		title:    "How do you use labels versus milestones?",
		body:     "We use labels for triage today. How do other teams separate \"what kind of work\" from \"when it ships\" in a tracker this small?",
		category: "general",
		age:      5 * 24 * time.Hour,
		threads: []seedDThread{
			{body: "Labels for *kind*, milestones for *when*. In this app labels already cover the kind side.", age: 4*24*time.Hour + 12*time.Hour},
			{
				body: "Agreed — I would not add milestones until release planning actually hurts.",
				age:  4 * 24 * time.Hour,
				comments: []seedDComment{
					{body: "Seconded. The closed/open split plus labels has been enough so far.", age: 3*24*time.Hour + 18*time.Hour},
				},
			},
		},
	},
	{
		title:    "Keyboard shortcuts for moving between pages",
		body:     "I keep reaching for `j`/`k` to move between rows. Worth adding, or is the tab order already good enough?",
		category: "ideas",
		age:      3 * 24 * time.Hour,
		threads: []seedDThread{
			{body: "The no-JS constraint makes this awkward, but a `<kbd>` friendly tab order would already help.", age: 2*24*time.Hour + 6*time.Hour},
		},
	},
	{
		title:    "Why is my CSV export empty?",
		body:     "I filtered the issue list and hit export, but the file only contains the header row. What am I doing wrong?",
		category: "qa",
		age:      2 * 24 * time.Hour,
		threads: []seedDThread{
			{body: "Does the filter itself return rows? An empty export usually means the filter matched nothing.", age: 2*24*time.Hour - 4*time.Hour},
			{
				body:     "The export honors the `status` filter too — `status=open` drops closed rows. Try `status=all` in the URL before exporting.",
				age:      24 * time.Hour,
				isAnswer: true,
				comments: []seedDComment{
					{body: "That was it — thank you!", age: 20 * time.Hour},
				},
			},
		},
	},
}

// SeedCategories inserts the fixed discussion categories when the table is
// empty. Categories are reference data with no other source (there is no
// category-create flow), so `init` calls this too — otherwise a schema-only
// database could never accept a discussion. Idempotent.
func (s *Store) SeedCategories(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureCategories(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ensureCategories seeds categories inside tx when the table is empty.
func ensureCategories(ctx context.Context, tx *sql.Tx) error {
	var cn int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM discussion_categories").Scan(&cn); err != nil {
		return err
	}
	if cn == 0 {
		return seedCategories(ctx, tx)
	}
	return nil
}

// seedCategories inserts the fixed discussion categories inside tx (called
// only when the categories table is empty).
func seedCategories(ctx context.Context, tx *sql.Tx) error {
	for _, c := range seedCategoryRows {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO discussion_categories(name, slug, description, format) VALUES(?, ?, ?, ?)",
			c.Name, c.Slug, c.Description, c.Format); err != nil {
			return err
		}
	}
	return nil
}

// seedWorkspaceID picks the workspace demo content lands in — 'default'
// when it exists, the earliest workspace otherwise (a deleted default
// stays deleted, but demo content still needs a home).
func seedWorkspaceID(ctx context.Context, tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM workspaces ORDER BY (slug = 'default') DESC, id LIMIT 1").Scan(&id)
	return id, err
}

// seedDiscussions inserts demo discussions with threaded comments inside tx
// (called only when the discussions table is empty). Category rows are
// resolved by slug so this also works on a database that already had
// categories.
func seedDiscussions(ctx context.Context, tx *sql.Tx) error {
	wsID, err := seedWorkspaceID(ctx, tx)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, slug FROM discussion_categories")
	if err != nil {
		return err
	}
	catIDs := map[string]int64{}
	for rows.Next() {
		var id int64
		var slug string
		if err := rows.Scan(&id, &slug); err != nil {
			rows.Close()
			return err
		}
		catIDs[slug] = id
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, sd := range seedDiscussionRows {
		created := now.Add(-sd.age).Unix()
		res, err := tx.ExecContext(ctx,
			"INSERT INTO discussions(workspace_id, category_id, title, body, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)",
			wsID, catIDs[sd.category], sd.title, sd.body, created, created)
		if err != nil {
			return err
		}
		did, err := res.LastInsertId()
		if err != nil {
			return err
		}
		updated := created
		insertThread := func(t seedDThread) (int64, error) {
			res, err := tx.ExecContext(ctx,
				"INSERT INTO discussion_threads(discussion_id, is_answer, body, created_at) VALUES(?, ?, ?, ?)",
				did, t.isAnswer, t.body, now.Add(-t.age).Unix())
			if err != nil {
				return 0, err
			}
			if ts := now.Add(-t.age).Unix(); ts > updated {
				updated = ts
			}
			return res.LastInsertId()
		}
		insertComment := func(c seedDComment, threadID int64) error {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO discussion_comments(thread_id, body, created_at) VALUES(?, ?, ?)",
				threadID, c.body, now.Add(-c.age).Unix()); err != nil {
				return err
			}
			if ts := now.Add(-c.age).Unix(); ts > updated {
				updated = ts
			}
			return nil
		}
		for _, t := range sd.threads {
			tid, err := insertThread(t)
			if err != nil {
				return err
			}
			for _, rc := range t.comments {
				if err := insertComment(rc, tid); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE discussions SET updated_at = ? WHERE id = ?", updated, did); err != nil {
			return err
		}
	}
	return nil
}
