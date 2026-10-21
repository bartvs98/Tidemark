package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/notify"
	"github.com/bartvs98/tidemark/internal/store"
)

const (
	secretToken = "123456:SECRET-bot-token"
	secretChat  = "-1009876543"
)

// fakeNotifier holds the secrets a real notifier would, and fails on demand.
type fakeNotifier struct {
	token, chat string
	err         error
	sent        []notify.Batch
}

func (f *fakeNotifier) Name() string { return "telegram" }
func (f *fakeNotifier) Send(_ context.Context, b notify.Batch) error {
	f.sent = append(f.sent, b)
	return f.err
}

type notifyFixture struct {
	h   http.Handler
	srv *Server
	n   *fakeNotifier
	db  *store.Store
}

func newNotifyFixture(t *testing.T, withNotify bool) notifyFixture {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	checks := check.New(src, tagsRegistry{}, db, log)
	checks.Start(t.Context(), check.TriggerManual)
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	f := notifyFixture{n: &fakeNotifier{token: secretToken, chat: secretChat}, db: db}
	deps := Deps{Version: "t", Log: log, Source: src, Checks: checks, Store: db}
	if withNotify {
		deps.Notify = notify.New(db, notify.Options{Log: log})
		if err := deps.Notify.Configure(t.Context(), f.n, analysis.Attention, "https://tidemark.example.lan", ""); err != nil {
			t.Fatal(err)
		}
	}
	f.srv = New(deps)
	f.h = f.srv.Handler(fstest.MapFS{})
	return f
}

func TestUpdates(t *testing.T) {
	f := newNotifyFixture(t, false)
	review := store.Review{ContainerName: "jellyfin", Kind: "reviewed", Version: "10.10.7", CreatedAt: time.Now().UTC()}
	if err := f.db.SaveReview(t.Context(), review); err != nil {
		t.Fatal(err)
	}
	ups, err := f.srv.Updates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, u := range ups {
		if u.Class == "none" || u.Class == "" {
			t.Errorf("update with class %q: %+v", u.Class, u)
		}
		if u.Container != "jellyfin" {
			if u.Review != nil {
				t.Errorf("%s has a review: %+v", u.Container, u.Review)
			}
			continue
		}
		found = true
		if u.Running != "10.9.11" || u.Latest != "10.10.7" || u.Class != "minor" || u.Key != "10.10.7" {
			t.Errorf("jellyfin = %+v", u)
		}
		if u.Review == nil || u.Review.Kind != "reviewed" || u.Review.Version != "10.10.7" {
			t.Errorf("jellyfin review = %+v", u.Review)
		}
	}
	if !found {
		t.Fatalf("no jellyfin update in %+v", ups)
	}
}

func TestUpdatesSourceError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Deps{Log: log, Source: failingSource{}})
	if _, err := srv.Updates(t.Context()); err == nil {
		t.Error("expected the source error")
	}
}

func TestTestNotification(t *testing.T) {
	f := newNotifyFixture(t, true)
	if rec := send(t, f.h, http.MethodPost, "/api/notifications/test", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if len(f.n.sent) != 1 || f.n.sent[0].Kind != notify.KindTest {
		t.Errorf("sent = %+v", f.n.sent)
	}
}

func TestTestNotificationFails(t *testing.T) {
	f := newNotifyFixture(t, true)
	f.n.err = errors.New("telegram: Bad Request: chat not found")
	rec := send(t, f.h, http.MethodPost, "/api/notifications/test", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "chat not found") {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestTestNotificationNotConfigured(t *testing.T) {
	f := newNotifyFixture(t, false)
	rec := send(t, f.h, http.MethodPost, "/api/notifications/test", "")
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "notifications are not configured") {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
}

// A service that's off (no notifier configured, or turned off in the dialog) answers 409.
func TestTestNotificationOff(t *testing.T) {
	f := newNotifyFixture(t, true)
	if err := f.srv.notify.Configure(t.Context(), nil, analysis.None, "", ""); err != nil {
		t.Fatal(err)
	}
	rec := send(t, f.h, http.MethodPost, "/api/notifications/test", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"notifications are off"`) {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	if len(f.n.sent) != 0 {
		t.Errorf("sent while off: %+v", f.n.sent)
	}
	// Settings show no notifications while off.
	if rec := do(t, f.h, "/api/settings"); strings.Contains(rec.Body.String(), `"notifications"`) {
		t.Errorf("notifications present while off: %s", rec.Body)
	}
}

func TestTestNotificationNeedsHeader(t *testing.T) {
	f := newNotifyFixture(t, true)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/notifications/test", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d", rec.Code)
	}
	if len(f.n.sent) != 0 {
		t.Errorf("sent without the header: %+v", f.n.sent)
	}
}

func TestSettingsNotifications(t *testing.T) {
	off := newNotifyFixture(t, false)
	rec := do(t, off.h, "/api/settings")
	if strings.Contains(rec.Body.String(), "notifications") {
		t.Errorf("notifications present while off: %s", rec.Body)
	}

	on := newNotifyFixture(t, true)
	rec = do(t, on.h, "/api/settings")
	var body struct {
		Notifications *notificationsJSON `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Notifications == nil {
		t.Fatalf("no notifications in %s", rec.Body)
	}
	for _, secret := range []string{secretToken, secretChat} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("settings contain %q: %s", secret, rec.Body)
		}
	}
}

func TestToNotificationsJSON(t *testing.T) {
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	got := toNotificationsJSON(notify.Status{
		Notifier: "telegram", MinLevel: analysis.Breaking, Digest: "0 9 * * *", PublicURL: "https://x.lan",
		Last: &store.Notification{SentAt: at, Kind: "test", Items: 0, Error: "boom"},
	})
	b, _ := json.Marshal(got)
	want := `{"notifier":"telegram","level":"breaking","digest":"0 9 * * *","publicUrl":"https://x.lan","last":{"at":"2026-10-21T09:00:00Z","kind":"test","items":0,"error":"boom"}}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	b, _ = json.Marshal(toNotificationsJSON(notify.Status{Notifier: "telegram"}))
	if string(b) != `{"notifier":"telegram","level":"all"}` {
		t.Errorf("minimal = %s", b)
	}
}
