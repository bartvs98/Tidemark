package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/docker"
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
	History   []historyJSON  `json:"history"`             // image changes, newest first; always an array
	// AI is the update's AI summary state; nil when AI is off or there's nothing to summarise
	// (ignored, no checked update, a channel build, no changelog).
	AI *aiJSON `json:"ai,omitempty"`
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
	How  string `json:"how"`  // label | config | image | registry | known | guess
}

type releaseJSON struct {
	Tag         string     `json:"tag"`
	Name        string     `json:"name"`
	Body        string     `json:"body"` // markdown from the repo: untrusted, sanitise before rendering
	URL         string     `json:"url"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	// Level and the flagged lines of this release's notes (Hit.Line is the trimmed markdown
	// source line, so the UI can find it again when rendering).
	Level analysis.Level `json:"level"`
	Hits  []analysis.Hit `json:"hits"`
}

type linksJSON struct {
	Compare  string `json:"compare,omitempty"`  // running...latest diff on the forge
	Releases string `json:"releases,omitempty"` // releases page or CHANGELOG file
	Running  string `json:"running,omitempty"`  // source tree at the running version
	// InList opens the forge's release list on the page with the running version, scrolled
	// to it with a text fragment, to see it in relation to the newer releases (GitHub only).
	InList string `json:"inList,omitempty"`
}

// githubReleasesPerPage is how many releases github.com shows per page of /releases.
const githubReleasesPerPage = 10

func (s *Server) containerDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := s.findContainer(w, r)
	if !ok {
		return
	}

	out := detailJSON{
		Container: s.toContainerJSON(r.Context(), c, s.reviews(r.Context())),
		Image:     imageDetail{Digests: orEmptySlice(c.Image.RepoDigests), Labels: ociLabels(c.Image.Labels)},
		History:   s.history(r.Context(), c.Name),
	}

	// Ignore means no outside calls for this container, release notes included.
	rs := s.rulesFor(c)
	if rs.Ignore {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ref, err := imageref.Parse(c.ImageRef)
	repo, how, ok := source.Resolve(c, ref, rs)
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
	if err := s.changelogs.Sync(ctx, repo, running, latest); err != nil {
		s.log.Debug("changelog sync failed", "repo", repo.Key(), "err", err)
	}
	cancel()

	ch, err := s.changelogs.Changes(r.Context(), repo, running, latest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out.Changelog = toChangelogJSON(repo, how, ch)
	// The notes may be fresher than what the list endpoint saw, so assess again.
	if u := out.Container.Update; u != nil && u.CheckedAt != nil { // a result, possibly kept after a failed refresh
		res := assess(u.Class, running, latest, ch)
		if u.Class != string(version.None) && u.Channel == "" { // a channel build isn't judged by stable notes
			u.Level, u.Reasons = res.Level, res.Reasons
			s.applyAI(u, repo, running, latest)
		}
		for i, rel := range res.Releases {
			out.Changelog.Releases[i].Level = rel.Level
			if rel.Hits != nil {
				out.Changelog.Releases[i].Hits = rel.Hits
			}
		}
		if t, ok := targetFor(u, ref, repo, ch); ok {
			out.AI = s.aiState(t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// findContainer looks up the container named in the path. If there's none, or Docker
// fails, it answers the request itself and returns false.
func (s *Server) findContainer(w http.ResponseWriter, r *http.Request) (docker.Container, bool) {
	name := r.PathValue("name")
	list, err := s.source.Containers(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return docker.Container{}, false
	}
	for _, c := range list {
		if c.Name == name {
			return c, true
		}
	}
	writeError(w, http.StatusNotFound, "no container named "+name)
	return docker.Container{}, false
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
		rj := releaseJSON{Tag: r.Tag, Name: r.Name, Body: r.Body, URL: r.URL, Hits: []analysis.Hit{}}
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
	if ch.FromName != "" && ch.FromIndex >= 0 && repo.IsGitHub() {
		list := repo.URL() + "/releases"
		if page := ch.FromIndex/githubReleasesPerPage + 1; page > 1 {
			list += "?page=" + strconv.Itoa(page)
		}
		// GitHub's sticky "Release list" sidebar also contains the version, and comes first in
		// the page. Requiring the "Compare" button right after the text matches only the
		// release heading. If GitHub's markup changes, the fragment just doesn't match and the
		// page still opens at the right release page.
		cj.Links.InList = list + "#:~:text=" + textFragment(ch.FromName) + ",-Compare"
	}
	if ch.FromTag != "" {
		cj.Links.Running = repo.URL() + sep + "tree/" + url.PathEscape(ch.FromTag)
		if ch.ToTag != "" && ch.ToTag != ch.FromTag {
			cj.Links.Compare = repo.URL() + sep + "compare/" + url.PathEscape(ch.FromTag) + "..." + url.PathEscape(ch.ToTag)
		}
	}
	return cj
}

// textFragment encodes text for a #:~:text= fragment, where "-", "," and "&" are syntax.
func textFragment(s string) string {
	e := strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
	return strings.ReplaceAll(e, "-", "%2D")
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
