package notify

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/review"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

// Options configure a Service.
type Options struct {
	Log *slog.Logger     // nil = slog.Default()
	Now func() time.Time // nil = time.Now (tests)
}

// ErrOff is returned by Run and Test while no notifier is configured.
var ErrOff = errors.New("notifications are off")

// Service decides which updates are new and sends them through a Notifier: after each
// check run, or on a digest schedule. It exists also while notifications are off, so
// Configure can turn them on without a restart.
type Service struct {
	st  *store.Store
	opt Options

	run sync.Mutex // one Run or Test at a time

	mu         sync.Mutex
	src        Source
	cfg        setup
	digestCron *cron.Cron  // nil without a digest (or while off)
	stopDigest func() bool // undoes the ctx hook that stops digestCron
}

// setup is what Configure sets. Run and Test take a copy at their start, so a run in
// progress finishes with the setup it began with.
type setup struct {
	n         Notifier // nil = off
	minLevel  analysis.Level
	publicURL string // no trailing "/"; "" = no links
	digest    string // cron spec; "" = after each check run
}

// New returns a Service that is off until Configure gives it a notifier.
func New(st *store.Store, opt Options) *Service {
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	return &Service{st: st, opt: opt}
}

// UseSource sets where the current updates come from (api.Server.Updates).
func (s *Service) UseSource(src Source) {
	s.mu.Lock()
	s.src = src
	s.mu.Unlock()
}

// Configure replaces the notifier (nil = off), the minimum level (updates below it are
// skipped and not remembered), the public URL for links ("" = none) and the digest
// ("" = after each check run; same cron syntax as the check schedule, "CRON_TZ=" allowed).
// The previous digest cron is stopped; a run in progress finishes with the old notifier.
// ctx bounds the digest cron. An invalid digest changes nothing.
func (s *Service) Configure(ctx context.Context, n Notifier, minLevel analysis.Level, publicURL, digest string) error {
	var c *cron.Cron
	if digest != "" {
		c = cron.New()
		if _, err := c.AddFunc(digest, func() { s.runAndLog(ctx, "digest") }); err != nil {
			return fmt.Errorf("schedule %q: %w", digest, err)
		}
		if n == nil {
			c = nil // valid, but there's nothing to send while off
		} else {
			c.Start()
		}
	}
	s.mu.Lock()
	old, oldStop := s.digestCron, s.stopDigest
	s.cfg = setup{n: n, minLevel: minLevel, publicURL: strings.TrimRight(publicURL, "/"), digest: digest}
	s.digestCron, s.stopDigest = c, nil
	if c != nil {
		s.stopDigest = context.AfterFunc(ctx, func() { c.Stop() })
	}
	s.mu.Unlock()
	if old != nil {
		oldStop()
		old.Stop() // only stops the timer; a digest being sent finishes
	}
	return nil
}

// Enabled reports whether a notifier is configured.
func (s *Service) Enabled() bool { return s.setup().n != nil }

func (s *Service) setup() setup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Digest returns the digest spec, "" when sending after each run.
func (s *Service) Digest() string { return s.setup().digest }

// AfterRun is called by the check service when a run finished without a run error; it
// calls Run unless notifications are off or digest mode is on.
func (s *Service) AfterRun(ctx context.Context) {
	if cfg := s.setup(); cfg.n == nil || cfg.digest != "" {
		return
	}
	s.runAndLog(ctx, "check")
}

func (s *Service) runAndLog(ctx context.Context, after string) {
	n, name, err := s.runNamed(ctx)
	if errors.Is(err, ErrOff) {
		return // turned off since this was scheduled
	}
	if err != nil {
		s.opt.Log.Warn("notification failed", "notifier", name, "after", after, "err", err)
		return
	}
	if n > 0 {
		s.opt.Log.Info("notification sent", "notifier", name, "after", after, "items", n)
	}
}

// Run selects the new items, sends one Batch if there are any, logs the attempt and
// records the items on success. It returns the number of items sent. A failed send
// records no items, so the next Run sends them again. While off it returns ErrOff.
func (s *Service) Run(ctx context.Context) (int, error) {
	n, _, err := s.runNamed(ctx)
	return n, err
}

// runNamed is Run that also returns the notifier's name, for logs.
func (s *Service) runNamed(ctx context.Context) (int, string, error) {
	s.run.Lock()
	defer s.run.Unlock()

	s.mu.Lock()
	src, cfg := s.src, s.cfg
	s.mu.Unlock()
	if cfg.n == nil {
		return 0, "", ErrOff
	}
	n, err := s.send(ctx, src, cfg)
	return n, cfg.n.Name(), err
}

func (s *Service) send(ctx context.Context, src Source, cfg setup) (int, error) {
	if src == nil {
		return 0, errors.New("notify: no update source")
	}
	updates, err := src(ctx)
	if err != nil {
		return 0, fmt.Errorf("notify: list updates: %w", err)
	}
	notified, err := s.st.Notified(ctx)
	if err != nil {
		return 0, fmt.Errorf("notify: read notified: %w", err)
	}
	now := s.opt.Now()
	items := cfg.selectItems(updates, notified, now)
	if len(items) == 0 {
		return 0, nil
	}
	sent, err := s.st.SentUpdatesBefore(ctx)
	if err != nil {
		return 0, fmt.Errorf("notify: read notifications: %w", err)
	}
	kind := KindUpdates
	if !sent {
		kind = KindFirst
	}
	sortItems(items)

	sendErr := cfg.n.Send(ctx, Batch{Kind: kind, Items: items, DashboardURL: cfg.dashboardURL()})
	logRow := store.Notification{SentAt: now, Notifier: cfg.n.Name(), Kind: kind, Items: len(items)}
	if sendErr != nil {
		logRow.Error = sendErr.Error()
	}
	if err := s.st.AddNotification(ctx, logRow); err != nil {
		s.opt.Log.Warn("notification log failed", "err", err)
	}
	if sendErr != nil {
		return 0, sendErr
	}
	rows := make([]store.Notified, len(items))
	for i, it := range items {
		rows[i] = store.Notified{ContainerName: it.Container, Key: it.Key, Level: it.Level.String(), NotifiedAt: now}
	}
	if err := s.st.MarkNotified(ctx, rows); err != nil {
		return len(items), fmt.Errorf("notify: record notified: %w", err)
	}
	return len(items), nil
}

// selectItems picks what to send: updates with a key, not under an active review, at or
// above the minimum level, and either never sent, sent with a lower level, or sent before
// a time snooze that has run out since.
func (c setup) selectItems(updates []Update, notified map[store.NotifiedKey]store.Notified, now time.Time) []Item {
	var items []Item
	for _, u := range updates {
		if u.Key == "" || u.Class == version.None || u.Class == "" {
			continue
		}
		if u.Review != nil && review.Active(*u.Review, u.Key, now) {
			continue
		}
		if u.Level < c.minLevel {
			continue // not recorded: when it rises to the threshold later, it's new
		}
		var reason string
		row, seen := notified[store.NotifiedKey{Container: u.Container, Key: u.Key}]
		switch {
		case !seen:
			reason = ReasonNew
		case u.Level > parseLevel(row.Level):
			reason = ReasonRaised
		case snoozeEnded(u.Review, row.NotifiedAt, now):
			reason = ReasonSnoozeEnded
		default:
			continue
		}
		items = append(items, Item{Update: u, Reason: reason, URL: c.detailURL(u.Container)})
	}
	return items
}

// snoozeEnded: a snooze by time ran out at or before now, after the update was sent.
func snoozeEnded(r *store.Review, sentAt, now time.Time) bool {
	if r == nil || r.Kind != review.Snoozed || r.Until.IsZero() {
		return false
	}
	return !r.Until.After(now) && r.Until.After(sentAt)
}

func parseLevel(s string) analysis.Level {
	var l analysis.Level
	_ = l.UnmarshalText([]byte(s)) // unknown stays None
	return l
}

func (c setup) dashboardURL() string {
	if c.publicURL == "" {
		return ""
	}
	return c.publicURL + "/"
}

func (c setup) detailURL(name string) string {
	if c.publicURL == "" {
		return ""
	}
	return c.publicURL + "/containers/" + url.PathEscape(name)
}

// sortItems orders items by stack (stacks by their highest level, highest first, then
// name; the "" stack last), within a stack by level (highest first), then container name.
func sortItems(items []Item) {
	top := map[string]analysis.Level{}
	for _, it := range items {
		top[it.Stack] = max(top[it.Stack], it.Level)
	}
	slices.SortStableFunc(items, func(a, b Item) int {
		if a.Stack != b.Stack {
			if (a.Stack == "") != (b.Stack == "") {
				if a.Stack == "" {
					return 1
				}
				return -1
			}
			if c := cmp.Compare(top[b.Stack], top[a.Stack]); c != 0 {
				return c
			}
			return strings.Compare(a.Stack, b.Stack)
		}
		if c := cmp.Compare(b.Level, a.Level); c != 0 {
			return c
		}
		return strings.Compare(a.Container, b.Container)
	})
}

// Test sends a test Batch and logs it (kind test). While off it returns ErrOff.
func (s *Service) Test(ctx context.Context) error {
	s.run.Lock()
	defer s.run.Unlock()
	cfg := s.setup()
	if cfg.n == nil {
		return ErrOff
	}
	sendErr := cfg.n.Send(ctx, Batch{Kind: KindTest, DashboardURL: cfg.dashboardURL()})
	row := store.Notification{SentAt: s.opt.Now(), Notifier: cfg.n.Name(), Kind: KindTest}
	if sendErr != nil {
		row.Error = sendErr.Error()
	}
	if err := s.st.AddNotification(ctx, row); err != nil {
		s.opt.Log.Warn("notification log failed", "err", err)
	}
	return sendErr
}

// Status is what the settings dialog shows (Notifier "" while off). A store error leaves Last nil.
func (s *Service) Status(ctx context.Context) Status {
	cfg := s.setup()
	st := Status{MinLevel: cfg.minLevel, Digest: cfg.digest, PublicURL: cfg.publicURL}
	if cfg.n != nil {
		st.Notifier = cfg.n.Name()
	}
	last, ok, err := s.st.LastNotification(ctx)
	if err != nil {
		s.opt.Log.Warn("read last notification", "err", err)
	} else if ok {
		st.Last = &last
	}
	return st
}

// Groups splits sorted items into consecutive runs with the same stack (for renderers).
func Groups(items []Item) []Group {
	var out []Group
	for _, it := range items {
		if n := len(out); n > 0 && out[n-1].Stack == it.Stack {
			out[n-1].Items = append(out[n-1].Items, it)
			continue
		}
		out = append(out, Group{Stack: it.Stack, Items: []Item{it}})
	}
	return out
}
