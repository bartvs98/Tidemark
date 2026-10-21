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
	"github.com/bartvs98/tidemark/internal/resolve"
	"github.com/bartvs98/tidemark/internal/rules"
	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/store"
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
	// RulesError: the config file couldn't be loaded, so its rules aren't applied (the
	// last good version is, if any; labels always are).
	RulesError string `json:"rulesError,omitempty"`
}

// updateJSON describes the last successful check. When the latest attempt failed, Error
// and FailedAt say so and the rest is the result kept from before (CheckedAt absent if no
// check ever succeeded: then the result is empty, source unknown, class none).
type updateJSON struct {
	Running       string `json:"running,omitempty"`       // running version (tag text), empty if unknown
	Source        string `json:"source"`                  // tag | label | digest | env | unknown
	Latest        string `json:"latest,omitempty"`        // newest release in the same family
	LatestInMajor string `json:"latestInMajor,omitempty"` // newest within the running major, for major jumps
	Class         string `json:"class"`                   // none | patch | minor | major | digest
	Channel       string `json:"channel,omitempty"`       // pre-release channel tag (nightly, dev, edge): Latest is only the newest stable release
	Floating      bool   `json:"floating"`
	NewImage      bool   `json:"newImage"`
	TagDigest     string `json:"tagDigest,omitempty"` // the floating tag's current registry digest, when known
	// ReviewKey is what a review or "snooze until newer" is set against: Latest, or TagDigest
	// when the tag is a channel or Latest is unknown (then it is tied to the build). Absent when neither.
	ReviewKey string     `json:"reviewKey,omitempty"`
	Held      string     `json:"held,omitempty"`   // newest version a pin or major hold keeps back
	HeldBy    string     `json:"heldBy,omitempty"` // pin | allow-major
	Error     string     `json:"error,omitempty"`
	ErrorKind string     `json:"errorKind,omitempty"` // with Error: rate-limit | not-found | timeout | network | registry; absent when unclassified
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
	FailedAt  *time.Time `json:"failedAt,omitempty"` // set with Error
	// TagsFrom: the result is from a successful check, but its tag list was reused from
	// this time because the registry rate-limited the refresh.
	TagsFrom *time.Time `json:"tagsFrom,omitempty"`
	// Level is the breaking-change verdict (none | attention | breaking) from the version
	// jump and the stored release notes in range; Reasons explain it, most severe first.
	Level   analysis.Level    `json:"level"`
	Reasons []analysis.Reason `json:"reasons,omitempty"`
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
	Rules   *rulesJSON  `json:"rules,omitempty"`  // only when a rule is set or one is invalid
	Update  *updateJSON `json:"update,omitempty"` // result of the last check, if any; never when ignored
	Review  *reviewJSON `json:"review,omitempty"` // only while a stored review applies to the update
}

// rulesJSON shows only rules that differ from the defaults, plus where explicitly set
// rules come from (rule key → label | config) and invalid values that were skipped.
type rulesJSON struct {
	Ignore     bool              `json:"ignore,omitempty"`
	AllowMajor *bool             `json:"allowMajor,omitempty"` // only when false
	Pin        string            `json:"pin,omitempty"`
	Prerelease bool              `json:"prerelease,omitempty"`
	TagPattern string            `json:"tagPattern,omitempty"` // regex source
	Repo       string            `json:"repo,omitempty"`
	From       map[string]string `json:"from"`
	Problems   []string          `json:"problems,omitempty"`
}

func toRulesJSON(rs rules.Rules) *rulesJSON {
	if !rs.Active() && len(rs.Problems) == 0 {
		return nil
	}
	rj := &rulesJSON{Ignore: rs.Ignore, Pin: rs.Pin, Prerelease: rs.Prerelease, Repo: rs.Repo, From: rs.From, Problems: rs.Problems}
	if !rs.AllowMajor {
		rj.AllowMajor = new(bool)
	}
	if rs.TagPattern != nil {
		rj.TagPattern = rs.TagPattern.String()
	}
	if rj.From == nil {
		rj.From = map[string]string{}
	}
	return rj
}

// rulesFor returns a container's effective rules; without a check service only its labels.
func (s *Server) rulesFor(c docker.Container) rules.Rules {
	if s.checks == nil {
		return (*rules.File)(nil).For(c.Name, c.ImageRef, c.Labels)
	}
	return s.checks.Rules(c)
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
		out.Check = s.checkJSON()
	}
	reviews := s.reviews(r.Context())
	for _, c := range list {
		out.Containers = append(out.Containers, s.toContainerJSON(r.Context(), c, reviews))
	}
	slices.SortFunc(out.Containers, func(a, b containerJSON) int {
		return cmp.Or(cmp.Compare(a.Stack, b.Stack), cmp.Compare(a.Name, b.Name))
	})
	writeJSON(w, http.StatusOK, out)
}

// toContainerJSON describes a container; reviews are the stored reviews by name (nil for none).
func (s *Server) toContainerJSON(ctx context.Context, c docker.Container, reviews map[string]store.Review) containerJSON {
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
	if svc := s.icons.Load(); svc != nil {
		if ic, ok := svc.Resolve(ref, c.Name, c.Service, c.Labels); ok {
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
	rs := s.rulesFor(c)
	update := s.updateFor(ctx, c, ref, rs)
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
		Rules:   toRulesJSON(rs),
		Update:  update,
		Review:  reviewFor(reviews, c.Name, update, time.Now()),
	}
}

func (s *Server) updateFor(ctx context.Context, c docker.Container, ref imageref.Ref, rs rules.Rules) *updateJSON {
	if s.checks == nil || rs.Ignore {
		return nil
	}
	e, ok := s.checks.Result(c)
	if !ok {
		return nil
	}
	r := e.Result
	u := &updateJSON{
		Running:       r.Running,
		Source:        string(cmp.Or(r.Source, resolve.SourceUnknown)),
		Latest:        r.Latest,
		LatestInMajor: r.LatestInMajor,
		Class:         string(cmp.Or(r.Class, version.None)),
		Channel:       resolve.Channel(ref.Tag),
		Floating:      r.Floating,
		NewImage:      r.NewImage,
		Held:          r.Held,
		HeldBy:        r.HeldBy,
		TagDigest:     r.TagDigest,
		Error:         e.Err,
		ErrorKind:     e.ErrKind,
		CheckedAt:     timeOrNil(e.CheckedAt),
		FailedAt:      timeOrNil(e.FailedAt),
		TagsFrom:      timeOrNil(r.TagsFrom),
	}
	u.ReviewKey = reviewKey(u)
	// A channel build isn't compared with stable releases, so their notes say nothing about it.
	if !e.CheckedAt.IsZero() && r.Class != version.None && u.Channel == "" {
		s.assessUpdate(ctx, u, c, ref, rs)
	}
	return u
}

// reviewKey is what a review is set against: the latest version, or the build (the tag's
// digest) when the tag is a channel or there is no latest version to compare.
func reviewKey(u *updateJSON) string {
	if u.Channel != "" || u.Latest == "" {
		return u.TagDigest
	}
	return u.Latest
}

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// assessUpdate sets the update's level from the version jump and the release notes that
// are already stored (the check run prefetches them). It never fetches. Latest is already
// within any pin or major hold, so held-back releases don't count.
func (s *Server) assessUpdate(ctx context.Context, u *updateJSON, c docker.Container, ref imageref.Ref, rs rules.Rules) {
	running, latest := parseVersion(u.Running), parseVersion(u.Latest)
	var ch changelog.Changes
	if s.changelogs != nil && ref.Repository != "" {
		if repo, _, ok := source.Resolve(c, ref, rs); ok {
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

func (s *Server) checkJSON() *checkJSON {
	st := s.checks.Status()
	return &checkJSON{
		Running: st.Running, Trigger: st.Trigger, NextRunAt: st.NextRunAt,
		StartedAt: st.StartedAt, FinishedAt: st.FinishedAt, Done: st.Done, Total: st.Total, Error: st.Err,
		RulesError: s.checks.RulesError(),
	}
}

// startCheck starts a check run unless one is running, and answers with the run's status.
func (s *Server) startCheck(w http.ResponseWriter, r *http.Request) {
	if s.checks == nil {
		writeError(w, http.StatusNotImplemented, "update checks are not enabled")
		return
	}
	s.checks.Start(s.background, check.TriggerManual)
	writeJSON(w, http.StatusAccepted, s.checkJSON())
}
