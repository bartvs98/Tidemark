package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/bartvs98/tidemark/internal/imageref"
)

// testRegistry is an in-memory OCI registry with a few tags pushed, counting requests.
func testRegistry(t *testing.T) (host string, digests map[string]string, requests *atomic.Int32) {
	t.Helper()
	requests = &atomic.Int32{}
	reg := registry.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		reg.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	host = strings.TrimPrefix(srv.URL, "http://")

	digests = map[string]string{}
	for _, tag := range []string{"1.0.0", "1.1.0", "latest"} {
		img, err := random.Image(64, 1)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := name.NewTag(host+"/team/app:"+tag, name.Insecure)
		if err := remote.Write(ref, img); err != nil {
			t.Fatal(err)
		}
		d, _ := img.Digest()
		digests[tag] = d.String()
	}
	requests.Store(0)
	return host, digests, requests
}

func TestTagsAndDigest(t *testing.T) {
	host, digests, requests := testRegistry(t)
	c := New("tidemark-test")
	ref, err := imageref.Parse(host + "/team/app:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tags, err := c.Tags(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(tags)
	if !slices.Equal(tags, []string{"1.0.0", "1.1.0", "latest"}) {
		t.Errorf("tags = %v", tags)
	}

	d, err := c.Digest(ctx, ref, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if d != digests["latest"] {
		t.Errorf("digest = %s, want %s", d, digests["latest"])
	}

	// Cached: no new requests.
	before := requests.Load()
	_, _ = c.Tags(ctx, ref)
	_, _ = c.Digest(ctx, ref, "latest")
	if requests.Load() != before {
		t.Errorf("cache miss: %d extra requests", requests.Load()-before)
	}
}

func TestMissingRepoIsFriendly(t *testing.T) {
	host, _, _ := testRegistry(t)
	c := New("tidemark-test")
	ref, _ := imageref.Parse(host + "/team/nope:1")
	_, err := c.Digest(context.Background(), ref, "1")
	if err == nil || !strings.Contains(err.Error(), "not found, or private") {
		t.Errorf("err = %v", err)
	}
}
