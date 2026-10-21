package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/version"
)

// maxTelegramText is Telegram's limit for one message. It counts characters after
// parsing the HTML; counting the raw HTML in UTF-16 units stays safely under it.
const maxTelegramText = 4096

// Telegram sends batches through a Telegram bot (sendMessage, HTML parse mode).
type Telegram struct {
	token, chatID string
	api           string
	client        *http.Client
}

type TelegramOption func(*Telegram)

// WithTelegramAPI sets the Bot API base URL (default "https://api.telegram.org").
func WithTelegramAPI(base string) TelegramOption {
	return func(t *Telegram) { t.api = strings.TrimRight(base, "/") }
}

// WithHTTPClient sets the HTTP client (default: 15 s timeout).
func WithHTTPClient(c *http.Client) TelegramOption {
	return func(t *Telegram) { t.client = c }
}

func NewTelegram(token, chatID string, opts ...TelegramOption) *Telegram {
	t := &Telegram{token: token, chatID: chatID, api: "https://api.telegram.org", client: &http.Client{Timeout: 15 * time.Second}}
	for _, o := range opts {
		o(t)
	}
	return t
}

func (t *Telegram) Name() string { return "telegram" }

// Send renders the batch and sends it, split into several messages when it's too long.
func (t *Telegram) Send(ctx context.Context, b Batch) error {
	for _, text := range splitMessage(renderTelegram(b), maxTelegramText) {
		if err := t.send(ctx, text); err != nil {
			return err
		}
	}
	return nil
}

func (t *Telegram) send(ctx context.Context, text string) error {
	body, _ := json.Marshal(map[string]any{
		"chat_id":              t.chatID,
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": map[string]bool{"is_disabled": true},
	})
	// The token is part of the URL: no error below may include the URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.api+"/bot"+t.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return t.clean(fmt.Errorf("telegram: build request: %w", unwrapURL(err)))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return t.clean(fmt.Errorf("telegram: send: %w", unwrapURL(err)))
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	jsonErr := json.Unmarshal(raw, &out)
	if resp.StatusCode/100 == 2 && jsonErr == nil && out.OK {
		return nil
	}
	if out.Description != "" {
		return t.clean(fmt.Errorf("telegram: %s", out.Description))
	}
	return t.clean(fmt.Errorf("telegram: HTTP %d", resp.StatusCode))
}

// unwrapURL drops the *url.Error wrapper, whose message contains the URL (and so the token).
func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// clean replaces the token in an error message, in case it slipped in anyway.
func (t *Telegram) clean(err error) error {
	if t.token == "" || !strings.Contains(err.Error(), t.token) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), t.token, "***"))
}

// ── Rendering ────────────────────────────────────────────

// telegramBlock is a piece of a message: sep joins it to the previous piece when both end
// up in the same message.
type telegramBlock struct {
	sep, text string
}

// renderTelegram renders a batch as message blocks: the title, one block per stack
// group, the dashboard link.
func renderTelegram(b Batch) []telegramBlock {
	var blocks []telegramBlock
	if b.Kind == KindTest {
		blocks = append(blocks, telegramBlock{text: "<b>Tidemark · Test message</b>\n" +
			"Notifications work. You'll get a message here when a check finds new updates."})
	} else {
		blocks = append(blocks, telegramBlock{text: "<b>" + html.EscapeString(telegramTitle(b)) + "</b>"})
		for _, g := range Groups(b.Items) {
			name := g.Stack
			if name == "" {
				name = "No stack"
			}
			lines := []string{"<b>" + html.EscapeString(name) + "</b>"}
			for _, it := range g.Items {
				lines = append(lines, telegramLine(it, b.Kind))
			}
			blocks = append(blocks, telegramBlock{sep: "\n\n", text: strings.Join(lines, "\n")})
		}
	}
	if b.DashboardURL != "" {
		blocks = append(blocks, telegramBlock{sep: "\n\n", text: `<a href="` + html.EscapeString(b.DashboardURL) + `">Open Tidemark</a>`})
	}
	return blocks
}

func telegramTitle(b Batch) string {
	var breaking, attention int
	for _, it := range b.Items {
		switch it.Level {
		case analysis.Breaking:
			breaking++
		case analysis.Attention:
			attention++
		}
	}
	n := len(b.Items)
	var parts []string
	if b.Kind == KindFirst {
		parts = append(parts, plural(n, "update", "updates")+" pending")
	} else {
		parts = append(parts, plural(n, "new update", "new updates"))
	}
	if breaking > 0 {
		parts = append(parts, fmt.Sprintf("%d breaking", breaking))
	}
	if attention > 0 {
		verb := "needs"
		if attention > 1 {
			verb = "need"
		}
		parts = append(parts, fmt.Sprintf("%d %s attention", attention, verb))
	}
	return "Tidemark · " + strings.Join(parts, " · ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// telegramLine is one update: marker and level word, name (linked), running → latest,
// class, and why it's sent again.
func telegramLine(it Item, kind string) string {
	var sb strings.Builder
	switch it.Level {
	case analysis.Breaking:
		sb.WriteString("🔴 Breaking · ")
	case analysis.Attention:
		sb.WriteString("🟡 Attention · ")
	default:
		sb.WriteString("· ")
	}
	name := html.EscapeString(it.Container)
	if it.URL != "" {
		name = `<a href="` + html.EscapeString(it.URL) + `">` + name + `</a>`
	}
	sb.WriteString(name)
	sb.WriteString(" ")
	if it.Channel != "" || it.Latest == "" || it.Class == version.Digest {
		// A channel tag follows builds: Latest is then the newest stable release, not the tag.
		tag := it.Channel
		if tag == "" {
			tag = it.Latest
		}
		if tag == "" {
			tag = it.Running
		}
		if tag == "" {
			sb.WriteString("new build")
		} else {
			sb.WriteString("new build of " + code(tag))
		}
	} else {
		if it.Running != "" {
			sb.WriteString(code(it.Running) + " → ")
		}
		sb.WriteString(code(it.Latest) + " " + html.EscapeString(string(it.Class)))
	}
	if kind != KindFirst {
		switch it.Reason {
		case ReasonRaised:
			sb.WriteString(" · level raised")
		case ReasonSnoozeEnded:
			sb.WriteString(" · snooze ended")
		}
	}
	return sb.String()
}

func code(s string) string { return "<code>" + html.EscapeString(s) + "</code>" }

// splitMessage joins blocks into messages of at most limit UTF-16 units. It breaks
// between blocks; a block that alone is too long is broken at its lines. The title thus
// only starts the first message and the dashboard link only ends the last.
func splitMessage(blocks []telegramBlock, limit int) []string {
	var pieces []telegramBlock
	for _, b := range blocks {
		if textLen(b.text) <= limit {
			pieces = append(pieces, b)
			continue
		}
		for i, line := range strings.Split(b.text, "\n") {
			sep := "\n"
			if i == 0 {
				sep = b.sep
			}
			pieces = append(pieces, telegramBlock{sep: sep, text: line})
		}
	}
	var out []string
	var cur string
	for _, p := range pieces {
		if cur != "" && textLen(cur)+textLen(p.sep)+textLen(p.text) > limit {
			out = append(out, cur)
			cur = ""
		}
		if cur == "" {
			cur = p.text
		} else {
			cur += p.sep + p.text
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func textLen(s string) int { return len(utf16.Encode([]rune(s))) }
