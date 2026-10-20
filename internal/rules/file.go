package rules

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/bartvs98/tidemark/internal/imageref"
)

// File is a parsed config file. A nil *File means there is no file.
type File struct {
	containers []entry
	images     []entry
}

// entry is one key of the containers: or images: section, with its rules
// already validated (problems are reported only for containers it matches).
type entry struct {
	key      string
	glob     bool
	match    *regexp.Regexp // glob keys only
	registry string         // images only: "" if the key names no registry
	repo     string         // images only, non-glob: repository as written ("postgres", "linuxserver/sonarr")
	rules    set
	problems []string
}

// value is any YAML scalar, kept as written: "pin: 1.10" must not become 1.1.
type value string

func (v *value) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: expected a single value", n.Line)
	}
	*v = value(n.Value)
	return nil
}

type rawEntry struct {
	Ignore     *value `yaml:"ignore"`
	AllowMajor *value `yaml:"allow-major"`
	Pin        *value `yaml:"pin"`
	Prerelease *value `yaml:"prerelease"`
	TagPattern *value `yaml:"tag-pattern"`
	Repo       *value `yaml:"repo"`
}

func (e rawEntry) values() map[string]string {
	raw := map[string]string{}
	add := func(key string, v *value) {
		if v != nil {
			raw[key] = string(*v)
		}
	}
	add(KeyIgnore, e.Ignore)
	add(KeyAllowMajor, e.AllowMajor)
	add(KeyPin, e.Pin)
	add(KeyPrerelease, e.Prerelease)
	add(KeyTagPattern, e.TagPattern)
	add(KeyRepo, e.Repo)
	return raw
}

type rawFile struct {
	Containers map[string]rawEntry `yaml:"containers"`
	Images     map[string]rawEntry `yaml:"images"`
}

// Load reads the config file at path. A missing file gives an empty File; YAML
// errors, unknown keys and malformed image keys are errors. Invalid rule values are
// not: they become problems on the containers they apply to.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{}, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := parseFile(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func parseFile(data []byte) (*File, error) {
	var raw rawFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	f := &File{}
	spelled := map[string]string{} // trimmed name -> the key as written
	for key, re := range raw.Containers {
		name := strings.TrimPrefix(strings.TrimSpace(key), "/")
		if name == "" {
			return nil, fmt.Errorf("containers: empty container name")
		}
		// Map order is random, so two spellings of one container would merge in random order.
		if other, dup := spelled[name]; dup {
			a, b := key, other
			if a > b {
				a, b = b, a
			}
			return nil, fmt.Errorf("containers: %q and %q are the same container", a, b)
		}
		spelled[name] = key
		e := entry{key: name, glob: isGlob(name)}
		if e.glob {
			e.match = globRegexp(name)
		}
		e.rules, e.problems = parse(re.values(), "config containers."+key+" ")
		f.containers = append(f.containers, e)
	}
	for key, re := range raw.Images {
		e, err := imageEntry(strings.TrimSpace(key))
		if err != nil {
			return nil, fmt.Errorf("images.%s: %w", key, err)
		}
		e.rules, e.problems = parse(re.values(), "config images."+key+" ")
		f.images = append(f.images, e)
	}
	sortBySpecificity(f.containers)
	sortBySpecificity(f.images)
	return f, nil
}

// imageEntry prepares an images: key. Keys without a registry ("postgres",
// "linuxserver/sonarr") match that repository on any registry, so the same
// linuxserver image matches whether it's pulled from Docker Hub, lscr.io or ghcr.io;
// keys with one ("ghcr.io/foo/bar") match only that registry.
func imageEntry(key string) (entry, error) {
	if key == "" {
		return entry{}, errors.New("empty image name")
	}
	last := key[strings.LastIndex(key, "/")+1:]
	if strings.ContainsAny(last, ":@") {
		return entry{}, errors.New("image keys name a repository without tag or digest")
	}
	// Parse validates the name and normalises the registry (index.docker.io → docker.io).
	ref, err := imageref.Parse(key)
	if err != nil {
		return entry{}, err
	}
	e := entry{key: key, glob: isGlob(key)}
	repo := key
	if first, rest, ok := strings.Cut(key, "/"); ok && looksLikeHost(first) {
		e.registry = ref.Registry
		repo = rest
	}
	if e.glob {
		e.match = globRegexp(repo)
	} else {
		e.repo = repo
	}
	return e, nil
}

// looksLikeHost mirrors imageref's rule for a registry as first path component.
func looksLikeHost(s string) bool {
	return strings.ContainsAny(s, ".:") || s == "localhost"
}

func isGlob(s string) bool { return strings.ContainsAny(s, "*?") }

// globRegexp turns a glob into an anchored regexp: * is any run of characters
// (including /, so "ghcr.io/foo/*" covers nested repositories), ? one character.
func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// sortBySpecificity orders entries most specific first: exact keys before globs,
// then longer keys first. Longer is used as "more specific" because a longer pattern
// has more literal characters and so matches fewer names ("immich-server*" over
// "immich-*"); for images it also puts registry-qualified keys first. Ties sort by
// key so the result never depends on map order.
func sortBySpecificity(es []entry) {
	sort.Slice(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.glob != b.glob {
			return !a.glob
		}
		if len(a.key) != len(b.key) {
			return len(a.key) > len(b.key)
		}
		return a.key < b.key
	})
}

func (e entry) matchesContainer(name string) bool {
	if e.glob {
		return e.match.MatchString(name)
	}
	return e.key == name
}

// matchesImage compares against the repository as stored (library/postgres) and,
// for Docker Hub, as people write it (postgres).
func (e entry) matchesImage(ref imageref.Ref) bool {
	if e.registry != "" && e.registry != ref.Registry {
		return false
	}
	repos := []string{ref.Repository}
	if ref.Registry == imageref.DockerHub {
		if short, ok := strings.CutPrefix(ref.Repository, "library/"); ok {
			repos = append(repos, short)
		}
	}
	for _, r := range repos {
		if e.glob && e.match.MatchString(r) || !e.glob && e.repo == r {
			return true
		}
	}
	return false
}

// For returns the effective rules for a container. name may carry Docker's leading
// "/"; imageRef is the container's image reference (an unparsable one only skips
// the images: section). Per field, the most specific source that sets it wins:
// label, then matching containers: entries, then matching images: entries, each
// section in specificity order. Fields merge individually.
func (f *File) For(name, imageRef string, labels map[string]string) Rules {
	r := Default()
	r.From = map[string]string{}
	done := map[string]bool{}

	s, problems := fromLabels(labels)
	r.Problems = append(r.Problems, problems...)
	s.apply(&r, done, FromLabel)

	if f == nil {
		return r
	}
	name = strings.TrimPrefix(name, "/")
	for _, e := range f.containers {
		if e.matchesContainer(name) {
			r.Problems = append(r.Problems, e.problems...)
			e.rules.apply(&r, done, FromConfig)
		}
	}
	if ref, err := imageref.Parse(imageRef); err == nil {
		for _, e := range f.images {
			if e.matchesImage(ref) {
				r.Problems = append(r.Problems, e.problems...)
				e.rules.apply(&r, done, FromConfig)
			}
		}
	}
	return r
}
