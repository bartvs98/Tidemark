package notify

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/review"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

type fakeNotifier struct {
	batches []Batch
	err     error
}

func (f *fakeNotifier) Name() string { return "telegram" }
func (f *fakeNotifier) Send(_ context.Context, b Batch) error {
	if f.err != nil {
		return f.err
	}
	f.batches = append(f.batches, b)
	return nil
}

type harness struct {
	t       *testing.T
	svc     *Service
	st      *store.Store
	n       *fakeNotifier
	now     time.Time
	updates []Update
}

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// opts is what the harness passes to Configure.
type opts struct {
	MinLevel  analysis.Level
	PublicURL string
}

func newHarness(t *testing.T, o opts) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "tidemark.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := &harness{t: t, st: st, n: &fakeNotifier{}, now: t0}
	h.svc = New(st, Options{Now: func() time.Time { return h.now }})
	if err := h.svc.Configure(t.Context(), h.n, o.MinLevel, o.PublicURL, ""); err != nil {
		t.Fatal(err)
	}
	h.svc.UseSource(func(context.Context) ([]Update, error) { return slices.Clone(h.updates), nil })
	return h
}

// run runs once and returns the containers sent with their reasons ("name:reason").
func (h *harness) run() []string {
	h.t.Helper()
	before := len(h.n.batches)
	if _, err := h.svc.Run(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	if len(h.n.batches) == before {
		return nil
	}
	var out []string
	for _, it := range h.n.batches[len(h.n.batches)-1].Items {
		out = append(out, it.Container+":"+it.Reason)
	}
	return out
}

func (h *harness) lastKind() string { return h.n.batches[len(h.n.batches)-1].Kind }

func upd(name, latest string, lvl analysis.Level) Update {
	return Update{Container: name, Running: "1.0.0", Latest: latest, Class: version.Minor, Level: lvl, Key: latest}
}

func want(t *testing.T, got []string, w ...string) {
	t.Helper()
	if !slices.Equal(got, w) {
		t.Errorf("sent %q, want %q", got, w)
	}
}

func TestRunNewOnceThenNothing(t *testing.T) {
	h := newHarness(t, opts{})
	h.updates = []Update{upd("sonarr", "1.1.0", analysis.None)}
	want(t, h.run(), "sonarr:new")
	if h.lastKind() != KindFirst {
		t.Errorf("first message kind = %s", h.lastKind())
	}
	want(t, h.run()) // already sent

	// A newer latest is a new key: new again, now as a normal update message.
	h.updates = []Update{upd("sonarr", "1.2.0", analysis.None), upd("radarr", "2.0.0", analysis.None)}
	want(t, h.run(), "radarr:new", "sonarr:new")
	if h.lastKind() != KindUpdates {
		t.Errorf("kind = %s, want updates", h.lastKind())
	}
}

func TestRunSkips(t *testing.T) {
	h := newHarness(t, opts{})
	noKey := upd("a", "1.1.0", analysis.None)
	noKey.Key = ""
	noClass := upd("b", "1.1.0", analysis.None)
	noClass.Class = version.None
	reviewed := upd("c", "1.1.0", analysis.Breaking)
	reviewed.Review = &store.Review{ContainerName: "c", Kind: review.Reviewed, Version: "1.1.0"}
	snoozed := upd("d", "1.1.0", analysis.None)
	snoozed.Review = &store.Review{ContainerName: "d", Kind: review.Snoozed, Until: t0.Add(time.Hour)}
	staleReview := upd("e", "1.2.0", analysis.None) // reviewed up to an older version: not active
	staleReview.Review = &store.Review{ContainerName: "e", Kind: review.Reviewed, Version: "1.1.0"}
	h.updates = []Update{noKey, noClass, reviewed, snoozed, staleReview}
	want(t, h.run(), "e:new")
}

func TestRunMinLevel(t *testing.T) {
	h := newHarness(t, opts{MinLevel: analysis.Attention})
	h.updates = []Update{upd("quiet", "1.1.0", analysis.None), upd("loud", "1.1.0", analysis.Attention)}
	want(t, h.run(), "loud:new")
	got, _ := h.st.Notified(context.Background())
	if _, ok := got[store.NotifiedKey{Container: "quiet", Key: "1.1.0"}]; ok {
		t.Error("an update below the minimum level was recorded")
	}
	// It rises to the threshold: it's new, not raised.
	h.updates[0].Level = analysis.Attention
	want(t, h.run(), "quiet:new")
}

func TestRunRaised(t *testing.T) {
	h := newHarness(t, opts{})
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.None)}
	want(t, h.run(), "sonarr:new")
	h.updates[0].Level = analysis.Breaking
	want(t, h.run(), "sonarr:raised")
	want(t, h.run())
	h.updates[0].Level = analysis.Attention // lower: nothing
	want(t, h.run())
}

func TestRunSnoozeEnded(t *testing.T) {
	h := newHarness(t, opts{})
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.None)}
	want(t, h.run(), "sonarr:new")

	until := t0.Add(24 * time.Hour)
	h.updates[0].Review = &store.Review{ContainerName: "sonarr", Kind: review.Snoozed, Until: until}
	h.now = t0.Add(time.Hour)
	want(t, h.run()) // snoozed
	h.now = until    // ran out
	want(t, h.run(), "sonarr:snooze-ended")
	h.now = until.Add(time.Hour)
	want(t, h.run()) // once, not again
}

// A snooze that ran out before the update was sent doesn't count as ended since.
func TestRunSnoozeEndedBeforeSent(t *testing.T) {
	h := newHarness(t, opts{})
	u := upd("sonarr", "2.0.0", analysis.None)
	u.Review = &store.Review{ContainerName: "sonarr", Kind: review.Snoozed, Until: t0.Add(-time.Hour)}
	h.updates = []Update{u}
	want(t, h.run(), "sonarr:new")
	h.now = t0.Add(time.Hour)
	want(t, h.run())
}

func TestRunFailedSendRetries(t *testing.T) {
	h := newHarness(t, opts{})
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.None)}
	h.n.err = errors.New("telegram: Bad Request: chat not found")
	if n, err := h.svc.Run(context.Background()); err == nil || n != 0 {
		t.Fatalf("run = %d, %v; want the send error", n, err)
	}
	ctx := context.Background()
	if got, _ := h.st.Notified(ctx); len(got) != 0 {
		t.Errorf("a failed send recorded %v", got)
	}
	last, ok, _ := h.st.LastNotification(ctx)
	if !ok || last.Error != "telegram: Bad Request: chat not found" || last.Kind != KindFirst || last.Items != 1 {
		t.Errorf("logged %+v", last)
	}
	h.n.err = nil
	want(t, h.run(), "sonarr:new")
	if h.lastKind() != KindFirst {
		t.Errorf("after a failed first message the retry is still the first, got %s", h.lastKind())
	}
}

func TestNothingNewLogsNothing(t *testing.T) {
	h := newHarness(t, opts{})
	want(t, h.run())
	if _, ok, _ := h.st.LastNotification(context.Background()); ok {
		t.Error("an empty run was logged")
	}
}

func TestAfterRunAndDigest(t *testing.T) {
	h := newHarness(t, opts{})
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.None)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if h.svc.Digest() != "" {
		t.Error("digest set without one")
	}
	if err := h.svc.Configure(ctx, h.n, analysis.None, "", "not a cron"); err == nil {
		t.Error("invalid spec accepted")
	}
	if h.svc.Digest() != "" {
		t.Error("a failed Configure turned on digest mode")
	}
	if err := h.svc.Configure(ctx, h.n, analysis.None, "", "CRON_TZ=Europe/Amsterdam 0 9 * * *"); err != nil {
		t.Fatal(err)
	}
	if h.svc.Digest() != "CRON_TZ=Europe/Amsterdam 0 9 * * *" {
		t.Errorf("digest = %q", h.svc.Digest())
	}
	h.svc.AfterRun(ctx)
	if len(h.n.batches) != 0 {
		t.Error("AfterRun sent in digest mode")
	}

	h2 := newHarness(t, opts{})
	h2.updates = h.updates
	h2.svc.AfterRun(ctx)
	if len(h2.n.batches) != 1 {
		t.Errorf("AfterRun sent %d batches, want 1", len(h2.n.batches))
	}
}

// Configure replaces the digest: the old cron is stopped, back to after-each-run sends.
func TestConfigureReplacesDigest(t *testing.T) {
	h := newHarness(t, opts{})
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.None)}
	ctx := t.Context()
	if err := h.svc.Configure(ctx, h.n, analysis.None, "", "@every 1h"); err != nil {
		t.Fatal(err)
	}
	first := h.svc.digestCron
	if err := h.svc.Configure(ctx, h.n, analysis.None, "", "@every 2h"); err != nil {
		t.Fatal(err)
	}
	if h.svc.digestCron == first || h.svc.Digest() != "@every 2h" {
		t.Errorf("digest not replaced: %q", h.svc.Digest())
	}
	if err := h.svc.Configure(ctx, h.n, analysis.None, "", ""); err != nil {
		t.Fatal(err)
	}
	if h.svc.digestCron != nil || h.svc.Digest() != "" {
		t.Errorf("digest still on: %q", h.svc.Digest())
	}
	h.svc.AfterRun(ctx)
	if len(h.n.batches) != 1 {
		t.Errorf("AfterRun sent %d batches, want 1", len(h.n.batches))
	}
}

func TestOff(t *testing.T) {
	h := newHarness(t, opts{MinLevel: analysis.Breaking, PublicURL: "https://t.example"})
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.Breaking)}
	ctx := t.Context()
	if !h.svc.Enabled() {
		t.Fatal("not enabled after Configure")
	}
	if err := h.svc.Configure(ctx, nil, analysis.None, "", "@every 1h"); err != nil {
		t.Fatal(err)
	}
	if h.svc.Enabled() || h.svc.digestCron != nil {
		t.Errorf("enabled %v, digest cron %v", h.svc.Enabled(), h.svc.digestCron)
	}
	h.svc.AfterRun(ctx)
	if _, err := h.svc.Run(ctx); !errors.Is(err, ErrOff) {
		t.Errorf("Run err = %v", err)
	}
	if err := h.svc.Test(ctx); !errors.Is(err, ErrOff) {
		t.Errorf("Test err = %v", err)
	}
	if len(h.n.batches) != 0 {
		t.Errorf("sent while off: %+v", h.n.batches)
	}
	if st := h.svc.Status(ctx); st.Notifier != "" || st.Last != nil {
		t.Errorf("status = %+v", st)
	}
	// New services start off.
	if New(h.st, Options{}).Enabled() {
		t.Error("a new service is enabled")
	}
	// And on again, with the new level.
	if err := h.svc.Configure(ctx, h.n, analysis.None, "", ""); err != nil {
		t.Fatal(err)
	}
	h.updates = []Update{upd("radarr", "2.0.0", analysis.None)}
	want(t, h.run(), "radarr:new")
}

func TestSortAndGroups(t *testing.T) {
	h := newHarness(t, opts{})
	stack := func(u Update, s string) Update { u.Stack = s; return u }
	h.updates = []Update{
		stack(upd("zeta", "1.1.0", analysis.None), ""),
		stack(upd("alpha", "1.1.0", analysis.Breaking), ""), // breaking, but the "" stack stays last
		stack(upd("b2", "1.1.0", analysis.None), "b"),
		stack(upd("b1", "1.1.0", analysis.None), "b"),
		stack(upd("a1", "1.1.0", analysis.None), "a"),
		stack(upd("m2", "1.1.0", analysis.None), "media"),
		stack(upd("m1", "1.1.0", analysis.Attention), "media"),
		stack(upd("n1", "1.1.0", analysis.Breaking), "net"),
	}
	want(t, h.run(), "n1:new", "m1:new", "m2:new", "a1:new", "b1:new", "b2:new", "alpha:new", "zeta:new")
	var stacks []string
	for _, g := range Groups(h.n.batches[0].Items) {
		stacks = append(stacks, g.Stack)
	}
	if !slices.Equal(stacks, []string{"net", "media", "a", "b", ""}) {
		t.Errorf("groups = %q", stacks)
	}
	if Groups(nil) != nil {
		t.Error("Groups(nil) != nil")
	}
}

func TestURLs(t *testing.T) {
	h := newHarness(t, opts{PublicURL: "https://tidemark.example.lan/"})
	h.updates = []Update{upd("my app/1", "1.1.0", analysis.None)}
	h.run()
	b := h.n.batches[0]
	if b.DashboardURL != "https://tidemark.example.lan/" {
		t.Errorf("dashboard = %q", b.DashboardURL)
	}
	if got := b.Items[0].URL; got != "https://tidemark.example.lan/containers/my%20app%2F1" {
		t.Errorf("detail = %q", got)
	}

	h2 := newHarness(t, opts{})
	h2.updates = h.updates
	h2.run()
	if b := h2.n.batches[0]; b.DashboardURL != "" || b.Items[0].URL != "" {
		t.Errorf("links without a public URL: %+v", b)
	}
}

func TestTestAndStatus(t *testing.T) {
	h := newHarness(t, opts{MinLevel: analysis.Attention, PublicURL: "https://t.example"})
	ctx := context.Background()
	if st := h.svc.Status(ctx); st.Last != nil || st.Notifier != "telegram" || st.MinLevel != analysis.Attention || st.PublicURL != "https://t.example" || st.Digest != "" {
		t.Errorf("status = %+v", st)
	}
	if err := h.svc.Test(ctx); err != nil {
		t.Fatal(err)
	}
	if b := h.n.batches[0]; b.Kind != KindTest || len(b.Items) != 0 || b.DashboardURL != "https://t.example/" {
		t.Errorf("test batch = %+v", b)
	}
	h.n.err = errors.New("telegram: Unauthorized")
	if err := h.svc.Test(ctx); err == nil {
		t.Error("failed test returned nil")
	}
	st := h.svc.Status(ctx)
	if st.Last == nil || st.Last.Kind != KindTest || st.Last.Error != "telegram: Unauthorized" {
		t.Errorf("last = %+v", st.Last)
	}
	// A test doesn't count as the first message.
	h.n.err = nil
	h.updates = []Update{upd("sonarr", "2.0.0", analysis.Breaking)}
	h.run()
	if h.lastKind() != KindFirst {
		t.Errorf("kind after tests = %s, want first", h.lastKind())
	}
}
