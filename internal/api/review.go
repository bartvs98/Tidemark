package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bartvs98/tidemark/internal/review"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

// reviewJSON is a container's review state: reviewed (read the changes up to Version) or
// snoozed (until Until, or until a version newer than Version).
type reviewJSON struct {
	Kind    string     `json:"kind"`              // reviewed | snoozed
	Version string     `json:"version,omitempty"` // reviewed: read up to this; snoozed: until a version newer than this
	Until   *time.Time `json:"until,omitempty"`   // snoozed until this time
	At      time.Time  `json:"at"`                // when it was set
}

// historyJSON is an image change seen between successful checks; the first one (no
// Prev*) is when tracking started.
type historyJSON struct {
	At          time.Time `json:"at"`
	Running     string    `json:"running,omitempty"`
	ImageID     string    `json:"imageId"`
	PrevRunning string    `json:"prevRunning,omitempty"`
	PrevImageID string    `json:"prevImageId,omitempty"` // absent on the first row: tracking started
}

func toReviewJSON(r store.Review) *reviewJSON {
	return &reviewJSON{Kind: r.Kind, Version: r.Version, Until: timeOrNil(r.Until), At: r.CreatedAt}
}

// reviews returns the stored reviews by container name; nil without a store or on error
// (the list then just shows no review states).
func (s *Server) reviews(ctx context.Context) map[string]store.Review {
	if s.store == nil {
		return nil
	}
	m, err := s.store.Reviews(ctx)
	if err != nil {
		s.log.Warn("loading reviews failed", "err", err)
		return nil
	}
	return m
}

// reviewFor returns a container's stored review if it still applies to the shown update,
// which must be a successful result (possibly kept after a failed refresh) with an update.
func reviewFor(reviews map[string]store.Review, name string, u *updateJSON, now time.Time) *reviewJSON {
	r, ok := reviews[name]
	if !ok || u == nil || u.CheckedAt == nil || u.Class == string(version.None) {
		return nil
	}
	if !review.Active(r, u.ReviewKey, now) {
		return nil
	}
	return toReviewJSON(r)
}

// history returns a container's image changes, newest first; empty without a store.
func (s *Server) history(ctx context.Context, name string) []historyJSON {
	out := []historyJSON{}
	if s.store == nil {
		return out
	}
	list, err := s.store.History(ctx, name)
	if err != nil {
		s.log.Warn("loading history failed", "container", name, "err", err)
		return out
	}
	for _, h := range list {
		out = append(out, historyJSON{At: h.DetectedAt, Running: h.Running, ImageID: h.ImageID, PrevRunning: h.PrevRunning, PrevImageID: h.PrevImageID})
	}
	return out
}

// reviewRequest is the body of PUT /api/containers/{name}/review.
type reviewRequest struct {
	Kind    string     `json:"kind"`
	Version string     `json:"version"`
	Until   *time.Time `json:"until"`
}

// parseReview reads a review request strictly: reviewed needs a version and no until;
// snoozed needs exactly one of until (in the future) or version.
func parseReview(body io.Reader, now time.Time) (reviewRequest, error) {
	var req reviewRequest
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, errors.New("invalid body: " + err.Error())
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return req, errors.New("invalid body: expected a single JSON object")
	}
	hasVersion, hasUntil := strings.TrimSpace(req.Version) != "", req.Until != nil
	switch req.Kind {
	case review.Reviewed:
		if !hasVersion || hasUntil {
			return req, errors.New(`"reviewed" needs a version and no until`)
		}
	case review.Snoozed:
		if hasVersion == hasUntil {
			return req, errors.New(`"snoozed" needs either until or version, not both`)
		}
		if hasUntil && !req.Until.After(now) {
			return req, errors.New("until must be in the future")
		}
	default:
		return req, errors.New(`kind must be "reviewed" or "snoozed"`)
	}
	return req, nil
}

// putReview marks a container's update as reviewed, or snoozes it, replacing any earlier
// review state.
func (s *Server) putReview(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusNotImplemented, "reviews are not enabled")
		return
	}
	now := time.Now()
	req, err := parseReview(http.MaxBytesReader(w, r.Body, 4<<10), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, ok := s.findContainer(w, r)
	if !ok {
		return
	}
	rv := store.Review{ContainerName: c.Name, Kind: req.Kind, Version: strings.TrimSpace(req.Version), CreatedAt: now.UTC()}
	if req.Until != nil {
		rv.Until = req.Until.UTC()
	}
	if err := s.store.SaveReview(r.Context(), rv); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.InvalidateStats()
	writeJSON(w, http.StatusOK, toReviewJSON(rv))
}

// deleteReview undoes a container's review state; nothing stored is fine.
func (s *Server) deleteReview(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusNotImplemented, "reviews are not enabled")
		return
	}
	c, ok := s.findContainer(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteReview(r.Context(), c.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.InvalidateStats()
	w.WriteHeader(http.StatusNoContent)
}
