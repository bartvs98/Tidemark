package review

import (
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/store"
)

func TestActive(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		r      store.Review
		latest string
		want   bool
	}{
		{"reviewed, same latest", store.Review{Kind: Reviewed, Version: "1.6.0"}, "1.6.0", true},
		{"reviewed, newer latest", store.Review{Kind: Reviewed, Version: "1.6.0"}, "1.7.0", false},
		{"reviewed, older latest", store.Review{Kind: Reviewed, Version: "1.6.0"}, "1.5.2", true},
		{"reviewed, v prefix", store.Review{Kind: Reviewed, Version: "v1.6.0"}, "1.6.0", true},
		{"reviewed, newer patch", store.Review{Kind: Reviewed, Version: "1.6.0"}, "1.6.1", false},
		{"snoozed by version, same latest", store.Review{Kind: Snoozed, Version: "1.6.0"}, "1.6.0", true},
		{"snoozed by version, newer latest", store.Review{Kind: Snoozed, Version: "1.6.0"}, "1.7.0", false},
		{"snoozed by time, before", store.Review{Kind: Snoozed, Until: now.Add(time.Hour)}, "9.9.9", true},
		{"snoozed by time, after", store.Review{Kind: Snoozed, Until: now.Add(-time.Hour)}, "1.0.0", false},
		{"snoozed by time, at", store.Review{Kind: Snoozed, Until: now}, "1.0.0", false},
		{"unparseable, equal", store.Review{Kind: Reviewed, Version: "latest"}, "latest", true},
		{"unparseable, different", store.Review{Kind: Reviewed, Version: "latest"}, "stable", false},
		{"one side unparseable", store.Review{Kind: Snoozed, Version: "1.6.0"}, "edge", false},
		{"no version", store.Review{Kind: Reviewed}, "", false},
		{"unknown kind", store.Review{Kind: "other", Version: "1.6.0"}, "1.6.0", false},
	}
	for _, tt := range tests {
		if got := Active(tt.r, tt.latest, now); got != tt.want {
			t.Errorf("%s: Active = %v, want %v", tt.name, got, tt.want)
		}
	}
}
