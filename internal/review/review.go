// Package review decides whether a container's stored review state (reviewed, or
// snoozed) still applies to its current update.
package review

import (
	"time"

	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

// Kinds of review.
const (
	Reviewed = "reviewed"
	Snoozed  = "snoozed"
)

// Active reports whether a stored review still applies to an update whose latest version is
// latest, at now. Snoozed by time: now before Until. Version based (reviewed, snoozed by
// version): latest is not newer than r.Version (version.Parse + Compare when both parse,
// otherwise plain string equality).
func Active(r store.Review, latest string, now time.Time) bool {
	if r.Kind == Snoozed && !r.Until.IsZero() {
		return now.Before(r.Until)
	}
	if r.Kind != Reviewed && r.Kind != Snoozed {
		return false
	}
	if r.Version == "" {
		return false
	}
	lv, lok := version.Parse(latest)
	rv, rok := version.Parse(r.Version)
	if lok && rok {
		return version.Compare(lv, rv) <= 0
	}
	return latest == r.Version
}
