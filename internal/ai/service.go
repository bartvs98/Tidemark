package ai

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/singleflight"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/store"
)

// autoBackoff is how long Auto leaves a key alone after a failed attempt.
const autoBackoff = 6 * time.Hour

// Triggers of a provider call, as recorded in the usage records.
const (
	triggerManual = "manual"
	triggerAuto   = "auto"
)

// Modes of a provider call: a full summary of the range, or an extension of an earlier
// summary with only the newer releases' notes.
const (
	modeFull     = "full"
	modeExtended = "extended"
)

// Service keeps every stored summary in memory (Lookup is called per container on every
// list request) and generates new ones, one provider call per key at a time.
type Service struct {
	st  *store.Store
	log *slog.Logger
	now func() time.Time

	mu         sync.Mutex
	provider   Provider // nil: off
	autoMin    int
	entries    map[Key]Entry
	generating map[Key]bool
	flight     singleflight.Group
}

func New(st *store.Store, log *slog.Logger) *Service {
	return &Service{st: st, log: log, now: time.Now, entries: map[Key]Entry{}, generating: map[Key]bool{}}
}

// Load reads the stored entries into memory.
func (s *Service) Load(ctx context.Context) error {
	rows, err := s.st.AISummaries(ctx)
	if err != nil {
		return err
	}
	entries := make(map[Key]Entry, len(rows))
	for _, a := range rows {
		entries[Key{a.Repo, a.From, a.To}] = entryFromStore(a)
	}
	s.mu.Lock()
	s.entries = entries
	s.mu.Unlock()
	return nil
}

// Configure sets the provider (nil = off) and the auto threshold. A generation already
// running finishes with the provider it started with.
func (s *Service) Configure(p Provider, autoMin int) {
	s.mu.Lock()
	s.provider, s.autoMin = p, autoMin
	s.mu.Unlock()
}

// Status returns the provider (nil when off) and the auto threshold.
func (s *Service) Status() (p Provider, autoMin int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.provider, s.autoMin
}

// Lookup returns the entry for a key from memory. The Summary must not be modified.
func (s *Service) Lookup(k Key) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[k]
	return e, ok
}

// Generating reports whether a summary for k is being generated.
func (s *Service) Generating(k Key) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generating[k]
}

// Generate always calls the provider (one call per key at a time; a concurrent caller
// waits for and shares the running one). With full false it extends an earlier summary of
// the same repo and running version when one covers the oldest part of the range unchanged
// (see base), and makes a full summary otherwise; full true always makes a full one.
// Stores success (clears error) or failure (keeps an earlier summary, sets Error/FailedAt)
// and records the call as manual. Returns ErrOff / ErrNoNotes without a call.
func (s *Service) Generate(ctx context.Context, k Key, r Request, full bool) (Entry, error) {
	return s.generateAs(ctx, k, r, triggerManual, full)
}

func (s *Service) generateAs(ctx context.Context, k Key, r Request, trigger string, full bool) (Entry, error) {
	p, _ := s.Status()
	if p == nil {
		return Entry{}, ErrOff
	}
	if !hasNotes(r) {
		return Entry{}, ErrNoNotes
	}
	v, err, _ := s.flight.Do(k.Repo+"\x00"+k.From+"\x00"+k.To, func() (any, error) {
		s.setGenerating(k, true)
		defer s.setGenerating(k, false)
		return s.generate(ctx, p, k, r, trigger, full)
	})
	e, _ := v.(Entry)
	return e, err
}

func (s *Service) generate(ctx context.Context, p Provider, k Key, r Request, trigger string, full bool) (Entry, error) {
	var base *Summary
	var prompt Prompt
	var sel selection
	mode := modeFull
	if !full {
		s.mu.Lock()
		b, fresh := s.base(k, r)
		s.mu.Unlock()
		if b != nil {
			if pr, sl, ok := buildExtendPrompt(r, b, fresh); ok {
				base, prompt, sel, mode = b, pr, sl, modeExtended
			}
		}
	}
	if base == nil {
		prompt, sel = buildPrompt(r)
	}

	start := s.now()
	raw, usage, err := p.Complete(ctx, prompt)
	took := s.now().Sub(start)
	var sum *Summary
	if err == nil {
		sum, err = summaryFrom(raw, sel, base)
	}

	s.mu.Lock()
	e := s.entries[k]
	if err != nil {
		e.Error, e.FailedAt = shortError(err), s.now()
	} else {
		sum.Provider, sum.Model = p.Name(), p.Model()
		sum.NotesHash, sum.CreatedAt = NotesHash(r), s.now()
		sum.Latest, sum.Covered, sum.CoveredHash = r.Latest, len(r.Releases), CoveredHash(r.Releases)
		sum.InputTokens, sum.OutputTokens, sum.Duration = usage.InputTokens, usage.OutputTokens, took
		e = Entry{Summary: sum}
	}
	s.entries[k] = e
	s.mu.Unlock()

	// A summary that worked is still worth showing when it couldn't be stored.
	sctx := context.WithoutCancel(ctx)
	if serr := s.st.PutAISummary(sctx, entryToStore(k, e)); serr != nil {
		s.log.Error("store AI summary", "repo", k.Repo, "err", serr)
	}
	call := store.AICall{At: start, Provider: p.Name(), Model: p.Model(), Trigger: trigger, Mode: mode,
		Repo: k.Repo, From: k.From, To: k.To, Releases: len(sel.sent),
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, Duration: took}
	if err != nil {
		call.Error = e.Error
	}
	if serr := s.st.AddAICall(sctx, call); serr != nil {
		s.log.Error("record AI call", "repo", k.Repo, "err", serr)
	}
	if err != nil {
		return e, errors.New(e.Error)
	}
	return e, nil
}

// base finds the stored summary k's range can extend: another key with the same repo and
// running version whose summary left nothing out (Omitted 0) and whose covered releases
// are exactly the oldest part of r's range, unchanged (CoveredHash), with at least one
// newer release on top. Of several, the one covering the most releases (then the newest).
// fresh is the newer releases, newest first. Call with s.mu held.
func (s *Service) base(k Key, r Request) (base *Summary, fresh []Notes) {
	for k2, e := range s.entries {
		b := e.Summary
		if k2 == k || k2.Repo != k.Repo || k2.From != k.From || !extendable(b) || b.Covered >= len(r.Releases) {
			continue
		}
		if base != nil && (b.Covered < base.Covered || b.Covered == base.Covered && !b.CreatedAt.After(base.CreatedAt)) {
			continue
		}
		if b.CoveredHash != CoveredHash(r.Releases[len(r.Releases)-b.Covered:]) {
			continue
		}
		base = b
	}
	if base == nil {
		return nil, nil
	}
	return base, r.Releases[:len(r.Releases)-base.Covered]
}

// extendable: a summary that could be the base of an extension.
func extendable(b *Summary) bool {
	return b != nil && b.Omitted == 0 && b.Covered > 0 && b.CoveredHash != ""
}

// WantsAuto says whether Auto would call the provider for k and r now (see Auto), so the
// after-check hook only queues the calls that will happen.
func (s *Service) WantsAuto(k Key, r Request) bool {
	s.mu.Lock()
	p, autoMin, e, busy := s.provider, s.autoMin, s.entries[k], s.generating[k]
	s.mu.Unlock()
	if p == nil || autoMin <= 0 || len(r.Releases) < autoMin || busy || !hasNotes(r) {
		return false
	}
	if strings.TrimSpace(r.Releases[0].Body) == "" {
		return false
	}
	if e.Summary != nil && e.Summary.NotesHash == NotesHash(r) {
		return false
	}
	return e.FailedAt.IsZero() || s.now().Sub(e.FailedAt) >= autoBackoff
}

// Auto is for the after-check hook: generates (extending when possible) when on,
// autoMin > 0, len(r.Releases) >= autoMin, the newest release in range has notes (they
// usually follow the tag within the hour, and the hourly sync picks them up), there's no
// summary or it's stale (NotesHash differs), and the last failure for the key is older
// than 6 h. Logs failures, never returns them.
func (s *Service) Auto(ctx context.Context, k Key, r Request) {
	if !s.WantsAuto(k, r) {
		return
	}
	p, _ := s.Status()
	if p == nil { // turned off since
		return
	}
	got, err := s.generateAs(ctx, k, r, triggerAuto, false)
	if err != nil {
		s.log.Warn("AI summary failed", "repo", k.Repo, "from", k.From, "to", k.To, "provider", p.Name(), "err", err)
		return
	}
	s.log.Info("AI summary generated", "repo", k.Repo, "from", k.From, "to", k.To, "releases", len(r.Releases),
		"extended_from", got.Summary.ExtendedFrom)
}

// Usage sums the provider calls since t.
func (s *Service) Usage(ctx context.Context, since time.Time) (store.AIUsage, error) {
	return s.st.AIUsageSince(ctx, since)
}

// Prune deletes stored summaries (memory + store) whose key isn't in keep, except, for
// each kept key, the summary a newer range of the same repo and running version would
// extend: of the other keys with that repo and From, the extendable one covering the most
// releases (then the newest). Skips keys that are generating right now. Returns how many went.
func (s *Service) Prune(ctx context.Context, keep map[Key]bool) (int, error) {
	// Held throughout, so memory and store change together and no generation starts
	// writing a key (generate stores under its generating flag) while it's deleted.
	s.mu.Lock()
	defer s.mu.Unlock()
	bases := map[Key]bool{}
	for need := range keep {
		var best *Summary
		var bestKey Key
		for k, e := range s.entries {
			b := e.Summary
			if k.Repo != need.Repo || k.From != need.From || k.To == need.To || !extendable(b) {
				continue
			}
			if best == nil || b.Covered > best.Covered || b.Covered == best.Covered && b.CreatedAt.After(best.CreatedAt) {
				best, bestKey = b, k
			}
		}
		if best != nil {
			bases[bestKey] = true
		}
	}
	kept := func(k Key) bool { return keep[k] || bases[k] || s.generating[k] }
	n, err := s.st.DeleteAISummaries(ctx, func(repo, from, to string) bool { return kept(Key{repo, from, to}) })
	if err != nil {
		return 0, err
	}
	for k := range s.entries {
		if !kept(k) {
			delete(s.entries, k)
		}
	}
	return n, nil
}

func (s *Service) setGenerating(k Key, on bool) {
	s.mu.Lock()
	if on {
		s.generating[k] = true
	} else {
		delete(s.generating, k)
	}
	s.mu.Unlock()
}

// hasNotes: at least one release in range has a non-empty body.
func hasNotes(r Request) bool {
	for _, n := range r.Releases {
		if strings.TrimSpace(n.Body) != "" {
			return true
		}
	}
	return false
}

// summaryFrom parses the model's answer into a summary. For an extension (base non-nil)
// reasons may also name the base reasons' tags, the base's verdict is a floor, and the
// summary counts the base's releases too.
func summaryFrom(raw []byte, sel selection, base *Summary) (*Summary, error) {
	tags := tagsOf(sel.sent)
	if base != nil {
		for _, r := range base.Reasons {
			if r.Tag != "" {
				tags[r.Tag] = true
			}
		}
	}
	changes, actions, verdict, reasons, err := parseOutput(raw, tags)
	if err != nil {
		return nil, err
	}
	sum := &Summary{Changes: changes, Actions: actions, Verdict: verdict, Reasons: reasons,
		Releases: len(sel.sent), Omitted: sel.omitted}
	if base != nil {
		sum.Verdict, sum.Reasons = applyFloor(verdict, reasons, base)
		sum.Releases += base.Releases
		sum.ExtendedFrom = base.Latest
	}
	return sum, nil
}

// shortError keeps a stored error message to one short line.
func shortError(err error) string {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if utf8.RuneCountInString(msg) > 200 {
		msg = string([]rune(msg)[:199]) + "…"
	}
	return msg
}

func entryFromStore(a store.AISummary) Entry {
	e := Entry{Error: a.Error, FailedAt: a.FailedAt}
	if a.CreatedAt.IsZero() {
		return e
	}
	var lvl analysis.Level
	_ = lvl.UnmarshalText([]byte(a.Verdict)) // unknown: none
	sum := &Summary{Changes: a.Changes, Actions: a.Actions, Verdict: lvl, Provider: a.Provider, Model: a.Model,
		Releases: a.Releases, Omitted: a.Omitted, NotesHash: a.NotesHash, CreatedAt: a.CreatedAt,
		InputTokens: a.InputTokens, OutputTokens: a.OutputTokens, Duration: a.Duration,
		Latest: a.Latest, ExtendedFrom: a.ExtendedFrom, Covered: a.Covered, CoveredHash: a.CoveredHash}
	for _, r := range a.Reasons {
		sum.Reasons = append(sum.Reasons, Reason{Text: r.Text, Tag: r.Tag})
	}
	e.Summary = sum
	return e
}

func entryToStore(k Key, e Entry) store.AISummary {
	a := store.AISummary{Repo: k.Repo, From: k.From, To: k.To, Verdict: "none", Error: e.Error, FailedAt: e.FailedAt}
	if sum := e.Summary; sum != nil {
		a.Provider, a.Model, a.Changes, a.Actions = sum.Provider, sum.Model, sum.Changes, sum.Actions
		a.Verdict, a.Releases, a.Omitted = sum.Verdict.String(), sum.Releases, sum.Omitted
		a.NotesHash, a.CreatedAt = sum.NotesHash, sum.CreatedAt
		a.InputTokens, a.OutputTokens, a.Duration = sum.InputTokens, sum.OutputTokens, sum.Duration
		a.Latest, a.ExtendedFrom, a.Covered, a.CoveredHash = sum.Latest, sum.ExtendedFrom, sum.Covered, sum.CoveredHash
		for _, r := range sum.Reasons {
			a.Reasons = append(a.Reasons, store.AIReason{Text: r.Text, Tag: r.Tag})
		}
	}
	return a
}
