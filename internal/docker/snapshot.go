package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Snapshot serves containers from a file written by scripts/snapshot.sh. The file is
// re-read on every call, so dropping in a fresh snapshot needs no restart.
type Snapshot struct {
	path string
}

type snapshotFile struct {
	CapturedAt time.Time      `json:"capturedAt"`
	Containers []apiContainer `json:"containers"`
	Images     []apiImage     `json:"images"`
}

func NewSnapshot(path string) *Snapshot { return &Snapshot{path: path} }

func (s *Snapshot) Info() SourceInfo {
	info := SourceInfo{Kind: "snapshot", Location: s.path}
	if f, err := s.read(); err == nil && !f.CapturedAt.IsZero() {
		info.CapturedAt = &f.CapturedAt
	}
	return info
}

func (s *Snapshot) Containers(ctx context.Context) ([]Container, error) {
	f, err := s.read()
	if err != nil {
		return nil, err
	}
	images := make(map[string]apiImage, len(f.Images))
	for _, img := range f.Images {
		images[img.ID] = img
	}
	out := make([]Container, 0, len(f.Containers))
	for _, c := range f.Containers {
		out = append(out, c.toContainer(images))
	}
	return out, nil
}

func (s *Snapshot) read() (snapshotFile, error) {
	var f snapshotFile
	data, err := os.ReadFile(s.path)
	if err != nil {
		return f, fmt.Errorf("snapshot: %w", err)
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("snapshot %s: %w", s.path, err)
	}
	return f, nil
}
