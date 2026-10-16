package analysis

import (
	"strings"
	"testing"

	"github.com/bartvs98/tidemark/internal/version"
)

// Lines taken from real release notes of containers.
func TestLineLevel(t *testing.T) {
	cases := []struct {
		line string
		want Level
	}{
		// breaking
		{"### ⚠ BREAKING CHANGES", Breaking},
		{"⚠️ **Breaking Changes**", Breaking},
		{"### Potentially breaking changes", Breaking},
		{"Modules API - Potentially breaking changes to new 7.4 features (compared to 7.4 RC1)", Breaking},
		{"## Backward-incompatible changes", Breaking},
		{"* feat!: drop Node 18", Breaking},
		{"- fix(api)!: return 404 for unknown ids", Breaking},
		{"- **Breaking:** the `/v1` API is gone", Breaking},
		{"💥 New config format", Breaking},
		{"## Breaking", Breaking},

		// attention
		{"- `AI_API_KEY` and  `AI_MODEL_NAME`  settings have been removed", Attention},
		{"- **removed** basic auth from rest API", Attention},
		{"- **changed** renamed API endpoints `food-property` and `food-property-type` to `property`", Attention},
		{"- **improved** renamed environment variable `CORS_ORIGIN_ALLOW_ALL` to `CORS_ALLOW_ALL_ORIGINS`", Attention},
		{"- ⚠️ previously shared items are no longer shared/visible.", Attention},
		{"⚠️  **Reminder**: If you are still using the `bitwardenrs/server*` Docker images, you need to migrate", Attention},
		{"- **Duo:** `DUO_USE_IFRAME` (the deprecated Traditional Prompt) is removed and ignored if set.", Attention},
		{"- Removed MySQL libraries from SQLite images.", Attention},
		{"**Important:** Please read the [migration guide](https://doc.traefik.io/traefik/v3.7/migrate/v3/#v376).", Attention},
		{"Updates for changes in 1.8.0. Initial DB migration may take several minutes.", Attention},
		{"Existing users should update their nginx confs to avoid http2 deprecation warnings.", Attention},
		{"__Important note__: If you are using the old `mprasil/bitwarden` image, you need to migrate", Attention},
		{"- Drop support for Python 3.12", Attention},
		{"> [!IMPORTANT]", Attention},
		{"- **Duo:** the legacy endpoints are removed. No current client uses them.", Attention},

		// not flagged: everyday wording
		{"* **warming:** fix undefined var breaking warming ([9472ca6](https://github.com/x/y/commit/9472ca6))", None},
		{"- **fixed** layout issue in some resolutions breaking recipe ingredients input", None},
		{"* allow breaking out of live log auto-scroll via wheel, touch, and keyboard", None},
		{"- Bump tplink-omada-client to fix breaking changes in Omada API ([@MarkGodwin] - [#166206])", None},
		{"- No breaking changes in this release", None},
		{"- Non-breaking refactor of the scheduler", None},
		{"- Migrate mqtt to lookup devices scoped to config entry (@emontnemery - #177971)", None},
		{"- Fix typo in v3.6 migration guide ([#12212](https://github.com/traefik/traefik/pull/12212))", None},
		{"* Fixed using deprecated GitHub API endpoints, causing new apps to get bad requests.", None},
		{"- Don't write the Cast entity state while it is being removed (@frenck - #184553)", None},
		{"* **jellyfin:** sign out devices of a user removed from the configuration", None},
		{"- Exclude incompatible entities from temperature automations (@emontnemery - #169901)", None},
		{"## What's Changed", None},
		{"- Add a Breaking change note to the changelog ([#12398](https://github.com/traefik/traefik/pull/12398) @nmengin)", None},
		{"- denonavr: keep Telnet connected when a zone's entity is removed (@TheEvilCoder42 - #184147)", None},
		{"- RediSearch/RediSearch#10375 `FT.CURSOR READ`: crash after the underlying index was dropped (MOD-16703)", None},
		{"* **collections:** stop saying an empty folder is dropped", None},
	}
	for _, c := range cases {
		if got := lineLevel(c.line); got != c.want {
			t.Errorf("lineLevel(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestAnalyzeSections(t *testing.T) {
	body := strings.Join([]string{
		"## ⚠ BREAKING CHANGES",
		"",
		"* the config file moved to /config/app.yml",
		"* default port is now 8080",
		"",
		"### Features",
		"",
		"* add dark mode",
		"",
		"```yaml",
		"# BREAKING: not a real line, it's in a code block",
		"```",
	}, "\n")
	rel := Analyze(body, nil)
	if rel.Level != Breaking {
		t.Fatalf("level = %v, want breaking", rel.Level)
	}
	got := hitLines(rel)
	want := []string{"## ⚠ BREAKING CHANGES", "* the config file moved to /config/app.yml", "* default port is now 8080"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("hits = %q, want %q", got, want)
	}
}

func TestAnalyzeNestedAndBoldSections(t *testing.T) {
	body := "⚠️ **Breaking Changes**\n- households replace shared spaces\n**Other**\n- faster search\n\n" +
		"## Backward-incompatible changes\n### Shelly\n- the `shelly` sensor was split into two entities\n## All changes\n- Fix a thing"
	got := hitLines(Analyze(body, nil))
	want := []string{
		"⚠️ **Breaking Changes**", "- households replace shared spaces",
		"## Backward-incompatible changes", "- the `shelly` sensor was split into two entities",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("hits = %q, want %q", got, want)
	}
}

func TestAnalyzeSectionRaisesLines(t *testing.T) {
	rel := Analyze("⚠️ **Breaking change** ⚠️\n\nIt may impact existing configurations; existing users should review their middlewares.", nil)
	if len(rel.Hits) != 2 || rel.Hits[1].Level != Breaking {
		t.Errorf("hits = %+v, want the attention line raised to breaking", rel.Hits)
	}
}

func TestAnalyzeEmptySectionSaysNothing(t *testing.T) {
	for _, body := range []string{
		"## 💥 Breaking Changes\n\n## 🚀 Features\n- add a thing",
		"### Breaking changes\nNone\n### Fixes\n- fix a thing",
		"### Breaking changes\n- N/A",
	} {
		if rel := Analyze(body, nil); rel.Level != None {
			t.Errorf("Analyze(%q) = %v %q, want none", body, rel.Level, hitLines(rel))
		}
	}
}

func TestAnalyzeSkipsKnownLines(t *testing.T) {
	old := "**Remote Changes:**\nExisting users should update their nginx confs to avoid http2 deprecation warnings."
	known := lineSet(old)
	if rel := Analyze("**Remote Changes:**\nUpdating to 1.20.0\n"+old, known); rel.Level != None {
		t.Errorf("repeated history flagged: %q", hitLines(rel))
	}
}

func TestAssess(t *testing.T) {
	v := func(s string) *version.Version {
		x, ok := version.Parse(s)
		if !ok {
			t.Fatalf("parse %q", s)
		}
		return &x
	}

	t.Run("major bump", func(t *testing.T) {
		r := Assess(Input{Class: version.Major, Running: v("2.17.2"), Latest: v("3.5.1")})
		if r.Level != Breaking || len(r.Reasons) != 1 || r.Reasons[0] != "Major version: 2.17.2 → 3.5.1" {
			t.Errorf("got %v %q", r.Level, r.Reasons)
		}
	})
	t.Run("0.x minor bump", func(t *testing.T) {
		r := Assess(Input{Class: version.Minor, Running: v("0.21.0"), Latest: v("0.22.1")})
		if r.Level != Breaking {
			t.Errorf("got %v %q", r.Level, r.Reasons)
		}
	})
	t.Run("calm patch", func(t *testing.T) {
		r := Assess(Input{Class: version.Patch, Running: v("1.37.2"), Latest: v("1.37.4"),
			Releases: []Notes{{"1.37.4", "## What's Changed\n- Fix icons"}, {"1.37.3", "- Update web vault"}}})
		if r.Level != None || len(r.Reasons) != 0 || len(r.Releases) != 2 {
			t.Errorf("got %v %q %d", r.Level, r.Reasons, len(r.Releases))
		}
	})
	t.Run("quotes breaking first, then the rest as a count", func(t *testing.T) {
		r := Assess(Input{Class: version.Minor, Running: v("2.6.13"), Latest: v("2.6.15"), Releases: []Notes{
			{"2.6.15", "- **removed** basic auth from rest API\n- Removed MySQL libraries\n- settings have been removed\n- Dropped support for X"},
			{"2.6.14", "- **[api]** BREAKING: `/v1` is gone"},
		}})
		want := []string{
			"2.6.14: [api] BREAKING: /v1 is gone",
			"2.6.15: removed basic auth from rest API",
			"2.6.15: Removed MySQL libraries",
			"and 2 more flagged lines",
		}
		if r.Level != Breaking || strings.Join(r.Reasons, "|") != strings.Join(want, "|") {
			t.Errorf("got %v\n%q\nwant\n%q", r.Level, r.Reasons, want)
		}
		if r.Releases[0].Level != Attention || r.Releases[1].Level != Breaking {
			t.Errorf("per release: %v, %v", r.Releases[0].Level, r.Releases[1].Level)
		}
	})
}

func hitLines(r Release) []string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, h.Line)
	}
	return out
}

func TestAnalyzeSkipsRepeatedBoilerplate(t *testing.T) {
	known := lineSet("**Important:** Please read the [migration guide](https://doc.traefik.io/traefik/v3.6/migrate/v3/#v3613).")
	body := "**Important:** Please read the [migration guide](https://doc.traefik.io/traefik/v3.7/migrate/v3/#v3715).\n\n**Bug fixes:**\n- fix a thing"
	if rel := Analyze(body, known); rel.Level != None {
		t.Errorf("boilerplate flagged: %q", hitLines(rel))
	}
}

func TestAnalyzeSkipsBoilerplateWithOtherLink(t *testing.T) {
	known := lineSet("**Important:** Please read the [migration guide](https://doc.traefik.io/traefik/migration/v3/#v3613).")
	if rel := Analyze("**Important:** Please read the [migration guide](https://doc.traefik.io/traefik/v3.7/migrate/v3/#v3715).", known); rel.Level != None {
		t.Errorf("boilerplate flagged: %q", hitLines(rel))
	}
}
