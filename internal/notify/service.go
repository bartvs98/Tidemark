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
	MinLevel  analysis.Level   // updates below this level are skipped (and not remembered)
	PublicURL string           // base for links, "" for no links
	Log       *slog.Logger     // nil = slog.Default()
	Now       func() time.Time // nil = time.Now (tests)
}

// Service decides which updates are new and sends them through a Notifier: after each
// check run, or on a digest schedule.
type Service struct {
	n   Notifier
	st  *store.Store
	opt Options

	run sync.Mutex // one Run or Test at a time

	mu     sync.Mutex
	src    Source
	digest string
}

func New(n Notifier, st *store.Store, opt Options) *Service {
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	opt.PublicURL = strings.TrimRight(opt.PublicURL, "/")
	return &Service{n: n, st: st, opt: opt}
}

// UseSource sets where the current updates come from (api.Server.Updates).
func (s *Service) UseSource(src Source) {
	s.mu.Lock()
	s.src = src
	s.mu.Unlock()
}

// Schedule turns on digest mode: Run on a cron spec instead of after each check run.
// Same syntax as the check schedule ("CRON_TZ=" allowed); stopped when ctx ends.
func (s *Service) Schedule(ctx context.Context, spec string) error {
	c := cron.New()
	if _, err := c.AddFunc(spec, func() { s.runAndLog(ctx, "digest") }); err != nil {
		return fmt.Errorf("schedule %q: %w", spec, err)
	}
	s.mu.Lock()
	s.digest = spec
	s.mu.Unlock()
	c.Start()
	go func() {
		<-ctx.Done()
		c.Stop()
	}()
	return nil
}

// Digest returns the digest spec, "" when sending after each run.
func (s *Service) Digest() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.digest
}

// AfterRun is called by the check service when a run finished without a run error; it
// calls Run unless digest mode is on.
func (s *Service) AfterRun(ctx context.Context) {
	if s.Digest() != "" {
		return
	}
	s.runAndLog(ctx, "check")
}

func (s *Service) runAndLog(ctx context.Context, after string) {
	n, err := s.Run(ctx)
	if err != nil {
		s.opt.Log.Warn("notification failed", "notifier", s.n.Name(), "after", after, "err", err)
		return
	}
	if n > 0 {
		s.opt.Log.Info("notification sent", "notifier", s.n.Name(), "after", after, "items", n)
	}
}

// Run selects the new items, sends one Batch if there are any, logs the attempt and
// records the items on success. It returns the number of items sent. A failed send
// records no items, so the next Run sends them again.
func (s *Service) Run(ctx context.Context) (int, error) {
	s.run.Lock()
	defer s.run.Unlock()

	s.mu.Lock()
	src := s.src
	s.mu.Unlock()
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
	items := s.selectItems(updates, notified, now)
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

	sendErr := s.n.Send(ctx, Batch{Kind: kind, Items: items, DashboardURL: s.dashboardURL()})
	logRow := store.Notification{SentAt: now, Notifier: s.n.Name(), Kind: kind, Items: len(items)}
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
func (s *Service) selectItems(updates []Update, notified map[store.NotifiedKey]store.Notified, now time.Time) []Item {
	var items []Item
	for _, u := range updates {
		if u.Key == "" || u.Class == version.None || u.Class == "" {
			continue
		}
		if u.Review != nil && review.Active(*u.Review, u.Key, now) {
			continue
		}
		if u.Level < s.opt.MinLevel {
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
		items = append(items, Item{Update: u, Reason: reason, URL: s.detailURL(u.Container)})
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

func (s *Service) dashboardURL() string {
	if s.opt.PublicURL == "" {
		return ""
	}
	return s.opt.PublicURL + "/"
}

func (s *Service) detailURL(name string) string {
	if s.opt.PublicURL == "" {
		return ""
	}
	return s.opt.PublicURL + "/containers/" + url.PathEscape(name)
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

// Test sends a test Batch and logs it (kind test).
func (s *Service) Test(ctx context.Context) error {
	s.run.Lock()
	defer s.run.Unlock()
	sendErr := s.n.Send(ctx, Batch{Kind: KindTest, DashboardURL: s.dashboardURL()})
	row := store.Notification{SentAt: s.opt.Now(), Notifier: s.n.Name(), Kind: KindTest}
	if sendErr != nil {
		row.Error = sendErr.Error()
	}
	if err := s.st.AddNotification(ctx, row); err != nil {
		s.opt.Log.Warn("notification log failed", "err", err)
	}
	return sendErr
}

// Status is what the settings dialog shows. A store error leaves Last nil.
func (s *Service) Status(ctx context.Context) Status {
	st := Status{Notifier: s.n.Name(), MinLevel: s.opt.MinLevel, Digest: s.Digest(), PublicURL: s.opt.PublicURL}
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
