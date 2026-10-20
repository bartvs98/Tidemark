-- The floating tag's registry digest at the last successful check: reviews and snoozes of
-- a container without a usable latest version are set against this build.
ALTER TABLE results ADD COLUMN tag_digest TEXT NOT NULL DEFAULT '';
