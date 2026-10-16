package api

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
)

type containerList struct {
	Source     sourceJSON      `json:"source"`
	Containers []containerJSON `json:"containers"`
}

type sourceJSON struct {
	Kind       string     `json:"kind"`
	Location   string     `json:"location"`
	CapturedAt *time.Time `json:"capturedAt,omitempty"`
}

type containerJSON struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Image   imageJSON `json:"image"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
	Stack   string    `json:"stack,omitempty"`
	Service string    `json:"service,omitempty"`
	Icon    *iconJSON `json:"icon,omitempty"` // app icon, if one matched
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
	}
}
