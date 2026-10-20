package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, yml string) *File {
	t.Helper()
	f, err := parseFile([]byte(yml))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	return f
}

func TestDefault(t *testing.T) {
	r := Default()
	if !r.AllowMajor || r.Ignore || r.Pin != "" || r.Prerelease || r.TagPattern != nil || r.Repo != "" {
		t.Errorf("Default() = %+v", r)
	}
	if r.Active() {
		t.Error("Default().Active() = true")
	}
	var f *File
	got := f.For("/app", "redis:7", nil)
	if got.Active() || len(got.Problems) != 0 || len(got.From) != 0 {
		t.Errorf("nil File, no labels: %+v", got)
	}
}

func TestLabels(t *testing.T) {
	r := (*File)(nil).For("app", "redis:7", map[string]string{
		"tidemark.ignore":      "Yes",
		"tidemark.allow-major": "FALSE",
		"tidemark.pin":         "v1.4",
		"tidemark.prerelease":  "1",
		"tidemark.tag-pattern": `^\d+\.\d+\.\d+-alpine$`,
		"tidemark.repo":        "github.com/owner/repo",
		"tidemark.icon":        "redis", // not a rule
		"other":                "x",
	})
	if !r.Ignore || r.AllowMajor || r.Pin != "1.4" || !r.Prerelease || r.Repo != "github.com/owner/repo" {
		t.Errorf("rules = %+v", r)
	}
	if r.TagPattern == nil || !r.TagPattern.MatchString("1.2.3-alpine") || r.TagPattern.MatchString("1.2.3") {
		t.Errorf("tag pattern = %v", r.TagPattern)
	}
	want := map[string]string{"ignore": "label", "allow-major": "label", "pin": "label", "prerelease": "label", "tag-pattern": "label", "repo": "label"}
	if !reflect.DeepEqual(r.From, want) {
		t.Errorf("From = %v", r.From)
	}
	if len(r.Problems) != 0 {
		t.Errorf("Problems = %v", r.Problems)
	}
}

func TestBool(t *testing.T) {
	for in, want := range map[string]bool{"true": true, "TRUE": true, "yes": true, "1": true, "false": false, "No": false, "0": false} {
		got, ok := parseBool(in)
		if !ok || got != want {
			t.Errorf("parseBool(%q) = %v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"maybe", "2", "on", "y"} {
		if _, ok := parseBool(in); ok {
			t.Errorf("parseBool(%q) accepted", in)
		}
	}
}

func TestPin(t *testing.T) {
	good := map[string]string{"1": "1", "1.4": "1.4", "v2": "2", "V2.0": "2.0", "16": "16", "1.10": "1.10", "01.4": "1.4", " 3 ": "3"}
	for in, want := range good {
		r := (*File)(nil).For("a", "x", map[string]string{"tidemark.pin": in})
		if r.Pin != want || len(r.Problems) != 0 {
			t.Errorf("pin %q = %q, problems %v", in, r.Pin, r.Problems)
		}
	}
	for _, in := range []string{"abc", "1.", ".1", "1..2", "1.x", "v", "1.4-alpine", "^1.4"} {
		if _, ok := parsePin(in); ok {
			t.Errorf("parsePin(%q) accepted", in)
		}
	}
}

func TestInvalidLabels(t *testing.T) {
	tests := []struct {
		label, value, problem string
	}{
		{"tidemark.ignore", "maybe", `tidemark.ignore: "maybe" is not true or false`},
		{"tidemark.allow-major", "maybe", `tidemark.allow-major: "maybe" is not true or false`},
		{"tidemark.prerelease", "sometimes", `tidemark.prerelease: "sometimes" is not true or false`},
		{"tidemark.pin", "abc", `tidemark.pin: "abc" is not a version prefix`},
		{"tidemark.tag-pattern", "(", `tidemark.tag-pattern: invalid regex: `},
	}
	for _, tt := range tests {
		r := (*File)(nil).For("a", "redis", map[string]string{tt.label: tt.value, "tidemark.repo": "github.com/a/b"})
		if len(r.Problems) != 1 || !strings.HasPrefix(r.Problems[0], tt.problem) {
			t.Errorf("%s=%s: problems %q, want prefix %q", tt.label, tt.value, r.Problems, tt.problem)
		}
		if r.Repo != "github.com/a/b" {
			t.Errorf("%s=%s: other rules lost: %+v", tt.label, tt.value, r)
		}
		if r.Fingerprint() != (Rules{AllowMajor: true, Repo: "github.com/a/b"}).Fingerprint() {
			t.Errorf("%s=%s: bad rule leaked into the rules: %+v", tt.label, tt.value, r)
		}
		key := strings.TrimPrefix(tt.label, "tidemark.")
		if _, ok := r.From[key]; ok {
			t.Errorf("%s=%s: From records a skipped rule", tt.label, tt.value)
		}
	}
}

func TestEmptyLabelIsUnset(t *testing.T) {
	r := (*File)(nil).For("a", "redis", map[string]string{"tidemark.pin": "", "tidemark.repo": "  "})
	if r.Active() || len(r.Problems) != 0 || len(r.From) != 0 {
		t.Errorf("empty labels: %+v", r)
	}
}

const sample = `
containers:
  vaultwarden:
    allow-major: false
  "immich-*":
    ignore: true
    prerelease: yes
  "immich-server*":
    ignore: false
  "immich-server":
    pin: "1.4"
images:
  postgres:
    pin: 16
    allow-major: false
  linuxserver/sonarr:
    repo: github.com/Sonarr/Sonarr
  lscr.io/linuxserver/sonarr:
    tag-pattern: '^\d+\.\d+\.\d+$'
  ghcr.io/foo/bar:
    repo: github.com/foo/bar-src
  "ghcr.io/immich-app/*":
    repo: github.com/immich-app/immich
    pin: "1"
  "*":
    prerelease: false
`

func TestPrecedence(t *testing.T) {
	f := mustParse(t, sample)
	tests := []struct {
		name, image string
		labels      map[string]string
		check       func(Rules) bool
	}{
		// containers: beats images: per field; images: still fills the rest.
		{"postgres", "postgres:16-alpine", nil, func(r Rules) bool {
			return r.Pin == "16" && !r.AllowMajor && r.From["pin"] == "config"
		}},
		{"db", "postgres:16-alpine", map[string]string{"tidemark.allow-major": "true"}, func(r Rules) bool {
			return r.Pin == "16" && r.AllowMajor && r.From["allow-major"] == "label" && r.From["pin"] == "config"
		}},
		// label pin doesn't wipe config ignore.
		{"immich-ml", "ghcr.io/immich-app/immich-machine-learning:v1", map[string]string{"tidemark.pin": "2"}, func(r Rules) bool {
			return r.Ignore && r.Prerelease && r.Pin == "2" && r.From["pin"] == "label" && r.Repo == "github.com/immich-app/immich"
		}},
		// exact beats glob, longest glob beats shorter glob; containers: prerelease beats images: "*".
		{"immich-server", "ghcr.io/immich-app/immich-server:v1.120.0", nil, func(r Rules) bool {
			return !r.Ignore && r.From["ignore"] == "config" && r.Pin == "1.4" && r.Prerelease
		}},
		{"immich-server-2", "ghcr.io/immich-app/immich-server:v1", nil, func(r Rules) bool {
			return !r.Ignore && r.Pin == "1" && r.Prerelease
		}},
		// container label beats everything.
		{"vaultwarden", "vaultwarden/server:latest", map[string]string{"tidemark.allow-major": "yes"}, func(r Rules) bool {
			return r.AllowMajor && r.From["allow-major"] == "label"
		}},
		{"vaultwarden", "vaultwarden/server:latest", nil, func(r Rules) bool {
			return !r.AllowMajor && r.From["allow-major"] == "config"
		}},
		// Docker's leading slash on names.
		{"/vaultwarden", "vaultwarden/server", nil, func(r Rules) bool { return !r.AllowMajor }},
		// unmatched: only the "*" images entry, which sets prerelease to its default.
		{"other", "redis:7", nil, func(r Rules) bool {
			return !r.Active() && r.From["prerelease"] == "config" && len(r.From) == 1
		}},
	}
	for _, tt := range tests {
		r := f.For(tt.name, tt.image, tt.labels)
		if !tt.check(r) {
			t.Errorf("For(%q, %q, %v) = %+v", tt.name, tt.image, tt.labels, r)
		}
		if len(r.Problems) != 0 {
			t.Errorf("For(%q): problems %v", tt.name, r.Problems)
		}
	}
}

func TestImageKeys(t *testing.T) {
	f := mustParse(t, sample)
	tests := []struct {
		image, pin, repo, pattern string
	}{
		{"postgres", "16", "", ""},
		{"postgres:16-alpine", "16", "", ""},
		{"docker.io/library/postgres:16", "16", "", ""},
		{"index.docker.io/library/postgres:16@sha256:abc", "16", "", ""},
		{"ghcr.io/someone/postgres:16", "", "", ""},
		{"bitnami/postgresql:16", "", "", ""},
		{"linuxserver/sonarr:latest", "", "github.com/Sonarr/Sonarr", ""},
		{"lscr.io/linuxserver/sonarr:latest", "", "github.com/Sonarr/Sonarr", `^\d+\.\d+\.\d+$`},
		{"ghcr.io/linuxserver/sonarr:4", "", "github.com/Sonarr/Sonarr", ""},
		{"ghcr.io/foo/bar:1", "", "github.com/foo/bar-src", ""},
		{"docker.io/foo/bar:1", "", "", ""},
		{"ghcr.io/immich-app/immich-server:v1", "1", "github.com/immich-app/immich", ""},
		{"not a ref", "", "", ""},
	}
	for _, tt := range tests {
		r := f.For("x", tt.image, nil)
		pattern := ""
		if r.TagPattern != nil {
			pattern = r.TagPattern.String()
		}
		if r.Pin != tt.pin || r.Repo != tt.repo || pattern != tt.pattern {
			t.Errorf("%s: pin %q repo %q pattern %q, want %q %q %q", tt.image, r.Pin, r.Repo, pattern, tt.pin, tt.repo, tt.pattern)
		}
	}
}

func TestImageKeyNormalisation(t *testing.T) {
	for _, key := range []string{"postgres", "library/postgres", "docker.io/postgres", "docker.io/library/postgres", "index.docker.io/library/postgres", "post*", "docker.io/library/post*"} {
		f := mustParse(t, "images:\n  "+key+":\n    ignore: true\n")
		for _, image := range []string{"postgres", "postgres:16-alpine", "docker.io/library/postgres:16"} {
			if !f.For("x", image, nil).Ignore {
				t.Errorf("key %q doesn't match %q", key, image)
			}
		}
		if f.For("x", "ghcr.io/x/postgres:16", nil).Ignore {
			t.Errorf("key %q matches ghcr.io/x/postgres", key)
		}
	}
}

func TestGlobSpecificity(t *testing.T) {
	f := mustParse(t, `
containers:
  "a*":     {pin: "1"}
  "ab*":    {pin: "2"}
  "abc?":   {pin: "3"}
  "*":      {pin: "4", prerelease: true}
`)
	for name, want := range map[string]string{"abcd": "3", "abc": "2", "abcde": "2", "ax": "1", "zz": "4"} {
		r := f.For(name, "redis", nil)
		if r.Pin != want || !r.Prerelease {
			t.Errorf("%s: pin %q prerelease %v, want %q", name, r.Pin, r.Prerelease, want)
		}
	}
	// Literal glob characters other than * and ? stay literal.
	f = mustParse(t, "containers:\n  \"a.b*\": {ignore: true}\n")
	if f.For("axb", "redis", nil).Ignore || !f.For("a.b-1", "redis", nil).Ignore {
		t.Error("glob escaping")
	}
}

func TestConfigProblems(t *testing.T) {
	f := mustParse(t, `
containers:
  vaultwarden:
    tag-pattern: "("
    allow-major: maybe
    pin: "1.4"
images:
  redis:
    pin: abc
`)
	r := f.For("vaultwarden", "redis:7", nil)
	if r.Pin != "1.4" || !r.AllowMajor || r.TagPattern != nil {
		t.Errorf("rules = %+v", r)
	}
	want := []string{
		`config containers.vaultwarden allow-major: "maybe" is not true or false`,
		`config containers.vaultwarden tag-pattern: invalid regex: `,
		`config images.redis pin: "abc" is not a version prefix`,
	}
	if len(r.Problems) != len(want) {
		t.Fatalf("problems = %q", r.Problems)
	}
	for i := range want {
		if !strings.HasPrefix(r.Problems[i], want[i]) {
			t.Errorf("problem %d = %q, want prefix %q", i, r.Problems[i], want[i])
		}
	}
	// Problems only show on containers the entry applies to.
	if p := f.For("other", "postgres", nil).Problems; len(p) != 0 {
		t.Errorf("unrelated container got problems %q", p)
	}
}

func TestConfigScalarsKeptAsWritten(t *testing.T) {
	f := mustParse(t, "images:\n  app:\n    pin: 1.10\n    ignore: True\n")
	r := f.For("x", "app", nil)
	if r.Pin != "1.10" || !r.Ignore {
		t.Errorf("rules = %+v", r)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := map[string]string{
		"unknown rule":    "containers:\n  a:\n    ignored: true\n",
		"unknown section": "container:\n  a:\n    ignore: true\n",
		"not yaml":        "containers: [",
		"list value":      "containers:\n  a:\n    pin: [1, 2]\n",
		"image with tag":  "images:\n  postgres:16:\n    pin: 16\n",
		"image digest":    "images:\n  postgres@sha256:abc:\n    pin: 16\n",
		"uppercase image": "images:\n  Postgres:\n    pin: 16\n",
		"duplicate name":  "containers:\n  app:\n    pin: 1\n  /app:\n    ignore: true\n",
		"duplicate glob":  "containers:\n  \"web-*\":\n    pin: 1\n  \" /web-*\":\n    ignore: true\n",
	}
	for name, yml := range tests {
		if _, err := parseFile([]byte(yml)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	// A registry port is not a tag.
	if _, err := parseFile([]byte("images:\n  localhost:5000/app:\n    pin: 1\n")); err != nil {
		t.Errorf("registry port: %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	f, err := Load(filepath.Join(dir, "missing.yaml"))
	if err != nil || f == nil {
		t.Fatalf("missing file: %v, %v", f, err)
	}
	if f.For("a", "redis", nil).Active() {
		t.Error("missing file has rules")
	}

	empty := filepath.Join(dir, "empty.yaml")
	os.WriteFile(empty, []byte("# nothing yet\n"), 0o644)
	if _, err := Load(empty); err != nil {
		t.Errorf("empty file: %v", err)
	}

	good := filepath.Join(dir, "good.yaml")
	os.WriteFile(good, []byte(sample), 0o644)
	f, err = Load(good)
	if err != nil {
		t.Fatal(err)
	}
	if !f.For("vaultwarden", "vaultwarden/server", nil).Active() {
		t.Error("loaded file has no rules")
	}

	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("containers:\n  a:\n    bogus: 1\n"), 0o644)
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("bad file: %v", err)
	}
}

func TestFingerprint(t *testing.T) {
	if Default().Fingerprint() != "" {
		t.Errorf("default fingerprint = %q", Default().Fingerprint())
	}
	f := mustParse(t, sample)
	a := f.For("immich-server", "ghcr.io/immich-app/immich-server:v1", nil)
	b := f.For("immich-server", "ghcr.io/immich-app/immich-server:v1", nil)
	if a.Fingerprint() != b.Fingerprint() || a.Fingerprint() == "" {
		t.Errorf("unstable: %q vs %q", a.Fingerprint(), b.Fingerprint())
	}
	// Same rules from a label instead of the config: From differs, fingerprint doesn't.
	c := (*File)(nil).For("x", "redis", map[string]string{
		"tidemark.pin": "1.4", "tidemark.prerelease": "true", "tidemark.ignore": "false",
		"tidemark.repo": "github.com/immich-app/immich", "tidemark.allow-major": "bogus",
	})
	if c.Fingerprint() != a.Fingerprint() {
		t.Errorf("label vs config: %q vs %q", c.Fingerprint(), a.Fingerprint())
	}

	base := Rules{AllowMajor: true, Pin: "1"}
	seen := map[string]string{base.Fingerprint(): "base"}
	variants := map[string]Rules{
		"ignore":      {AllowMajor: true, Pin: "1", Ignore: true},
		"allow-major": {AllowMajor: false, Pin: "1"},
		"pin":         {AllowMajor: true, Pin: "1.4"},
		"prerelease":  {AllowMajor: true, Pin: "1", Prerelease: true},
		"tag-pattern": {AllowMajor: true, Pin: "1", TagPattern: globRegexp("*-alpine")},
		"repo":        {AllowMajor: true, Pin: "1", Repo: "github.com/a/b"},
		"pin-in-repo": {AllowMajor: true, Pin: "", Repo: `";pin="1`},
	}
	for name, r := range variants {
		fp := r.Fingerprint()
		if other, dup := seen[fp]; dup {
			t.Errorf("%s has the same fingerprint as %s: %q", name, other, fp)
		}
		seen[fp] = name
	}
}

func TestActive(t *testing.T) {
	for name, r := range map[string]Rules{
		"ignore":      {AllowMajor: true, Ignore: true},
		"allow-major": {},
		"pin":         {AllowMajor: true, Pin: "1"},
		"prerelease":  {AllowMajor: true, Prerelease: true},
		"tag-pattern": {AllowMajor: true, TagPattern: globRegexp("x")},
		"repo":        {AllowMajor: true, Repo: "github.com/a/b"},
	} {
		if !r.Active() {
			t.Errorf("%s: not active", name)
		}
	}
	if (Rules{AllowMajor: true, Problems: []string{"x"}, From: map[string]string{"pin": "label"}}).Active() {
		t.Error("problems/From count as active")
	}
}
