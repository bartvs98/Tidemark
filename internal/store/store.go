// Package store persists check runs, per-container results and the registry tag
// cache in SQLite (pure Go driver, no CGO). Migrations are embedded and applied on Open.
package store

import (
	"cmp"
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
	TagDigest     string    // the floating tag's registry digest when it was fetched, "" otherwise
	Rules         string    // fingerprint of the rules it was checked with, "" for none
	Error         string    // the last attempt failed with this, "" if it succeeded
	ErrorKind     string    // registry.Kind of Error, "" if unclassified
	CheckedAt     time.Time // last successful check, zero if there never was one
	FailedAt      time.Time // the failed attempt Error is from, zero without one
	TagsFrom      time.Time // the result's tag list was reused from then (rate-limited), zero normally
	RunID         int64
}

func (s *Store) SaveResult(ctx context.Context, r Result) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO results (container_name, image_ref, image_id, running, source, latest, latest_in_major,
		                     class, floating, new_image, held, held_by, tag_digest, rules, error, error_kind, checked_at, failed_at, tags_from, run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (container_name) DO UPDATE SET
			image_ref = excluded.image_ref, image_id = excluded.image_id, running = excluded.running,
			source = excluded.source, latest = excluded.latest, latest_in_major = excluded.latest_in_major,
			class = excluded.class, floating = excluded.floating, new_image = excluded.new_image,
			held = excluded.held, held_by = excluded.held_by, tag_digest = excluded.tag_digest, rules = excluded.rules, error = excluded.error, error_kind = excluded.error_kind,
			checked_at = excluded.checked_at, failed_at = excluded.failed_at, tags_from = excluded.tags_from, run_id = excluded.run_id`,
		r.ContainerName, r.ImageRef, r.ImageID, r.Running, r.Source, r.Latest, r.LatestInMajor,
		r.Class, r.Floating, r.NewImage, r.Held, r.HeldBy, r.TagDigest, r.Rules, r.Error, r.ErrorKind,
		tsOrEmpty(r.CheckedAt), tsOrEmpty(r.FailedAt), tsOrEmpty(r.TagsFrom), nullID(r.RunID))
	return err
}

func (s *Store) Results(ctx context.Context) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT container_name, image_ref, image_id, running, source, latest, latest_in_major,
		       class, floating, new_image, held, held_by, tag_digest, rules, error, error_kind, checked_at, failed_at, tags_from, COALESCE(run_id, 0)
		FROM results`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		var checked, failed, tagsFrom string
		if err := rows.Scan(&r.ContainerName, &r.ImageRef, &r.ImageID, &r.Running, &r.Source, &r.Latest,
			&r.LatestInMajor, &r.Class, &r.Floating, &r.NewImage, &r.Held, &r.HeldBy, &r.TagDigest, &r.Rules, &r.Error, &r.ErrorKind, &checked, &failed, &tagsFrom, &r.RunID); err != nil {
			return nil, err
		}
		r.CheckedAt, r.FailedAt, r.TagsFrom = parseTS(checked), parseTS(failed), parseTS(tagsFrom)
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

// ── Reviews ──────────────────────────────────────────────

// Review is a container's review state, by name: reviewed (read the changes up to
// Version) or snoozed (until Until, or until a version newer than Version).
type Review struct {
	ContainerName string
	Kind          string    // reviewed | snoozed
	Version       string    // reviewed: up to; snoozed: until newer than ("" when by time)
	Until         time.Time // snoozed by time; zero otherwise
	CreatedAt     time.Time
}

// Reviews returns all stored reviews by container name.
func (s *Store) Reviews(ctx context.Context) (map[string]Review, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT container_name, kind, version, until, created_at FROM reviews`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Review{}
	for rows.Next() {
		var r Review
		var until, created string
		if err := rows.Scan(&r.ContainerName, &r.Kind, &r.Version, &until, &created); err != nil {
			return nil, err
		}
		r.Until, r.CreatedAt = parseTS(until), parseTS(created)
		out[r.ContainerName] = r
	}
	return out, rows.Err()
}

// SaveReview stores a container's review, replacing any earlier one (reviewed and
// snoozed exclude each other).
func (s *Store) SaveReview(ctx context.Context, r Review) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reviews (container_name, kind, version, until, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (container_name) DO UPDATE SET kind = excluded.kind, version = excluded.version,
			until = excluded.until, created_at = excluded.created_at`,
		r.ContainerName, r.Kind, r.Version, tsOrEmpty(r.Until), ts(r.CreatedAt))
	return err
}

// DeleteReview removes a container's review; nothing stored is fine.
func (s *Store) DeleteReview(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM reviews WHERE container_name = ?`, name)
	return err
}

// ── History ──────────────────────────────────────────────

// HistoryEntry is an image change of a container seen between successful checks. The
// first entry of a container has no Prev*: that's when tracking started.
type HistoryEntry struct {
	ID            int64
	ContainerName string
	ImageID       string
	Running       string
	PrevImageID   string // "" on the first row
	PrevRunning   string
	DetectedAt    time.Time
	Via           string // check
}

func (s *Store) AddHistory(ctx context.Context, h HistoryEntry) error {
	via := h.Via
	if via == "" {
		via = "check"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO history (container_name, image_id, running, prev_image_id, prev_running, detected_at, via)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		h.ContainerName, h.ImageID, h.Running, h.PrevImageID, h.PrevRunning, ts(h.DetectedAt), via)
	return err
}

// History returns a container's entries, newest first.
func (s *Store) History(ctx context.Context, name string) ([]HistoryEntry, error) {
	return s.queryHistory(ctx, `
		SELECT id, container_name, image_id, running, prev_image_id, prev_running, detected_at, via
		FROM history WHERE container_name = ? ORDER BY id DESC`, name)
}

// LastHistory returns the newest entry of every container, by name.
func (s *Store) LastHistory(ctx context.Context) (map[string]HistoryEntry, error) {
	list, err := s.queryHistory(ctx, `
		SELECT id, container_name, image_id, running, prev_image_id, prev_running, detected_at, via
		FROM history WHERE id IN (SELECT MAX(id) FROM history GROUP BY container_name)`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]HistoryEntry, len(list))
	for _, h := range list {
		out[h.ContainerName] = h
	}
	return out, nil
}

func (s *Store) queryHistory(ctx context.Context, query string, args ...any) ([]HistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		var detected string
		if err := rows.Scan(&h.ID, &h.ContainerName, &h.ImageID, &h.Running, &h.PrevImageID, &h.PrevRunning, &detected, &h.Via); err != nil {
			return nil, err
		}
		h.DetectedAt = parseTS(detected)
		out = append(out, h)
	}
	return out, rows.Err()
}

// ── Notifications ────────────────────────────────────────

// NotifiedKey identifies a sent update: a container and its update's review key.
type NotifiedKey struct{ Container, Key string }

// Notified is an update that was sent, with the level it was sent with.
type Notified struct {
	ContainerName, Key string
	Level              string // none | attention | breaking
	NotifiedAt         time.Time
}

// Notification is one send attempt: an update message, the first message or a test.
type Notification struct {
	ID       int64
	SentAt   time.Time
	Notifier string // telegram
	Kind     string // updates | first | test
	Items    int
	Error    string // "" when sent
}

// Notified returns every sent update by container and key.
func (s *Store) Notified(ctx context.Context) (map[NotifiedKey]Notified, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT container_name, key, level, notified_at FROM notified`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[NotifiedKey]Notified{}
	for rows.Next() {
		var n Notified
		var at string
		if err := rows.Scan(&n.ContainerName, &n.Key, &n.Level, &at); err != nil {
			return nil, err
		}
		n.NotifiedAt = parseTS(at)
		out[NotifiedKey{n.ContainerName, n.Key}] = n
	}
	return out, rows.Err()
}

// MarkNotified stores the sent updates in one transaction, replacing earlier rows with
// the same container and key.
func (s *Store) MarkNotified(ctx context.Context, rows []Notified) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range rows {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notified (container_name, key, level, notified_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (container_name, key) DO UPDATE SET level = excluded.level, notified_at = excluded.notified_at`,
			n.ContainerName, n.Key, n.Level, ts(n.NotifiedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddNotification logs a send attempt.
func (s *Store) AddNotification(ctx context.Context, n Notification) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notifications (sent_at, notifier, kind, items, error) VALUES (?, ?, ?, ?, ?)`,
		ts(n.SentAt), n.Notifier, n.Kind, n.Items, n.Error)
	return err
}

// LastNotification returns the newest send attempt of any kind, failed ones included.
func (s *Store) LastNotification(ctx context.Context) (Notification, bool, error) {
	var n Notification
	var sent string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, sent_at, notifier, kind, items, error FROM notifications ORDER BY id DESC LIMIT 1`).
		Scan(&n.ID, &sent, &n.Notifier, &n.Kind, &n.Items, &n.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return n, false, nil
	}
	if err != nil {
		return n, false, err
	}
	n.SentAt = parseTS(sent)
	return n, true, nil
}

// SentUpdatesBefore reports whether an update message (or the first message) was ever
// sent successfully. Tests and failed attempts don't count.
func (s *Store) SentUpdatesBefore(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notifications WHERE kind IN ('updates', 'first') AND error = ''`).Scan(&n)
	return n > 0, err
}

// Settings returns the stored editable settings by key.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SaveSettings upserts set and deletes the keys in remove, in one transaction.
func (s *Store) SaveSettings(ctx context.Context, set map[string]string, remove []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range set {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	for _, k := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ── AI summaries ─────────────────────────────────────────

// AISummary is the stored AI summary of a repo's notes between two versions, plus the
// last failed attempt.
type AISummary struct {
	Repo, From, To, Provider, Model string
	Changes, Actions                []string
	Verdict                         string // none | attention | breaking
	Reasons                         []AIReason
	Releases, Omitted               int
	NotesHash                       string
	CreatedAt                       time.Time // zero = never succeeded
	InputTokens, OutputTokens       int       // of the call that made the summary
	Duration                        time.Duration
	Error                           string
	FailedAt                        time.Time
}

type AIReason struct {
	Text string `json:"text"`
	Tag  string `json:"tag,omitempty"`
}

// AISummaries returns every stored summary.
func (s *Store) AISummaries(ctx context.Context) ([]AISummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT repo, from_version, to_version, provider, model, changes_json, actions_json, verdict,
			reasons_json, releases, omitted, notes_hash, created_at, error, failed_at,
			input_tokens, output_tokens, duration_ms
		FROM ai_summaries ORDER BY repo, from_version, to_version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AISummary
	for rows.Next() {
		var a AISummary
		var changes, actions, reasons, created, failed string
		var ms int64
		if err := rows.Scan(&a.Repo, &a.From, &a.To, &a.Provider, &a.Model, &changes, &actions, &a.Verdict,
			&reasons, &a.Releases, &a.Omitted, &a.NotesHash, &created, &a.Error, &failed,
			&a.InputTokens, &a.OutputTokens, &ms); err != nil {
			return nil, err
		}
		if err := errors.Join(json.Unmarshal([]byte(changes), &a.Changes), json.Unmarshal([]byte(actions), &a.Actions),
			json.Unmarshal([]byte(reasons), &a.Reasons)); err != nil {
			return nil, fmt.Errorf("store: ai summary %s %s→%s: %w", a.Repo, a.From, a.To, err)
		}
		a.CreatedAt, a.FailedAt = parseTS(created), parseTS(failed)
		a.Duration = time.Duration(ms) * time.Millisecond
		out = append(out, a)
	}
	return out, rows.Err()
}

// PutAISummary stores a summary, replacing the row with the same repo and versions.
func (s *Store) PutAISummary(ctx context.Context, a AISummary) error {
	changes, _ := json.Marshal(nonNil(a.Changes))
	actions, _ := json.Marshal(nonNil(a.Actions))
	reasons, _ := json.Marshal(nonNil(a.Reasons))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_summaries (repo, from_version, to_version, provider, model, changes_json, actions_json,
			verdict, reasons_json, releases, omitted, notes_hash, created_at, error, failed_at,
			input_tokens, output_tokens, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (repo, from_version, to_version) DO UPDATE SET provider = excluded.provider,
			model = excluded.model, changes_json = excluded.changes_json, actions_json = excluded.actions_json,
			verdict = excluded.verdict, reasons_json = excluded.reasons_json, releases = excluded.releases,
			omitted = excluded.omitted, notes_hash = excluded.notes_hash, created_at = excluded.created_at,
			error = excluded.error, failed_at = excluded.failed_at, input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens, duration_ms = excluded.duration_ms`,
		a.Repo, a.From, a.To, a.Provider, a.Model, string(changes), string(actions),
		cmp.Or(a.Verdict, "none"), string(reasons), a.Releases, a.Omitted, a.NotesHash,
		tsOrEmpty(a.CreatedAt), a.Error, tsOrEmpty(a.FailedAt), a.InputTokens, a.OutputTokens, a.Duration.Milliseconds())
	return err
}

// DeleteAISummaries deletes the stored summaries keep rejects and returns how many went.
func (s *Store) DeleteAISummaries(ctx context.Context, keep func(repo, from, to string) bool) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT repo, from_version, to_version FROM ai_summaries`)
	if err != nil {
		return 0, err
	}
	var drop [][3]string
	for rows.Next() {
		var k [3]string
		if err := rows.Scan(&k[0], &k[1], &k[2]); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep(k[0], k[1], k[2]) {
			drop = append(drop, k)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(drop) == 0 {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, k := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ai_summaries WHERE repo = ? AND from_version = ? AND to_version = ?`,
			k[0], k[1], k[2]); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(drop), nil
}

// AICall is one provider call made for an AI summary.
type AICall struct {
	At                                  time.Time // start of the call
	Provider, Model, Trigger            string    // trigger: manual | auto
	Repo, From, To                      string
	Releases, InputTokens, OutputTokens int // releases sent
	Duration                            time.Duration
	Error                               string // "" = success
}

// AIUsage sums AI calls: Calls counts all, Failed the ones with an error.
type AIUsage struct{ Calls, Failed, InputTokens, OutputTokens int }

// tsSortable is a fixed-width UTC timestamp, so text comparison orders it (ai_calls.at).
func tsSortable(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

// AddAICall records one provider call.
func (s *Store) AddAICall(ctx context.Context, c AICall) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_calls (at, provider, model, trigger, repo, from_version, to_version, releases,
			input_tokens, output_tokens, duration_ms, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tsSortable(c.At), c.Provider, c.Model, c.Trigger, c.Repo, c.From, c.To, c.Releases,
		c.InputTokens, c.OutputTokens, c.Duration.Milliseconds(), c.Error)
	return err
}

// AIUsageSince sums the calls since t.
func (s *Store) AIUsageSince(ctx context.Context, t time.Time) (AIUsage, error) {
	var u AIUsage
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(error != ''), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0)
		FROM ai_calls WHERE at >= ?`, tsSortable(t)).Scan(&u.Calls, &u.Failed, &u.InputTokens, &u.OutputTokens)
	return u, err
}

// nonNil makes a nil slice marshal as [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
