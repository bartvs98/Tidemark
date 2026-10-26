package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// ── Pruning and backup ───────────────────────────────────

// PrunedTotals are counts of release notes deleted by PruneRepos.
type PrunedTotals struct {
	Releases, Breaking, Words int
}

// PrunedReleases returns the totals of every pruned release so far.
func (s *Store) PrunedReleases(ctx context.Context) (PrunedTotals, error) {
	var t PrunedTotals
	err := s.rdb.QueryRowContext(ctx, `SELECT releases, breaking, words FROM pruned_releases WHERE id = 1`).
		Scan(&t.Releases, &t.Breaking, &t.Words)
	return t, err
}

// StaleRepos returns the keys of repos (repos table) not in inUse whose fetched_at is before `before`.
func (s *Store) StaleRepos(ctx context.Context, inUse map[string]bool, before time.Time) ([]string, error) {
	rows, err := s.rdb.QueryContext(ctx, `SELECT repo, fetched_at FROM repos ORDER BY repo`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var repo, fetched string
		if err := rows.Scan(&repo, &fetched); err != nil {
			return nil, err
		}
		// Compared parsed: ts() isn't fixed width. A never-fetched row ("") counts as stale.
		if !inUse[repo] && parseTS(fetched).Before(before) {
			out = append(out, repo)
		}
	}
	return out, rows.Err()
}

// PruneRepos deletes the releases and repos rows of the given repo keys and adds add to the
// pruned totals, in one transaction.
func (s *Store) PruneRepos(ctx context.Context, repos []string, add PrunedTotals) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, repo := range repos {
		if _, err := tx.ExecContext(ctx, `DELETE FROM releases WHERE repo = ?`, repo); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM repos WHERE repo = ?`, repo); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE pruned_releases SET releases = releases + ?, breaking = breaking + ?, words = words + ? WHERE id = 1`,
		add.Releases, add.Breaking, add.Words); err != nil {
		return err
	}
	return tx.Commit()
}

// PruneTagCache deletes tag_cache rows whose repository isn't in inUse and whose fetched_at is
// before `before`; it returns how many it deleted.
func (s *Store) PruneTagCache(ctx context.Context, inUse map[string]bool, before time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT repository, fetched_at FROM tag_cache`)
	if err != nil {
		return 0, err
	}
	var drop []string
	for rows.Next() {
		var repo, fetched string
		if err := rows.Scan(&repo, &fetched); err != nil {
			rows.Close()
			return 0, err
		}
		if !inUse[repo] && parseTS(fetched).Before(before) {
			drop = append(drop, repo)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(drop) == 0 {
		return 0, err
	}
	for _, repo := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM tag_cache WHERE repository = ?`, repo); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(drop), nil
}

// Backup writes a consistent copy of the database to path (VACUUM INTO). path must not exist.
func (s *Store) Backup(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("store: backup: %s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: backup: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("store: backup: %w", err)
	}
	return nil
}
