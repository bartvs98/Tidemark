-- Release notes per source repository (GitHub/GitLab releases, or CHANGELOG.md sections).
CREATE TABLE releases (
    repo         TEXT    NOT NULL,             -- lowercase key, e.g. github.com/home-assistant/core
    tag          TEXT    NOT NULL,             -- repo tag ("v1.4.0"); the version for CHANGELOG sections
    name         TEXT    NOT NULL DEFAULT '',
    body         TEXT    NOT NULL DEFAULT '',  -- markdown, untrusted
    url          TEXT    NOT NULL DEFAULT '',
    published_at TEXT    NOT NULL DEFAULT '',
    prerelease   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (repo, tag)
);

-- Fetch state per repository: where notes come from, conditional-request ETag, freshness.
CREATE TABLE repos (
    repo       TEXT PRIMARY KEY,
    kind       TEXT    NOT NULL DEFAULT '',    -- releases | changelog | none
    etag       TEXT    NOT NULL DEFAULT '',
    has_more   INTEGER NOT NULL DEFAULT 0,     -- older release pages exist that weren't fetched
    fetched_at TEXT    NOT NULL DEFAULT '',
    error      TEXT    NOT NULL DEFAULT ''
);
