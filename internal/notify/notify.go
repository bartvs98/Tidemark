// Package notify sends a message when a check run finds new updates: once per update
// (container + review key), again when its level rises or a time snooze ran out. The
// Notifier interface keeps the delivery (Telegram now) apart from what is sent.
package notify

import (
	"context"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

// Update is one container's current update, as the dashboard shows it.
type Update struct {
	Container string
	Stack     string        // Compose project, "" for none
	Running   string        // "" when unknown
	Latest    string        // "" when unknown (new build of a floating tag)
	Class     version.Class // patch | minor | major | digest (never none)
	Channel   string        // pre-release channel tag (nightly), "" otherwise
	Level     analysis.Level
	Key       string        // review key; updates without one are skipped
	Review    *store.Review // the container's stored review, active or not; nil if none
}

// Source lists the current updates (api.Server.Updates).
type Source func(ctx context.Context) ([]Update, error)

// Why an item is in a message.
const (
	ReasonNew         = "new"
	ReasonRaised      = "raised" // level rose since it was sent
	ReasonSnoozeEnded = "snooze-ended"
)

type Item struct {
	Update
	Reason string
	URL    string // detail page, "" without a public URL
}

// Kinds of message (also notifications.kind in the store).
const (
	KindUpdates = "updates"
	KindFirst   = "first" // first message: everything pending now
	KindTest    = "test"
)

// Batch is one message's content. Notifiers render it in their own format.
// Items are sorted: by stack (stacks ordered by their highest level, highest first, then
// name; the "" stack last), within a stack by level (highest first), then container name.
type Batch struct {
	Kind         string
	Items        []Item // empty for a test
	DashboardURL string // "" without a public URL
}

// Notifier delivers a Batch. Telegram now; ntfy, Discord or webhooks can be added later.
type Notifier interface {
	Name() string // "telegram"
	Send(ctx context.Context, b Batch) error
}

// Group is a run of consecutive items with the same stack.
type Group struct {
	Stack string // "" = no stack
	Items []Item
}

// Status is what the settings dialog shows.
type Status struct {
	Notifier  string // "telegram"
	MinLevel  analysis.Level
	Digest    string // "" = after each run
	PublicURL string
	Last      *store.Notification // newest attempt, nil if none
	NextAt    *time.Time          // when the digest runs next; nil without a digest
}
