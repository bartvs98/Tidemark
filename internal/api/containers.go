package api

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/version"
)

type containerList struct {
	Source     sourceJSON      `json:"source"`
	Check      *checkJSON      `json:"check,omitempty"`
	Containers []containerJSON `json:"containers"`
}

type checkJSON struct {
	Running    bool       `json:"running"`
	Trigger    string     `json:"trigger,omitempty"` // startup | schedule | manual
	NextRunAt  *time.Time `json:"nextRunAt,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Done       int        `json:"done"`
	Total      int        `json:"total"`
	Error      string     `json:"error,omitempty"`
}

type updateJSON struct {
	Running       string    `json:"running,omitempty"`       // running version (tag text), empty if unknown
	Source        string    `json:"source"`                  // tag | label | digest | unknown
	Latest        string    `json:"latest,omitempty"`        // newest release in the same family
	LatestInMajor string    `json:"latestInMajor,omitempty"` // newest within the running major, for major jumps
	Class         string    `json:"class"`                   // none | patch | minor | major | digest
	Floating      bool      `json:"floating"`
	NewImage      bool      `json:"newImage"`
	Error         string    `json:"error,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
	// Level is the breaking-change verdict (none | attention | breaking) from the version
	// jump and the stored release notes in range; Reasons explain it, most severe first.
	Level   analysis.Level `json:"level"`
	Reasons []string       `json:"reasons,omitempty"`
}

type sourceJSON struct {
	Kind       string     `json:"kind"`
	Location   string     `json:"location"`
	CapturedAt *time.Time `json:"capturedAt,omitempty"`
}

type containerJSON struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Image   imageJSON   `json:"image"`
	State   string      `json:"state"`
	Status  string      `json:"status"`
	Created time.Time   `json:"created"`
	Stack   string      `json:"stack,omitempty"`
	Service string      `json:"service,omitempty"`
	Icon    *iconJSON   `json:"icon,omitempty"`   // app icon, if one matched
	Update  *updateJSON `json:"update,omitempty"` // result of the last check, if any
}

// iconJSON holds icon URLs. The UI swaps to the light variant when the default icon
// turns out to be near-black on a dark theme, and to the dark one in the reverse case.
type iconJSON struct {
	Src   string `json:"src"`
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
}

type imageJSON struct {
	Ref        string     `json:"ref"`      // as the container was created
	Display    string     `json:"display"`  // familiar form, with implied :latest made explicit
	Registry   string     `json:"registry"` // empty if the reference couldn't be parsed
	Repository string     `json:"repository"`
	Tag        string     `json:"tag"`
	ID         string     `json:"id"`
	Created    *time.Time `json:"created,omitempty"`
	Platform   string     `json:"platform,omitempty"`
	Missing    bool       `json:"missing,omitempty"`
}

func (s *Server) containers(w http.ResponseWriter, r *http.Request) {
	list, err := s.source.Containers(r.Context())
	if err != nil {
		s.log.Warn("listing containers failed", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	info := s.source.Info()
	out := containerList{
		Source:     sourceJSON{Kind: info.Kind, Location: info.Location, CapturedAt: info.CapturedAt},
		Containers: make([]containerJSON, 0, len(list)),
	}
	if s.checks != nil {
		out.Check = toCheckJSON(s.checks.Status())
	}
	for _, c := range list {
		out.Containers = append(out.Containers, s.toContainerJSON(r.Context(), c))
	}
	slices.SortFunc(out.Containers, func(a, b containerJSON) int {
		return cmp.Or(cmp.Compare(a.Stack, b.Stack), cmp.Compare(a.Name, b.Name))
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) toContainerJSON(ctx context.Context, c docker.Container) containerJSON {
	img := imageJSON{
		Ref:      c.ImageRef,
		Display:  c.ImageRef,
		ID:       c.Image.ID,
		Platform: c.Image.Platform,
		Missing:  c.Image.Missing,
	}
	ref, err := imageref.Parse(c.ImageRef)
	if err == nil {
		img.Display = ref.Familiar()
		img.Registry = ref.Registry
		img.Repository = ref.Repository
		img.Tag = ref.Tag
	}
	var icon *iconJSON
	if s.icons != nil {
		if ic, ok := s.icons.Resolve(ref, c.Name, c.Service, c.Labels); ok {
			icon = &iconJSON{Src: "/api/icons/" + ic.Ref}
			if ic.Light {
				icon.Light = icon.Src + "?variant=light"
			}
			if ic.Dark {
				icon.Dark = icon.Src + "?variant=dark"
			}
		}
	}
	if !c.Image.Created.IsZero() {
		img.Created = &c.Image.Created
	}
	return containerJSON{
		ID:      c.ID,
		Name:    c.Name,
		Image:   img,
		State:   c.State,
		Status:  c.Status,
		Created: c.Created,
		Stack:   c.Project,
		Service: c.Service,
		Icon:    icon,
		Update:  s.updateFor(ctx, c, ref),
	}
}

func (s *Server) updateFor(ctx context.Context, c docker.Container, ref imageref.Ref) *updateJSON {
	if s.checks == nil {
		return nil
	}
	e, ok := s.checks.Result(c)
	if !ok {
		return nil
	}
	r := e.Result
	u := &updateJSON{
		Running:       r.Running,
		Source:        string(r.Source),
		Latest:        r.Latest,
		LatestInMajor: r.LatestInMajor,
		Class:         string(r.Class),
		Floating:      r.Floating,
		NewImage:      r.NewImage,
		Error:         e.Err,
		CheckedAt:     e.CheckedAt,
	}
	if e.Err == "" && r.Class != version.None {
		s.assessUpdate(ctx, u, c, ref)
	}
	return u
}

// assessUpdate sets the update's level from the version jump and the release notes that
// are already stored (the check run prefetches them). It never fetches.
func (s *Server) assessUpdate(ctx context.Context, u *updateJSON, c docker.Container, ref imageref.Ref) {
	running, latest := parseVersion(u.Running), parseVersion(u.Latest)
	var ch changelog.Changes
	if s.changelogs != nil && ref.Repository != "" {
		if repo, _, ok := source.Resolve(c, ref); ok {
			var err error
			if ch, err = s.changelogs.Changes(ctx, repo, running, latest); err != nil {
				s.log.Debug("stored release notes unavailable", "repo", repo.Key(), "err", err)
			}
		}
	}
	res := assess(u.Class, running, latest, ch)
	u.Level, u.Reasons = res.Level, res.Reasons
}

func assess(class string, running, latest *version.Version, ch changelog.Changes) analysis.Result {
	in := analysis.Input{Class: version.Class(class), Running: running, Latest: latest, RunningNotes: ch.FromBody}
	for _, r := range ch.Releases {
		in.Releases = append(in.Releases, analysis.Notes{Tag: r.Tag, Body: r.Body})
	}
	return analysis.Assess(in)
}

func toCheckJSON(st check.Status) *checkJSON {
	return &checkJSON{
		Running: st.Running, Trigger: st.Trigger, NextRunAt: st.NextRunAt,
		StartedAt: st.StartedAt, FinishedAt: st.FinishedAt, Done: st.Done, Total: st.Total, Error: st.Err,
	}
}

// startCheck starts a check run unless one is running, and answers with the run's status.
func (s *Server) startCheck(w http.ResponseWriter, r *http.Request) {
	if s.checks == nil {
		writeError(w, http.StatusNotImplemented, "update checks are not enabled")
		return
	}
	s.checks.Start(s.background, check.TriggerManual)
	writeJSON(w, http.StatusAccepted, toCheckJSON(s.checks.Status()))
}
