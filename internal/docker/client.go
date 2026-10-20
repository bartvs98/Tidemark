package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a minimal read-only Docker Engine API client. Tidemark only needs two
// GET endpoints, so this avoids the full Docker SDK and its dependency tree.
//
// It deliberately never calls GET /containers/{id}/json: that response includes
// environment variables (secrets), and the list endpoint has everything we need.
type Client struct {
	host string
	base string
	http *http.Client
}

// NewClient accepts unix:///var/run/docker.sock or tcp://host:2375 (e.g. docker-socket-proxy).
func NewClient(host string) (*Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("docker host %q: %w", host, err)
	}
	transport := &http.Transport{MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	c := &Client{host: host, http: &http.Client{Transport: transport, Timeout: 30 * time.Second}}

	switch u.Scheme {
	case "unix":
		socket := u.Path
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}
		c.base = "http://docker"
	case "tcp", "http":
		if u.Host == "" {
			return nil, fmt.Errorf("docker host %q: missing host:port", host)
		}
		c.base = "http://" + u.Host
	default:
		return nil, fmt.Errorf("docker host %q: unsupported scheme (use unix:// or tcp://)", host)
	}
	return c, nil
}

func (c *Client) Info() SourceInfo { return SourceInfo{Kind: "docker", Location: c.host} }

func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	var list []apiContainer
	if err := c.get(ctx, "/containers/json?all=1", &list); err != nil {
		return nil, err
	}

	images := map[string]apiImage{}
	for _, ct := range list {
		if _, seen := images[ct.ImageID]; seen || ct.ImageID == "" {
			continue
		}
		var img apiImage
		err := c.get(ctx, "/images/"+url.PathEscape(ct.ImageID)+"/json", &img)
		if err != nil {
			if !isNotFound(err) {
				// Only this container's check fails; the others are still listed.
				images[ct.ImageID] = apiImage{inspectErr: err.Error()}
			}
			continue // a removed image: the container is reported with Image.Missing
		}
		images[ct.ImageID] = img
	}

	out := make([]Container, 0, len(list))
	for _, ct := range list {
		out = append(out, ct.toContainer(images))
	}
	return out, nil
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("docker api: %d %s", e.Status, e.Message) }

func isNotFound(err error) bool {
	ae, ok := err.(*apiError)
	return ok && ae.Status == http.StatusNotFound
}

// get uses unversioned paths, so the daemon answers with its own API version.
// That keeps older engines (e.g. NAS builds) working; the fields we read are stable.
func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error // drop Go's `Get "http://docker/…":` prefix, it means nothing to users
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("can't reach Docker at %s: %w", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var m struct{ Message string }
		if json.Unmarshal(body, &m) != nil || m.Message == "" {
			m.Message = strings.TrimSpace(string(body))
		}
		return &apiError{Status: resp.StatusCode, Message: m.Message}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
