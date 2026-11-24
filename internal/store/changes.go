// Cross-process change notification: every mutator records its change
// hints in the changes table inside the same transaction as the mutation,
// and the server's WatchChanges re-publishes new rows to the SSE hub.
// Because the hint lives in the database, writers in other processes —
// the local CLI (issues --db) above all — reach open pages the same way
// in-process writes do. Hints stay volatile: a restarted watcher begins
// at the high-water mark and misses are healed by client resync
// (docs/sketch/ja/live-updates.md, 案(a)).
package store

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

const (
	// watchInterval is how often WatchChanges polls the changes table.
	watchInterval = 250 * time.Millisecond
	// changeRetention bounds the table: hints older than this serve no
	// consumer (a restarted watcher skips backlog anyway) and are purged.
	changeRetention = 5 * time.Minute
	// purgeInterval throttles the cleanup DELETE inside the watch loop.
	purgeInterval = time.Minute
)

// commitTx records the change hints inside tx, then commits. Writing the
// rows in the same transaction keeps hint and mutation atomic: a commit
// means the event rows exist, a rollback loses both. Mutators use it in
// place of tx.Commit() wherever a hint must be emitted.
func (s *Store) commitTx(ctx context.Context, tx *sql.Tx, changes ...events.Change) error {
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO changes(topic, kind, target, parent, created_at) VALUES(?, ?, ?, ?, ?)",
			c.Topic, c.Kind, c.Target, c.Parent, time.Now().UTC().Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LatestChangeSeq returns the high-water mark of the changes table — the
// seq a fresh watcher starts from so it does not replay backlog.
func (s *Store) LatestChangeSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) FROM changes").Scan(&seq)
	return seq, err
}

// ListChanges returns the recorded changes with seq > after, in seq order.
// The returned Seq is the table's position, distinct from the hub's
// in-process sequence assigned on publish.
func (s *Store) ListChanges(ctx context.Context, after int64) ([]events.Change, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT seq, topic, kind, target, parent FROM changes WHERE seq > ? ORDER BY seq", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []events.Change
	for rows.Next() {
		var c events.Change
		if err := rows.Scan(&c.Seq, &c.Topic, &c.Kind, &c.Target, &c.Parent); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PurgeChanges deletes change rows recorded before the given unix time.
func (s *Store) PurgeChanges(ctx context.Context, before int64) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM changes WHERE created_at < ?", before)
	return err
}

// WatchChanges polls the changes table and calls fn for every row with
// seq > after until ctx is done — how the server's SSE hub learns about
// writes made by other processes (main.go wires it to hub.Publish). The
// caller supplies the start watermark via a synchronous LatestChangeSeq
// call: sampling inside the goroutine would let a commit slip between
// goroutine start and its first read. Poll errors are logged and retried
// on the next tick; a dying database makes the loop noisy but not dead.
func (s *Store) WatchChanges(ctx context.Context, after int64, fn func(events.Change)) {
	s.watchChanges(ctx, watchInterval, after, fn)
}

func (s *Store) watchChanges(ctx context.Context, interval time.Duration, after int64, fn func(events.Change)) {
	last := after
	tick := time.NewTicker(interval)
	defer tick.Stop()
	lastPurge := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		changes, err := s.ListChanges(ctx, last)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("store: watch changes: %v", err)
			continue
		}
		for _, c := range changes {
			fn(c)
			last = c.Seq
		}
		if time.Since(lastPurge) > purgeInterval {
			lastPurge = time.Now()
			if err := s.PurgeChanges(ctx, lastPurge.Add(-changeRetention).Unix()); err != nil && ctx.Err() == nil {
				log.Printf("store: purge changes: %v", err)
			}
		}
	}
}
