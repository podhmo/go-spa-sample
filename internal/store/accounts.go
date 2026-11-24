// Accounts: users and sessions (schema version 2, see migrations in
// store.go). Authentication here means identification only — every page
// stays usable while logged out, and a NULL author_id means "anonymous".
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/podhmo/go-spa-sample/internal/auth"

	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrDuplicateName reports a user name that is already taken (names are
// unique case-insensitively via COLLATE NOCASE).
var ErrDuplicateName = errors.New("store: user name already taken")

// ErrInvalidCredentials reports a failed login. The same error covers
// "no such user" and "wrong password" so callers cannot enumerate names.
var ErrInvalidCredentials = errors.New("store: invalid name or password")

// User is a registered account. Name is both the login name and the
// display name (splitting them is future work).
type User struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

// CreateUser inserts a user with a pre-computed PHC password hash (see
// internal/auth.HashPassword). Returns ErrDuplicateName when the name is
// already taken.
func (s *Store) CreateUser(ctx context.Context, name, passwordHash string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO users(name, password_hash, created_at) VALUES(?, ?, ?)",
		name, passwordHash, time.Now().UTC().Unix())
	if err != nil {
		var sqliteErr *sqliteDriver.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return 0, ErrDuplicateName
		}
		return 0, err
	}
	return res.LastInsertId()
}

// UserByName returns one user by login name, or ErrNotFound. Name lookup
// is case-insensitive via the users.name COLLATE NOCASE unique index.
func (s *Store) UserByName(ctx context.Context, name string) (*User, error) {
	var u User
	var created int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, created_at FROM users WHERE name = ?", name).
		Scan(&u.ID, &u.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = time.Unix(created, 0).UTC()
	return &u, nil
}

// UserByID returns one user, or ErrNotFound.
func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	var created int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = time.Unix(created, 0).UTC()
	return &u, nil
}

// AuthenticateUser verifies name+password and returns the user, or
// ErrInvalidCredentials for both failure modes.
func (s *Store) AuthenticateUser(ctx context.Context, name, password string) (*User, error) {
	var u User
	var hash string
	var created int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, password_hash, created_at FROM users WHERE name = ?", name).
		Scan(&u.ID, &u.Name, &hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = time.Unix(created, 0).UTC()
	if !auth.VerifyPassword(password, hash) {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

// CreateSession starts a session for userID and returns its plaintext
// token. Only the token's SHA-256 is stored; the plaintext goes to the
// caller exactly once, for the Set-Cookie header.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions(id, user_id, created_at, expires_at) VALUES(?, ?, ?, ?)",
		auth.HashToken(token), userID, now.Unix(), now.Add(ttl).Unix()); err != nil {
		return "", err
	}
	return token, nil
}

// SessionUser resolves a plaintext session token to its user, or
// ErrNotFound for unknown/expired/deleted sessions. Expired rows are
// deleted on sight. Sessions renew slidingly: when less than half of ttl
// remains, expires_at extends to now+ttl. ttl must be the same lifetime
// passed to CreateSession — deriving it from the row would inflate the
// idle window after every renewal.
func (s *Store) SessionUser(ctx context.Context, token string, ttl time.Duration) (*User, error) {
	tokenHash := auth.HashToken(token)
	var u User
	var userCreated, expiresAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.name, u.created_at, s.expires_at
		 FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = ?`,
		tokenHash).
		Scan(&u.ID, &u.Name, &userCreated, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = time.Unix(userCreated, 0).UTC()

	now := time.Now().UTC().Unix()
	if expiresAt <= now {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", tokenHash)
		return nil, ErrNotFound
	}
	ttlSecs := int64(ttl.Seconds())
	if expiresAt-now < ttlSecs/2 {
		if _, err := s.db.ExecContext(ctx,
			"UPDATE sessions SET expires_at = ? WHERE id = ?", now+ttlSecs, tokenHash); err != nil {
			return nil, err
		}
	}
	return &u, nil
}

// DeleteSession ends one session (logout). Missing tokens are a no-op.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM sessions WHERE id = ?", auth.HashToken(token))
	return err
}

// DeleteExpiredSessions sweeps every session past its expires_at. Called
// on Open; also safe to run periodically.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM sessions WHERE expires_at <= ?", time.Now().UTC().Unix())
	return err
}

// Per-user Bearer tokens for /api/v1. They reuse the sessions primitives
// (auth.NewToken/HashToken: only the SHA-256 is stored) but have no
// expiry — an API token lives until its row is deleted (revoked).

// CreateAPIToken issues an opaque API token for userID and returns its
// plaintext. The plaintext is shown to the caller exactly once, like a
// session token.
func (s *Store) CreateAPIToken(ctx context.Context, userID int64) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO api_tokens(id, user_id, created_at) VALUES(?, ?, ?)",
		auth.HashToken(token), userID, time.Now().UTC().Unix()); err != nil {
		return "", err
	}
	return token, nil
}

// APITokenUser resolves a plaintext API token to its user, or ErrNotFound
// for an unknown or revoked token.
func (s *Store) APITokenUser(ctx context.Context, token string) (*User, error) {
	var u User
	var created int64
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.name, u.created_at
		 FROM api_tokens t JOIN users u ON u.id = t.user_id WHERE t.id = ?`,
		auth.HashToken(token)).
		Scan(&u.ID, &u.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = time.Unix(created, 0).UTC()
	return &u, nil
}

// DeleteAPIToken revokes one API token by its plaintext value. Missing
// tokens are a no-op.
func (s *Store) DeleteAPIToken(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM api_tokens WHERE id = ?", auth.HashToken(token))
	return err
}

// APIToken is the stored metadata of a per-user API token. The plaintext
// is never persisted — ID holds its SHA-256 hash (the primary key), so a
// listing can only show metadata, never the token itself.
type APIToken struct {
	ID        string // SHA-256 hex of the plaintext
	User      *User
	CreatedAt time.Time
}

// ListAPITokens returns all issued API tokens newest first, each with its
// owning user — the management view behind `issues auth token list`.
func (s *Store) ListAPITokens(ctx context.Context) ([]*APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.created_at, u.id, u.name, u.created_at
		 FROM api_tokens t JOIN users u ON u.id = t.user_id
		 ORDER BY t.created_at DESC, t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*APIToken
	for rows.Next() {
		var t APIToken
		var created int64
		var u User
		var ucreated int64
		if err := rows.Scan(&t.ID, &created, &u.ID, &u.Name, &ucreated); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(created, 0).UTC()
		u.CreatedAt = time.Unix(ucreated, 0).UTC()
		t.User = &u
		out = append(out, &t)
	}
	return out, rows.Err()
}

// DeleteAPITokenByID revokes one API token by its stored (hashed) id —
// the identifier `issues auth token list` shows, since the plaintext can
// no longer be recovered. Returns ErrNotFound for an unknown id.
func (s *Store) DeleteAPITokenByID(ctx context.Context, hash string) error {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM api_tokens WHERE id = ?", hash)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

// HasAPITokens reports whether any API token exists. Startup uses it to
// decide whether /api/v1 is mounted when no shared env token is set —
// the API is fail-closed, so with neither credential it is not exposed.
func (s *Store) HasAPITokens(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_tokens").Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
