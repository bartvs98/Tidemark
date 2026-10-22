package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bartvs98/tidemark/internal/ai"
	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/version"
)

// summaryTimeout caps one summary request; the provider clients time out at the same point.
const summaryTimeout = 2 * time.Minute

// aiJSON is a container's AI summary state on the detail endpoint. Model text is plain
// text from an untrusted source: the UI must never render it as markdown or HTML.
type aiJSON struct {
	Provider        string         `json:"provider"` // anthropic | openai | gemini
	Model           string         `json:"model"`
	AutoMinReleases int            `json:"autoMinReleases"` // 0 = auto off
	Releases        int            `json:"releases"`        // releases in range now (what a summary would cover)
	Generating      bool           `json:"generating"`
	Summary         *aiSummaryJSON `json:"summary,omitempty"`
	// Error and FailedAt: the last attempt failed (a kept summary may still be there).
	Error    string     `json:"error,omitempty"`
	FailedAt *time.Time `json:"failedAt,omitempty"`
}

type aiSummaryJSON struct {
	Changes   []string       `json:"changes"`
	Actions   []string       `json:"actions"`
	Verdict   analysis.Level `json:"verdict"`
	Reasons   []ai.Reason    `json:"reasons"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Releases  int            `json:"releases"`
	Omitted   int            `json:"omitted"`
	CreatedAt time.Time      `json:"createdAt"`
	Stale     bool           `json:"stale"` // the notes in range changed since
}

// aiOn reports whether AI summaries are configured; verdicts only apply while they are.
func (s *Server) aiOn() bool {
	if s.ai == nil {
		return false
	}
	p, _ := s.ai.Status()
	return p != nil
}

// applyAI raises u's level to the stored AI verdict for the update, never lowers it, and
// appends the AI's reasons after the heuristic ones. Memory only, so the list can call it
// per container. A stale summary still applies. Both the list and the detail endpoint go
// through here, so they always agree.
func (s *Server) applyAI(u *updateJSON, repo source.Repo, running, latest *version.Version) {
	if !s.aiOn() {
		return
	}
	k, ok := ai.KeyFor(repo.Key(), running, latest)
	if !ok {
		return
	}
	e, ok := s.ai.Lookup(k)
	if !ok || e.Summary == nil {
		return
	}
	u.Level = max(u.Level, e.Summary.Verdict)
	if e.Summary.Verdict > analysis.None {
		for _, r := range e.Summary.Reasons {
			u.Reasons = append(u.Reasons, analysis.Reason{Text: r.Text, Tag: r.Tag, Source: "ai"})
		}
	}
}

// summaryTarget is what a summary of the update would cover: its key and the request built
// from the stored notes in range.
type summaryTarget struct {
	key ai.Key
	req ai.Request
}

// targetFor builds the summary target for an update whose notes ch are already loaded. ok
// is false when there is nothing to judge: no checked result, no update, a channel build
// (stable notes say nothing about it) or no latest version.
func targetFor(u *updateJSON, ref imageref.Ref, repo source.Repo, ch changelog.Changes) (summaryTarget, bool) {
	if u == nil || u.CheckedAt == nil || u.Class == string(version.None) || u.Channel != "" {
		return summaryTarget{}, false
	}
	k, ok := ai.KeyFor(repo.Key(), parseVersion(u.Running), parseVersion(u.Latest))
	if !ok {
		return summaryTarget{}, false
	}
	return summaryTarget{key: k, req: ai.RequestFrom(ref.Familiar(), repo.Path, u.Running, u.Latest, ch)}, true
}

// aiState describes the target's summary state; nil when AI is off, or when there are no
// releases in range and nothing was ever stored for it (nothing to offer).
func (s *Server) aiState(t summaryTarget) *aiJSON {
	if s.ai == nil {
		return nil
	}
	p, autoMin := s.ai.Status()
	if p == nil {
		return nil
	}
	e, stored := s.ai.Lookup(t.key)
	if !stored && len(t.req.Releases) == 0 {
		return nil
	}
	out := &aiJSON{
		Provider:        p.Name(),
		Model:           p.Model(),
		AutoMinReleases: autoMin,
		Releases:        len(t.req.Releases),
		Generating:      s.ai.Generating(t.key),
		Error:           e.Error,
		FailedAt:        timeOrNil(e.FailedAt),
	}
	if sm := e.Summary; sm != nil {
		out.Summary = &aiSummaryJSON{
			Changes:   orEmptySlice(sm.Changes),
			Actions:   orEmptySlice(sm.Actions),
			Verdict:   sm.Verdict,
			Reasons:   sm.Reasons,
			Provider:  sm.Provider,
			Model:     sm.Model,
			Releases:  sm.Releases,
			Omitted:   sm.Omitted,
			CreatedAt: sm.CreatedAt,
			Stale:     sm.NotesHash != ai.NotesHash(t.req),
		}
		if out.Summary.Reasons == nil {
			out.Summary.Reasons = []ai.Reason{}
		}
	}
	return out
}

// postSummary summarises a container's release notes in range with the configured
// provider and answers with the new AI state. It waits for the provider (up to
// summaryTimeout) on the server's background context, so a closed tab doesn't waste the call.
func (s *Server) postSummary(w http.ResponseWriter, r *http.Request) {
	if s.ai == nil {
		writeError(w, http.StatusNotImplemented, "AI summaries are not enabled")
		return
	}
	c, ok := s.findContainer(w, r)
	if !ok {
		return
	}
	if !s.aiOn() {
		writeError(w, http.StatusConflict, ai.ErrOff.Error())
		return
	}
	t, status, msg := s.summaryTargetFor(r.Context(), c)
	if status != 0 {
		writeError(w, status, msg)
		return
	}

	ctx, cancel := context.WithTimeout(s.background, summaryTimeout)
	defer cancel()
	_, err := s.ai.Generate(ctx, t.key, t.req)
	switch {
	case errors.Is(err, ai.ErrOff):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, ai.ErrNoNotes):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil: // already scrubbed of the key, and stored with the entry
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	state := s.aiState(t)
	if state == nil { // AI was turned off while generating
		writeError(w, http.StatusConflict, ai.ErrOff.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// summaryTargetFor finds what a summary of c's update would cover, from the stored notes.
// Otherwise status and msg are the answer: 422 when there's nothing to summarise.
func (s *Server) summaryTargetFor(ctx context.Context, c docker.Container) (t summaryTarget, status int, msg string) {
	const none = http.StatusUnprocessableEntity
	rs := s.rulesFor(c)
	if rs.Ignore {
		return t, none, "the container is ignored"
	}
	ref, err := imageref.Parse(c.ImageRef)
	if err != nil {
		return t, none, "no update to summarise"
	}
	u := s.updateFor(ctx, c, ref, rs)
	repo, _, ok := source.Resolve(c, ref, rs)
	if !ok || s.changelogs == nil {
		return t, none, "no changelog found for this image"
	}
	if u == nil || u.CheckedAt == nil {
		return t, none, "no update to summarise"
	}
	ch, err := s.changelogs.Changes(ctx, repo, parseVersion(u.Running), parseVersion(u.Latest))
	if err != nil {
		return t, http.StatusInternalServerError, err.Error()
	}
	if t, ok = targetFor(u, ref, repo, ch); !ok {
		return t, none, "no update to summarise"
	}
	if len(t.req.Releases) == 0 {
		return t, none, ai.ErrNoNotes.Error()
	}
	return t, 0, ""
}
