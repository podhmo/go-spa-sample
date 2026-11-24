// Package store provides SQLite-backed persistence for the issue tracker.
//
// This file defines the API contract shared by the data layer and the web
// layer.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// Store wraps the SQLite database handle.
type Store struct {
	db *sql.DB
}

// Timestamps are stored as UTC unix seconds.
// schemaDDL is the complete current schema, built in one shot: the app is
// pre-release and databases are disposable, so there is no released
// upgrade path to preserve. Schema work edits this definition directly;
// upgrading old files becomes a concern only after a real release.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS workspaces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
-- The init-time default workspace (see EnsureDefaultWorkspace for the
-- later check that resurrects it when the table is empty).
INSERT INTO workspaces(slug, name, description, created_at)
	SELECT 'default', 'default', 'Default workspace.', CAST(strftime('%s','now') AS INTEGER)
	WHERE NOT EXISTS (SELECT 1 FROM workspaces);
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS api_tokens (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id);
CREATE TABLE IF NOT EXISTS issues (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'closed')),
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	author_id INTEGER REFERENCES users(id)
);
-- The comment-moderation columns (edited_at/hidden_at/hidden_reason)
-- are shared by comments, discussion_threads and discussion_comments;
-- hidden_reason holds one of the store.HideReason* values.
CREATE TABLE IF NOT EXISTS comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	author_id INTEGER REFERENCES users(id),
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	edited_at INTEGER,
	hidden_at INTEGER,
	hidden_reason TEXT CHECK(hidden_reason IN ('spam','abuse','off-topic','outdated','duplicate','resolved','low-quality'))
);
-- Labels are managed data, not constants: workspace_id NULL marks a
-- shared master row, a set one scopes the label to that workspace. kind
-- keeps issue and discussion labels as separate pools; archived hides a
-- label from pickers without unlinking it from existing issues.
CREATE TABLE IF NOT EXISTS labels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
	kind TEXT NOT NULL CHECK(kind IN ('issue', 'discussion')),
	name TEXT NOT NULL,
	color TEXT NOT NULL,
	archived INTEGER NOT NULL DEFAULT 0
);
` + defaultLabelsDDL + `
CREATE TABLE IF NOT EXISTS issue_labels (
	issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
	PRIMARY KEY (issue_id, label_id)
);
CREATE TABLE IF NOT EXISTS discussion_labels (
	discussion_id INTEGER NOT NULL REFERENCES discussions(id) ON DELETE CASCADE,
	label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
	PRIMARY KEY (discussion_id, label_id)
);
CREATE TABLE IF NOT EXISTS discussion_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	slug TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	format TEXT NOT NULL DEFAULT 'open' CHECK(format IN ('open', 'qa'))
);
CREATE TABLE IF NOT EXISTS discussions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	category_id INTEGER NOT NULL REFERENCES discussion_categories(id),
	author_id INTEGER REFERENCES users(id),
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS discussion_threads (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	discussion_id INTEGER NOT NULL REFERENCES discussions(id) ON DELETE CASCADE,
	author_id INTEGER REFERENCES users(id),
	is_answer INTEGER NOT NULL DEFAULT 0,
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	edited_at INTEGER,
	hidden_at INTEGER,
	hidden_reason TEXT CHECK(hidden_reason IN ('spam','abuse','off-topic','outdated','duplicate','resolved','low-quality'))
);
CREATE TABLE IF NOT EXISTS discussion_comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	thread_id INTEGER NOT NULL REFERENCES discussion_threads(id) ON DELETE CASCADE,
	author_id INTEGER REFERENCES users(id),
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	edited_at INTEGER,
	hidden_at INTEGER,
	hidden_reason TEXT CHECK(hidden_reason IN ('spam','abuse','off-topic','outdated','duplicate','resolved','low-quality'))
);
` + pullsDDL + changesDDL + `
CREATE INDEX IF NOT EXISTS idx_issues_workspace_status_created ON issues(workspace_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_comments_issue_created ON comments(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_issue_labels_label ON issue_labels(label_id);
CREATE INDEX IF NOT EXISTS idx_discussion_labels_label ON discussion_labels(label_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_kind_name_ws ON labels(kind, name, COALESCE(workspace_id, 0));
CREATE UNIQUE INDEX IF NOT EXISTS idx_dthreads_answer ON discussion_threads(discussion_id) WHERE is_answer = 1;
CREATE INDEX IF NOT EXISTS idx_dthreads_discussion_created ON discussion_threads(discussion_id, created_at);
CREATE INDEX IF NOT EXISTS idx_dcomments_thread_created ON discussion_comments(thread_id, created_at);
CREATE INDEX IF NOT EXISTS idx_discussions_workspace_created ON discussions(workspace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_discussions_workspace_category_created ON discussions(workspace_id, category_id, created_at);
`

// changesDDL is kept out of line so migrateV2 can apply it alone to
// databases stamped user_version=1 before the table existed — schemaDDL
// concatenates it so fresh v1 databases get the identical definition.
const changesDDL = `
-- Cross-process change hints (commitTx writes, WatchChanges tails → SSE
-- hub). Volatile by design; PurgeChanges bounds the table.
CREATE TABLE IF NOT EXISTS changes (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	topic TEXT NOT NULL,
	kind TEXT NOT NULL,
	target INTEGER NOT NULL,
	parent INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
`

// defaultLabelsDDL seeds the shared master labels every database needs.
// It is part of both the init schema and migrateV3: INSERT ... WHERE NOT
// EXISTS makes replaying it on a database that already has the row a
// no-op.
const defaultLabelsDDL = `
INSERT INTO labels(workspace_id, kind, name, color)
	SELECT NULL, 'issue', 'Bug', '#d73a4a'
	WHERE NOT EXISTS (SELECT 1 FROM labels WHERE kind = 'issue' AND name = 'Bug' AND workspace_id IS NULL);
INSERT INTO labels(workspace_id, kind, name, color)
	SELECT NULL, 'issue', 'Documentation', '#1f6feb'
	WHERE NOT EXISTS (SELECT 1 FROM labels WHERE kind = 'issue' AND name = 'Documentation' AND workspace_id IS NULL);
INSERT INTO labels(workspace_id, kind, name, color)
	SELECT NULL, 'issue', 'Enhancement', '#a2eeef'
	WHERE NOT EXISTS (SELECT 1 FROM labels WHERE kind = 'issue' AND name = 'Enhancement' AND workspace_id IS NULL);
INSERT INTO labels(workspace_id, kind, name, color)
	SELECT NULL, 'issue', 'Feature', '#0075ca'
	WHERE NOT EXISTS (SELECT 1 FROM labels WHERE kind = 'issue' AND name = 'Feature' AND workspace_id IS NULL);
`

// migrations is the ordered list of schema upgrades: migrations[i] moves a
// database from PRAGMA user_version i to i+1.
var migrations = []func(ctx context.Context, tx *sql.Tx) error{
	migrateV1, // current schema in one shot
	migrateV2, // changes table — reaches databases already stamped v1
	migrateV3, // labels gain workspace_id/kind/archived + discussion_labels
	migrateV4, // comment-moderation columns on the three post tables
	migrateV5, // pull_requests + pr_comments
}

// schemaVersion is the user_version a fully migrated database reports.
var schemaVersion = len(migrations)

func migrateV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, schemaDDL); err != nil {
		return err
	}
	// A database created before the schema was squashed already has an
	// issues table that CREATE IF NOT EXISTS leaves untouched; its missing
	// columns would surface later as confusing query errors, so fail fast
	// (pre-release databases are disposable).
	var n int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('issues') WHERE name = 'workspace_id'").Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("store: database schema predates workspaces — delete the file and re-run init")
	}
	return nil
}

// migrateV2 adds the changes table to a database that passed the squashed
// v1 before the table was introduced (migrateV1 is skipped for those, so
// the table would never appear and every commitTx insert would fail).
func migrateV2(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, changesDDL)
	return err
}

// migrate applies every schema version the database is missing, stamping
// PRAGMA user_version after each. Each version runs in its own
// transaction, so a failed migration leaves the database at the last good
// version and can be retried on the next Open. A database newer than
// schemaVersion is rejected rather than silently downgraded.
func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("store: database schema version %d is newer than supported %d", version, schemaVersion)
	}
	for ; version < schemaVersion; version++ {
		if rebuildNeedsFKOff[version] {
			if err := migrateWithFKOff(ctx, db, version); err != nil {
				return err
			}
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := migrations[version](ctx, tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migrate schema to version %d: %w", version+1, err)
		}
		// PRAGMA does not accept bound parameters.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: stamp schema version %d: %w", version+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// migrateV3 turns the constant-era labels table into managed rows: it
// gains workspace_id (NULL = shared master), kind (issue|discussion) and
// an archived flag, and its UNIQUE(name) constraint is replaced by
// uniqueness on (kind, name, workspace) so a workspace label may reuse a
// master name. The old rows survive as master issue labels; the four
// former constant labels become master rows via defaultLabelsDDL.
//
// rebuildNeedsFKOff marks this migration for migrate(): DROP TABLE labels
// fires issue_labels' ON DELETE CASCADE even inside a transaction, which
// would wipe the existing label links. The connection's foreign_keys
// pragma must be OFF around the rebuild, and PRAGMA foreign_keys is a
// no-op inside a transaction — so migrate() toggles it on the dedicated
// connection before BeginTx.
var rebuildNeedsFKOff = map[int]bool{2: true} // migrations[2] = migrateV3

func migrateV3(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
CREATE TABLE labels_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
	kind TEXT NOT NULL CHECK(kind IN ('issue', 'discussion')),
	name TEXT NOT NULL,
	color TEXT NOT NULL,
	archived INTEGER NOT NULL DEFAULT 0
);
INSERT INTO labels_new(id, workspace_id, kind, name, color, archived)
	SELECT id, NULL, 'issue', name, color, 0 FROM labels;
-- Preserve AUTOINCREMENT history: DROP removes the old table's
-- sqlite_sequence row, so copy it to the replacement or a previously
-- deleted (highest) label id gets reused.
UPDATE sqlite_sequence SET seq = MAX(seq, IFNULL((SELECT seq FROM sqlite_sequence WHERE name='labels'), 0))
	WHERE name='labels_new';
DROP TABLE labels;
ALTER TABLE labels_new RENAME TO labels;
CREATE UNIQUE INDEX idx_labels_kind_name_ws ON labels(kind, name, COALESCE(workspace_id, 0));
CREATE TABLE IF NOT EXISTS discussion_labels (
	discussion_id INTEGER NOT NULL REFERENCES discussions(id) ON DELETE CASCADE,
	label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
	PRIMARY KEY (discussion_id, label_id)
);
CREATE INDEX IF NOT EXISTS idx_discussion_labels_label ON discussion_labels(label_id);
`+defaultLabelsDDL)
	return err
}

// migrateV4 adds the comment-moderation columns shared by the three
// post tables (comments, discussion_threads, discussion_comments):
// edited_at stamps a body change, hidden_at+hidden_reason carry a
// minimize state. Each ALTER is guarded by a pragma_table_info probe
// because a fresh database already has the columns via schemaDDL — the
// squashed v1 — and would otherwise fail on "duplicate column name".
func migrateV4(ctx context.Context, tx *sql.Tx) error {
	const reasonCheck = " TEXT CHECK(hidden_reason IN ('spam','abuse','off-topic','outdated','duplicate','resolved','low-quality'))"
	cols := []struct{ name, ddl string }{
		{"edited_at", "edited_at INTEGER"},
		{"hidden_at", "hidden_at INTEGER"},
		{"hidden_reason", "hidden_reason" + reasonCheck},
	}
	for _, table := range []string{"comments", "discussion_threads", "discussion_comments"} {
		for _, col := range cols {
			var n int
			if err := tx.QueryRowContext(ctx, fmt.Sprintf(
				"SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", table), col.name).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+col.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// pullsDDL is kept out of line so migrateV5 can apply it alone to
// databases stamped v4 before the tables existed — schemaDDL concatenates
// it so fresh databases get the identical definition (the changesDDL /
// migrateV2 pattern).
const pullsDDL = `
-- Pull requests: the DB row is the PR's identity; code-derived data
-- (commits, diff, conflicts) is always computed from git. The *_sha
-- columns record close/merge-time resolutions so a completed PR's diff
-- stays reproducible after its branches are deleted — the pinned
-- refs/zatudan/pr/<id>/head keeps the commits reachable
-- (docs/sketch/ja/pull-requests.md).
CREATE TABLE IF NOT EXISTS pull_requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	base_ref TEXT NOT NULL,                -- normalized (symbolic-full-name) or sha
	head_ref TEXT NOT NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','closed','merged')),
	author_id INTEGER REFERENCES users(id),
	base_sha TEXT,                         -- resolved at create time (display)
	head_sha TEXT,                         -- resolved at close/merge time
	merge_base_sha TEXT,                   -- merge-base at close/merge time
	merge_sha TEXT,                        -- merged only: result commit
	merge_method TEXT,                     -- merged only: merge|squash|rebase
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	closed_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_pull_requests_status_created ON pull_requests(status, created_at DESC);
-- At most one open PR per (base, head) pair — closed/merged pairs may
-- be recreated (the idx_dthreads_answer precedent).
CREATE UNIQUE INDEX IF NOT EXISTS idx_pull_requests_open_pair ON pull_requests(base_ref, head_ref) WHERE status = 'open';
CREATE TABLE IF NOT EXISTS pr_comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	pr_id INTEGER NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
	-- anchor: NULL = conversation comment, set = review comment. line is a
	-- 1-based line number inside the blob anchor_sha names.
	path TEXT,
	line INTEGER,
	side TEXT NOT NULL DEFAULT 'right' CHECK(side IN ('left','right')),
	anchor_sha TEXT,                       -- commit whose blob the line refers to
	diff_snippet TEXT,                     -- surrounding lines captured at post time
	body TEXT NOT NULL,
	author_id INTEGER REFERENCES users(id),
	edited_at INTEGER,
	hidden_at INTEGER,
	hidden_reason TEXT CHECK(hidden_reason IN ('spam','abuse','off-topic','outdated','duplicate','resolved','low-quality')),
	created_at INTEGER NOT NULL,
	CHECK ((path IS NULL) = (line IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_pr_comments_pr ON pr_comments(pr_id, created_at);
`

// migrateV5 adds the pull_requests/pr_comments tables to a database
// stamped v4 (migrateV1 is skipped for those, so the tables would never
// appear otherwise).
func migrateV5(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, pullsDDL)
	return err
}

// migrateWithFKOff runs one migration on a dedicated connection with
// PRAGMA foreign_keys OFF, verifying the result before re-enabling.
// Used for migrations that DROP a parent table (see rebuildNeedsFKOff).
func migrateWithFKOff(ctx context.Context, db *sql.DB, version int) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// foreign_keys can only change outside a transaction.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("store: disable foreign keys for migration %d: %w", version+1, err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := migrations[version](ctx, tx); err != nil {
		tx.Rollback()
		return fmt.Errorf("store: migrate schema to version %d: %w", version+1, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version+1)); err != nil {
		tx.Rollback()
		return fmt.Errorf("store: stamp schema version %d: %w", version+1, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// FKs stayed off through the tx; check the rebuilt data before
	// re-enabling, then restore the pragma this connection's DSN set.
	violations := 0
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&violations); err != nil {
		return fmt.Errorf("store: foreign key check after migration %d: %w", version+1, err)
	}
	if violations > 0 {
		return fmt.Errorf("store: migration to version %d left %d foreign key violations", version+1, violations)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("store: re-enable foreign keys: %w", err)
	}
	return nil
}

// Open opens (or creates) the SQLite database at path, enables foreign keys,
// and migrates the schema to the latest version. It does NOT insert demo
// data; callers invoke SeedDemoData for that. Use ":memory:" (or a
// shared-cache memory DSN) for tests.
func Open(path string) (*Store, error) {
	dsn := path
	switch {
	case path == ":memory:":
		dsn = "file::memory:"
	case !strings.HasPrefix(path, "file:"):
		dsn = "file:" + path
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(dsn, "file::memory:") || strings.Contains(dsn, "mode=memory") {
		// database/sql opens a fresh in-memory DB per pooled connection, so
		// pin a single connection to keep one shared database alive.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	if err := migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.DeleteExpiredSessions(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: sweep expired sessions: %w", err)
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Analyze runs the SQLite ANALYZE command, refreshing the query planner's
// statistics (the internal sqlite_stat% tables). It is safe to re-run — the
// "init" subcommand uses it so a freshly set up database does not start
// with stale or missing planner statistics.
func (s *Store) Analyze(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("store: analyze: %w", err)
	}
	return nil
}

// SchemaObjects lists the database's tables and named indexes as recorded
// in sqlite_master, each sorted by name. Internal bookkeeping objects are
// filtered so the result reflects only the schema declared by Open:
// sqlite_stat% tables (ANALYZE output) are excluded from tables and
// sqlite_autoindex% (UNIQUE/PRIMARY KEY backing indexes) from indexes.
// sqlite_sequence (AUTOINCREMENT bookkeeping) is a real part of the
// declared schema and stays in the table list.
func (s *Store) SchemaObjects(ctx context.Context) (tables, indexes []string, err error) {
	list := func(where string) ([]string, error) {
		rows, err := s.db.QueryContext(ctx,
			"SELECT name FROM sqlite_master WHERE "+where+" ORDER BY name")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			out = append(out, name)
		}
		return out, rows.Err()
	}

	tables, err = list("type='table' AND name NOT LIKE 'sqlite_stat%'")
	if err != nil {
		return nil, nil, fmt.Errorf("store: list tables: %w", err)
	}
	indexes, err = list("type='index' AND name NOT LIKE 'sqlite_autoindex%'")
	if err != nil {
		return nil, nil, fmt.Errorf("store: list indexes: %w", err)
	}
	return tables, indexes, nil
}

type scanner interface{ Scan(...any) error }

// scanUser builds the author from a LEFT JOIN's nullable user columns; a
// NULL id means an anonymous post.
func scanUser(id sql.NullInt64, name sql.NullString, created sql.NullInt64) *User {
	if !id.Valid {
		return nil
	}
	return &User{ID: id.Int64, Name: name.String, CreatedAt: time.Unix(created.Int64, 0).UTC()}
}

// nullID maps a 0 author ID (anonymous) to SQL NULL.
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func checkAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
