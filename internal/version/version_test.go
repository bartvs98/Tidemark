package version

import (
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		tag     string
		ok      bool
		prefix  string
		nums    []int
		pre     string
		variant string
		build   int
	}{
		// Real tags from common images.
		{"10.9.11", true, "", []int{10, 9, 11}, "", "", 0},
		{"v3.5.1", true, "v", []int{3, 5, 1}, "", "", 0},
		{"v0.107.79", true, "v", []int{0, 107, 79}, "", "", 0},
		{"16-alpine", true, "", []int{16}, "", "alpine", 0},
		{"7.4.2-alpine3.21", true, "", []int{7, 4, 2}, "", "alpine", 0},
		{"8.3", true, "", []int{8, 3}, "", "", 0},
		{"2026.9.0", true, "", []int{2026, 9, 0}, "", "", 0},
		{"v1.15.0-ls175", true, "v", []int{1, 15, 0}, "", "", 175},
		{"1.20.0-ls166", true, "", []int{1, 20, 0}, "", "", 166},
		{"0.21.0-alpine", true, "", []int{0, 21, 0}, "", "alpine", 0},
		{"2.0.0-rc.1", true, "", []int{2, 0, 0}, "rc.1", "", 0},
		{"1.2.3-beta2", true, "", []int{1, 2, 3}, "beta2", "", 0},
		{"3.12.1-slim-bookworm", true, "", []int{3, 12, 1}, "", "slim-bookworm", 0},
		{"12.2_beta", true, "", []int{12, 2}, "beta", "", 0},
		{"1.0.0+build.5", true, "", []int{1, 0, 0}, "", "build", 0},
		{"V2.1", true, "v", []int{2, 1}, "", "", 0},
		{"5.0.0-r3", true, "", []int{5, 0, 0}, "", "", 3},
		{"1.2.3-alpine-rc1", true, "", []int{1, 2, 3}, "rc1", "alpine", 0},

		// Not versions.
		{"latest", false, "", nil, "", "", 0},
		{"alpine", false, "", nil, "", "", 0},
		{"nightly", false, "", nil, "", "", 0},
		{"e8a527a2-ls87", false, "", nil, "", "", 0},
		{"1234abc", false, "", nil, "", "", 0},
		{"20240101123", false, "", nil, "", "", 0}, // CI build number
		{"sha-3f9a1c2", false, "", nil, "", "", 0},
		{"", false, "", nil, "", "", 0},
		{"v", false, "", nil, "", "", 0},
	}
	for _, tt := range tests {
		v, ok := Parse(tt.tag)
		if ok != tt.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", tt.tag, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if v.Prefix != tt.prefix || !slices.Equal(v.Nums, tt.nums) || v.Pre != tt.pre || v.Variant != tt.variant || v.Build != tt.build {
			t.Errorf("Parse(%q) = %+v", tt.tag, v)
		}
		if v.Raw != tt.tag {
			t.Errorf("Parse(%q).Raw = %q", tt.tag, v.Raw)
		}
	}
}

func mustParse(t *testing.T, tag string) Version {
	t.Helper()
	v, ok := Parse(tag)
	if !ok {
		t.Fatalf("Parse(%q) failed", tag)
	}
	return v
}

func TestCompareSortsTags(t *testing.T) {
	tags := []string{"1.10.0", "1.2.0", "1.2", "1", "1.2.0-rc.1", "2.0.0-beta", "1.9.9", "1.2.0-ls5", "1.2.0-ls12", "2.0.0"}
	var vs []Version
	for _, tag := range tags {
		vs = append(vs, mustParse(t, tag))
	}
	slices.SortFunc(vs, Compare)
	var got []string
	for _, v := range vs {
		got = append(got, v.Raw)
	}
	want := []string{"1", "1.2", "1.2.0-rc.1", "1.2.0", "1.2.0-ls5", "1.2.0-ls12", "1.9.9", "1.10.0", "2.0.0-beta", "2.0.0"}
	if !slices.Equal(got, want) {
		t.Errorf("sorted = %v\nwant     %v", got, want)
	}
}

func TestComparePreReleases(t *testing.T) {
	// Each tag ranks above the one before it.
	order := []string{"1.4.9", "1.5.0-nightly", "1.5.0-alpha", "1.5.0-alpha.2", "1.5.0-beta1", "1.5.0-rc", "1.5.0-rc.2", "1.5.0-rc10", "1.5.0", "1.5.1-rc1"}
	for i := 1; i < len(order); i++ {
		a, b := mustParse(t, order[i-1]), mustParse(t, order[i])
		if Compare(a, b) >= 0 || Compare(b, a) <= 0 {
			t.Errorf("want %s < %s", a.Raw, b.Raw)
		}
	}
	if Compare(mustParse(t, "1.5.0-rc1"), mustParse(t, "1.5.0-rc.1")) != 0 {
		t.Error("rc1 and rc.1 should compare equal")
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		from, to string
		want     Class
	}{
		{"10.9.11", "10.10.7", Minor},
		{"10.10.6", "10.10.7", Patch},
		{"v1.135.3", "v2.0.1", Major},
		{"2026.9.0", "2026.10.1", Minor}, // calendar versions
		{"2025.12.4", "2026.1.0", Major},
		{"16", "16.4", Minor},
		{"16.4", "16", None},
		{"1.20.0-ls166", "1.20.0-ls170", Digest},
		{"1.20.0-ls166", "1.21.0-ls171", Minor},
		{"2.0.0-rc.2", "2.0.0", Patch},
		{"2.0.0-rc.2", "2.0.0-rc.10", Patch},
		{"2.0.0-rc.2", "2.0.0-beta.5", None},
		{"2.0.0", "2.0.0-rc.1", None},
		{"1.4.9", "1.5.0-rc1", Minor},
		{"3.6.13", "3.6.13", None},
		{"3.6.13", "3.5.0", None},
		{"v4.33.0", "4.33.0", None},
	}
	for _, tt := range tests {
		if got := Classify(mustParse(t, tt.from), mustParse(t, tt.to)); got != tt.want {
			t.Errorf("Classify(%s, %s) = %s, want %s", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestFamilyAndMatches(t *testing.T) {
	alpine16, alpine17 := mustParse(t, "16-alpine"), mustParse(t, "17.6-alpine3.21")
	plain17, rc := mustParse(t, "17.6"), mustParse(t, "17.7-alpine-rc1")
	if !alpine16.SameFamily(alpine17) {
		t.Error("16-alpine and 17.6-alpine3.21 should be one family")
	}
	if alpine16.SameFamily(plain17) || alpine16.SameFamily(rc) {
		t.Error("variant and pre-release must split families")
	}
	if !Matches(mustParse(t, "v4.33.0"), mustParse(t, "4.33.0")) {
		t.Error("v prefix should not matter")
	}
	if Matches(mustParse(t, "4.33"), mustParse(t, "4.33.0")) {
		t.Error("different precision should not match")
	}
	if got := mustParse(t, "v1.15.0-ls175").String(); got != "1.15.0" {
		t.Errorf("String() = %q", got)
	}
}

func TestHasPrefixAndRawVariant(t *testing.T) {
	if !mustParse(t, "16.4.1-alpine").HasPrefix(mustParse(t, "16-alpine")) {
		t.Error("16.4.1 should have prefix 16")
	}
	if mustParse(t, "16.4").HasPrefix(mustParse(t, "16.4.1")) || mustParse(t, "17.0").HasPrefix(mustParse(t, "16")) {
		t.Error("wrong prefix match")
	}
	if v := mustParse(t, "7.4.2-alpine3.21"); v.RawVariant != "alpine3.21" || v.Variant != "alpine" {
		t.Errorf("raw/variant = %q/%q", v.RawVariant, v.Variant)
	}
}
