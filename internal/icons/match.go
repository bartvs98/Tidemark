package icons

import (
	"sort"
	"strings"

	"github.com/bartvs98/tidemark/internal/imageref"
)

// Path segments and suffixes that name a role, not an app ("vaultwarden/server", "woodpecker-agent").
var generic = map[string]bool{
	"server": true, "app": true, "web": true, "docker": true, "agent": true, "backend": true,
	"frontend": true, "api": true, "ui": true, "core": true, "db": true, "backup": true,
	"worker": true, "image": true, "library": true,
}

// Official images whose name differs from the app's icon reference.
var aliases = map[string]string{
	"postgres":          "postgresql",
	"mongo":             "mongodb",
	"eclipse-mosquitto": "mosquitto",
	"httpd":             "apache",
}

// Resolve picks an icon for a container. Order:
//  1. the tidemark.icon label ("none" turns icons off for that container)
//  2. the image's last path segment: exact, alias, trimmed (woodpecker-agent → uptime-kuma), unique prefix
//  3. the image's namespaces (vaultwarden/server → vaultwarden), exact only
//  4. the Compose service and container name, which users choose freely, so they come last
func (s *Service) Resolve(ref imageref.Ref, name, service string, labels map[string]string) (Icon, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.byRef == nil {
		return Icon{}, false
	}

	if v := strings.TrimSpace(labels[LabelIcon]); v != "" {
		if v == "none" {
			return Icon{}, false
		}
		if ic, ok := s.byRef[v]; ok {
			return ic, true
		}
		// Unknown reference: fall through to automatic matching.
	}

	segs := strings.Split(ref.Repository, "/")
	if ic, ok := s.match(segs[len(segs)-1], true); ok {
		return ic, true
	}
	for i := len(segs) - 2; i >= 0; i-- {
		if ic, ok := s.match(segs[i], false); ok {
			return ic, true
		}
	}
	for _, n := range []string{service, name} {
		if ic, ok := s.match(n, true); ok {
			return ic, true
		}
	}
	return Icon{}, false
}

// match tries one candidate name. Callers hold s.mu.
func (s *Service) match(candidate string, loose bool) (Icon, bool) {
	c := strings.ToLower(strings.TrimSpace(candidate))
	if c == "" || generic[c] {
		return Icon{}, false
	}
	if a, ok := aliases[c]; ok {
		if ic, ok := s.byRef[a]; ok {
			return ic, true
		}
	}
	if ref, ok := s.byKey[normalize(c)]; ok {
		return s.byRef[ref], true
	}
	if !loose {
		return Icon{}, false
	}

	// Drop trailing parts: plex-meta-manager → plex, woodpecker-agent → uptime-kuma.
	parts := strings.FieldsFunc(c, func(r rune) bool { return r == '-' || r == '_' })
	for n := len(parts) - 1; n >= 1; n-- {
		trimmed := strings.Join(parts[:n], "-")
		if len(trimmed) < 3 || generic[trimmed] {
			continue
		}
		if ref, ok := s.byKey[normalize(trimmed)]; ok {
			return s.byRef[ref], true
		}
	}

	// A single reference that extends the name: stirling → stirling-pdf, cup → cup-updates.
	prefix := strings.Join(parts, "-") + "-"
	i := sort.SearchStrings(s.sorted, prefix)
	if i < len(s.sorted) && strings.HasPrefix(s.sorted[i], prefix) &&
		(i+1 == len(s.sorted) || !strings.HasPrefix(s.sorted[i+1], prefix)) {
		return s.byRef[s.sorted[i]], true
	}
	return Icon{}, false
}

// normalize compares names loosely: "AdGuard Home", "adguard-home" and "adguardhome" are equal.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
