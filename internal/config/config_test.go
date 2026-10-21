package config

import (
	"strings"
	"testing"

	"github.com/bartvs98/tidemark/internal/analysis"
)

func TestNotificationDefaults(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.TelegramBotToken != "" || c.TelegramChatID != "" || c.NotifyLevel != analysis.None || c.NotifyDigest != "" || c.PublicURL != "" {
		t.Errorf("config = %+v", c)
	}
}

func TestNotificationEnv(t *testing.T) {
	t.Setenv("TIDEMARK_TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TIDEMARK_TELEGRAM_CHAT_ID", "-100200")
	t.Setenv("TIDEMARK_NOTIFY_LEVEL", "Breaking")
	t.Setenv("TIDEMARK_NOTIFY_DIGEST", "CRON_TZ=Europe/Amsterdam 0 9 * * *")
	t.Setenv("TIDEMARK_PUBLIC_URL", "https://tidemark.example.lan/")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.TelegramBotToken != "123:abc" || c.TelegramChatID != "-100200" || c.NotifyLevel != analysis.Breaking ||
		c.NotifyDigest != "CRON_TZ=Europe/Amsterdam 0 9 * * *" || c.PublicURL != "https://tidemark.example.lan" {
		t.Errorf("config = %+v", c)
	}
}

func TestNotifyLevels(t *testing.T) {
	for in, want := range map[string]analysis.Level{"all": analysis.None, "ALL": analysis.None, "attention": analysis.Attention, "breaking": analysis.Breaking} {
		t.Setenv("TIDEMARK_NOTIFY_LEVEL", in)
		c, err := Load()
		if err != nil || c.NotifyLevel != want {
			t.Errorf("%q: level %v, err %v", in, c.NotifyLevel, err)
		}
	}
}

func TestNotificationLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unknown level", map[string]string{"TIDEMARK_NOTIFY_LEVEL": "loud"}, "TIDEMARK_NOTIFY_LEVEL"},
		{"token without chat", map[string]string{"TIDEMARK_TELEGRAM_BOT_TOKEN": "123:abc"}, "set together"},
		{"chat without token", map[string]string{"TIDEMARK_TELEGRAM_CHAT_ID": "42"}, "set together"},
		{"relative public url", map[string]string{"TIDEMARK_PUBLIC_URL": "tidemark.lan"}, "TIDEMARK_PUBLIC_URL"},
		{"ftp public url", map[string]string{"TIDEMARK_PUBLIC_URL": "ftp://tidemark.lan"}, "TIDEMARK_PUBLIC_URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "123:abc") {
				t.Errorf("error leaks the token: %v", err)
			}
		})
	}
}
