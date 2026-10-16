package api

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/version"
)

// How long a detail request may wait for release notes before answering with what's stored.
const detailSyncTimeout = 15 * time.Second

type detailJSON struct {
	Container containerJSON  `json:"container"`
	Image     imageDetail    `json:"image"`
	Changelog *changelogJSON `json:"changelog,omitempty"` // nil: no source repository found
}

type imageDetail struct {
	Digests []string          `json:"digests"`
	Labels  map[string]string `json:"labels"` // OCI labels only
}

type changelogJSON struct {
	Repo      repoJSON      `json:"repo"`
	Kind      string        `json:"kind"` // releases | changelog | none
	Releases  []releaseJSON `json:"releases"`
	Links     linksJSON     `json:"links"`
	Partial   bool          `json:"partial"`
	Error     string        `json:"error,omitempty"`
	FetchedAt *time.Time    `json:"fetchedAt,omitempty"`
}

type repoJSON struct {
	URL  string `json:"url"`
	Name string `json:"name"` // owner/name
	How  string `json:"how"`  // label | image | registry | known | guess
}

type releaseJSON struct {
	Tag         string     `json:"tag"`
	Name        string     `json:"name"`
	Body        string     `json:"body"` // markdown from the repo: untrusted, sanitise before rendering
	URL         string     `json:"url"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type linksJSON struct {
	Compare  string `json:"compare,omitempty"`  // running...latest diff on the forge
	Releases string `json:"releases,omitempty"` // releases page or CHANGELOG file
	Running  string `json:"running,omitempty"`  // source tree at the running version
}

func (s *Server) containerDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	list, err := s.source.Containers(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	idx := -1
	for i, c := range list {
		if c.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeError(w, http.StatusNotFound, "no container named "+name)
		return
	}
	c := list[idx]

	out := detailJSON{
		Container: s.toContainerJSON(c),
		Image:     imageDetail{Digests: orEmptySlice(c.Image.RepoDigests), Labels: ociLabels(c.Image.Labels)},
	}

	ref, err := imageref.Parse(c.ImageRef)
	repo, how, ok := source.Resolve(c, ref)
	if s.changelogs == nil || err != nil || !ok {
		writeJSON(w, http.StatusOK, out)
		return
	}

	var running, latest *version.Version
	if u := out.Container.Update; u != nil {
		running, latest = parseVersion(u.Running), parseVersion(u.Latest)
	}
	// Usually prefetched by the check run; otherwise fetch now, but don't hang the page.
	ctx, cancel := context.WithTimeout(r.Context(), detailSyncTimeout)
	if err := s.changelogs.Sync(ctx, repo, running); err != nil {
		s.log.Debug("changelog sync failed", "repo", repo.Key(), "err", err)
	}
	cancel()

	ch, err := s.changelogs.Changes(r.Context(), repo, running, latest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out.Changelog = toChangelogJSON(repo, how, ch)
	writeJSON(w, http.StatusOK, out)
}

func toChangelogJSON(repo source.Repo, how source.How, ch changelog.Changes) *changelogJSON {
	cj := &changelogJSON{
		Repo:     repoJSON{URL: repo.URL(), Name: repo.Path, How: string(how)},
		Kind:     ch.Kind,
		Releases: []releaseJSON{},
		Partial:  ch.Partial,
		Error:    ch.Error,
	}
	if !ch.FetchedAt.IsZero() {
		cj.FetchedAt = &ch.FetchedAt
	}
	for _, r := range ch.Releases {
		rj := releaseJSON{Tag: r.Tag, Name: r.Name, Body: r.Body, URL: r.URL}
		if !r.PublishedAt.IsZero() {
			rj.PublishedAt = &r.PublishedAt
		}
		cj.Releases = append(cj.Releases, rj)
	}

	sep := "/" // GitHub: /compare, /releases, /tree; GitLab puts them under /-/
	if !repo.IsGitHub() {
		sep = "/-/"
	}
	switch ch.Kind {
	case changelog.KindReleases:
		cj.Links.Releases = repo.URL() + sep + "releases"
	case changelog.KindChangelog:
		if len(ch.Releases) > 0 {
			cj.Links.Releases = ch.Releases[0].URL
		}
	}
	if ch.FromTag != "" {
		cj.Links.Running = repo.URL() + sep + "tree/" + url.PathEscape(ch.FromTag)
		if ch.ToTag != "" && ch.ToTag != ch.FromTag {
			cj.Links.Compare = repo.URL() + sep + "compare/" + url.PathEscape(ch.FromTag) + "..." + url.PathEscape(ch.ToTag)
		}
	}
	return cj
}

func parseVersion(s string) *version.Version {
	if v, ok := version.Parse(s); ok {
		return &v
	}
	return nil
}

func ociLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range labels {
		if len(k) > len("org.opencontainers.image.") && k[:len("org.opencontainers.image.")] == "org.opencontainers.image." {
			out[k] = v
		}
	}
	return out
}

func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
