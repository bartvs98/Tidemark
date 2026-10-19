-- error_kind classifies a failed check (rate-limit, not-found, timeout, network,
-- registry; '' when unclassified or without an error). tags_from is set when a check
-- succeeded on a tag list kept from an earlier one, because the registry rate-limited
-- the refresh: when that list was fetched ('' normally).
ALTER TABLE results ADD COLUMN error_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE results ADD COLUMN tags_from TEXT NOT NULL DEFAULT '';
