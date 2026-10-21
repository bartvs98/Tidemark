package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/version"
)

const testToken = "123456:SECRET-token"

type sentMessage struct {
	Path               string
	ChatID             string `json:"chat_id"`
	Text               string `json:"text"`
	ParseMode          string `json:"parse_mode"`
	LinkPreviewOptions struct {
		IsDisabled bool `json:"is_disabled"`
	} `json:"link_preview_options"`
}

func telegramServer(t *testing.T, status int, body string) (*httptest.Server, *[]sentMessage) {
	t.Helper()
	var got []sentMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m sentMessage
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("body: %v", err)
		}
		m.Path = r.URL.Path
		got = append(got, m)
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func sampleBatch() Batch {
	return Batch{
		Kind:         KindUpdates,
		DashboardURL: "https://tidemark.example.lan/",
		Items: []Item{
			{Update: Update{Container: "sonarr", Stack: "media", Running: "4.0.1", Latest: "5.0.0", Class: version.Major, Level: analysis.Breaking, Key: "5.0.0"},
				Reason: ReasonNew, URL: "https://tidemark.example.lan/containers/sonarr"},
			{Update: Update{Container: "radarr", Stack: "media", Running: "1.2.3", Latest: "1.3.0", Class: version.Minor, Level: analysis.Attention, Key: "1.3.0"},
				Reason: ReasonRaised, URL: "https://tidemark.example.lan/containers/radarr"},
			{Update: Update{Container: "prowlarr", Stack: "media", Running: "1.2.3", Latest: "1.2.4", Class: version.Patch, Key: "1.2.4"},
				Reason: ReasonSnoozeEnded, URL: "https://tidemark.example.lan/containers/prowlarr"},
			{Update: Update{Container: "whoami", Running: "latest", Class: version.Digest, Key: "sha256:abc"},
				Reason: ReasonNew, URL: "https://tidemark.example.lan/containers/whoami"},
			{Update: Update{Container: "app", Latest: "2.1.0", Channel: "nightly", Class: version.Digest, Key: "sha256:def"},
				Reason: ReasonNew},
		},
	}
}

func TestTelegramSend(t *testing.T) {
	srv, got := telegramServer(t, 200, `{"ok":true,"result":{}}`)
	tg := NewTelegram(testToken, "-1001", WithTelegramAPI(srv.URL+"/"))
	if err := tg.Send(context.Background(), sampleBatch()); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("sent %d messages", len(*got))
	}
	m := (*got)[0]
	if m.Path != "/bot"+testToken+"/sendMessage" || m.ChatID != "-1001" || m.ParseMode != "HTML" || !m.LinkPreviewOptions.IsDisabled {
		t.Errorf("request = %+v", m)
	}
	wantText := `<b>Tidemark · 5 new updates · 1 breaking · 1 needs attention</b>

<b>media</b>
🔴 Breaking · <a href="https://tidemark.example.lan/containers/sonarr">sonarr</a> <code>4.0.1</code> → <code>5.0.0</code> major
🟡 Attention · <a href="https://tidemark.example.lan/containers/radarr">radarr</a> <code>1.2.3</code> → <code>1.3.0</code> minor · level raised
· <a href="https://tidemark.example.lan/containers/prowlarr">prowlarr</a> <code>1.2.3</code> → <code>1.2.4</code> patch · snooze ended

<b>No stack</b>
· <a href="https://tidemark.example.lan/containers/whoami">whoami</a> new build of <code>latest</code>
· app new build of <code>nightly</code>

<a href="https://tidemark.example.lan/">Open Tidemark</a>`
	if m.Text != wantText {
		t.Errorf("text =\n%s\nwant\n%s", m.Text, wantText)
	}
}

func TestTelegramTitles(t *testing.T) {
	first := sampleBatch()
	first.Kind = KindFirst
	first.Items = first.Items[:1]
	if got := firstLine(renderTelegram(first)); got != "<b>Tidemark · 1 update pending · 1 breaking</b>" {
		t.Errorf("first title = %s", got)
	}
	// The first message doesn't say why each item is in it: everything pending is.
	first.Items = sampleBatch().Items[1:2]
	if s := join(renderTelegram(first)); strings.Contains(s, "raised") {
		t.Errorf("first message has a reason: %s", s)
	}
	test := join(renderTelegram(Batch{Kind: KindTest}))
	if test != "<b>Tidemark · Test message</b>\nNotifications work. You'll get a message here when a check finds new updates." {
		t.Errorf("test = %s", test)
	}
}

func TestTelegramEscapesHTML(t *testing.T) {
	b := Batch{Kind: KindUpdates, DashboardURL: `https://t.example/?a=1&b="2"`, Items: []Item{{
		Update: Update{Container: "<x>&co", Stack: "a<b>", Running: "1<2", Latest: "2&3", Class: version.Minor, Key: "k"},
		URL:    `https://t.example/containers/"q"`, Reason: ReasonNew,
	}}}
	s := join(renderTelegram(b))
	for _, w := range []string{"<b>a&lt;b&gt;</b>", "&lt;x&gt;&amp;co</a>", "<code>1&lt;2</code>", "<code>2&amp;3</code>",
		`href="https://t.example/containers/&#34;q&#34;"`, `href="https://t.example/?a=1&amp;b=&#34;2&#34;"`} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %s in\n%s", w, s)
		}
	}
	if strings.Contains(s, "<x>") || strings.Contains(s, "a<b>") {
		t.Errorf("unescaped text in\n%s", s)
	}
}

func TestTelegramSplitsLongMessages(t *testing.T) {
	var items []Item
	for s := 0; s < 30; s++ {
		for i := 0; i < 10; i++ {
			items = append(items, Item{Update: Update{
				Container: fmt.Sprintf("container-%02d-%02d", s, i), Stack: fmt.Sprintf("stack-%02d", s),
				Running: "1.0.0", Latest: "1.1.0", Class: version.Minor, Level: analysis.Breaking, Key: "1.1.0"},
				URL: fmt.Sprintf("https://tidemark.example.lan/containers/container-%02d-%02d", s, i)})
		}
	}
	srv, got := telegramServer(t, 200, `{"ok":true}`)
	tg := NewTelegram(testToken, "1", WithTelegramAPI(srv.URL))
	if err := tg.Send(context.Background(), Batch{Kind: KindUpdates, Items: items, DashboardURL: "https://tidemark.example.lan/"}); err != nil {
		t.Fatal(err)
	}
	msgs := *got
	if len(msgs) < 2 {
		t.Fatalf("sent %d messages, want several", len(msgs))
	}
	var all strings.Builder
	for i, m := range msgs {
		if n := textLen(m.Text); n > maxTelegramText {
			t.Errorf("message %d is %d long", i, n)
		}
		if hasTitle := strings.HasPrefix(m.Text, "<b>Tidemark · "); hasTitle != (i == 0) {
			t.Errorf("message %d: title = %v", i, hasTitle)
		}
		if hasLink := strings.HasSuffix(m.Text, "Open Tidemark</a>"); hasLink != (i == len(msgs)-1) {
			t.Errorf("message %d: dashboard link = %v", i, hasLink)
		}
		// Splits fall between groups: every message after the first starts with a heading.
		if i > 0 && !strings.HasPrefix(m.Text, "<b>stack-") {
			t.Errorf("message %d starts mid-group: %.40s", i, m.Text)
		}
		all.WriteString(m.Text)
	}
	for _, it := range items {
		if !strings.Contains(all.String(), ">"+it.Container+"</a>") {
			t.Errorf("%s missing", it.Container)
		}
	}
}

func TestSplitOneLongGroupAtLines(t *testing.T) {
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, strings.Repeat("x", 30))
	}
	blocks := []telegramBlock{{text: "title"}, {sep: "\n\n", text: "<b>big</b>\n" + strings.Join(lines, "\n")}, {sep: "\n\n", text: "link"}}
	msgs := splitMessage(blocks, 200)
	if len(msgs) < 5 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if !strings.HasPrefix(msgs[0], "title\n\n<b>big</b>\n") || !strings.HasSuffix(msgs[len(msgs)-1], "\n\nlink") {
		t.Errorf("first %q, last %q", msgs[0], msgs[len(msgs)-1])
	}
	total := 0
	for _, m := range msgs {
		if len(m) > 200 {
			t.Errorf("%d > 200", len(m))
		}
		total += strings.Count(m, strings.Repeat("x", 30))
	}
	if total != 50 {
		t.Errorf("lines = %d, want 50", total)
	}
}

func TestTelegramErrorsHideToken(t *testing.T) {
	ctx := context.Background()
	b := Batch{Kind: KindTest}
	check := func(name string, err error, want string) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		if strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: token in error: %v", name, err)
		}
		if want != "" && err.Error() != want {
			t.Errorf("%s: error = %q, want %q", name, err, want)
		}
	}

	srv, _ := telegramServer(t, 400, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
	check("telegram error", NewTelegram(testToken, "1", WithTelegramAPI(srv.URL)).Send(ctx, b), "telegram: Bad Request: chat not found")

	srv, _ = telegramServer(t, 502, `<html>bad gateway</html>`)
	check("non-JSON", NewTelegram(testToken, "1", WithTelegramAPI(srv.URL)).Send(ctx, b), "telegram: HTTP 502")

	srv, _ = telegramServer(t, 200, `{"ok":false,"description":"Forbidden: bot was blocked by the user"}`)
	check("ok false", NewTelegram(testToken, "1", WithTelegramAPI(srv.URL)).Send(ctx, b), "telegram: Forbidden: bot was blocked by the user")

	// A description that echoes the token is masked too.
	srv, _ = telegramServer(t, 401, `{"ok":false,"description":"Unauthorized: `+testToken+`"}`)
	check("echoed token", NewTelegram(testToken, "1", WithTelegramAPI(srv.URL)).Send(ctx, b), "telegram: Unauthorized: ***")

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	err := NewTelegram(testToken, "1", WithTelegramAPI(closed.URL)).Send(ctx, b)
	check("closed server", err, "")
	if !strings.HasPrefix(err.Error(), "telegram: send: ") {
		t.Errorf("closed server: %v", err)
	}

	check("bad base", NewTelegram(testToken, "1", WithTelegramAPI("http://[::1")).Send(ctx, b), "")
}

func firstLine(blocks []telegramBlock) string { return strings.SplitN(join(blocks), "\n", 2)[0] }

func join(blocks []telegramBlock) string {
	return strings.Join(splitMessage(blocks, 1<<20), "")
}
