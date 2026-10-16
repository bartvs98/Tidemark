package api

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
)

type containerList struct {
	Source     sourceJSON      `json:"source"`
	Check      *checkJSON      `json:"check,omitempty"`
	Containers []containerJSON `json:"containers"`
}

type checkJSON struct {
	Running    bool       `json:"running"`
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
		out.Containers = append(out.Containers, s.toContainerJSON(c))
	}
	slices.SortFunc(out.Containers, func(a, b containerJSON) int {
		return cmp.Or(cmp.Compare(a.Stack, b.Stack), cmp.Compare(a.Name, b.Name))
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) toContainerJSON(c docker.Container) containerJSON {
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
		Update:  s.updateFor(c.ID),
	}
}

func (s *Server) updateFor(containerID string) *updateJSON {
	if s.checks == nil {
		return nil
	}
	e, ok := s.checks.Result(containerID)
	if !ok {
		return nil
	}
	r := e.Result
	return &updateJSON{
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
}

func toCheckJSON(st check.Status) *checkJSON {
	return &checkJSON{Running: st.Running, StartedAt: st.StartedAt, FinishedAt: st.FinishedAt, Done: st.Done, Total: st.Total, Error: st.Err}
}

// startCheck starts a check run unless one is running, and answers with the run's status.
func (s *Server) startCheck(w http.ResponseWriter, r *http.Request) {
	if s.checks == nil {
		writeError(w, http.StatusNotImplemented, "update checks are not enabled")
		return
	}
	s.checks.Start(s.background)
	writeJSON(w, http.StatusAccepted, toCheckJSON(s.checks.Status()))
}
