// Package store persists check runs, per-container results and the registry tag
// cache in SQLite (pure Go driver, no CGO). Migrations are embedded and applied on Open.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies pending migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	// WAL lets the API read while a check run writes; busy_timeout absorbs short lock waits.
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)&_pragma=synchronous(normal)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite has one writer; serialising here avoids SQLITE_BUSY
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("store: migrations table: %w", err)
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}

	files, _ := fs.Glob(migrations, "migrations/*.sql")
	sort.Strings(files)
	for _, f := range files {
		version, err := strconv.Atoi(strings.SplitN(filepath.Base(f), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("store: bad migration name %s", f)
		}
		if version <= current {
			continue
		}
		body, _ := migrations.ReadFile(f)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %s: %w", f, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ── Check runs ───────────────────────────────────────────

type Run struct {
	ID         int64
	Trigger    string
	StartedAt  time.Time
	FinishedAt *time.Time
	Images     int
	Error      string
}

func (s *Store) StartRun(ctx context.Context, trigger string, at time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO check_runs (trigger, started_at) VALUES (?, ?)`, trigger, ts(at))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishRun(ctx context.Context, id int64, at time.Time, images int, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE check_runs SET finished_at = ?, images = ?, error = ? WHERE id = ?`, ts(at), images, errMsg, id)
	return err
}

// LastFinishedRun returns the most recent completed run, if any.
func (s *Store) LastFinishedRun(ctx context.Context) (Run, bool, error) {
	var r Run
	var started, finished string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, trigger, started_at, finished_at, images, error FROM check_runs
		WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT 1`).
		Scan(&r.ID, &r.Trigger, &started, &finished, &r.Images, &r.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r.StartedAt = parseTS(started)
	f := parseTS(finished)
	r.FinishedAt = &f
	return r, true, nil
}

// ── Results ──────────────────────────────────────────────

type Result struct {
	ContainerName string
	ImageRef      string
	ImageID       string
	Running       string
	Source        string
	Latest        string
	LatestInMajor string
	Class         string
	Floating      bool
	NewImage      bool
	Held          string
	HeldBy        string
	Rules         string // fingerprint of the rules it was checked with, "" for none
	Error         string
	CheckedAt     time.Time
	RunID         int64
}

func (s *Store) SaveResult(ctx context.Context, r Result) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO results (container_name, image_ref, image_id, running, source, latest, latest_in_major,
		                     class, floating, new_image, held, held_by, rules, error, checked_at, run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (container_name) DO UPDATE SET
			image_ref = excluded.image_ref, image_id = excluded.image_id, running = excluded.running,
			source = excluded.source, latest = excluded.latest, latest_in_major = excluded.latest_in_major,
			class = excluded.class, floating = excluded.floating, new_image = excluded.new_image,
			held = excluded.held, held_by = excluded.held_by, rules = excluded.rules, error = excluded.error, checked_at = excluded.checked_at, run_id = excluded.run_id`,
		r.ContainerName, r.ImageRef, r.ImageID, r.Running, r.Source, r.Latest, r.LatestInMajor,
		r.Class, r.Floating, r.NewImage, r.Held, r.HeldBy, r.Rules, r.Error, ts(r.CheckedAt), nullID(r.RunID))
	return err
}

func (s *Store) Results(ctx context.Context) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT container_name, image_ref, image_id, running, source, latest, latest_in_major,
		       class, floating, new_image, held, held_by, rules, error, checked_at, COALESCE(run_id, 0)
		FROM results`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		var checked string
		if err := rows.Scan(&r.ContainerName, &r.ImageRef, &r.ImageID, &r.Running, &r.Source, &r.Latest,
			&r.LatestInMajor, &r.Class, &r.Floating, &r.NewImage, &r.Held, &r.HeldBy, &r.Rules, &r.Error, &checked, &r.RunID); err != nil {
			return nil, err
		}
		r.CheckedAt = parseTS(checked)
		out = append(out, r)
	}
	return out, rows.Err()
}

// KeepResults deletes results for containers not in names (removed containers).
func (s *Store) KeepResults(ctx context.Context, names []string) error {
	keep, err := json.Marshal(names)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM results WHERE container_name NOT IN (SELECT value FROM json_each(?))`, string(keep))
	return err
}

// ── Tag cache ────────────────────────────────────────────

// LoadTags returns the cached tag list of a repository and when it was fetched.
func (s *Store) LoadTags(ctx context.Context, repository string) ([]string, time.Time, bool) {
	var raw, fetched string
	err := s.db.QueryRowContext(ctx, `SELECT tags, fetched_at FROM tag_cache WHERE repository = ?`, repository).Scan(&raw, &fetched)
	if err != nil {
		return nil, time.Time{}, false
	}
	var tags []string
	if json.Unmarshal([]byte(raw), &tags) != nil {
		return nil, time.Time{}, false
	}
	return tags, parseTS(fetched), true
}

func (s *Store) SaveTags(ctx context.Context, repository string, tags []string, at time.Time) error {
	raw, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tag_cache (repository, tags, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT (repository) DO UPDATE SET tags = excluded.tags, fetched_at = excluded.fetched_at`,
		repository, string(raw), ts(at))
	return err
}

// ── helpers ──────────────────────────────────────────────

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// ── Changelogs ───────────────────────────────────────────

// Release is one entry of a repository's release notes.
type Release struct {
	Tag         string
	Name        string
	Body        string
	URL         string
	PublishedAt time.Time
	Prerelease  bool
}

// RepoState records how a repository's release notes were last fetched.
type RepoState struct {
	Repo      string
	Kind      string // releases | changelog | none
	ETag      string
	HasMore   bool
	FetchedAt time.Time
	Error     string
}

// SaveReleases upserts releases; with replace, the repo's existing releases are dropped
// first (a re-parsed CHANGELOG.md replaces the old sections).
func (s *Store) SaveReleases(ctx context.Context, repo string, rs []Release, replace bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if replace {
		if _, err := tx.ExecContext(ctx, `DELETE FROM releases WHERE repo = ?`, repo); err != nil {
			return err
		}
	}
	for _, r := range rs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO releases (repo, tag, name, body, url, published_at, prerelease) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (repo, tag) DO UPDATE SET name = excluded.name, body = excluded.body, url = excluded.url,
				published_at = excluded.published_at, prerelease = excluded.prerelease`,
			repo, r.Tag, r.Name, r.Body, r.URL, tsOrEmpty(r.PublishedAt), r.Prerelease); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Releases(ctx context.Context, repo string) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag, name, body, url, published_at, prerelease FROM releases WHERE repo = ?`, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Release
	for rows.Next() {
		var r Release
		var published string
		if err := rows.Scan(&r.Tag, &r.Name, &r.Body, &r.URL, &published, &r.Prerelease); err != nil {
			return nil, err
		}
		r.PublishedAt = parseTS(published)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RepoState(ctx context.Context, repo string) (RepoState, bool, error) {
	st := RepoState{Repo: repo}
	var fetched string
	err := s.db.QueryRowContext(ctx, `SELECT kind, etag, has_more, fetched_at, error FROM repos WHERE repo = ?`, repo).
		Scan(&st.Kind, &st.ETag, &st.HasMore, &fetched, &st.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return st, false, nil
	}
	if err != nil {
		return st, false, err
	}
	st.FetchedAt = parseTS(fetched)
	return st, true, nil
}

func (s *Store) SaveRepoState(ctx context.Context, st RepoState) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO repos (repo, kind, etag, has_more, fetched_at, error) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (repo) DO UPDATE SET kind = excluded.kind, etag = excluded.etag, has_more = excluded.has_more,
			fetched_at = excluded.fetched_at, error = excluded.error`,
		st.Repo, st.Kind, st.ETag, st.HasMore, tsOrEmpty(st.FetchedAt), st.Error)
	return err
}

func tsOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return ts(t)
}
