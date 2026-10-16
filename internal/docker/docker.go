// Package docker reads containers and their images, either live from the Docker
// Engine API (read-only) or from a snapshot file written by scripts/snapshot.sh.
// Both decode the same Engine API JSON, so they behave identically.
package docker

import (
	"context"
	"strings"
	"time"
)

// Source lists containers with their image metadata.
type Source interface {
	Containers(ctx context.Context) ([]Container, error)
	Info() SourceInfo
}

type SourceInfo struct {
	Kind       string     // "docker" or "snapshot"
	Location   string     // socket/host URL or snapshot path
	CapturedAt *time.Time // snapshots only
}

type Container struct {
	ID       string
	Name     string
	ImageRef string // reference the container was created from, e.g. "louislam/uptime-kuma"
	Image    Image
	State    string // running, exited, paused, restarting, created, dead
	Status   string // human status from Docker, e.g. "Up 3 days"
	Created  time.Time
	Project  string // com.docker.compose.project, empty if not from Compose
	Service  string // com.docker.compose.service
	Labels   map[string]string
}

type Image struct {
	ID          string
	RepoTags    []string
	RepoDigests []string // registry manifest digests, e.g. "redis@sha256:…"
	Created     time.Time
	Platform    string // linux/amd64, linux/arm64/v8
	Labels      map[string]string
	Missing     bool // the image could not be inspected (deleted since the container was created)
}

const (
	labelProject = "com.docker.compose.project"
	labelService = "com.docker.compose.service"
)

// Engine API shapes (GET /containers/json, GET /images/{id}/json); only the fields we use.
type apiContainer struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Labels  map[string]string `json:"Labels"`
}

type apiImage struct {
	ID           string   `json:"Id"`
	RepoTags     []string `json:"RepoTags"`
	RepoDigests  []string `json:"RepoDigests"`
	Created      string   `json:"Created"`
	Architecture string   `json:"Architecture"`
	Os           string   `json:"Os"`
	Variant      string   `json:"Variant"`
	Config       struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

func (c apiContainer) toContainer(images map[string]apiImage) Container {
	img, found := images[c.ImageID]
	out := Container{
		ID:       c.ID,
		Name:     strings.TrimPrefix(firstOr(c.Names, c.ID), "/"),
		ImageRef: c.Image,
		Image:    img.toImage(c.ImageID, found),
		State:    c.State,
		Status:   c.Status,
		Created:  time.Unix(c.Created, 0).UTC(),
		Project:  c.Labels[labelProject],
		Service:  c.Labels[labelService],
		Labels:   orEmpty(c.Labels),
	}
	// When an image is re-tagged after the container started, Docker reports the bare
	// image ID as the container's image. Fall back to the image's own tag if it has one.
	if strings.HasPrefix(out.ImageRef, "sha256:") && len(img.RepoTags) > 0 {
		out.ImageRef = img.RepoTags[0]
	}
	return out
}

func (i apiImage) toImage(id string, found bool) Image {
	if !found {
		return Image{ID: id, Missing: true, Labels: map[string]string{}}
	}
	created, _ := time.Parse(time.RFC3339Nano, i.Created)
	platform := i.Os + "/" + i.Architecture
	if i.Variant != "" {
		platform += "/" + i.Variant
	}
	return Image{
		ID:          i.ID,
		RepoTags:    i.RepoTags,
		RepoDigests: i.RepoDigests,
		Created:     created,
		Platform:    platform,
		Labels:      orEmpty(i.Config.Labels),
	}
}

func firstOr(s []string, fallback string) string {
	if len(s) > 0 {
		return s[0]
	}
	return fallback
}

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
