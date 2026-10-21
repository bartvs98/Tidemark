package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/notify"
	"github.com/bartvs98/tidemark/internal/version"
)

type notificationsJSON struct {
	Notifier  string            `json:"notifier"` // telegram
	Level     string            `json:"level"`    // all | attention | breaking
	Digest    string            `json:"digest,omitempty"`
	PublicURL string            `json:"publicUrl,omitempty"`
	Last      *notificationJSON `json:"last,omitempty"` // the newest send attempt; absent if none yet
}

type notificationJSON struct {
	At    time.Time `json:"at"`
	Kind  string    `json:"kind"` // updates | first | test
	Items int       `json:"items"`
	Error string    `json:"error,omitempty"` // absent when it was sent
}

func toNotificationsJSON(st notify.Status) *notificationsJSON {
	out := &notificationsJSON{Notifier: st.Notifier, Level: "all", Digest: st.Digest, PublicURL: st.PublicURL}
	if st.MinLevel.String() != "none" {
		out.Level = st.MinLevel.String()
	}
	if st.Last != nil {
		out.Last = &notificationJSON{At: st.Last.SentAt, Kind: st.Last.Kind, Items: st.Last.Items, Error: st.Last.Error}
	}
	return out
}

// Updates lists the containers that have an update from a successful check, as the
// dashboard shows them, for the notifier. A container's Review is its stored review,
// active or not.
func (s *Server) Updates(ctx context.Context) ([]notify.Update, error) {
	list, err := s.source.Containers(ctx)
	if err != nil {
		return nil, err
	}
	reviews := s.reviews(ctx)
	var out []notify.Update
	for _, c := range list {
		ref, _ := imageref.Parse(c.ImageRef)
		u := s.updateFor(ctx, c, ref, s.rulesFor(c))
		if u == nil || u.CheckedAt == nil || u.Class == string(version.None) {
			continue
		}
		nu := notify.Update{
			Container: c.Name, Stack: c.Project, Running: u.Running, Latest: u.Latest,
			Class: version.Class(u.Class), Channel: u.Channel, Level: u.Level, Key: u.ReviewKey,
		}
		if r, ok := reviews[c.Name]; ok {
			nu.Review = &r
		}
		out = append(out, nu)
	}
	return out, nil
}

// testNotification sends a test message.
func (s *Server) testNotification(w http.ResponseWriter, r *http.Request) {
	if s.notify == nil {
		writeError(w, http.StatusNotImplemented, "notifications are not configured")
		return
	}
	if !s.notify.Enabled() {
		writeError(w, http.StatusConflict, notify.ErrOff.Error())
		return
	}
	if err := s.notify.Test(r.Context()); errors.Is(err, notify.ErrOff) {
		writeError(w, http.StatusConflict, err.Error()) // turned off in between
		return
	} else if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
